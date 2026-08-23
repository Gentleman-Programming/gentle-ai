package cli

import (
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestPrepareCompactReviewerResultsPreservesSelectedLensOrderAndAliases(t *testing.T) {
	selected := []string{
		reviewtransaction.LensRisk,
		reviewtransaction.LensResilience,
		reviewtransaction.LensReadability,
		reviewtransaction.LensReliability,
	}
	results := []facadeReviewerResult{
		{Lens: "risk", Findings: []facadeFinding{}, Evidence: []string{"risk review evidence"}},
		{Lens: reviewtransaction.LensResilience, Findings: []facadeFinding{}, Evidence: []string{"resilience review evidence"}},
		{Lens: "readability", Findings: []facadeFinding{}, Evidence: []string{"readability review evidence"}},
		{Lens: reviewtransaction.LensReliability, Findings: []facadeFinding{}, Evidence: []string{"reliability review evidence"}},
	}
	input, err := prepareCompactReviewerResults(reviewtransaction.CompactState{SelectedLenses: selected}, results, facadeRefuterResult{})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(input.LensResults))
	for index := range input.LensResults {
		got[index] = input.LensResults[index].Lens
	}
	if !reflect.DeepEqual(got, selected) {
		t.Fatalf("canonical lens order = %v, want %v", got, selected)
	}
}
