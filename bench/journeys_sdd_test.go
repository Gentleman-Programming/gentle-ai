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
	// 85 since j76-claude-advisory-result-reaches-delivery (#2692, #2566),
	// j77-capture-result-input-preflight-is-read-only (#2630 D2),
	// j78-lens-finding-id-prefix-discovery (#1844), j79-consecutive-rescope-
	// refuses-before-publication (#2830), and j80-rescope-authorized-evidence-
	// only-retry (#2621).
	// j81's RC-created repair fixture (#2839) follows the independently-owned
	// #2621 journey. j85 proves #1956's START and FINALIZE parser refusals are preflight.
	// j82 proves #2127's reviewed full candidate can publish an unpublished
	// monotonic subset without reopening review.
	// j83 proves #2127's pre-PR path binds its candidate to the unique merge-base
	// while the advertised main ref remains a moving publication boundary.
	// Bump this deliberately when a journey is added, and name it here: the
	// count exists so a journey cannot appear or vanish unnoticed.
	// 86 since j87-scope-slice-verify (#2268 chained slice lifecycle):
	// slice PASS admitted under dual authority while a sibling slice remains
	// pending; whole-path admission untouched because slice envelope fields
	// are allowed but inert (design D7, Requirement: Whole-Change Backward
	// Compatibility / Requirement: Slice PASS Never Implies Whole-Change
	// Completion). Journey ID bumped from j82 to j87 because upstream/main
	// added j82/j83/j86 (#2127, #2388) between this branch's base 3c6a6341
	// and the rebase tip 9d250804. Bumped from 85 once j87 landed.
	if got := len(seen); got != 86 {
		t.Errorf("core journey count = %d, want 86", got)
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
