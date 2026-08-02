package reviewtransaction

import (
	"reflect"
	"strings"
	"testing"
)

func TestCompactReviewRejectsUnavailableInspectionButHistoricalStateRemainsReadable(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "result-reopen-inspection")
	if len(state.SelectedLenses) != 1 {
		t.Fatalf("selected lenses = %v, want one medium-risk lens", state.SelectedLenses)
	}
	before := state
	unavailable := CompactReviewInput{LensResults: []LensResult{{
		Lens: state.SelectedLenses[0], Findings: []Finding{},
		Evidence: []string{"Access denied; candidate was not inspected."},
	}}}
	if err := state.CompleteReview(unavailable); err == nil || !strings.Contains(err.Error(), "unavailable candidate inspection") {
		t.Fatalf("unavailable inspection completion error = %v", err)
	}
	if !reflect.DeepEqual(state, before) {
		t.Fatal("rejected unavailable inspection mutated reviewing authority")
	}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{{
		Lens: state.SelectedLenses[0], Findings: []Finding{}, Evidence: []string{"reviewed exact candidate tree"},
	}}}); err != nil {
		t.Fatal(err)
	}
	historical := state.LensResults[0]
	historical.Evidence = []string{"Access denied; candidate was not inspected."}
	historical.ResultHash = ""
	canonical, err := CanonicalCompactLensResult(historical)
	if err != nil {
		t.Fatalf("canonicalize historical reviewer evidence: %v", err)
	}
	state.LensResults[0] = canonical
	if err := state.Validate(); err != nil {
		t.Fatalf("historical validating authority became unreadable: %v", err)
	}
}
