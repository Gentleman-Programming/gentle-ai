package main

import (
	"os"
	"testing"
)

// Retiring the SDD source must not remove the independent review journeys.
func TestRetiredSDDSourceAbsent(t *testing.T) {
	if _, err := os.Stat("journeys_sdd.go"); !os.IsNotExist(err) {
		t.Fatalf("retired SDD source must be absent after cleanup: %v", err)
	}
}

func TestReviewRecoveryGuardRailsPreserved(t *testing.T) {
	journeys := reviewRecoveryJourneys()
	if len(journeys) != 1 || journeys[0].ID != "j43-recovery-guard-rails-as-an-operator-meets-them" {
		t.Fatalf("review recovery journey ID changed: %+v", journeys)
	}
	wantSteps := []string{
		"fixture: repo",
		"fixture: stage docs",
		"review start",
		"review finalize",
		"walk into the recovery guard rails",
		"invalidate the healthy approved authority",
		"the refused invalidation changed nothing",
		"fixture: change the candidate, which is what all three asked for",
		"recover, following exactly what the gate then names",
	}
	if len(journeys[0].Steps) != len(wantSteps) {
		t.Fatalf("j43 steps = %d, want %d", len(journeys[0].Steps), len(wantSteps))
	}
	for index, want := range wantSteps {
		if got := journeys[0].Steps[index].Name; got != want {
			t.Errorf("j43 step %d = %q, want %q", index, got, want)
		}
	}
}

var portableReviewAuthorityJourneyIDs = []string{
	"j59-current-status-and-start-ignore-sibling-worktree-transaction",
	"j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
	"j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
}

func portableReviewAuthorityJourneySet(found bool) map[string]bool {
	journeys := make(map[string]bool, len(portableReviewAuthorityJourneyIDs))
	for _, id := range portableReviewAuthorityJourneyIDs {
		journeys[id] = found
	}
	return journeys
}

func TestPortableReviewAuthorityJourneysAreRegistered(t *testing.T) {
	want := portableReviewAuthorityJourneySet(false)
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
	// The corpus count is pinned by bench/testdata/journeys.manifest. The
	// atomic journeys preserve selected-worktree isolation, explicit active
	// continuation, and terminal burn after durable authority retirement.
	for id, found := range want {
		if !found {
			t.Errorf("required review authority journey %q is not registered", id)
		}
	}
}

func TestPortableReviewAuthorityJourneys(t *testing.T) {
	binary := os.Getenv("GENTLE_AI_BENCH_BINARY")
	if binary == "" {
		t.Skip("set GENTLE_AI_BENCH_BINARY to run the native review authority journeys")
	}
	want := portableReviewAuthorityJourneySet(true)
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

func TestIndependentReviewAuthorityProofRemains(t *testing.T) {
	want := map[string]bool{
		"j59-current-status-and-start-ignore-sibling-worktree-transaction":          false,
		"j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow": false,
		"j111-approved-transaction-burns-and-shipped-gates-are-unmanaged":           false,
	}
	for _, journey := range Journeys() {
		if _, ok := want[journey.ID]; ok {
			want[journey.ID] = true
		}
	}
	for id, present := range want {
		if !present {
			t.Errorf("retained independent review authority proof %q is missing", id)
		}
	}
}
