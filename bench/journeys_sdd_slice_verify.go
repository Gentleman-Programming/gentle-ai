package main

// j87-scope-slice-verify is the chain-projected SDD journey required by
// Requirement: Chained Slice Lifecycle (#2268). The validator-side decision
// table (slice PASS admitted, unknown identity rejected, whole-path slice
// fields inert) is pinned by the sddstatus unit tests in bounded_review_test.go
// and verification_test.go; this journey declares the end-to-end CLI shape so
// the bench corpus records that the product surface exists, and so a future
// driven run can pick it up by registering an execute transition (the
// fixture-free path the design's D10 contingency permits when the per-journey
// budget is too tight for full Git lifecycle plumbing).
//
// Journey ID bumped from j82 to j87 (count pin remains 83 here; will be
// bumped to 86 after rebase onto upstream/main which added j82-reviewed-superset,
// j83-pre-pr-moving-base, and j86-approved-base-diff-local-parent-merge) because
// upstream/main added
// j82-reviewed-superset-pre-push-allows-unpublished-subset (#2127),
// j83-pre-pr-moving-advertised-base-binds-merge-base (#2127), and
// j86-approved-base-diff-local-parent-merge-preserves-approved-receipt (#2388)
// between this branch's base 3c6a6341 and the current tip 9d250804.
func j87ScopeSliceVerifyJourneys() []Journey {
	return []Journey{
		{
			ID:     "j87-scope-slice-verify",
			Title:  "Chained slice lifecycle: slice PASS admitted under dual authority; whole-path slice fields are inert",
			Source: "issue #2268 / Requirement: Chained Slice Lifecycle + Requirement: Slice PASS Never Implies Whole-Change Completion + Requirement: Whole-Change Backward Compatibility",
			Steps:  []Step{},
		},
	}
}
