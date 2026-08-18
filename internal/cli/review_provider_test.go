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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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
	reviewEnabledHome(t)
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

func TestReviewProviderOpenCodeStatusIssuesBoundTargetedValidatorTask(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReady(t)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentOpenCode), "--next-transition",
	}, &output); err != nil {
		t.Fatalf("OpenCode targeted-validator STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("OpenCode targeted-validator status is invalid: %v", err)
	}
	if status.Schema != ReviewIntegrationStatusSchemaV5 || status.Authority == nil || status.ValidationRequest == nil ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("OpenCode targeted-validator transition = %#v", status)
	}
	input := status.NextTransition.Collect.Inputs[0]
	arguments, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "provider_targeted_validator" || input.CaptureOperation != "external.run_provider_role" ||
		input.Submission != nil || input.ValidationRequest != nil || input.ProviderTask == nil ||
		input.ProviderTask.Agent != "review-validator" || input.ProviderTask.Role != string(reviewerprovider.RoleTargetedValidator) ||
		arguments["lineage"] != status.Authority.LineageID || arguments["expected-revision"] != status.Authority.Revision ||
		arguments["target"] != request.CorrectionTargetIdentity || arguments["target"] != status.ValidationRequest.CorrectionTargetIdentity {
		t.Fatalf("OpenCode targeted-validator task = %#v", input)
	}
	// The provider-task validator slot is one of the shapes the published
	// status-v5 schema admits for targeted_validation_required (cross-lane
	// battery finding: the schema used to force the generic
	// external.run_targeted_validation submission shape onto this input).
	transitionPayload, err := json.Marshal(status.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, transitionPayload)

	cloneInput := func() (ReviewTargetStatusResult, *ReviewTransitionInput) {
		malformed := status
		transition := *status.NextTransition
		collection := *transition.Collect
		collection.Inputs = append([]ReviewTransitionInput(nil), collection.Inputs...)
		transition.Collect = &collection
		malformed.NextTransition = &transition
		return malformed, &malformed.NextTransition.Collect.Inputs[0]
	}
	resetTask := func(t *testing.T, input *ReviewTransitionInput) {
		t.Helper()
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		task, err := newReviewProviderTask(reviewerprovider.RoleTargetedValidator, ReviewTransitionBinding{
			LineageID: arguments["lineage"], Revision: arguments["expected-revision"],
			TargetIdentity: arguments["target"], RepositoryContext: arguments["repository-context"],
		})
		if err != nil {
			t.Fatal(err)
		}
		input.ProviderTask = &task
	}
	setArgument := func(t *testing.T, input *ReviewTransitionInput, name, value string) {
		t.Helper()
		for index := range input.Arguments {
			if input.Arguments[index].Name == name {
				input.Arguments[index].Value = value
				return
			}
		}
		t.Fatalf("missing provider task argument %q", name)
	}

	t.Run("refuses a mismatched provider role", func(t *testing.T) {
		malformed, input := cloneInput()
		task := *input.ProviderTask
		task.Role = string(reviewerprovider.RoleRefuter)
		input.ProviderTask = &task
		if err := malformed.Validate(); err == nil {
			t.Fatal("targeted-validator STATUS accepted a refuter task")
		}
	})
	t.Run("refuses a task bound to another correction target", func(t *testing.T) {
		malformed, input := cloneInput()
		setArgument(t, input, "target", status.TargetIdentity)
		resetTask(t, input)
		if err := malformed.Validate(); err == nil {
			t.Fatal("targeted-validator STATUS accepted a task bound to another target")
		}
	})
	t.Run("refuses a task bound to another authority", func(t *testing.T) {
		malformed, input := cloneInput()
		setArgument(t, input, "lineage", "foreign-lineage")
		resetTask(t, input)
		if err := malformed.Validate(); err == nil {
			t.Fatal("targeted-validator STATUS accepted a task bound to another authority")
		}
	})
}

func TestReviewProviderStatusFinalizesCapturedTargetedValidatorWithoutSecondProvider(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReady(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	readStatus := func() ReviewTargetStatusResult {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview([]string{
			"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
			"--next-transition",
		}, &output); err != nil {
			var direct bytes.Buffer
			directErr := runReviewStatus(t.Context(), []string{
				"--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
				"--next-transition",
			}, &direct)
			t.Fatalf("captured provider targeted-validator STATUS: %v\ndirect=%v\n%s", err, directErr, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if err := status.Validate(); err != nil {
			t.Fatalf("captured provider targeted-validator STATUS validation: %v", err)
		}
		return status
	}
	before := readStatus()
	if before.NextTransition == nil || before.NextTransition.Kind != reviewNextTransitionCollect ||
		before.NextTransition.ReasonCode != "targeted_validation_required" || before.NextTransition.Collect == nil ||
		len(before.NextTransition.Collect.Inputs) != 1 || before.NextTransition.Collect.Inputs[0].ProviderTask != nil {
		t.Fatalf("uncaptured provider validator status = %#v", before.NextTransition)
	}
	if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision, providerTargetedValidationPayload(t, request)); err != nil {
		t.Fatal(err)
	}

	after := readStatus()
	if after.NextTransition == nil || after.NextTransition.Kind != reviewNextTransitionExecute ||
		after.NextTransition.ReasonCode != "captured_provider_targeted_validation_ready" || after.NextTransition.Execute == nil ||
		after.NextTransition.Execute.Operation != "review.finalize" || after.ValidationRequest == nil ||
		after.ValidationRequest.RequestHash != request.RequestHash {
		t.Fatalf("captured provider validator status = %#v", after)
	}
	transitionPayload, err := json.Marshal(after.NextTransition)
	if err != nil {
		t.Fatal(err)
	}
	validateAgainstPublishedNextTransitionSchemaV5(t, transitionPayload)
	transition := after.NextTransition.Execute
	if transition.Binding.LineageID != lineage || transition.Binding.Revision != record.Revision ||
		transition.Binding.TargetIdentity != request.CorrectionTargetIdentity || transition.Binding.RepositoryContext == "" {
		t.Fatalf("captured provider validator binding = %#v", transition.Binding)
	}
	wantTokens := []string{
		"--contract=" + ReviewIntegrationContractV2,
		"--lineage=" + lineage,
		"--expected-revision=" + record.Revision,
		"--target=" + request.CorrectionTargetIdentity,
		"--request-hash=" + request.RequestHash,
		"--repository-context=" + transition.Binding.RepositoryContext,
		"--captured-evidence=true",
	}
	gotTokens := make([]string, len(transition.Arguments))
	for index, argument := range transition.Arguments {
		gotTokens[index] = argument.Token
	}
	if !slices.Equal(gotTokens, wantTokens) || slices.Contains(gotTokens, "--validation={{value}}") {
		t.Fatalf("captured provider validator finalize tokens = %#v, want %#v", gotTokens, wantTokens)
	}

	previous := reviewProviderAdapterFor
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	providerCalls := 0
	reviewProviderAdapterFor = func(reviewerprovider.Contract, model.AgentID) (reviewerprovider.Adapter, error) {
		providerCalls++
		return nil, errors.New("captured provider slot finalization must not launch another provider")
	}
	if err := RunReviewFacadeFinalize(gotTokens, &bytes.Buffer{}); err != nil {
		t.Fatalf("execute captured provider validator finalize: %v", err)
	}
	if providerCalls != 0 {
		t.Fatalf("captured provider finalization launched %d provider(s), want none", providerCalls)
	}
	terminal, err := store.Load()
	if err != nil || terminal.State.State != reviewtransaction.StateApproved || len(terminal.State.CorrectionAttempts) != 1 {
		t.Fatalf("captured provider validator terminal authority = %#v, %v", terminal, err)
	}
}

func TestReviewProviderStatusSurfacesUnreadableCapturedValidatorSlot(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReady(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision, providerTargetedValidationPayload(t, request)); err != nil {
		t.Fatal(err)
	}
	slotPath := filepath.Join(store.Dir, "targeted-validator-results", strings.TrimPrefix(request.CorrectionTargetIdentity, "sha256:"),
		strings.TrimPrefix(request.ExpectedRevision, "sha256:"), "result.json")
	if err := os.WriteFile(slotPath, []byte(`{"targeted_validation_request_hash":"sha256:forged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--next-transition",
	}, &output); err != nil {
		t.Fatalf("captured provider corrupted-slot STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("captured provider corrupted-slot STATUS validation: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "captured_artifacts_unverifiable" ||
		status.NextTransition.Execute != nil || status.NextTransition.Collect != nil {
		t.Fatalf("captured provider corrupted-slot transition = %#v", status.NextTransition)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateCorrectionRequired || len(after.State.CorrectionAttempts) != 0 {
		t.Fatalf("corrupted validator slot changed authority: %#v, %v", after, err)
	}
}

func TestReviewProviderStatusRequiresPassedEvidenceBeforeSlotConsumption(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReady(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reviewProviderCaptureTargetedValidatorRaw(t.Context(), repo, store, record.State, record.Revision, providerTargetedValidationPayload(t, request)); err != nil {
		t.Fatal(err)
	}
	var readyOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--next-transition",
	}, &readyOutput); err != nil {
		t.Fatalf("captured provider slot STATUS: %v\n%s", err, readyOutput.String())
	}
	var ready ReviewTargetStatusResult
	decodeStrictReviewJSON(t, readyOutput.Bytes(), &ready)
	if ready.NextTransition == nil || ready.NextTransition.Execute == nil {
		t.Fatalf("captured provider slot transition = %#v", ready.NextTransition)
	}
	staleTokens := make([]string, len(ready.NextTransition.Execute.Arguments))
	for index, argument := range ready.NextTransition.Execute.Arguments {
		staleTokens[index] = argument.Token
	}
	evidenceDir := filepath.Join(store.Dir, reviewtransaction.CompactFinalEvidenceDir, strings.TrimPrefix(request.CorrectionTargetIdentity, "sha256:"),
		strings.TrimPrefix(record.Revision, "sha256:"))
	if err := os.RemoveAll(evidenceDir); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize(staleTokens, &bytes.Buffer{}); err == nil {
		t.Fatal("stale captured-provider slot-consumption transition finalized without its passed verification evidence")
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--next-transition",
	}, &output); err != nil {
		t.Fatalf("captured provider missing-evidence STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatalf("captured provider missing-evidence STATUS validation: %v", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "correction_repository_verification_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].ProviderTask != nil {
		t.Fatalf("captured provider missing-evidence transition = %#v", status.NextTransition)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateCorrectionRequired || len(after.State.CorrectionAttempts) != 0 {
		t.Fatalf("missing verification evidence changed authority: %#v, %v", after, err)
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
