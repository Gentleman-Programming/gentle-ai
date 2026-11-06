package main

import "testing"

func TestTransitionCorpusPreservesStandaloneReviewModeChange(t *testing.T) {
	journeys := transitionJourneys()
	if len(journeys) != 1 || journeys[0].ID != "tr09-mode-flip-while-a-review-lineage-is-open" {
		t.Fatal("standalone review transition missing")
	}
}
