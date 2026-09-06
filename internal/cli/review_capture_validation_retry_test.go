package cli

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Issue #4061: the compiled in-process targeted validator (--agent=claude-code
// --execute=true) could return a syntactically invalid JSON document (a live
// claude adapter run hit this: 5 objects opened and closed, 5 arrays opened
// but only 3 closed, the scan ending mid-payload) and the only exit was
// relaunching the identical slot and hoping for a different result. The lens
// role already solves exactly this with one corrective re-invocation carrying
// the admission error; these tests prove the targeted validator and refuter
// roles now share that same mechanism, and that a refusal surviving both
// attempts is classified as a provider-side defect rather than an operator
// request error.

// truncatedTargetedValidatorPayload cuts a syntactically valid targeted
// validator result inside an open array, leaving both the outer object and
// the evidence array unterminated.
func truncatedTargetedValidatorPayload(request reviewtransaction.TargetedValidationRequest) []byte {
	return []byte(`{"targeted_validation_request_hash":"` + request.RequestHash +
		`","correction_target_identity":"` + request.CorrectionTargetIdentity +
		`","original_criteria":{"passed":true,"evidence":["cut mid array"`)
}

func TestValidationCaptureRetriesOnceWithAdmissionFeedbackAndPreservesOneRejectedPayload(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	truncated := truncatedTargetedValidatorPayload(request)
	valid := providerTargetedValidationPayload(t, request)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return truncated, nil },
		func() ([]byte, error) { return valid, nil },
	)

	var output bytes.Buffer
	if err := RunReview([]string{
		"capture-validation",
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &output); err != nil {
		t.Fatalf("capture-validation with one corrective attempt: %v\n%s", err, output.String())
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider validator invocations = %d, want exactly 2", len(*prompts))
	}
	first, second := (*prompts)[0], (*prompts)[1]
	if !bytes.HasPrefix(second, first) {
		t.Fatal("corrective prompt does not start with the original materialized prompt")
	}
	for _, want := range []string{reviewProviderCorrectiveFeedbackHeader, "no complete JSON object", "scan ended at byte"} {
		if !bytes.Contains(second, []byte(want)) {
			t.Fatalf("corrective prompt lacks %q naming the census error:\n%s", want, second[len(first):])
		}
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, lineage))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected results = %d, want 1", len(preserved))
	}
	for _, envelope := range preserved {
		assertRejectedEnvelope(t, envelope, lineage, string(reviewerprovider.RoleTargetedValidator), 1, truncated)
	}

	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("terminal closure = %#v, want an approved review/capture-validation closure", terminal)
	}
}

func TestValidationCaptureRefusedAfterTwoMalformedResultsIsClassifiedAsProviderSide(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	truncated := truncatedTargetedValidatorPayload(request)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return truncated, nil },
		func() ([]byte, error) { return truncated, nil },
	)

	var output bytes.Buffer
	err = RunReview([]string{
		"capture-validation",
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", before.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &output)
	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code != reviewPreflightProviderCaptureRefusedReason.Code || failure.NextAction != "review.status" ||
		failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("refusal envelope = %#v, want code %q, next_action review.status, mutation_outcome not_started",
			failure, reviewPreflightProviderCaptureRefusedReason.Code)
	}
	if failure.Code == reviewIntegrationInvalidRequestCode {
		t.Fatal("a provider-side malformed result was classified as an operator-correctable request")
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider validator invocations = %d, want exactly 2", len(*prompts))
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, lineage))
	if len(preserved) != 2 {
		t.Fatalf("preserved rejected results = %d, want 2", len(preserved))
	}
	for _, envelope := range preserved {
		if envelope.Attempt != 1 && envelope.Attempt != 2 {
			t.Fatalf("unexpected preserved attempt %#v", envelope)
		}
	}

	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("authority mutated on a fully refused capture: before=%s after=%s", before.Revision, after.Revision)
	}

	var status ReviewTargetStatusResult
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("status after refused capture: %v\n%s", err, statusOutput.String())
	}
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.ReasonCode != "targeted_validation_required" {
		t.Fatalf("status after refused capture = %#v, want the same validation slot reoffered", status.NextTransition)
	}
}

func TestValidationCaptureTransportErrorIsNotRetried(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return nil, errors.New("exit status 137") },
	)

	var output bytes.Buffer
	err = RunReview([]string{
		"capture-validation",
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &output)
	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code == reviewPreflightProviderCaptureRefusedReason.Code {
		t.Fatalf("a transport failure was classified as a provider result refusal: %#v", failure)
	}
	if len(*prompts) != 1 {
		t.Fatalf("provider validator invocations = %d, want exactly 1 (a transport failure is not retried)", len(*prompts))
	}
	if !strings.Contains(err.Error(), "exit status 137") {
		t.Fatalf("refusal does not name the transport failure: %v", err)
	}
}

// The refuter batch capture is a mechanical reuse of the same corrective-retry
// core: no role-specific sentinel exists for it, so a malformed refuter
// document gets exactly the same single corrective re-invocation.
func TestRefuterCaptureRetriesOnceWithAdmissionFeedback(t *testing.T) {
	reviewEnabledHome(t)
	repo, store, record, handle := piRefuterReview(t)
	binding := piRefuterBinding(repo, record, handle)
	refuterRequest, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}

	valid := []byte(`{"refuter_request_hash":"` + refuterRequest.RequestHash + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["independent reproduction"]}]}`)
	truncated := []byte(`{"refuter_request_hash":"` + refuterRequest.RequestHash + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["cut mid array"`)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return truncated, nil },
		func() ([]byte, error) { return valid, nil },
	)

	var output bytes.Buffer
	if err := RunReview(append(append([]string{"capture-refuter"}, binding...), "--agent", string(model.AgentClaudeCode), "--execute=true"), &output); err != nil {
		t.Fatalf("capture-refuter with one corrective attempt: %v\n%s", err, output.String())
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider refuter invocations = %d, want exactly 2", len(*prompts))
	}
	first, second := (*prompts)[0], (*prompts)[1]
	if !bytes.HasPrefix(second, first) || !bytes.Contains(second, []byte(reviewProviderCorrectiveFeedbackHeader)) {
		t.Fatal("corrective refuter prompt does not carry the admission feedback over the original prompt")
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected results = %d, want 1", len(preserved))
	}
	for _, envelope := range preserved {
		assertRejectedEnvelope(t, envelope, record.State.LineageID, string(reviewerprovider.RoleRefuter), 1, truncated)
	}
}

// R3-refuter-capture-guard-identity: a drifted request TargetIdentity must be
// refused before any authority write, ahead of payload canonicalization.
func TestRefuterCaptureRefusesRequestTargetIdentityDriftFromState(t *testing.T) {
	reviewEnabledHome(t)
	repo, store, record, _ := piRefuterReview(t)
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	request.TargetIdentity = "sha256:" + strings.Repeat("a", 64)
	if _, err := reviewProviderCaptureAdmittedRefuterResult(t.Context(), repo, store, record.State, record.State.CapturePhaseRevision, request, facadeRefuterResult{}, nil); err == nil {
		t.Fatal("expected refusal on a drifted request target identity")
	}
	if after, loadErr := store.Load(); loadErr != nil || !reflect.DeepEqual(after.State, record.State) {
		t.Fatalf("refused capture mutated reviewing authority: after=%#v err=%v", after.State, loadErr)
	}
}
