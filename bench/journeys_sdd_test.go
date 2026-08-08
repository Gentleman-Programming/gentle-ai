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
	// 78 since j76-claude-advisory-result-reaches-delivery (#2692, #2566)
	// and j77-capture-result-input-preflight-is-read-only (#2630 D2).
	// Bump this deliberately when a journey is added, and name it here: the
	// count exists so a journey cannot appear or vanish unnoticed.
	if got := len(seen); got != 78 {
		t.Errorf("core journey count = %d, want 78", got)
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
