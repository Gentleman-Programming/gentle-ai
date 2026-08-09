package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// capturedInferentialArtifact freezes a standard-risk candidate and captures
// its single selected lens carrying exactly one severe, candidate-causal
// finding whose evidence class is inferential — the one shape compact
// finalize cannot resolve without a refuter outcome.
func capturedInferentialArtifact(t *testing.T) (string, ReviewFacadeStartResult, reviewtransaction.CompactRecord) {
	t.Helper()
	repo, started, _, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	result.Findings = []facadeFinding{{
		ID:                reviewtransaction.FindingIDPrefixForLens(lens) + "001",
		Lens:              lens,
		Location:          "tracked.txt:1",
		Severity:          "CRITICAL",
		Claim:             "the candidate line depends on an external contract this review cannot observe",
		ProofRefs:         []string{"tracked.txt:1"},
		EvidenceClass:     reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(input, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return repo, started, record
}

// TestStatusOffersRefuterBeforeFinalizeForInferentialFinding pins issue #2823:
// STATUS used to answer `execute review.finalize` the moment every artifact was
// captured, without looking at what those artifacts said. Finalize then refused
// with "inferential finding requires one refuter outcome", and the next STATUS
// returned the same finalize — a loop with no route to the outcome it demands.
// The negotiated route must ask for the refuter outcomes it needs.
func TestStatusOffersRefuterBeforeFinalizeForInferentialFinding(t *testing.T) {
	repo, started, record := capturedInferentialArtifact(t)

	var out bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}, &out); err != nil {
		t.Fatalf("%v\nSTDOUT: %s", err, out.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, out.Bytes(), &status)

	transition := status.NextTransition
	if transition == nil {
		t.Fatal("status returned no next_transition")
	}
	if transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "refuter_outcomes_required" {
		t.Fatalf("next_transition = %s/%s, want collect/refuter_outcomes_required", transition.Kind, transition.ReasonCode)
	}
	if transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("collect inputs = %#v, want exactly one", transition.Collect)
	}
	input := transition.Collect.Inputs[0]
	if input.Name != "refuter_outcomes" || input.CaptureOperation != "external.run_refuter" {
		t.Fatalf("collect input = %q/%q", input.Name, input.CaptureOperation)
	}
	if input.Schema != reviewRefuterSchemaID {
		t.Fatalf("collect input schema = %q, want %q", input.Schema, reviewRefuterSchemaID)
	}
	if input.RefuterClaims == nil || len(*input.RefuterClaims) != 1 {
		t.Fatalf("refuter claims = %#v, want exactly one", input.RefuterClaims)
	}
	claim := (*input.RefuterClaims)[0]
	if claim.FindingID == "" || claim.Proof == "" {
		t.Fatalf("refuter claim carries no finding or proof: %#v", claim)
	}
	if claim.SnapshotIdentity != record.State.InitialSnapshot.Identity {
		t.Fatalf("refuter claim snapshot identity = %q, want initial snapshot identity %q", claim.SnapshotIdentity, record.State.InitialSnapshot.Identity)
	}
	// The refuter reaches finalize through its existing --refuter flag, so this
	// input intentionally carries no submission descriptor: only the two
	// provider-bound finalize value slots have one.
	if input.Submission != nil {
		t.Fatalf("refuter input carries a submission descriptor: %#v", input.Submission)
	}
	// Every claim binding the refuter needs must already be argv.
	for _, want := range []string{"lineage", "target", "expected-revision"} {
		if !hasTransitionArgument(input.Arguments, want) {
			t.Fatalf("refuter input arguments miss %q: %#v", want, input.Arguments)
		}
	}
}

func hasTransitionArgument(arguments []ReviewTransitionArgument, name string) bool {
	for _, argument := range arguments {
		if argument.Name == name {
			return true
		}
	}
	return false
}

// TestStatusStillOffersFinalizeForDeterministicFinding pins the complement: a
// severe finding native finalize can resolve on its own must not be diverted
// through a refuter it does not need.
func TestStatusStillOffersFinalizeForDeterministicFinding(t *testing.T) {
	repo, started, _, _, _ := capturedArtifacts(t, false)

	var out bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}, &out); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, out.Bytes(), &status)

	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "captured_results_ready" {
		t.Fatalf("next_transition = %#v, want execute/captured_results_ready", status.NextTransition)
	}
}
