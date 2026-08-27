package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func assertApprovedCompactAuthorityBurned(t *testing.T, store reviewtransaction.CompactStore, lineage string) {
	t.Helper()
	base := filepath.Dir(filepath.Dir(store.Dir))
	for _, path := range []string{
		filepath.Join(base, "v2", lineage),
		filepath.Join(base, "effect-markers", "v1", lineage),
		filepath.Join(base, "incidents", lineage),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("approved compact authority survived at %s: %v", path, err)
		}
	}
}

func captureCleanCLIReviewerResult(t *testing.T, repo string, started ReviewFacadeStartResult, order int, stdout *bytes.Buffer) {
	t.Helper()
	captureCLIReviewerResultWithFindings(t, repo, started, order, []facadeFinding{}, stdout)
}

func captureCLIReviewerResultWithFindings(t *testing.T, repo string, started ReviewFacadeStartResult, order int, findings []facadeFinding, stdout *bytes.Buffer) {
	t.Helper()
	args := cliReviewerCaptureArgs(t, repo, started, order, findings)
	if err := RunReviewCaptureResult(args, stdout); err != nil {
		t.Fatalf("capture-result for lens %q: %v\n%s", started.SelectedLenses[order], err, stdout.String())
	}
}

func cliReviewerCaptureArgs(t *testing.T, repo string, started ReviewFacadeStartResult, order int, findings []facadeFinding) []string {
	t.Helper()
	lens := started.SelectedLenses[order]
	binding := []string{
		"--cwd", repo,
		"--lineage", started.LineageID,
		"--target", started.TargetIdentity,
		"--lens", lens,
		"--order", strconv.Itoa(order),
	}
	var preflightOutput bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--preflight"), &preflightOutput); err != nil {
		t.Fatalf("capture-result --preflight for lens %q: %v", lens, err)
	}
	var preflight reviewCapturePreflightResult
	if err := json.Unmarshal(preflightOutput.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(preflight.ChangedPathManifest))
	for index, entry := range preflight.ChangedPathManifest {
		paths[index] = entry.Path
	}
	resultPath := filepath.Join(t.TempDir(), "reviewer-"+strconv.Itoa(order)+".json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		SubjectHash: preflight.ArtifactSubject.SubjectHash,
		Inspection: reviewtransaction.ArtifactInspection{
			Status: reviewtransaction.ArtifactInspectionCompleted,
			Paths:  paths,
		},
		Findings: findings,
		Evidence: []string{"inspected the complete frozen candidate scope named by the capture binding"},
	})
	return append(binding, "--input", resultPath)
}

func TestLastReviewerCaptureApprovesAndBurnsWithoutFinalize(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}

	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
		if _, err := store.Load(); err != nil {
			t.Fatalf("authority disappeared before the final lens: %v", err)
		}
	}

	var terminalOutput bytes.Buffer
	captureCleanCLIReviewerResult(t, repo, started, len(started.SelectedLenses)-1, &terminalOutput)

	var terminal struct {
		Schema             string                     `json:"schema"`
		Operation          string                     `json:"operation"`
		LineageID          string                     `json:"lineage_id"`
		State              reviewtransaction.State    `json:"state"`
		Action             string                     `json:"action"`
		StatusContinuation *ReviewTransitionExecution `json:"status_continuation"`
	}
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatalf("decode terminal capture result: %v\n%s", err, terminalOutput.String())
	}
	if terminal.Operation != "review/capture-result" || terminal.LineageID != started.LineageID ||
		terminal.State != reviewtransaction.StateApproved || !strings.Contains(terminal.Action, "burned") || terminal.StatusContinuation != nil {
		t.Fatalf("last capture terminal result = %#v, want approved burned capture completion", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestLastReviewerCaptureReturnsAdvisoriesBeforeBurn(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}

	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	var terminalOutput bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{{
		ID: "R3-W01", Location: "internal/auth/session.go:4", Severity: "WARNING",
		Claim: "the token check could be easier to read", ProofRefs: []string{"the exact changed line was inspected"},
	}}, &terminalOutput)

	var terminal struct {
		State            reviewtransaction.State               `json:"state"`
		AdvisoryFindings *reviewtransaction.AdvisoryFindingSet `json:"advisory_findings"`
	}
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.State != reviewtransaction.StateApproved || terminal.AdvisoryFindings == nil ||
		len(terminal.AdvisoryFindings.Findings) != 1 || terminal.AdvisoryFindings.Findings[0].ID != "R3-W01" {
		t.Fatalf("last capture advisories = %#v, want the admitted warning before burn", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestLastReviewerCaptureOpensBoundedCorrectionForSevereFinding(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}

	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	var correctionOutput bytes.Buffer
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{{
		ID: "R3-001", Location: "internal/auth/session.go:4", Severity: "CRITICAL",
		Claim:         "the candidate introduces an observable authentication failure",
		ProofRefs:     []string{"the changed line deterministically causes the reproduced failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &correctionOutput)

	var correction struct {
		Operation string                  `json:"operation"`
		State     reviewtransaction.State `json:"state"`
		Action    string                  `json:"action"`
	}
	if err := json.Unmarshal(correctionOutput.Bytes(), &correction); err != nil {
		t.Fatal(err)
	}
	if correction.Operation != "review/capture-result" || correction.State != reviewtransaction.StateCorrectionRequired ||
		!strings.Contains(correction.Action, "bounded correction") {
		t.Fatalf("last capture correction result = %#v", correction)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatalf("correction authority was burned: %v", err)
	}
	if record.State.State != reviewtransaction.StateCorrectionRequired || len(record.State.FixFindingIDs) != 1 || record.State.FixFindingIDs[0] != "R3-001" {
		t.Fatalf("persisted correction state = %#v", record.State)
	}
}

func TestConcurrentFinalLensCaptureElectsOneCloserAndLeavesNoAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	args := cliReviewerCaptureArgs(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{})

	const attempts = 8
	outputs := make([]bytes.Buffer, attempts)
	errorsByAttempt := make([]error, attempts)
	var wait sync.WaitGroup
	wait.Add(attempts)
	for index := 0; index < attempts; index++ {
		go func(index int) {
			defer wait.Done()
			errorsByAttempt[index] = RunReviewCaptureResult(args, &outputs[index])
		}(index)
	}
	wait.Wait()

	terminal := 0
	for index := range outputs {
		if errorsByAttempt[index] != nil {
			continue
		}
		var result struct {
			Schema string                  `json:"schema"`
			State  reviewtransaction.State `json:"state"`
		}
		if err := json.Unmarshal(outputs[index].Bytes(), &result); err != nil {
			t.Fatalf("decode concurrent capture %d: %v\n%s", index, err, outputs[index].String())
		}
		if result.Schema == reviewLastEventClosureSchema {
			if result.State != reviewtransaction.StateApproved {
				t.Fatalf("concurrent closer %d state = %q", index, result.State)
			}
			terminal++
		}
	}
	if terminal != 1 {
		t.Fatalf("concurrent final-lens terminal responses = %d, want exactly one; errors=%v", terminal, errorsByAttempt)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func providerCorrectionReadyWithoutVerificationEvidence(t *testing.T) (string, string, reviewtransaction.TargetedValidationRequest) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "provider-targeted-validator-no-evidence")
	legacyStarted := ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity,
		SelectedLenses: started.SelectedLenses,
	}
	args := cliReviewerCaptureArgs(t, repo, legacyStarted, 0, []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "terminal value is incorrect",
		ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}})
	if err := RunReviewCaptureResult(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := reviewtransaction.BuildCorrectionPlanRequest(record.State, record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureCorrectionPlan([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", plan.TargetIdentity,
		"--expected-revision", record.Revision, "--request-hash", plan.RequestHash, "--correction-lines", "2",
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err = reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, record.State, record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return repo, started.LineageID, request
}

func TestTargetedValidatorCaptureRequiresNoVerificationEvidenceAndLeavesNoStrandedSlot(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	originalAdapter := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = originalAdapter })

	var terminalOutput bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo,
		"--lineage", lineage,
		"--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.Revision,
		"--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi),
		"--execute=true",
	}, &terminalOutput); err != nil {
		t.Fatalf("capture targeted validator without repository verification evidence: %v\n%s", err, terminalOutput.String())
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
	slot, err := reviewtransaction.ReadCompactTargetedValidatorResultSlot(store.Dir, request)
	if err != nil || slot.Occupied {
		t.Fatalf("terminal validator capture stranded slot = %#v, %v", slot, err)
	}
}

func TestStatusRoutesCompiledValidatorToTerminalCapture(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &output); err != nil {
		t.Fatalf("compiled validator status: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.NextTransition == nil || status.NextTransition.ReasonCode != "targeted_validation_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation {
		t.Fatalf("compiled validator transition = %#v", status.NextTransition)
	}
}

func TestCompiledTargetedValidatorCaptureClosesOnItsTerminalEvent(t *testing.T) {
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
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapter{raw: providerTargetedValidationPayload(t, request)}, nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	var output bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.Revision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &output); err != nil {
		t.Fatalf("compiled capture-validation: %v\n%s", err, output.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved ||
		!strings.Contains(terminal.Action, "burned") {
		t.Fatalf("compiled validator terminal closure = %#v", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestCorrectionStatusRoutesFrozenCandidateDirectlyToTargetedValidation(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2,
		"--next-transition",
	}, &output); err != nil {
		t.Fatalf("status with a frozen corrected candidate: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "manual_intervention_required" {
		t.Fatalf("unbound correction status transition = %#v", status.NextTransition)
	}
}

func TestTargetedValidationCaptureClosesWithoutVerificationEvidence(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	originalAdapter := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = originalAdapter })
	var terminalOutput bytes.Buffer
	if err := RunReview([]string{
		"capture-validation", "--cwd", repo, "--lineage", lineage,
		"--target", request.CorrectionTargetIdentity, "--expected-revision", record.Revision,
		"--request-hash", request.RequestHash, "--agent", string(model.AgentPi), "--execute=true",
	}, &terminalOutput); err != nil {
		t.Fatalf("execute targeted validation capture: %v\n%s", err, terminalOutput.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, terminalOutput.Bytes(), &terminal)
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("targeted validation capture result = %#v", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestConcurrentAndReplayedTargetedValidatorCaptureHasOneCloser(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	originalAdapter := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = originalAdapter })
	args := []string{
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.Revision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi), "--execute=true",
	}

	const attempts = 8
	outputs := make([]bytes.Buffer, attempts)
	errs := make([]error, attempts)
	var wait sync.WaitGroup
	wait.Add(attempts)
	for index := range outputs {
		go func(index int) {
			defer wait.Done()
			errs[index] = RunReviewCaptureValidation(args, &outputs[index])
		}(index)
	}
	wait.Wait()
	closers := 0
	for index := range outputs {
		if errs[index] != nil {
			continue
		}
		var result reviewLastEventClosureResult
		if err := json.Unmarshal(outputs[index].Bytes(), &result); err != nil {
			t.Fatalf("decode concurrent targeted validator capture %d: %v\n%s", index, err, outputs[index].String())
		}
		if result.State != reviewtransaction.StateApproved || result.Operation != "review/capture-validation" {
			t.Fatalf("concurrent targeted validator closer %d = %#v", index, result)
		}
		closers++
	}
	if closers != 1 {
		t.Fatalf("targeted validator closers = %d, want 1; errors=%v", closers, errs)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
	if err := RunReviewCaptureValidation(args, &bytes.Buffer{}); err == nil {
		t.Fatal("replayed targeted validator capture resurrected burned authority")
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestTargetedValidatorCaptureApprovesAndBurnsWithoutFinalize(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	originalAdapter := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = originalAdapter })

	var terminalOutput bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo,
		"--lineage", lineage,
		"--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.Revision,
		"--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi),
		"--execute=true",
	}, &terminalOutput); err != nil {
		t.Fatalf("capture targeted validator: %v\n%s", err, terminalOutput.String())
	}
	var terminal struct {
		Schema    string                  `json:"schema"`
		Operation string                  `json:"operation"`
		LineageID string                  `json:"lineage_id"`
		State     reviewtransaction.State `json:"state"`
		Action    string                  `json:"action"`
	}
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Schema != reviewLastEventClosureSchema || terminal.Operation != "review/capture-validation" ||
		terminal.LineageID != lineage || terminal.State != reviewtransaction.StateApproved || !strings.Contains(terminal.Action, "burned") {
		t.Fatalf("targeted validator terminal result = %#v", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestReviewStartWithZeroLensesClosesAndBurnsImmediately(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := startLowRiskFacadeReview(t, repo)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("zero-lens START left authority behind: %v", err)
	}
}

func correctionRequiredForPlanCapture(t *testing.T) (string, ReviewFacadeStartResult, reviewtransaction.CompactStore, reviewtransaction.CompactRecord, reviewtransaction.CorrectionPlanRequest) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	started := startHighRiskCLIReview(t, repo)
	for order := 0; order < len(started.SelectedLenses)-1; order++ {
		captureCleanCLIReviewerResult(t, repo, started, order, &bytes.Buffer{})
	}
	captureCLIReviewerResultWithFindings(t, repo, started, len(started.SelectedLenses)-1, []facadeFinding{{
		ID: "R3-001", Location: "internal/auth/session.go:4", Severity: "CRITICAL",
		Claim:         "the candidate introduces an observable authentication failure",
		ProofRefs:     []string{"the changed line deterministically causes the reproduced failure"},
		EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, &bytes.Buffer{})
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildCorrectionPlanRequest(record.State, record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return repo, started, store, record, request
}

func TestCaptureCorrectionPlanBindsExactRequestAndRefusesBudgetOverrun(t *testing.T) {
	repo, started, store, record, request := correctionRequiredForPlanCapture(t)
	var output bytes.Buffer
	if err := RunReview([]string{
		"capture-correction-plan", "--cwd", repo, "--lineage", started.LineageID,
		"--target", request.TargetIdentity, "--expected-revision", record.Revision,
		"--request-hash", request.RequestHash, "--correction-lines", "1",
	}, &output); err != nil {
		t.Fatalf("capture exact correction plan: %v\n%s", err, output.String())
	}
	after, err := store.Load()
	if err != nil || after.State.ProposedCorrectionLines == nil || *after.State.ProposedCorrectionLines != 1 {
		t.Fatalf("exact correction plan did not persist its forecast: %#v, %v", after, err)
	}

	repo, started, store, record, request = correctionRequiredForPlanCapture(t)
	err = RunReview([]string{
		"capture-correction-plan", "--cwd", repo, "--lineage", started.LineageID,
		"--target", request.TargetIdentity, "--expected-revision", record.Revision,
		"--request-hash", request.RequestHash, "--correction-lines", strconv.Itoa(request.CorrectionBudget + 1),
	}, io.Discard)
	if !reviewtransaction.IsCorrectionBudgetExceeded(err) {
		t.Fatalf("over-budget correction plan error = %v, want CorrectionBudgetExceededError", err)
	}
	after, err = store.Load()
	if err != nil || after.Revision != record.Revision || after.State.ProposedCorrectionLines != nil {
		t.Fatalf("over-budget correction plan mutated authority: %#v, %v", after, err)
	}
}

func TestPublicReviewFinalizeIsUnknown(t *testing.T) {
	for _, args := range [][]string{
		{"finalize"},
		{"finalize", "--contract", ReviewIntegrationContractV2, "--lineage", "retired-finalize"},
	} {
		err := RunReview(args, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "unknown review command") {
			t.Fatalf("public review finalize args=%q error = %v, want unknown command", args, err)
		}
	}
}

func TestStatusNeverEmitsFinalizeOrFinalEvidenceRoutes(t *testing.T) {
	repo, started, _, _, _ := correctionRequiredForPlanCapture(t)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("status for correction plan: %v\n%s", err, output.String())
	}
	for _, forbidden := range []string{"review.finalize", "final_evidence", "verification_evidence", "retry-final-verification"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("status emitted retired route %q:\n%s", forbidden, output.String())
		}
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.ReasonCode != "correction_plan_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureCorrectionPlanOperation {
		t.Fatalf("status correction capture = %#v", status.NextTransition)
	}
}

func TestCompiledRefuterClosesOnTheFinalLensCapture(t *testing.T) {
	reviewEnabledHome(t)
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	providerResult := admittedReviewerResultForTest(t, repo, record, record.State.SelectedLenses[0], 0)
	providerResult.Findings = []facadeFinding{{
		ID: "R3-001", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate failure",
		ProofRefs: []string{"tracked.txt:1 candidate-specific proof"}, EvidenceClass: reviewtransaction.EvidenceInferential,
		CausalDisposition: reviewtransaction.CausalBehaviorActivated,
	}}
	payload, err := json.Marshal(providerResult)
	if err != nil {
		t.Fatal(err)
	}
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
			if bytes.Contains(invocation.Prompt(), []byte(`"claims"`)) {
				return []byte(`{"refuter_request_hash":"` + reviewProviderRequestHashForTest(t, invocation.Prompt()) + `","results":[{"finding_id":"R3-001","outcome":"corroborated","proof_refs":["independent reproduction"]}]}`), nil
			}
			return payload, nil
		}), nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })

	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), &output); err != nil {
		t.Fatalf("compiled final lens capture: %v\n%s", err, output.String())
	}
	var terminal reviewLastEventClosureResult
	if err := json.Unmarshal(output.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Operation != "review/capture-result" || terminal.State != reviewtransaction.StateCorrectionRequired || terminal.StatusContinuation == nil {
		t.Fatalf("compiled refuter terminal closure = %#v", terminal)
	}
	arguments, err := reviewTransitionArgumentMap(terminal.StatusContinuation.Arguments)
	if err != nil || arguments["agent"] != string(model.AgentClaudeCode) {
		t.Fatalf("compiled refuter continuation = %#v, arguments=%#v, err=%v", terminal.StatusContinuation, arguments, err)
	}
}

func TestTargetedValidatorCaptureEscalatesRejectedCorrectionWithoutFinalize(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	failedPayload, err := json.Marshal(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria: facadeValidationCheck{Passed: false, Evidence: []string{
			"the exact corrected candidate still fails the original criterion",
		}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{
			"the bounded correction introduced no unrelated regression",
		}},
		FollowUps: []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	originalAdapter := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return failedPayload, nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = originalAdapter })

	var terminalOutput bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo,
		"--lineage", lineage,
		"--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.Revision,
		"--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi),
		"--execute=true",
	}, &terminalOutput); err != nil {
		t.Fatalf("capture rejected targeted validator: %v\n%s", err, terminalOutput.String())
	}
	var terminal struct {
		Operation string                  `json:"operation"`
		State     reviewtransaction.State `json:"state"`
		Action    string                  `json:"action"`
	}
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateEscalated ||
		!strings.Contains(terminal.Action, "rejected") {
		t.Fatalf("rejected validator terminal result = %#v", terminal)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateEscalated || after.State.OriginalCriteria == nil || after.State.OriginalCriteria.Passed {
		t.Fatalf("rejected validator authority = %#v, %v", after, err)
	}
}
