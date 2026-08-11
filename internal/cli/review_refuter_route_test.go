package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func capturedInferentialArtifact(t *testing.T) (string, ReviewFacadeStartResult, reviewtransaction.CompactRecord) {
	t.Helper()
	repo, started, _, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	result := admittedReviewerResultForTest(t, repo, record, lens, 0)
	result.Findings = []facadeFinding{{
		ID: reviewtransaction.FindingIDPrefixForLens(lens) + "001", Lens: lens, Location: "tracked.txt:1",
		Severity: "CRITICAL", Claim: "the candidate depends on an external contract", ProofRefs: []string{"tracked.txt:1"},
		EvidenceClass: reviewtransaction.EvidenceInferential, CausalDisposition: reviewtransaction.CausalIntroduced,
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

func TestStatusOffersBoundRefuterBeforeFinalizeForInferentialFinding(t *testing.T) {
	repo, started, record := capturedInferentialArtifact(t)
	status := reviewStatusWithTransition(t, repo, started.LineageID)
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "refuter_outcomes_required" {
		t.Fatalf("next_transition = %#v, want collect/refuter_outcomes_required", transition)
	}
	if transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("collect = %#v, want one input", transition.Collect)
	}
	input := transition.Collect.Inputs[0]
	if input.CaptureOperation != "external.run_refuter" || input.Schema != reviewRefuterSchemaID {
		t.Fatalf("refuter input = %#v", input)
	}
	repositoryContext := refuterTransitionArgumentValue(t, input.Arguments, "repository-context")
	for _, name := range []string{"lineage", "expected-revision", "target"} {
		if refuterTransitionArgumentValue(t, input.Arguments, name) == "" {
			t.Fatalf("refuter input misses %q", name)
		}
	}

	var context bytes.Buffer
	if err := RunReviewLensContext([]string{
		"--repository-context", repositoryContext, "--lens", "review-refuter", "--delivery", "runtime_interception",
	}, &context); err != nil {
		t.Fatal(err)
	}
	findingID := reviewtransaction.FindingIDPrefixForLens(record.State.SelectedLenses[0]) + "001"
	for _, want := range []string{
		`"finding_id":"` + findingID + `"`, `"snapshot_identity":"` + record.State.InitialSnapshot.Identity + `"`,
		`"required":["results"]`, `"proof_refs"`, "+candidate\n", "GENTLE_AI_REVIEW_CONTEXT_END\n",
	} {
		if !strings.Contains(context.String(), want) {
			t.Fatalf("refuter context misses %q:\n%s", want, context.String())
		}
	}

	refuter := filepath.Join(t.TempDir(), "refuter.json")
	writeReviewCLIJSON(t, refuter, facadeRefuterResult{Results: []facadeRefuterOutcome{{
		FindingID: findingID, Outcome: reviewtransaction.OutcomeRefuted, ProofRefs: []string{"immutable patch does not establish the external contract"},
	}}})
	var finalized bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--captured-results", "--refuter", refuter,
	}, &finalized); err != nil {
		t.Fatalf("finalize canonical refuter result: %v", err)
	}
	result := decodeFacadeFinalize(t, finalized.Bytes())
	if result.State != reviewtransaction.StateValidating {
		t.Fatalf("finalize refuted finding = %#v, want validating state", result)
	}
	status = reviewStatusWithTransition(t, repo, started.LineageID)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "verification_evidence_required" {
		t.Fatalf("post-refuter next_transition = %#v, want collect/verification_evidence_required", status.NextTransition)
	}
}

func TestStatusFinalizesDeterministicFindingWithoutRefuter(t *testing.T) {
	repo, started, _, _, _ := capturedArtifacts(t, false)
	status := reviewStatusWithTransition(t, repo, started.LineageID)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "captured_results_ready" || status.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("next_transition = %#v, want direct finalize", status.NextTransition)
	}
}

func TestPendingRefuterClaimDiscoveryReturnsCapturedArtifactFailure(t *testing.T) {
	repo, _, store, record, _ := capturedArtifacts(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	claims, err := discoverPendingRefuterClaims(ctx, repo, store.Dir, record.State, record.Revision)
	if err == nil || claims != nil {
		t.Fatalf("claims = %#v, err = %v; want captured artifact failure", claims, err)
	}
}

func reviewStatusWithTransition(t *testing.T, repo, lineage string) ReviewTargetStatusResult {
	t.Helper()
	var out bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition", "--cwd", repo, "--lineage", lineage,
	}, &out); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, out.Bytes(), &status)
	return status
}

func refuterTransitionArgumentValue(t *testing.T, arguments []ReviewTransitionArgument, name string) string {
	t.Helper()
	for _, argument := range arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	t.Fatalf("transition arguments miss %q: %#v", name, arguments)
	return ""
}
