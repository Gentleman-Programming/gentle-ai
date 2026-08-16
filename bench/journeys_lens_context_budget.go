package main

import (
	"fmt"
	"path/filepath"
)

const lensContextBudgetCandidatePaths = 33

func oversizedLensContextCandidate(sandbox *Sandbox) error {
	for index := range lensContextBudgetCandidatePaths {
		path := filepath.Join(sandbox.Repo, fmt.Sprintf("candidate/path-%02d.txt", index))
		if err := sandbox.write(path, "candidate evidence\n"); err != nil {
			return err
		}
		if err := sandbox.git(sandbox.Repo, "add", "--", path); err != nil {
			return err
		}
	}
	return nil
}

func stopOversizedLensContextReview(r *journeyRun) error {
	start, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if start.NextTransition.Kind != "execute" || start.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("oversized candidate START transition = %+v", start.NextTransition)
	}
	args, err := printedCommandArguments(start.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	granted := false
	for index, argument := range args {
		if argument == "--consent=relay" {
			args[index], granted = "--consent=granted", true
		}
	}
	if !granted {
		return fmt.Errorf("oversized candidate START did not request consent: %q", start.NextTransition.Execute.Command)
	}
	started := r.run(args, false)
	if started.ExitCode != 0 {
		return fmt.Errorf("printed oversized candidate START exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}

	status, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if status.Authority.State != "reviewing" || len(status.Projection.Paths) != lensContextBudgetCandidatePaths ||
		status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "lens_context_budget_exceeded" {
		return fmt.Errorf("oversized candidate STATUS = authority=%+v paths=%d transition=%+v", status.Authority, len(status.Projection.Paths), status.NextTransition)
	}
	return nil
}

func lensContextBudgetJourneys() []Journey {
	return []Journey{{
		ID:     "j102-status-stops-oversized-lens-context",
		Title:  "Oversized immutable reviewer context stops instead of reoffering an impossible reviewer slot",
		Source: "issue #2773: a frozen candidate exceeding the native evidence capacity must end STATUS with lens_context_budget_exceeded",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: 33 staged reviewer-evidence paths", Fixture: oversizedLensContextCandidate},
			{Name: "enable review mode only in the disposable journey clone", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "negotiated STATUS starts the review then stops the oversized frozen reviewer context", Requires: frozenLineageStatusCapability, Composite: stopOversizedLensContextReview},
		},
	}}
}
