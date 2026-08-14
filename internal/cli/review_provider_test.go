package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewProviderRoleRegistryIsClosedAndSchemaValid(t *testing.T) {
	contracts := reviewProviderRoleContracts()
	if got := []string{contracts[0].ID, contracts[1].ID, contracts[2].ID}; !slices.Equal(got, []string{
		reviewProviderRoleLens, reviewProviderRoleRefuter, reviewProviderRoleTargetedValidator,
	}) {
		t.Fatalf("role registry = %v", got)
	}
	for _, contract := range contracts {
		t.Run(contract.ID, func(t *testing.T) {
			if !json.Valid(contract.ResultSchema) || contract.StorageSlot == "" || contract.RequestSchemaID == "" || contract.ResultSchemaID == "" || contract.ResultLimit <= 0 {
				t.Fatalf("invalid role contract: %#v", contract)
			}
			if !slices.Equal(contract.RequiredCapabilities, []string{reviewProviderTransportCapability}) {
				t.Fatalf("role capabilities = %v", contract.RequiredCapabilities)
			}
		})
	}
	if _, err := reviewProviderRoleContractFor("unknown"); err == nil {
		t.Fatal("unknown provider role was accepted")
	}
}

func TestReviewProviderMaterializationMatchesNativeLensContext(t *testing.T) {
	_, args, _, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	lens := args[slices.Index(args, "--lens")+1]

	request, err := reviewProviderMaterialize(context.Background(), reviewLensContextDependencies(), handle, lens)
	if err != nil {
		t.Fatal(err)
	}
	var native bytes.Buffer
	if err := RunReview([]string{"lens-context", "--repository-context", handle, "--lens", lens}, &native); err != nil {
		t.Fatal(err)
	}
	if got := request.Invocation.Prompt(); !bytes.Equal(got, native.Bytes()) {
		t.Fatalf("provider materialization diverged from native lens context\nprovider:\n%s\nnative:\n%s", got, native.Bytes())
	}
}

func TestReviewProviderLensAdmissionUsesNativeValidator(t *testing.T) {
	repo, _, _, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]
	raw := admittedReviewerPayloadForTest(t, repo, record, lens, 0)
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, frozen, lens, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := reviewProviderAdmitLensRaw(t.Context(), repo, record.State, record.Revision, frozen, subject, raw)
	if err != nil || result.Lens != lens {
		t.Fatalf("provider lens admission = %#v, %v", result, err)
	}
	if _, err := reviewProviderExtractRoleRaw(reviewProviderRoleLens, append(raw, raw...)); err == nil {
		t.Fatal("multiple objects passed provider raw extraction")
	}
}

func TestReviewProviderTargetedValidatorCapturesExactRequestUnderLock(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	previous := reviewProviderAdapterFor
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapter{raw: providerTargetedValidationPayload(t, request)}, nil
	}

	result, _, err := reviewProviderCaptureTargetedValidator(t.Context(), repo, store, record.State, record.Revision, model.AgentClaudeCode)
	if err != nil || !result.OriginalCriteria.Passed || !result.CorrectionRegression.Passed {
		t.Fatalf("capture targeted validator = %#v, %v", result, err)
	}
	slot, err := reviewtransaction.ReadCompactTargetedValidatorResultSlot(store.Dir, request)
	if err != nil || !slot.Occupied {
		t.Fatalf("targeted validator slot = %#v, %v", slot, err)
	}
	if got, err := readCapturedProviderTargetedValidatorResult(t.Context(), repo, store.Dir, record.State, record.Revision); err != nil || got.CorrectionTargetIdentity != request.CorrectionTargetIdentity {
		t.Fatalf("read captured targeted validator = %#v, %v", got, err)
	}
}

func TestReviewProviderTargetedValidatorTransportFailureDoesNotCapture(t *testing.T) {
	repo, lineage, request := providerCorrectionReady(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	previous := reviewProviderAdapterFor
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	reviewProviderAdapterFor = func(reviewerprovider.Contract, model.AgentID) (reviewerprovider.Adapter, error) {
		return providerTestAdapter{err: errors.New("transport unavailable")}, nil
	}

	if _, _, err := reviewProviderCaptureTargetedValidator(t.Context(), repo, store, record.State, record.Revision, model.AgentClaudeCode); err == nil {
		t.Fatal("targeted validator transport failure captured a result")
	}
	slot, err := reviewtransaction.ReadCompactTargetedValidatorResultSlot(store.Dir, request)
	if err != nil || slot.Occupied {
		t.Fatalf("targeted validator slot after failure = %#v, %v", slot, err)
	}
}

func TestReviewProviderCaptureResultInvokesOnlyTheGoMaterializedLensRequest(t *testing.T) {
	repo, args, record, _ := newCandidateInspectionReview(t, "candidate\n", true)
	handle := args[slices.Index(args, "--repository-context")+1]
	previous := reviewProviderAdapterFor
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	called := false
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
			called = true
			if !bytes.Contains(invocation.Prompt(), []byte(record.State.InitialSnapshot.Identity)) {
				t.Fatal("provider lens request omitted the frozen target identity")
			}
			return admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), nil
		}), nil
	}
	var output bytes.Buffer
	err := RunReviewCaptureResult([]string{
		"--repository-context", handle,
		"--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--expected-revision", record.Revision,
		"--agent", string(model.AgentClaudeCode),
	}, &output)
	if err != nil || !called {
		t.Fatalf("provider capture result = %v; called=%t; output=%s", err, called, output.String())
	}
	if err := RunReviewCaptureResult([]string{
		"--repository-context", handle,
		"--lineage", record.State.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--expected-revision", record.Revision,
		"--agent", string(model.AgentClaudeCode), "--input", filepath.Join(t.TempDir(), "forbidden.json"),
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("provider capture accepted caller-authored result input")
	}
}

func TestReviewProviderRefuterCapturesTransactionWideBatch(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	previous := reviewProviderAdapterFor
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
			if !bytes.Contains(invocation.Prompt(), []byte(record.State.LineageID)) {
				t.Fatal("provider refuter request omitted the reviewing lineage")
			}
			return []byte(`{"refuter_request_hash":"` + reviewProviderRequestHashForTest(t, invocation.Prompt()) + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["independent reproduction"]}]}`), nil
		}), nil
	}

	refuterResult, captured, err := reviewProviderCaptureRefuter(t.Context(), repo, store, record.State, record.Revision, model.AgentClaudeCode)
	if err != nil || !captured || len(refuterResult.Results) != 1 || refuterResult.Results[0].FindingID != "R3-001" {
		t.Fatalf("provider refuter capture = %#v; captured:%t error:%v", refuterResult, captured, err)
	}
	if slot, err := reviewtransaction.ReadCompactRefuterResultSlot(store.Dir); err != nil || !slot.Occupied {
		t.Fatalf("provider refuter slot = %#v, %v", slot, err)
	}
}

func TestReviewProviderOpenCodeStatusIssuesBoundRefuterTask(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	result := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	result.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	input := filepath.Join(t.TempDir(), "result.json")
	writeReviewCLIJSON(t, input, result)
	if err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentOpenCode), "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("OpenCode provider role status is invalid: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.ReasonCode != "provider_refuter_required" || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("OpenCode provider role transition = %#v", status.NextTransition)
	}
	task := status.NextTransition.Collect.Inputs[0].ProviderTask
	if task == nil || task.Agent != "review-refuter" || task.Role != string(reviewerprovider.RoleRefuter) || !strings.HasPrefix(task.Prompt, reviewProviderTaskBindingHeader+" ") {
		t.Fatalf("OpenCode provider task = %#v", task)
	}
}

func reviewProviderRequestHashForTest(t *testing.T, prompt []byte) string {
	t.Helper()
	input := bytes.SplitN(prompt, []byte("\n\nInput:\n"), 2)
	if len(input) != 2 {
		t.Fatalf("provider prompt does not contain input: %s", prompt)
	}
	payload := bytes.SplitN(input[1], []byte("\n\nOutput schema:\n"), 2)
	if len(payload) != 2 {
		t.Fatalf("provider prompt does not contain output schema: %s", prompt)
	}
	var request reviewProviderRefuterRequest
	if err := json.Unmarshal(payload[0], &request); err != nil {
		t.Fatal(err)
	}
	if request.RequestHash == "" {
		t.Fatal("provider refuter request hash is empty")
	}
	return request.RequestHash
}

type providerTestAdapter struct {
	raw []byte
	err error
}

func (adapter providerTestAdapter) Review(context.Context, reviewerprovider.Invocation) ([]byte, error) {
	return adapter.raw, adapter.err
}

type providerTestAdapterFunc func(context.Context, reviewerprovider.Invocation) ([]byte, error)

func (adapter providerTestAdapterFunc) Review(ctx context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
	return adapter(ctx, invocation)
}

func providerCorrectionReady(t *testing.T) (string, string, reviewtransaction.TargetedValidationRequest) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "provider-targeted-validator")
	resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "terminal value is incorrect",
			ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
			CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"inspected correction target"},
	})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo, started.LineageID, capturePassedCorrectionEvidenceForTest(t, repo, started.LineageID)
}

func providerTargetedValidationPayload(t *testing.T, request reviewtransaction.TargetedValidationRequest) []byte {
	t.Helper()
	payload, err := json.Marshal(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"correction regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
