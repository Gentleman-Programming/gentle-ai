package main

import (
	"os"
	"testing"
)

var portableSDDFailClosedAuthorityJourneyIDs = []string{
	"j52-sdd-stale-authority-does-not-shadow-approved-candidate",
	"j53-sdd-ambiguous-authorities-fail-closed",
	"j54-sdd-missing-authority-receipt-fails-closed",
	"j55-sdd-mismatched-authority-receipt-fails-closed",
	"j56-sdd-non-allow-post-apply-gate-fails-closed",
	"j58-sdd-foreign-openspec-path-fails-closed",
	"j80-rescope-authorized-evidence-only-retry",
	"j81-rc1-consecutive-rescope-repair-executes-printed-command",
}

func portableSDDFailClosedAuthorityJourneySet(found bool) map[string]bool {
	journeys := make(map[string]bool, len(portableSDDFailClosedAuthorityJourneyIDs))
	for _, id := range portableSDDFailClosedAuthorityJourneyIDs {
		journeys[id] = found
	}
	return journeys
}

func TestPortableSDDFailClosedAuthorityJourneysAreRegistered(t *testing.T) {
	want := portableSDDFailClosedAuthorityJourneySet(false)
	seen := map[string]bool{}
	for _, journey := range Journeys() {
		if seen[journey.ID] {
			t.Errorf("journey ID %q collides in the corpus", journey.ID)
		}
		seen[journey.ID] = true
		if _, ok := want[journey.ID]; ok {
			want[journey.ID] = true
		}
	}
	// 88 total after j94's #1867 unreadable/truncated capture recovery journey,
	// j77-capture-result-input-preflight-is-read-only (#2630 D2),
	// j78-lens-finding-id-prefix-discovery (#1844), j79-consecutive-rescope-
	// refuses-before-publication (#2830), and j80-rescope-authorized-evidence-
	// only-retry (#2621).
	// j81's RC-created repair fixture (#2839) follows the independently-owned
	// #2621 journey. j85 proves #1956's START and FINALIZE parser refusals are preflight.
	// j82 proves #2127's reviewed full candidate can publish an unpublished
	// monotonic subset without reopening review.
	// j83 proves #2127's pre-PR path binds its candidate to the unique merge-base
	// while the advertised main ref remains a moving publication boundary. j87
	// proves #2871's correction binds the immutable failure across a later interrupt;
	// j88 proves #2843's unborn STATUS collects explicit untracked intent first;
	// j89 proves #2758 never offers a workspace receipt for a different index; j90 proves #2016 resumes an explicit frozen reviewing lineage after workspace drift; j91 proves #1800's pre-plan exit is audited abandon.
	// j93 proves #2822 classifies stale managed assets before START can persist.
	// #1993 REMOVED two: j38 (the bound-passing-finish refusal routing to the
	// review router) and j39 (the stranded-successor exit it named). Review
	// acts after implementation and verification, so that refusal is gone and
	// both journeys had no subject left. j37 survives, rewritten to prove the
	// opposite of what it used to: the bound passing finish now CLOSES over a
	// corrected candidate and keeps the binding recorded.
	//
	// Bump this deliberately when a journey is added OR removed, and name it
	// here: the count exists so a journey cannot appear or vanish unnoticed.
	if got := len(seen); got != 89 {
		t.Errorf("core journey count = %d, want 89", got)
	}
	for id, found := range want {
		if !found {
			t.Errorf("required SDD authority journey %q is not registered", id)
		}
	}
}

func TestPortableSDDFailClosedAuthorityJourneys(t *testing.T) {
	binary := os.Getenv("GENTLE_AI_BENCH_BINARY")
	if binary == "" {
		t.Skip("set GENTLE_AI_BENCH_BINARY to run the native SDD authority journeys")
	}
	want := portableSDDFailClosedAuthorityJourneySet(true)
	for _, journey := range Journeys() {
		if !want[journey.ID] {
			continue
		}
		t.Run(journey.ID, func(t *testing.T) {
			result := runJourney(binary, journey)
			if result.Status != StatusCompleted {
				t.Fatalf("journey result = %#v", result)
			}
		})
		delete(want, journey.ID)
	}
	for id := range want {
		t.Errorf("native journey %q was not registered", id)
	}
}
