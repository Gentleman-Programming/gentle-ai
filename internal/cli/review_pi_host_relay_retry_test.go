package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const piHostPrivateAbsoluteCitation = "/absolute/private.txt:1"

func TestPiHostLensRetriesRejectedAbsoluteCitation(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, started, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	valid := admittedReviewerResultForTest(t, repo, record, lens, 0)
	invalid := valid
	invalid.Evidence = []string{"reviewed " + piHostPrivateAbsoluteCitation}
	validBytes := marshalFacadeTestJSON(t, valid)
	invalidBytes := marshalFacadeTestJSON(t, invalid)

	var prompts [][]byte
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
		prompts = append(prompts, invocation.Prompt())
		if len(prompts) == 1 {
			return invalidBytes, nil
		}
		return validBytes, nil
	}))
	status := hostReviewStatus(t, repo, started.LineageID, model.AgentPi)
	input := soleHostCollectInput(t, status, reviewCaptureResultCaptureOperation)
	args := append([]string{"capture-result"}, reviewTransitionInputTokens(t, repo, input)...)
	if err := RunReview(args, io.Discard); err != nil {
		t.Fatalf("host lens retry: %v", err)
	}
	if len(prompts) != 2 || !bytes.Contains(prompts[1], []byte(reviewProviderCorrectiveFeedbackHeader)) ||
		!bytes.Contains(prompts[1], []byte("evidence_path_out_of_scope")) {
		t.Fatalf("host lens prompts = %d, corrective admission feedback missing", len(prompts))
	}
	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected lens payloads = %d, want 1", len(preserved))
	}
	for _, envelope := range preserved {
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalidBytes)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !recordHasAdmittedRole(after.State, reviewtransaction.CompactRoleLens) {
		t.Fatal("valid corrective lens result was not admitted")
	}
}

func TestPiHostLensDoesNotNormalizeWrongSubjectHash(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, started, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	valid := admittedReviewerResultForTest(t, repo, record, lens, 0)
	wrongSubject := valid
	wrongSubject.SubjectHash = record.State.InitialSnapshot.Identity
	wrongBytes := marshalFacadeTestJSON(t, wrongSubject)
	validBytes := marshalFacadeTestJSON(t, valid)

	calls := 0
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
		calls++
		if calls == 1 {
			return wrongBytes, nil
		}
		return validBytes, nil
	}))
	status := hostReviewStatus(t, repo, started.LineageID, model.AgentPi)
	input := soleHostCollectInput(t, status, reviewCaptureResultCaptureOperation)
	args := append([]string{"capture-result"}, reviewTransitionInputTokens(t, repo, input)...)
	if err := RunReview(args, io.Discard); err != nil {
		t.Fatalf("host lens strict raw retry: %v", err)
	}
	if calls != maxReviewerResultAdmissionAttempts {
		t.Fatalf("wrong subject hash was normalized instead of rejected: calls=%d", calls)
	}
	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved wrong-subject payloads = %d, want 1", len(preserved))
	}
	for _, envelope := range preserved {
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, wrongBytes)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !recordHasAdmittedRole(after.State, reviewtransaction.CompactRoleLens) {
		t.Fatal("valid corrective result after wrong subject hash was not admitted")
	}
}

func TestPiHostLensRefusesTwoAbsoluteCitationsWithoutMutationOrLeak(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, started, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	invalid := admittedReviewerResultForTest(t, repo, record, lens, 0)
	invalid.Evidence = []string{"reviewed " + piHostPrivateAbsoluteCitation}
	invalidBytes := marshalFacadeTestJSON(t, invalid)

	calls := 0
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
		calls++
		return invalidBytes, nil
	}))
	status := hostReviewStatus(t, repo, started.LineageID, model.AgentPi)
	input := soleHostCollectInput(t, status, reviewCaptureResultCaptureOperation)
	args := append([]string{"capture-result"}, reviewTransitionInputTokens(t, repo, input)...)
	var output bytes.Buffer
	runErr := RunReview(args, &output)
	failure := decodeCaptureRefusalEnvelope(t, runErr, output.Bytes())
	if failure.Code != reviewPreflightProviderCaptureRefusedReason.Code || failure.NextAction != "review.status" ||
		failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("host lens refusal = %#v", failure)
	}
	if calls != maxReviewerResultAdmissionAttempts {
		t.Fatalf("host lens attempts = %d, want %d", calls, maxReviewerResultAdmissionAttempts)
	}
	if strings.Contains(runErr.Error(), piHostPrivateAbsoluteCitation) || strings.Contains(output.String(), piHostPrivateAbsoluteCitation) ||
		strings.Contains(output.String(), repo) {
		t.Fatalf("private path escaped the public refusal: %v\n%s", runErr, output.String())
	}
	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != maxReviewerResultAdmissionAttempts {
		t.Fatalf("preserved rejected lens payloads = %d, want %d", len(preserved), maxReviewerResultAdmissionAttempts)
	}
	for _, envelope := range preserved {
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, envelope.Attempt, invalidBytes)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != record.Revision || recordHasAdmittedRole(after.State, reviewtransaction.CompactRoleLens) {
		t.Fatalf("two rejected lens results mutated authority: before=%s after=%s", record.Revision, after.Revision)
	}
	reoffered := hostReviewStatus(t, repo, started.LineageID, model.AgentPi)
	_ = soleHostCollectInput(t, reoffered, reviewCaptureResultCaptureOperation)
}

func TestPiHostLensTransportErrorIsNotRetried(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, started, store, record := newArtifactReview(t, false)
	calls := 0
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
		calls++
		return nil, errors.New("transport failed")
	}))
	status := hostReviewStatus(t, repo, started.LineageID, model.AgentPi)
	input := soleHostCollectInput(t, status, reviewCaptureResultCaptureOperation)
	args := append([]string{"capture-result"}, reviewTransitionInputTokens(t, repo, input)...)
	runErr := RunReview(args, io.Discard)
	if runErr == nil || !strings.Contains(runErr.Error(), "transport failed") || calls != 1 {
		t.Fatalf("host lens transport failure: err=%v calls=%d", runErr, calls)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != record.Revision {
		t.Fatalf("transport failure mutated lens authority: before=%s after=%s", record.Revision, after.Revision)
	}
	if _, statErr := os.Stat(rejectedResultsDir(t, repo, record.State.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("transport failure preserved a reviewer result: %v", statErr)
	}
}

func hostReviewStatus(t *testing.T, repo, lineage string, agent model.AgentID) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--agent", string(agent), "--next-transition"}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}

func soleHostCollectInput(t *testing.T, status ReviewTargetStatusResult, operation string) ReviewTransitionInput {
	t.Helper()
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("missing sole host collect input: %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != operation {
		t.Fatalf("host collect operation = %q, want %q", input.CaptureOperation, operation)
	}
	return input
}

func TestPiHostRefuterRetriesTruncatedResult(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	valid := piRefuterRawResult(t, repo, store, record)
	truncated := []byte(`{"refuter_request_hash":"` + request.RequestHash + `","results":[{"finding_id":"R3-001"`)
	var prompts [][]byte
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
		prompts = append(prompts, invocation.Prompt())
		if len(prompts) == 1 {
			return truncated, nil
		}
		return valid, nil
	}))
	args := append([]string{"capture-refuter", "--cwd=" + repo}, piRefuterBinding(repo, record, handle)...)
	args = append(args, "--agent", string(model.AgentPi), "--execute=true")
	if err := RunReview(args, io.Discard); err != nil {
		t.Fatalf("host refuter retry: %v", err)
	}
	if len(prompts) != 2 || !bytes.Contains(prompts[1], []byte(reviewProviderCorrectiveFeedbackHeader)) ||
		!bytes.Contains(prompts[1], []byte("no complete JSON object")) {
		t.Fatalf("host refuter prompts = %d, structural census feedback missing", len(prompts))
	}
	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected refuter payloads = %d, want 1", len(preserved))
	}
	for _, envelope := range preserved {
		assertRejectedEnvelope(t, envelope, record.State.LineageID, reviewProviderRoleRefuter, 1, truncated)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !recordHasAdmittedRole(after.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatal("valid corrective refuter result was not admitted")
	}
}

func TestPiHostRefuterRefusesTwoTruncatedResultsWithoutMutation(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	request, err := reviewProviderNewRefuterRequest(t.Context(), repo, store.Dir, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	truncated := []byte(`{"refuter_request_hash":"` + request.RequestHash + `","results":[{"finding_id":"R3-001"`)
	calls := 0
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
		calls++
		return truncated, nil
	}))
	args := append([]string{"capture-refuter", "--cwd=" + repo}, piRefuterBinding(repo, record, handle)...)
	args = append(args, "--agent", string(model.AgentPi), "--execute=true")
	var output bytes.Buffer
	runErr := RunReview(args, &output)
	failure := decodeCaptureRefusalEnvelope(t, runErr, output.Bytes())
	if failure.Code != reviewPreflightProviderCaptureRefusedReason.Code || failure.NextAction != "review.status" ||
		failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("host refuter refusal = %#v", failure)
	}
	if calls != maxReviewerResultAdmissionAttempts {
		t.Fatalf("host refuter attempts = %d, want %d", calls, maxReviewerResultAdmissionAttempts)
	}
	after, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if after.Revision != record.Revision || recordHasAdmittedRole(after.State, reviewtransaction.CompactRoleRefuter) {
		t.Fatal("two rejected refuter payloads mutated authority")
	}
	if len(readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))) != maxReviewerResultAdmissionAttempts {
		t.Fatal("both rejected refuter payloads were not preserved")
	}
	reoffered := hostReviewStatus(t, repo, record.State.LineageID, model.AgentPi)
	_ = soleHostCollectInput(t, reoffered, reviewCaptureRefuterCaptureOperation)
}

func TestPiHostRefuterTransportErrorIsNotRetried(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, store, record, handle := piRefuterReview(t)
	calls := 0
	overrideProviderRoleHostAdapter(t, providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
		calls++
		return nil, errors.New("transport failed")
	}))
	args := append([]string{"capture-refuter", "--cwd=" + repo}, piRefuterBinding(repo, record, handle)...)
	args = append(args, "--agent", string(model.AgentPi), "--execute=true")
	runErr := RunReview(args, io.Discard)
	if runErr == nil || !strings.Contains(runErr.Error(), "transport failed") || calls != 1 {
		t.Fatalf("host refuter transport failure: err=%v calls=%d", runErr, calls)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != record.Revision {
		t.Fatalf("transport failure mutated refuter authority: before=%s after=%s", record.Revision, after.Revision)
	}
	if _, statErr := os.Stat(rejectedResultsDir(t, repo, record.State.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("transport failure preserved a refuter result: %v", statErr)
	}
}

func marshalFacadeTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
