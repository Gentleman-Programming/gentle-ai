package reviewtransaction

import "testing"

func TestCompleteReviewApprovesCleanAdmittedResultsWithoutFinalEvidence(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state := newCompactTestState(t, repo, "clean-last-event-closure")

	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{
			Lens:     lens,
			Findings: []Finding{},
			Evidence: []string{"inspected the exact frozen candidate"},
		}
	}

	if err := state.CompleteReview(CompactReviewInput{
		LensResults:     results,
		Classifications: []FindingEvidence{},
		RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if state.State != StateValidating {
		t.Fatalf("clean admitted review state = %q, want %q", state.State, StateValidating)
	}
	if err := state.CloseCleanReviewOnLastEvent(); err != nil {
		t.Fatal(err)
	}
	if state.State != StateApproved {
		t.Fatalf("clean admitted review state = %q, want %q", state.State, StateApproved)
	}
}
