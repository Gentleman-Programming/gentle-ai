package main

import (
	"os"
	"testing"
)

var portableSDDFailClosedAuthorityJourneyIDs = []string{
	"j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
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
	// The corpus total used to be asserted here as a hand-written integer. It
	// moved to bench/testdata/journeys.manifest, because two branches that each
	// add one journey each write the same next number and git resolves that
	// silently by taking one side. See TestRegisteredJourneysMatchTheManifest.
	//
	// #3417 retired the former durable-receipt and delivery-gate authority
	// fixtures because a completed transaction no longer remains discoverable.
	// The three atomic journeys above preserve the executable proof surface:
	// selected-worktree isolation, explicit active continuation, and terminal burn.
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
