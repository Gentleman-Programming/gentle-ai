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
	"j107-sdd-approved-active-change-allows-shared-openspec-scaffolding",
	"j108-sdd-post-review-verify-report-is-natively-bound",
	"j109-sdd-legacy-post-review-report-requires-current-attestation",
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
	// Kept because it is knowledge rather than bookkeeping: #1993 REMOVED two
	// journeys, j38 (the bound-passing-finish refusal routing to the review
	// router) and j39 (the stranded-successor exit it named). Review acts after
	// implementation and verification, so that refusal is gone and both
	// journeys had no subject left. j37 survives, rewritten to prove the
	// opposite of what it used to: the bound passing finish now CLOSES over a
	// corrected candidate and keeps the binding recorded.
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
