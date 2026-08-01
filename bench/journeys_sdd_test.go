package main

import (
	"os"
	"testing"
)

func TestSDDFailClosedAuthorityJourneysAreRegistered(t *testing.T) {
	want := map[string]bool{
		"j44-sdd-stale-authority-does-not-shadow-approved-candidate": false,
		"j45-sdd-ambiguous-authorities-fail-closed":                  false,
		"j46-sdd-missing-authority-receipt-fails-closed":             false,
		"j47-sdd-mismatched-authority-receipt-fails-closed":          false,
		"j48-sdd-non-allow-post-apply-gate-fails-closed":             false,
		"j49-sdd-authority-drift-during-discovery-fails-closed":      false,
		"j50-sdd-foreign-openspec-path-fails-closed":                 false,
	}
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
	want := map[string]bool{
		"j44-sdd-stale-authority-does-not-shadow-approved-candidate": true,
		"j45-sdd-ambiguous-authorities-fail-closed":                  true,
		"j46-sdd-missing-authority-receipt-fails-closed":             true,
		"j47-sdd-mismatched-authority-receipt-fails-closed":          true,
		"j48-sdd-non-allow-post-apply-gate-fails-closed":             true,
		"j49-sdd-authority-drift-during-discovery-fails-closed":      true,
		"j50-sdd-foreign-openspec-path-fails-closed":                 true,
	}
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
