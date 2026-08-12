package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// j101 reconstructs #3065's public negotiated STATUS dead end: a staged
// workspace-overlay predecessor is escalated, the operator selects the default
// workspace projection, and representability must be decided before auth.
func issue3065Journeys() []Journey {
	return []Journey{{
		ID:     "j101-unrepresentable-workspace-recovery-collects-selector",
		Title:  "Default workspace-overlay recovery collects a selector before authorization",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3065",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: committed base-diff candidate", Fixture: commitStagedRecoveryCandidate},
			{Name: "negotiate and execute the base-diff predecessor start", Requires: statusCapability, Composite: startIssue3065Predecessor},
			{Name: "capture the predecessor finding", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
				return captureCorrectableFindingFor(r, stagedPredecessorSelectors(r.sandbox)...)
			}},
			{Name: "enter correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--captured-results=true")},
			{Name: "forecast the bounded correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--correction-lines", "3")},
			{Name: "fixture: stage the corrected overlay", Fixture: stageExpandedCorrection},
			{Name: "execute the legal staged-overlay recovery", Requires: statusCapability, Composite: recoverStagedCorrection},
			{Name: "capture the staged successor review", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
				return captureAllLensesFor(r, stagedRecoverySelectors(r.sandbox)...)
			}},
			{Name: "enter final verification", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", stagedSuccessorLineage, "--captured-results=true")},
			{Name: "drive the staged successor to escalation", Requires: finalizeFailedCapability, Composite: failIssue3065Verification, After: requireReviewState("escalated", stagedSuccessorLineage)},
			{Name: "fixture: clean noise and change the staged candidate", Fixture: mutateIssue3065Candidate},
			{Name: "default workspace-overlay STATUS collects selector before authorization", Requires: statusCapability, Composite: proveIssue3065RecoveryCollection},
		},
	}}
}

func startIssue3065Predecessor(r *journeyRun) error {
	envelope, err := readStatusFor(r, stagedPredecessorSelectors(r.sandbox)...)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("base-diff predecessor start transition = %+v", envelope.NextTransition)
	}
	started, err := runPrintedTransition(r, envelope)
	if err != nil {
		return err
	}
	result, err := decodeWaveOperation(started, "issue #3065 predecessor start")
	if err != nil || result.LineageID != stagedRecoveryLineage || result.State != "reviewing" {
		return fmt.Errorf("issue #3065 predecessor start = %+v, %v", result, err)
	}
	return nil
}

func failIssue3065Verification(r *journeyRun) error {
	path, err := writeScratch(r.sandbox, "issue-3065-failed-verification.txt", []byte("go test ./... FAIL\n"))
	if err != nil {
		return err
	}
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", stagedSuccessorLineage,
		"--evidence", path, "--failed=true"), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("failed verification exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	return nil
}

func mutateIssue3065Candidate(sandbox *Sandbox) error {
	for _, name := range []string{"README.md", "scratch.txt"} {
		path := filepath.Join(sandbox.Repo, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int { return 3 }\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err != nil {
		return err
	}
	bytes, err := gitOut(sandbox, sandbox.Repo, "show", ":candidate.go")
	if err != nil {
		return err
	}
	sandbox.Scratch["issue3065-candidate-tree"] = tree
	sandbox.Scratch["issue3065-candidate-bytes"] = bytes
	return nil
}

func proveIssue3065RecoveryCollection(r *journeyRun) error {
	before, err := readStatusFor(r, "--lineage", stagedSuccessorLineage)
	if err != nil || before.Authority.State != "escalated" {
		return fmt.Errorf("issue #3065 predecessor status = %+v, %v", before, err)
	}
	r.sandbox.Scratch["issue3065-authority-revision"] = before.Authority.Revision
	r.sandbox.Scratch["issue3065-receipt"] = fmt.Sprintf("%s\x00%s", before.Receipt.Status, before.Receipt.Identity)
	r.sandbox.Scratch["issue3065-budget"] = fmt.Sprint(before.Frozen.CorrectionBudget)
	inspection := r.sandbox.readBack("review", "inspect-authority", "--cwd", r.sandbox.Repo)
	if inspection.ExitCode != 0 {
		return fmt.Errorf("inspect issue #3065 predecessor: %s", firstLine(inspection.Stderr))
	}
	beforeInspection := strings.TrimSpace(inspection.Stdout)
	selectors := []string{"--lineage", stagedSuccessorLineage, "--base-ref", r.sandbox.Scratch["staged-recovery-base"], "--workspace-overlay"}
	probe, err := readStatusFor(r, selectors...)
	if err != nil {
		return err
	}
	assertCollection := func(name string, status statusEnvelope) error {
		if status.Authority.LineageID != stagedSuccessorLineage || status.Authority.Revision != r.sandbox.Scratch["issue3065-authority-revision"] ||
			status.ActionDisposition != "escalated" || status.NextTransition.Kind != "collect" ||
			status.NextTransition.ReasonCode != "recovery_target_unrepresentable" || status.NextTransition.Execute.Operation != "" ||
			len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Name != "recovery_target_selector" ||
			status.NextTransition.Collect.Inputs[0].CaptureOperation != "external.select_recovery_target" {
			return fmt.Errorf("%s STATUS = %+v, want selector collection before authorization", name, status.NextTransition)
		}
		return nil
	}
	if err := assertCollection("selector-only", probe); err != nil {
		return err
	}
	const actor, reason, successor = "bench-maintainer", "recover staged candidate after failed verification", "issue-3065-decoy-successor"
	authorization := strings.Join([]string{
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + stagedSuccessorLineage,
		"predecessor_revision=" + probe.Authority.Revision,
		"target_identity=" + probe.TargetIdentity,
		"successor_lineage=" + successor,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	authorized, err := readStatusFor(r, append(selectors,
		"--recovery-successor-lineage", successor, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)...)
	if err != nil {
		return err
	}
	if err := assertCollection("authorization-supplied", authorized); err != nil {
		return err
	}
	if authorized.NextTransition.Collect.Inputs[0].Name == "recovery_authorization" {
		return errors.New("default workspace-overlay STATUS collected authorization before representability")
	}
	if authorized.Receipt.Status+"\x00"+authorized.Receipt.Identity != r.sandbox.Scratch["issue3065-receipt"] ||
		fmt.Sprint(authorized.Frozen.CorrectionBudget) != r.sandbox.Scratch["issue3065-budget"] ||
		authorized.Projection.CurrentCandidateTree != probe.Projection.CurrentCandidateTree {
		return errors.New("unrepresentable STATUS changed the predecessor receipt, budget, or projected candidate")
	}
	if indexTree, err := gitOut(r.sandbox, r.sandbox.Repo, "write-tree"); err != nil || indexTree != r.sandbox.Scratch["issue3065-candidate-tree"] {
		return fmt.Errorf("unrepresentable STATUS changed index tree: %q, %v", indexTree, err)
	}
	if candidate, err := gitOut(r.sandbox, r.sandbox.Repo, "show", ":candidate.go"); err != nil || candidate != r.sandbox.Scratch["issue3065-candidate-bytes"] {
		return fmt.Errorf("unrepresentable STATUS changed candidate bytes: %q, %v", candidate, err)
	}
	afterInspection := r.sandbox.readBack("review", "inspect-authority", "--cwd", r.sandbox.Repo)
	if afterInspection.ExitCode != 0 || strings.TrimSpace(afterInspection.Stdout) != beforeInspection {
		return errors.New("unrepresentable STATUS changed the predecessor authority store")
	}
	return nil
}
