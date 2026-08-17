package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// lensContextBudgetCandidatePaths is two paths, not thirty-three. Path count
// stopped deciding representability with issue #3367: the bound is the
// aggregate byte budget the reviewer block has to fit inside, so an
// unrepresentable candidate is made of bytes now.
const lensContextBudgetCandidatePaths = 2

func oversizedLensContextCandidate(sandbox *Sandbox) error {
	body := strings.Repeat("oversized evidence line\n", 100_000)
	for index := range lensContextBudgetCandidatePaths {
		path := filepath.Join(sandbox.Repo, fmt.Sprintf("candidate/bulk-%02d.txt", index))
		if err := sandbox.write(path, body); err != nil {
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
	if started.ExitCode == 0 {
		return fmt.Errorf("oversized candidate START succeeded and left authority STATUS could only refuse: %s", firstLine(started.Stdout))
	}
	if !strings.Contains(started.Stdout+started.Stderr, "lens_context_budget_exceeded") {
		return fmt.Errorf("oversized candidate START refused without naming its budget: %s", firstLine(started.Stderr))
	}

	// Nothing was persisted, so the repository is exactly where it was: STATUS
	// still offers the same start, rather than a permanent stop on a lineage
	// no reviewer action can ever advance.
	status, err := readStatusForContract(r, reviewContractV2)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != "" || status.NextTransition.Kind != "execute" ||
		status.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("refused oversized candidate STATUS = authority=%+v transition=%+v", status.Authority, status.NextTransition)
	}
	return nil
}

func lensContextBudgetJourneys() []Journey {
	return []Journey{{
		ID:     "j102-status-stops-oversized-lens-context",
		Title:  "An unrepresentable immutable reviewer context is refused by START instead of becoming a dead lineage",
		Source: "issues #2773 and #3367: a frozen candidate whose complete reviewer evidence exceeds the native budget must be refused before any authority is created",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: staged reviewer evidence beyond the byte budget", Fixture: oversizedLensContextCandidate},
			{Name: "enable review mode only in the disposable journey clone", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "negotiated START refuses the oversized frozen reviewer context without persisting authority", Requires: frozenLineageStatusCapability, Composite: stopOversizedLensContextReview},
		},
	}}
}
