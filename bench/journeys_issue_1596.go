package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

var issue1596StatusCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{
	"--cwd", "--contract", "--agent", "--next-transition",
}}

// selectorFreeFrozenOverlayStatus proves the fresh-process STATUS request has
// no selector to reconstruct the overlay itself, so it must rediscover the one
// frozen reviewing authority rather than offer a new START (issue #1596).
func selectorFreeFrozenOverlayStatus(r *journeyRun) error {
	before, beforeCount, err := frozenAuthorityInventory(r)
	if err != nil {
		return err
	}

	status, document, err := frozenLineageStatus(r, "")
	if err != nil {
		return err
	}
	if status.Authority.LineageID != stagedSuccessorLineage || status.Authority.State != "reviewing" || status.Authority.Revision == "" ||
		status.TargetIdentity == "" || status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "reviewer_results_required" ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("selector-free frozen overlay STATUS = authority=%+v target=%q transition=%+v", status.Authority, status.TargetIdentity, status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "reviewer_result" || input.CaptureOperation != "review.capture-result" || input.ArtifactSubject.SubjectHash == "" ||
		status.argument("lineage") != stagedSuccessorLineage || status.argument("target") != status.TargetIdentity ||
		status.argument("expected-revision") != status.Authority.Revision || status.argument("order") != "0" {
		return fmt.Errorf("selector-free frozen overlay collect binding = %+v", input)
	}
	var projection struct {
		Kind       string `json:"kind"`
		Projection string `json:"projection"`
		BaseTree   string `json:"base_tree"`
	}
	if err := json.Unmarshal(document["projection"], &projection); err != nil || projection.Kind != "base-workspace-overlay" ||
		projection.Projection != "staged" || projection.BaseTree == "" {
		return fmt.Errorf("selector-free frozen overlay projection = %+v, %v", projection, err)
	}

	after, afterCount, err := frozenAuthorityInventory(r)
	if err != nil || afterCount != beforeCount || !bytes.Equal(before, after) {
		return fmt.Errorf("selector-free frozen overlay STATUS mutated authority inventory: count %d/%d err=%v", beforeCount, afterCount, err)
	}
	return nil
}

func issue1596Journeys() []Journey {
	return []Journey{{
		ID:     "j115-selectorless-frozen-workspace-overlay-resumes-after-restart",
		Review: reviewOptedIn,
		Title:  "Selector-free STATUS rediscovers a frozen recovered workspace-overlay reviewer slot after restart",
		Source: "issue #1596: selector-free negotiated STATUS resumes one canonical frozen workspace-overlay authority without creating a new lineage",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: committed predecessor for staged workspace-overlay recovery", Fixture: commitStagedRecoveryCandidate},
			{Name: "start the predecessor base-diff review", Requires: statusCapability, Composite: func(r *journeyRun) error {
				envelope, err := readStatusFor(r, "--lineage", stagedRecoveryLineage, "--base-ref", r.sandbox.Scratch["staged-recovery-base"])
				if err != nil || envelope.NextTransition.Kind != "execute" {
					return fmt.Errorf("predecessor START = %+v, %v", envelope.NextTransition, err)
				}
				observation, err := runPrintedTransition(r, envelope)
				if err != nil || observation.ExitCode != 0 {
					return fmt.Errorf("predecessor START exited %d: %v", observation.ExitCode, err)
				}
				return nil
			}},
			{Name: "capture the predecessor blocker", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
				return captureCorrectableFindingFor(r, stagedPredecessorSelectors(r.sandbox)...)
			}},
			{Name: "enter correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--captured-results=true")},
			{Name: "forecast the predecessor correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--correction-lines", "3")},
			{Name: "fixture: stage the recovered workspace-overlay candidate", Fixture: stageExpandedCorrection},
			{Name: "recover the staged authority into its frozen reviewing successor", Requires: statusCapability, Composite: recoverStagedCorrection},
			{Name: "fresh-process selector-free STATUS rediscovers the frozen reviewer collect slot without mutation", Requires: issue1596StatusCapability, Composite: selectorFreeFrozenOverlayStatus},
		},
	}}
}
