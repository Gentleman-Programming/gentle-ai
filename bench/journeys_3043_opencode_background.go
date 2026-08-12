package main

// journeys_3043_opencode_background.go is the base for issue #3043's
// public/runtime benchmark flow. The full flow pins activation, capability
// gating, foreground fallback, safe dispatch, completion handling, and
// restart-loss semantics once slice 2 wires the CLI/env controls; the base
// journey shipped with slice 1 establishes the corpus entry, the source
// reference, and the shared fixture so slice 2 can extend it without
// re-litigating the journey ID or the import path.
//
// The journey is intentionally a single fixture step today: the seam is
// observable in unit tests (composeSDDOrchestrator applies the addendum
// exactly once, byte-for-byte preserves every non-OpenCode runtime, and
// refuses to leak the marker to Kilocode), but the CLI controls that make
// the addendum activatable, capability-detectable, and rollback-safe live
// in slice 2. Adding a `gentle-ai install --agent opencode` step here would
// silently lock the journey to whatever install verb slice 1 leaves
// behind; doing it after slice 2 keeps the journey truthful to the public
// surface it actually exercises.

func opencodeBackgroundSubagentJourneys() []Journey {
	return []Journey{{
		ID:     "j96-opencode-background-subagent-shared-seam-base",
		Title:  "OpenCode SDD orchestrator ships through the canonical shared seam, with the background-policy addendum appended for OpenCode only",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3043",
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
		},
	}}
}
