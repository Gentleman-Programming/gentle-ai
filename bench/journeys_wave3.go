package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Wave 3 Slice 5 (task 6.7): black-box new-lineage lifecycle journeys.
// GENTLE_AI_RDD_NEW_LINEAGE is opted into per-journey via
// Journey.NewLineageActivation (bench/runner.go) — every wave1/wave2/edge/sdd
// journey stays on the byte-identical legacy `review start` path.
//
// Wave 3 remediation (verify-report CRITICAL C1, then CRITICAL C5): `review
// finalize` is now CLI-wired for new lineages, and default-deny (C5) means a
// `review start`-only lineage — never finalized — correctly DENIES at every
// gate rather than allowing. These two journeys now drive the complete
// product-reachable lifecycle S3's own SUGGESTION named as missing:
// tier-0 `review start` freezing a v3 authority, `review finalize` reaching
// a genuine approved receipt, then `review validate` across the gates task
// 6.7's own fix (governingAuthorityLiveEvidence's per-gate-accurate
// projection) makes correct — an exact match allowing at both post-apply and
// pre-commit, and an unstaged-only drift correctly denying scope-changed at
// post-apply while still allowing at pre-commit, exactly the distinction
// review_new_lineage_gate_selector_test.go proves at the Go integration
// layer, reproduced here end-to-end through the built binary.

const (
	newLineageExactLineage    = "wave-three-new-lineage-exact"
	newLineageSelectorLineage = "wave-three-new-lineage-selector"
)

type newLineageStartResult struct {
	Operation string `json:"operation"`
	LineageID string `json:"lineage_id"`
	State     string `json:"state"`
	RiskLevel string `json:"risk_level"`
}

// requireNewLineageStarted proves the v3-specific start response shape
// (ReviewFacadeStartNewLineageResult, internal/cli/review_facade_new_lineage.go)
// rather than assuming the legacy start envelope's fields apply.
func requireNewLineageStarted(lineage string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var result newLineageStartResult
		if err := decodeWaveObservation(observation, &result, "new-lineage start"); err != nil {
			return err
		}
		if result.Operation != "review/start" || result.LineageID != lineage || result.State != "reviewing" {
			return fmt.Errorf("new-lineage start = %+v, want operation=review/start lineage_id=%q state=reviewing", result, lineage)
		}
		return nil
	}
}

// requireNewLineageScopeChangedDenied proves the collect(changed) ->
// GateScopeChanged mapping task 6.2 wires (newLineageGateEvaluation) reaches
// the built binary's stdout, not just the Go unit-test layer: a denial that
// still exits non-zero but carries the parseable scope-changed envelope.
func requireNewLineageScopeChangedDenied(lineage string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var gate waveGateResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
			return fmt.Errorf("parse new-lineage scope-changed denial: %w (stderr: %s)", err, firstLine(observation.Stderr))
		}
		if observation.ExitCode == 0 || gate.Allowed || gate.Result != "scope-changed" || gate.Context.LineageID != lineage {
			return fmt.Errorf("new-lineage post-apply drift = exit=%d gate=%+v, want a scope-changed denial for lineage %q", observation.ExitCode, gate, lineage)
		}
		return nil
	}
}

type newLineageFinalizeResult struct {
	Operation string `json:"operation"`
	LineageID string `json:"lineage_id"`
	State     string `json:"state"`
}

// requireNewLineageFinalized proves the v3-specific finalize response shape
// (ReviewFacadeFinalizeNewLineageResult, internal/cli/review_facade_finalize_new_lineage.go)
// reaches a genuine approved terminal state and receipt through the built
// binary — the C1/C5 remediation's own product-surface evidence.
func requireNewLineageFinalized(lineage string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var result newLineageFinalizeResult
		if err := decodeWaveObservation(observation, &result, "new-lineage finalize"); err != nil {
			return err
		}
		if result.Operation != "review/finalize" || result.LineageID != lineage || result.State != "approved" {
			return fmt.Errorf("new-lineage finalize = %+v, want operation=review/finalize lineage_id=%q state=approved", result, lineage)
		}
		return nil
	}
}

// stagePassiveTierZeroDoc stages one genuinely tier-0 (RiskLow) candidate:
// ClassifyRisk's OnlyPassiveContentChanges carve-out (risk.go) requires
// every authored path to be passive prose content (.md/.mdx/.rst/.adoc/
// images) — ordinary code never qualifies, landing in RiskMedium instead,
// whose non-empty SelectedLenses now requires captured lens results before
// finalize approves (absorbed N1, Wave 5 Slice 4). j59/j60 test the gate
// selector's staged-vs-workspace projection distinction, which is orthogonal
// to file type, so a passive doc keeps them genuinely tier-0 (their own
// stated title) with a product-reachable finalize path, rather than
// (before N1) silently self-approving a RiskMedium candidate with zero
// captured lens results.
func stagePassiveTierZeroDoc(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "docs", "gate-selector-notes.md")
	content := "# gate selector notes\n\nOrdinary prose with no executable content.\n"
	if err := sandbox.write(path, content); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

// dirtyTrackedFileWithoutStaging edits stagePassiveTierZeroDoc's own tracked
// file again, without staging the edit — the exact unstaged-drift fixture
// review_new_lineage_gate_selector_test.go proves at the Go layer.
func dirtyTrackedFileWithoutStaging(sandbox *Sandbox) error {
	path := sandbox.Repo + "/docs/gate-selector-notes.md"
	content := "# gate selector notes\n\nOrdinary prose with no executable content.\nUnstaged drift: never committed, never staged.\n"
	return sandbox.write(path, content)
}

func waveThreeJourneys() []Journey {
	return []Journey{
		{
			ID:                   "j59-new-lineage-exact-candidate-allows-post-apply-and-pre-commit",
			Title:                "New-lineage tier-0 candidate: an unchanged candidate allows at both post-apply and pre-commit",
			Source:               "Wave 3 Slice 5, task 6.7 (governingAuthorityLiveEvidence per-gate-accurate selector)",
			NewLineageActivation: true,
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: one exact passive tier-0 doc candidate staged", Fixture: stagePassiveTierZeroDoc},
				{Name: "new-lineage review start", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", newLineageExactLineage), After: requireNewLineageStarted(newLineageExactLineage)},
				{Name: "new-lineage review finalize", Requires: finalizeCapability,
					Args: productArgs("review", "finalize", "--lineage", newLineageExactLineage), After: requireNewLineageFinalized(newLineageExactLineage)},
				{Name: "unchanged candidate allows at post-apply", Requires: validateCapability,
					Args: productArgs("review", "validate", "--lineage", newLineageExactLineage, "--gate", "post-apply"),
					After: func(_ *Sandbox, observation Observation) error {
						return requireGateForLineage(observation, newLineageExactLineage, false)
					}},
				{Name: "unchanged candidate still allows at pre-commit", Requires: validateCapability,
					Args: productArgs("review", "validate", "--lineage", newLineageExactLineage, "--gate", "pre-commit"),
					After: func(_ *Sandbox, observation Observation) error {
						return requireGateForLineage(observation, newLineageExactLineage, false)
					}},
			},
		},
		{
			ID:                   "j60-new-lineage-unstaged-drift-denies-post-apply-allows-pre-commit",
			Title:                "New-lineage gate-accurate selector: unstaged-only drift denies scope-changed at post-apply but still allows at pre-commit",
			Source:               "Wave 3 Slice 5, task 6.7 (S4's documented scope cut, governingAuthorityLiveEvidence)",
			NewLineageActivation: true,
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: one exact passive tier-0 doc candidate staged", Fixture: stagePassiveTierZeroDoc},
				{Name: "new-lineage review start", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", newLineageSelectorLineage), After: requireNewLineageStarted(newLineageSelectorLineage)},
				{Name: "new-lineage review finalize", Requires: finalizeCapability,
					Args: productArgs("review", "finalize", "--lineage", newLineageSelectorLineage), After: requireNewLineageFinalized(newLineageSelectorLineage)},
				{Name: "fixture: same tracked file edited again, never staged", Fixture: dirtyTrackedFileWithoutStaging},
				{Name: "unstaged drift denies scope-changed at post-apply", Requires: validateCapability,
					Args:  productArgs("review", "validate", "--lineage", newLineageSelectorLineage, "--gate", "post-apply"),
					After: requireNewLineageScopeChangedDenied(newLineageSelectorLineage)},
				{Name: "the identical unstaged drift still allows at pre-commit: pre-commit reads the staged index, not the live workspace", Requires: validateCapability,
					Args: productArgs("review", "validate", "--lineage", newLineageSelectorLineage, "--gate", "pre-commit"),
					After: func(_ *Sandbox, observation Observation) error {
						return requireGateForLineage(observation, newLineageSelectorLineage, false)
					}},
			},
		},
	}
}
