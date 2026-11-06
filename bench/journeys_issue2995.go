package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Issue #2995: a store whose authority diagnostics are ALL historical
// (outdated released-v2.2.x records) previously had no sanctioned exit at
// all — the historical disposition plan hard-gated to a single diagnostic,
// so a store with N>=2 historical entries was a dead end. Unit 2 mints one
// exact successor-only selector per historical entry, surfaces them through
// `review repair --preflight`, reflects them as per-entry sanctioned exits in
// `review inspect-authority`, and scopes execution to exactly the entry the
// selector names — the sibling stays byte-identical.
//
// The two fixtures are byte-identical copies of compact review authority
// written by the released gentle-ai v2.2.0 binary (see
// testdata/issue2995/PROVENANCE.md for the tarball, tag, and per-record
// SHA-256 pins; a drifted fixture proves nothing, so the fixture step
// re-hashes both against their pins).

const (
	issue2995ApprovedLineage  = "review-a39b858db5f00bbb"
	issue2995EscalatedLineage = "review-a3118da0f4a4c425"
	issue2995ApprovedDigest   = "sha256:bdc0db81f30867ba11eb6179f68d8a76651d77c4b0af668dabd855c23961ccbb"
	issue2995EscalatedDigest  = "sha256:3d9a686b5a9f43abcb8b474e44986f8b72e57631cf851619096eefaeba854b12"
	issue2995Class            = "retired_compact_snapshot_identity"
)

//go:embed testdata/issue2995/released-v2.2.0-approved-review-state.json
var issue2995ApprovedRecord []byte

//go:embed testdata/issue2995/released-v2.2.0-escalated-review-state.json
var issue2995EscalatedRecord []byte

var issue2995RepairPreflightSelectorCapability = &Capability{
	Verb:  []string{"review", "repair"},
	Flags: []string{"--cwd", "--preflight", "--successor-lineage", "--successor-revision"},
}

var issue2995RepairDispositionSelectorExecuteCapability = &Capability{
	Verb:  []string{"review", "repair"},
	Flags: []string{"--cwd", "--plan-digest", "--inventory-revision", "--actor", "--reason", "--authorization", "--successor-lineage", "--successor-revision"},
}

func issue2995Journeys() []Journey {
	return []Journey{{
		ID:     "j2995-selector-scoped-historical-disposition-quarantine",
		Title:  "Two historical authority entries expose per-entry selector-scoped repair exits",
		Source: "issue #2995",
		Review: reviewOptedIn,
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "clear any clone-local review override (a clone may only ever assert off)", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "fixture: stage unrelated current target", Fixture: stageProse("", "issue2995-current")},
			{Name: "fixture: replay exact released v2.2.0 authority bytes", Fixture: issue2995HistoricalFixtures},
			{Name: "inspect-authority offers one per-entry exit per historical entry", Requires: inspectAuthorityCapability, Args: productArgs("review", "inspect-authority"), After: inspectionAssertion("two-historical inspection", requireIssue2995Inspection)},
			{Name: "selectorless preflight mints no plan but surfaces both selectors", Requires: repairPreflightCapability, Args: productArgs("review", "repair", "--preflight"), After: requireIssue2995SelectorsSurfaced},
			{Name: "selector-scoped preflight mints exactly one entry's plan", Requires: issue2995RepairPreflightSelectorCapability, Composite: issue2995SelectorPreflight},
			{Name: "authorized selector-scoped repair quarantines exactly once", Requires: issue2995RepairDispositionSelectorExecuteCapability, Composite: issue2995Execute},
			{Name: "sibling historical entry remains inspectable after the scoped repair", Requires: inspectAuthorityCapability, Args: productArgs("review", "inspect-authority"), After: inspectionAssertion("post-repair inspection", requireIssue2995SiblingInspection)},
		},
	}}
}

func issue2995HistoricalFixtures(sandbox *Sandbox) error {
	for _, fixture := range []struct {
		lineage string
		payload []byte
		digest  string
	}{
		{issue2995ApprovedLineage, issue2995ApprovedRecord, issue2995ApprovedDigest},
		{issue2995EscalatedLineage, issue2995EscalatedRecord, issue2995EscalatedDigest},
	} {
		sum := sha256.Sum256(fixture.payload)
		if "sha256:"+hex.EncodeToString(sum[:]) != fixture.digest {
			return fmt.Errorf("released fixture for %s digest drifted from its PROVENANCE pin", fixture.lineage)
		}
		path, err := storeStatePath(sandbox, fixture.lineage)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, fixture.payload, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// requireIssue2995Inspection pins the store-wide view (step a): both entries
// diagnose as outdated (never malformed), and each historical entry carries
// its own selector-scoped `review repair` exit, escalated lineage first
// (selectors sort by lineage).
func requireIssue2995Inspection(report storeInspection) error {
	if len(report.EntryDiagnostics) != 2 ||
		report.EntryDiagnostics[0].LineageID != issue2995EscalatedLineage || report.EntryDiagnostics[0].Problem != "outdated_compact_state" ||
		report.EntryDiagnostics[1].LineageID != issue2995ApprovedLineage || report.EntryDiagnostics[1].Problem != "outdated_compact_state" {
		return fmt.Errorf("two-historical inspection diagnostics = %+v", report.EntryDiagnostics)
	}
	if len(report.SanctionedExits) != 2 ||
		report.SanctionedExits[0].LineageID != issue2995EscalatedLineage || report.SanctionedExits[0].Operation != "review repair" ||
		report.SanctionedExits[1].LineageID != issue2995ApprovedLineage || report.SanctionedExits[1].Operation != "review repair" {
		return fmt.Errorf("two-historical inspection sanctioned exits = %+v", report.SanctionedExits)
	}
	return nil
}

// requireIssue2995SelectorsSurfaced pins the selectorless preflight (step b):
// no plan is minted (selectorless derivation still refuses any store whose
// total diagnostics exceed one), and exactly two successor-only selectors are
// enumerated, binding each entry's raw-byte digest. The escalated lineage
// sorts first; it becomes the selected entry, and the approved lineage's
// bytes are the sibling identity the execution step proves untouched.
func requireIssue2995SelectorsSurfaced(sandbox *Sandbox, observation Observation) error {
	var result dispositionRepairResult
	if err := decodeWaveObservation(observation, &result, "review repair --preflight two-historical shape"); err != nil {
		return err
	}
	if result.DispositionProviderInputs != nil || len(result.DispositionSelectors) != 2 {
		return fmt.Errorf("review repair --preflight did not surface two per-entry historical selectors: %+v", result)
	}
	want := []struct {
		lineage string
		digest  string
	}{{issue2995EscalatedLineage, issue2995EscalatedDigest}, {issue2995ApprovedLineage, issue2995ApprovedDigest}}
	for index, selector := range result.DispositionSelectors {
		if selector.PredecessorLineageID != "" || selector.PredecessorExpectedRevision != "" {
			return fmt.Errorf("historical selector %d carries predecessor identity: %+v", index, selector)
		}
		if selector.SuccessorLineageID != want[index].lineage || selector.SuccessorExpectedRevision != want[index].digest {
			return fmt.Errorf("historical selector %d = %+v, want %s bound to %s", index, selector, want[index].lineage, want[index].digest)
		}
	}
	sandbox.Scratch[scratchDispositionSelector] = strings.Join([]string{"", "", want[0].lineage, want[0].digest}, "\n")
	sandbox.Scratch[scratchDispositionRemainingSelector] = strings.Join([]string{"", "", want[1].lineage, want[1].digest}, "\n")
	return nil
}

// issue2995SelectorPreflight pins the scoped preflight (step c): re-running
// preflight with one surfaced selector mints exactly that entry's plan — no
// selectors re-surfaced — while the same selector with a stale expected
// revision refuses as concurrent-update drift.
func issue2995SelectorPreflight(r *journeyRun) error {
	fields, err := issue2995SelectedSelector(r.sandbox)
	if err != nil {
		return err
	}
	scoped := r.run(productArgsFor(r, "review", "repair", "--preflight", "--successor-lineage", fields[2], "--successor-revision", fields[3]), false)
	var result dispositionRepairResult
	if scoped.ExitCode != 0 || json.Unmarshal([]byte(scoped.Stdout), &result) != nil ||
		result.DispositionProviderInputs == nil || len(result.DispositionSelectors) != 0 {
		return fmt.Errorf("selector-scoped preflight did not mint exactly one entry's plan: %d / %s / %s", scoped.ExitCode, scoped.Stdout, scoped.Stderr)
	}
	r.sandbox.Scratch[scratchDispositionPlanDigest] = result.DispositionProviderInputs.PlanDigest
	r.sandbox.Scratch[scratchDispositionInventoryRevision] = result.DispositionProviderInputs.AuthorityInventoryRevision
	stale := r.run(productArgsFor(r, "review", "repair", "--preflight", "--successor-lineage", fields[2], "--successor-revision", strings.Repeat("0", 64)), false)
	if stale.ExitCode == 0 || !strings.Contains(stale.Stderr, "no longer matches") {
		return fmt.Errorf("stale historical selector = %d / %s", stale.ExitCode, stale.Stderr)
	}
	return nil
}

// issue2995Execute pins the committed repair (step d): the authorized
// selector-scoped execution quarantines exactly the selected entry with its
// exact released bytes, replays to the same single committed record, and
// leaves the sibling historical entry byte-identical.
func issue2995Execute(r *journeyRun) error {
	fields, err := issue2995SelectedSelector(r.sandbox)
	if err != nil {
		return err
	}
	sibling := strings.Split(r.sandbox.Scratch[scratchDispositionRemainingSelector], "\n")
	plan, inventory := r.sandbox.Scratch[scratchDispositionPlanDigest], r.sandbox.Scratch[scratchDispositionInventoryRevision]
	binding, err := dispositionRepositoryBinding(r.sandbox)
	if err != nil {
		return err
	}
	reason := "quarantine released historical entry"
	args := []string{
		"review", "repair", "--cwd", r.sandbox.Repo,
		"--plan-digest", plan, "--inventory-revision", inventory,
		"--actor", "bench", "--reason", reason,
		"--authorization", dispositionAuthorization(binding, plan, inventory, "bench", reason, issue2995Class),
		"--successor-lineage", fields[2], "--successor-revision", fields[3],
	}
	for attempt := 0; attempt < 2; attempt++ {
		observation := r.run(args, false)
		var result dispositionRepairResult
		if observation.ExitCode != 0 || json.Unmarshal([]byte(observation.Stdout), &result) != nil ||
			result.DispositionExecution == nil || result.DispositionExecution.Status != "committed" ||
			result.DispositionExecution.LineageID != fields[2] {
			return fmt.Errorf("selector-scoped historical repair attempt %d = %d / %s / %s", attempt, observation.ExitCode, observation.Stdout, observation.Stderr)
		}
	}
	selectedPath, err := storeStatePath(r.sandbox, fields[2])
	if err != nil {
		return err
	}
	if _, err := os.Stat(selectedPath); !os.IsNotExist(err) {
		return fmt.Errorf("selected historical entry remains active: %v", err)
	}
	siblingPath, err := storeStatePath(r.sandbox, sibling[2])
	if err != nil {
		return err
	}
	siblingAfter, err := os.ReadFile(siblingPath)
	if err != nil || !bytes.Equal(siblingAfter, issue2995ApprovedRecord) {
		return errors.New("selector-scoped repair changed the sibling historical entry")
	}
	base, err := reviewTransactionsBase(r.sandbox)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		return fmt.Errorf("selector-scoped repair quarantine inventory = %v (%v), want exactly one entry after replay", entries, err)
	}
	residue, err := os.ReadFile(filepath.Join(base, "quarantine", entries[0].Name(), "residue", "review-state.json"))
	if err != nil || !bytes.Equal(residue, issue2995EscalatedRecord) {
		return errors.New("quarantine did not preserve the selected entry's exact released bytes")
	}
	return nil
}

// requireIssue2995SiblingInspection pins the post-repair retained store: the
// selected entry is gone, the sibling still diagnoses as outdated, and — now
// the store's only diagnostic — it carries the same single-entry sanctioned
// exit the j92 posture already guaranteed.
func requireIssue2995SiblingInspection(report storeInspection) error {
	if len(report.EntryDiagnostics) != 1 ||
		report.EntryDiagnostics[0].LineageID != issue2995ApprovedLineage || report.EntryDiagnostics[0].Problem != "outdated_compact_state" {
		return fmt.Errorf("post-repair inspection diagnostics = %+v", report.EntryDiagnostics)
	}
	if len(report.SanctionedExits) != 1 ||
		report.SanctionedExits[0].LineageID != issue2995ApprovedLineage || report.SanctionedExits[0].Operation != "review repair" {
		return fmt.Errorf("post-repair inspection sanctioned exits = %+v", report.SanctionedExits)
	}
	return nil
}

// issue2995SelectedSelector reads the selected (escalated) entry's
// predecessor-empty selector fields from the scratch slot the surfaced-
// selectors step published.
func issue2995SelectedSelector(sandbox *Sandbox) ([]string, error) {
	value, err := scratchValue(sandbox, scratchDispositionSelector)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(value, "\n")
	if len(fields) != 4 || fields[0] != "" || fields[1] != "" || fields[2] == "" || fields[3] == "" {
		return nil, errors.New("j2995 retained an incomplete historical selector")
	}
	return fields, nil
}
