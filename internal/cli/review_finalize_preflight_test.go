package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestNegotiatedReviewFinalizeRejectsReviewerPreflightWithoutAuthorityMutation(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		missing bool
	}{
		{name: "missing result file", missing: true},
		{name: "malformed JSON", payload: `{`},
		{name: "invalid shape", payload: `{}`},
		{name: "non-concrete exact sentinel", payload: `{"findings":[],"evidence":["tbd"]}`},
		{name: "selected lens mismatch", payload: `{"lens":"review-risk","findings":[],"evidence":["reviewed exact candidate tree"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			lineage := "review-preflight-" + reviewTestSlug(tt.name)
			var startedOutput bytes.Buffer
			if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, &startedOutput); err != nil {
				t.Fatal(err)
			}
			var started ReviewFacadeStartResult
			decodeStrictReviewJSON(t, startedOutput.Bytes(), &started)
			if !reflect.DeepEqual(started.SelectedLenses, []string{reviewtransaction.LensReliability}) {
				t.Fatalf("selected lenses = %v, want reliability", started.SelectedLenses)
			}
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			beforeBytes := readReviewOperationFile(t, store.StatePath())

			resultPath := filepath.Join(t.TempDir(), "reviewer.json")
			if !tt.missing {
				if err := os.WriteFile(resultPath, []byte(tt.payload), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			runErr := RunReview([]string{
				"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
				"--lineage", lineage, "--result", resultPath,
			}, &output)
			if runErr == nil {
				t.Fatalf("invalid reviewer payload succeeded: %s", output.String())
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != ReviewIntegrationOperationFinalize || failure.Phase != "preflight" ||
				failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted ||
				!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
				failure.NextAction != "correct_request" || failure.LineageID != lineage {
				t.Fatalf("preflight failure = %#v", failure)
			}
			after, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if after.Revision != before.Revision || !bytes.Equal(readReviewOperationFile(t, store.StatePath()), beforeBytes) {
				t.Fatalf("invalid reviewer payload changed compact authority: before=%s after=%s", before.Revision, after.Revision)
			}
		})
	}
}

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

func reviewTestSlug(value string) string {
	result := make([]byte, 0, len(value))
	separator := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			result = append(result, byte(char))
			separator = false
			continue
		}
		if len(result) > 0 && !separator {
			result = append(result, '-')
			separator = true
		}
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return string(result)
}
