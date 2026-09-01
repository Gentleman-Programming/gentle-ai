package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const issue3321Lineage = "issue-3321-base-equivalent-correction"

// issue3321Journeys proves a valid correction may remove a candidate-only path.
// The correction delta remains bound to the frozen genesis path even though the
// restored base-to-current projection no longer lists that path.
func issue3321Journeys() []Journey {
	return []Journey{{
		ID:     "j113-correction-removes-candidate-only-path",
		Review: reviewOptedIn,
		Title:  "A bounded correction may remove a candidate-only file and continue through STATUS",
		Source: "issue #3321 under #3587 and #3797: correction paths are frozen-to-current deltas, and the terminal validator capture exposes acknowledgement before the corrected lineage burns",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: candidate-only defect and unchanged reviewed companion", Fixture: stageIssue3321Candidate},
			{Name: "start the exact review lineage", Requires: startNamedCapability,
				Args: productArgs("review", "start", "--lineage", issue3321Lineage)},
			{Name: "capture the blocking candidate-only finding and finish selected lenses", Requires: captureResultCapability,
				Composite: func(r *journeyRun) error { return captureCorrectableFindingFor(r, "--lineage", issue3321Lineage) }},
			{Name: "capture the three-line deletion plan from STATUS", Requires: captureCorrectionPlanCapability,
				Composite: func(r *journeyRun) error { return captureCorrectionPlanFor(r, issue3321Lineage, 3) }},
			{Name: "fixture: remove the candidate-only file exactly back to base", Fixture: removeIssue3321CandidateOnlyPath},
			{Name: "targeted validator approves and exposes acknowledgement before the corrected lineage burns", Requires: capturedProviderValidatorStatusCapability,
				Composite: func(r *journeyRun) error { return captureProviderValidatorSlotFor(r, issue3321Lineage) }},
			{Name: "no correction authority survives the terminal validator capture", Requires: statusCapability,
				Composite: func(r *journeyRun) error { return requireAtomicLineageAcknowledged(r, issue3321Lineage) }},
		},
	}}
}

func stageIssue3321Candidate(sandbox *Sandbox) error {
	candidate := "package candidate\n\nfunc value() int { return 1 }\n"
	companion := "package candidate\n\nfunc companion() int {\n\tvalue := 1\n\tvalue++\n\tvalue++\n\treturn value\n}\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), candidate); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "companion.go"), companion); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go", "companion.go"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil || paths != "candidate.go\ncompanion.go" {
		return fmt.Errorf("issue #3321 staged candidate paths = %q, %v", paths, err)
	}
	return nil
}

func removeIssue3321CandidateOnlyPath(sandbox *Sandbox) error {
	if err := os.Remove(filepath.Join(sandbox.Repo, "candidate.go")); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "HEAD", "--name-only")
	if err != nil || paths != "companion.go" {
		return fmt.Errorf("base-equivalent correction paths = %q, %v; candidate.go must disappear from the current projection", paths, err)
	}
	status, err := gitOut(sandbox, sandbox.Repo, "status", "--short", "--", "candidate.go")
	if err != nil || status != "AD candidate.go" {
		return fmt.Errorf("candidate-only correction index state = %q, %v", status, err)
	}
	return nil
}

func completeIssue3321Correction(r *journeyRun) error {
	if err := completeCorrectedReviewForContract(r, issue3321Lineage, reviewContractV2); err != nil {
		return err
	}
	return requireAtomicLineageAcknowledged(r, issue3321Lineage)
}
