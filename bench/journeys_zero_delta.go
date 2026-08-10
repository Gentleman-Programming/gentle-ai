package main

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
	}
}
