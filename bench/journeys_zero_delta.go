package main

import "fmt"

// journeys_zero_delta.go covers issue #2586's fix: `review start` used to
// refuse an empty (zero-delta) candidate only on the negotiated route. A
// plain, non-negotiated `review start` (no --contract) on a clean,
// fully-committed worktree created a lineage and finalized it to an
// approved receipt that inspected nothing (base_tree == candidate_tree ==
// HEAD, paths: []) -- exactly the receipt the issue reports being
// discoverable afterward as "governing" a later, genuinely unreviewed
// candidate that happens to share its final tree. The fix moved the
// refusal into the one shared preflight path both routes go through, so
// this journey drives the DIRECT route specifically: the one this defect
// actually reached in practice (the negotiated route's own regression
// coverage already lives in internal/cli/review_start_empty_candidate_test.go).

func zeroDeltaJourneys() []Journey {
	return []Journey{
		{
			ID:     "j72-direct-review-start-refuses-empty-candidate",
			Title:  "Direct (non-negotiated) `review start` on a clean, fully-committed worktree refuses instead of minting a zero-delta receipt",
			Source: "issue #2586: a plain `review start` (no --contract) on a clean worktree used to create a lineage and finalize an approved receipt that inspected nothing; the empty-candidate refusal was previously negotiated-route-only",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "direct review start on a clean worktree refuses, naming --base-ref", Requires: startCapability,
					Args: productArgs("review", "start"), After: assertStderrContains("--base-ref"), AbortOnBlock: true},
			},
		},
		{
			ID:     "j101-zero-path-base-diff-stops-before-start",
			Title:  "Committed-only STATUS stops an empty base-diff instead of offering a START that preflight rejects",
			Source: "issue #3102: #1641's authorized empty-root bootstrap reaches a base-diff whose selected base can equal the candidate; STATUS must return typed STOP, not fresh_target_ready",
			Steps: []Step{
				{Name: "fixture: root-commit repository", Fixture: baseRepo},
				{Name: "committed-only STATUS with the root as base returns the empty-root bootstrap STOP", Requires: statusCapability, Composite: requireZeroPathBaseDiffStop},
			},
		},
	}
}

func requireZeroPathBaseDiffStop(run *journeyRun) error {
	base, err := gitOut(run.sandbox, run.sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	status, err := readStatusForContract(run, reviewContractV2, "--base-ref", base, "--committed-only")
	if err != nil {
		return err
	}
	if status.Projection.BaseTree != status.Projection.CurrentCandidateTree || len(status.Projection.Paths) != 0 ||
		status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "empty_base_diff_bootstrap_required" ||
		status.NextTransition.Execute.Operation != "" || status.NextTransition.Execute.Command != "" ||
		len(status.NextTransition.Collect.Inputs) != 0 || status.Authority.LineageID != "" {
		return fmt.Errorf("zero-path committed base-diff status = %#v", status)
	}
	return nil
}
