package main

import (
	"os"
	"testing"
)

var sddFailClosedAuthorityJourneyIDs = []string{
	"j52-sdd-stale-authority-does-not-shadow-approved-candidate",
	"j53-sdd-ambiguous-authorities-fail-closed",
	"j54-sdd-missing-authority-receipt-fails-closed",
	"j55-sdd-mismatched-authority-receipt-fails-closed",
	"j56-sdd-non-allow-post-apply-gate-fails-closed",
	"j57-sdd-authority-drift-during-discovery-fails-closed",
	"j58-sdd-foreign-openspec-path-fails-closed",
}

func sddFailClosedAuthorityJourneySet(found bool) map[string]bool {
	journeys := make(map[string]bool, len(sddFailClosedAuthorityJourneyIDs))
	for _, id := range sddFailClosedAuthorityJourneyIDs {
		journeys[id] = found
	}
	return journeys
}

func TestSDDFailClosedAuthorityJourneysAreRegistered(t *testing.T) {
	want := sddFailClosedAuthorityJourneySet(false)
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
	if got := len(seen); got != 58 {
		t.Errorf("core journey count = %d, want 58", got)
	}
	for id, found := range want {
		if !found {
			t.Errorf("required SDD authority journey %q is not registered", id)
		}
	}
}

func TestSDDFailClosedAuthorityJourneys(t *testing.T) {
	binary := os.Getenv("GENTLE_AI_BENCH_BINARY")
	if binary == "" {
		t.Skip("set GENTLE_AI_BENCH_BINARY to run the native SDD authority journeys")
	}
	want := sddFailClosedAuthorityJourneySet(true)
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
