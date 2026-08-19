package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const issue3194Lineage = "issue-3194-preplan-correction-forecast"

// issue3194Journeys proves the honest correction order: a consumer cannot know
// a line count before writing the lines, so the bounded correction is already
// in the working tree when the forecast is submitted. Every token of the
// correction transition must name the frozen reviewed candidate the correction
// request and the repository-context handle already commit to, and the
// descriptor must execute exactly as issued.
//
// This is the within-budget sibling of j91 (issue #1800). D1800's audited
// abandon stays the exit for a pre-plan edit that no longer fits the frozen
// budget; it was never the exit for one that does, and issue #3194 collected
// occurrences where a budget-compliant correction could not be submitted at
// all because STATUS paired a live `--target` with a frozen handle.
func issue3194Journeys() []Journey {
	return []Journey{{
		ID:     "j111-preplan-correction-forecast-executes-as-issued",
		Review: reviewOptedIn,
		Title:  "Within-budget correction written before its forecast submits as issued and keeps routing",
		Source: "issue #3194: the correction transition binds the frozen candidate, not the moved live snapshot",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "clear any clone-local review override (a clone may only ever assert off)", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "fixture: stage candidate", Fixture: stageWaveCandidate},
			{Name: "start product-created compact review", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", issue3194Lineage)},
			{Name: "capture candidate finding", Requires: captureResultCapability, Composite: captureIssue3194CorrectableFinding},
			{Name: "finalize into correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", issue3194Lineage, "--captured-results=true"), After: requireReviewState("correction_required", issue3194Lineage)},
			{Name: "freeze the pre-forecast candidate identity", Requires: statusCapability, Composite: recordIssue3194FrozenCandidate},
			{Name: "fixture: write the within-budget correction before forecasting it", Fixture: writeCorrectedCandidate},
			{Name: "STATUS binds the frozen candidate, not the moved live snapshot", Requires: statusCapability, Composite: proveIssue3194FrozenBinding},
			{Name: "the issued forecast descriptor executes verbatim", Requires: finalizeCorrectionCapability, Composite: submitIssue3194CorrectionForecast},
			{Name: "the corrected candidate keeps routing to repository verification", Requires: statusCapability, Composite: proveIssue3194CorrectionContinues},
		},
	}}
}

func captureIssue3194CorrectableFinding(r *journeyRun) error {
	return captureCorrectableFindingFor(r, "--lineage", issue3194Lineage)
}

func recordIssue3194FrozenCandidate(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, issue3194Lineage, reviewContractV2)
	if err != nil {
		return err
	}
	if status.Authority == nil || status.Authority.State != "correction_required" || status.NextTransition == nil ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "correction_plan_required" ||
		status.Projection.CurrentSnapshotIdentity == "" || status.TargetIdentity != status.Projection.CurrentSnapshotIdentity {
		return fmt.Errorf("pre-forecast correction status = %+v", status)
	}
	r.sandbox.Scratch["issue3194-frozen-candidate"] = status.Projection.CurrentSnapshotIdentity
	r.sandbox.Scratch["issue3194-revision"] = status.Authority.Revision
	return nil
}

// proveIssue3194FrozenBinding is the defect's exact shape. The reported
// occurrences all failed here: `--target` named the live snapshot the
// correction edit had just produced, while the repository-context handle in the
// same descriptor could only ever commit to the frozen candidate, so no value
// substitution could satisfy both.
func proveIssue3194FrozenBinding(r *journeyRun) error {
	frozen := r.sandbox.Scratch["issue3194-frozen-candidate"]
	status, err := readCorrectionStatusForContract(r, issue3194Lineage, reviewContractV2)
	if err != nil {
		return err
	}
	if status.NextTransition == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.ReasonCode != "correction_plan_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("post-edit correction transition = %+v", status.NextTransition)
	}
	if status.TargetIdentity == frozen {
		return fmt.Errorf("the correction edit never moved the live snapshot away from %s", frozen)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Submission == nil {
		return fmt.Errorf("correction transition carries no submission descriptor: %+v", input)
	}
	for _, argument := range input.Arguments {
		if argument.Name == "target" && argument.Value != frozen {
			return fmt.Errorf("collect target argument = %s, want the frozen candidate %s", argument.Value, frozen)
		}
	}
	target, found := "", false
	for _, token := range input.Submission.ArgumentTokens {
		if value, ok := strings.CutPrefix(token, "--target="); ok {
			target, found = value, true
		}
	}
	if !found || target != frozen {
		return fmt.Errorf("descriptor --target = %q (found %t), want the frozen candidate %s", target, found, frozen)
	}
	return nil
}

func submitIssue3194CorrectionForecast(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, issue3194Lineage, reviewContractV2)
	if err != nil {
		return err
	}
	arguments, err := correctionSubmissionArguments(r, status, "correction_plan_required", "correction_lines", "1")
	if err != nil {
		return err
	}
	result, err := decodeWaveOperation(r.runAt(r.sandbox.Root, arguments, false), "pre-forecast correction submission")
	if err != nil || result.State != "correction_required" || result.LineageID != issue3194Lineage {
		return fmt.Errorf("pre-forecast correction submission result = %+v, %v", result, err)
	}
	return nil
}

// proveIssue3194CorrectionContinues proves the unblock, not just the accepted
// forecast: the lineage routes on to repository verification for the corrected
// candidate instead of dead-ending in correction_required.
func proveIssue3194CorrectionContinues(r *journeyRun) error {
	frozen := r.sandbox.Scratch["issue3194-frozen-candidate"]
	status, err := readCorrectionStatusForContract(r, issue3194Lineage, reviewContractV2)
	if err != nil {
		return err
	}
	if status.Authority == nil || status.Authority.State != "correction_required" || status.ValidationRequest == nil ||
		status.NextTransition == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.ReasonCode != "correction_repository_verification_required" {
		return fmt.Errorf("post-forecast correction routing = %+v", status)
	}
	if status.ValidationRequest.CorrectionTargetIdentity == frozen {
		return fmt.Errorf("correction target never advanced past the frozen candidate %s", frozen)
	}
	forecast, err := issue3194ProposedCorrectionLines(r)
	if err != nil {
		return err
	}
	if forecast != 1 {
		return fmt.Errorf("recorded correction forecast = %d, want 1", forecast)
	}
	return nil
}

func issue3194ProposedCorrectionLines(r *journeyRun) (int, error) {
	common, err := gitOut(r.sandbox, r.sandbox.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return 0, err
	}
	stateBytes, err := os.ReadFile(filepath.Join(strings.TrimSpace(common), "gentle-ai", "review-transactions", "v2", issue3194Lineage, "review-state.json"))
	if err != nil {
		return 0, err
	}
	var record struct {
		State struct {
			ProposedCorrectionLines *int `json:"proposed_correction_lines"`
		} `json:"state"`
	}
	if err := json.Unmarshal(stateBytes, &record); err != nil {
		return 0, err
	}
	if record.State.ProposedCorrectionLines == nil {
		return 0, fmt.Errorf("authority recorded no correction forecast")
	}
	return *record.State.ProposedCorrectionLines, nil
}
