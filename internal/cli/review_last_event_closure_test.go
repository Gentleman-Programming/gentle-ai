package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if record, err := store.Load(); err == nil {
		if acknowledgement, pending := reviewtransaction.PendingApprovedCompactAcknowledgement(record); pending {
			repo := filepath.Dir(filepath.Dir(filepath.Dir(base)))
			if err := RunReview([]string{
				"acknowledge-approved", "--cwd", repo, "--lineage", acknowledgement.LineageID,
				"--target", acknowledgement.TargetIdentity, "--expected-revision", acknowledgement.ExpectedRevision, "--token", acknowledgement.Token,
			}, io.Discard); err != nil {
				t.Fatalf("acknowledge pending approved authority: %v", err)
			}
		}
	}
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

func TestLastReviewerCaptureIssuesReplayableAcknowledgementThenBurns(t *testing.T) {
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

	var terminal reviewLastEventClosureResult
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatalf("decode terminal capture result: %v\n%s", err, terminalOutput.String())
	}
	if terminal.Operation != "review/capture-result" || terminal.LineageID != started.LineageID ||
		terminal.State != reviewtransaction.StateApproved || !strings.Contains(terminal.Action, "acknowledgement") || terminal.StatusContinuation != nil {
		t.Fatalf("last capture terminal result = %#v, want approved pending acknowledgement", terminal)
	}
	assertApprovedAcknowledgementTransition(t, terminal.Acknowledgement, repo, started.LineageID, started.TargetIdentity, terminal.StoreRevision)
	if _, err := store.Load(); err != nil {
		t.Fatalf("terminal capture burned pending authority: %v", err)
	}

	var restartOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &restartOutput); err != nil {
		t.Fatalf("restart status: %v\n%s", err, restartOutput.String())
	}
	var restarted ReviewTargetStatusResult
	decodeStrictReviewJSON(t, restartOutput.Bytes(), &restarted)
	if restarted.NextTransition == nil || restarted.NextTransition.Kind != reviewNextTransitionExecute ||
		restarted.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("restart status transition = %#v, want pending acknowledgement", restarted.NextTransition)
	}
	assertApprovedAcknowledgementTransition(t, restarted.NextTransition.Execute, repo, started.LineageID, started.TargetIdentity, terminal.StoreRevision)
	if restarted.NextTransition.Execute.Command != terminal.Acknowledgement.Command {
		t.Fatalf("restart acknowledgement command = %q, want %q", restarted.NextTransition.Execute.Command, terminal.Acknowledgement.Command)
	}

	acknowledgement := terminal.Acknowledgement
	wrongToken := strings.Repeat("0", 64)
	if wrongToken == acknowledgement.Arguments[4].Value {
		wrongToken = strings.Repeat("1", 64)
	}
	if err := RunReview([]string{
		"acknowledge-approved", "--cwd", repo, "--lineage", started.LineageID,
		"--target", started.TargetIdentity, "--expected-revision", terminal.StoreRevision, "--token", wrongToken,
	}, io.Discard); err == nil {
		t.Fatal("wrong acknowledgement token burned authority")
	}
	if _, err := store.Load(); err != nil {
		t.Fatalf("wrong acknowledgement token mutated authority: %v", err)
	}

	var acknowledged bytes.Buffer
	if err := RunReview([]string{
		"acknowledge-approved", "--cwd", repo, "--lineage", started.LineageID,
		"--target", started.TargetIdentity, "--expected-revision", terminal.StoreRevision, "--token", acknowledgement.Arguments[4].Value,
	}, &acknowledged); err != nil {
		t.Fatalf("acknowledge approved authority: %v", err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	assertAcknowledgedEnvelope(t, acknowledged.Bytes(), started.LineageID, started.TargetIdentity, terminal.StoreRevision)
	if err := RunReview([]string{
		"acknowledge-approved", "--cwd", repo, "--lineage", started.LineageID,
		"--target", started.TargetIdentity, "--expected-revision", terminal.StoreRevision, "--token", acknowledgement.Arguments[4].Value,
	}, io.Discard); err == nil {
		t.Fatal("replayed acknowledgement recreated or burned authority")
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func assertApprovedAcknowledgementTransition(t *testing.T, transition *ReviewTransitionExecution, repo, lineage, target, revision string) {
	t.Helper()
	if transition == nil || transition.Operation != "review.acknowledge-approved" ||
		transition.Binding.LineageID != lineage || transition.Binding.TargetIdentity != target || transition.Binding.Revision != revision ||
		len(transition.Arguments) != 5 {
		t.Fatalf("acknowledgement transition = %#v, want exact v2 acknowledgement binding", transition)
	}
	want := []ReviewTransitionArgument{
		{Name: "cwd", Value: repo},
		{Name: "lineage", Value: lineage},
		{Name: "target", Value: target},
		{Name: "expected-revision", Value: revision},
	}
	for index, argument := range want {
		if transition.Arguments[index].Name != argument.Name || transition.Arguments[index].Value != argument.Value || transition.Arguments[index].Token != reviewTransitionArgumentToken(argument) {
			t.Fatalf("acknowledgement argument %d = %#v, want %#v", index, transition.Arguments[index], argument)
		}
	}
	token := transition.Arguments[4]
	if token.Name != "token" || len(token.Value) != 64 || token.Token != reviewTransitionArgumentToken(token) {
		t.Fatalf("acknowledgement token = %#v, want canonical opaque 256-bit argv token", token)
	}
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

func providerCorrectionReadyWithoutVerificationEvidence(t *testing.T, startArgs ...string) (string, string, reviewtransaction.TargetedValidationRequest) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStartWith(t, repo, "provider-targeted-validator-no-evidence", startArgs...)
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
	plan, err := reviewtransaction.BuildCorrectionPlanRequest(record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureCorrectionPlan([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", plan.TargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", plan.RequestHash, "--correction-lines", "2",
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
	request, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, record.State, record.State.CapturePhaseRevision)
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
		"--expected-revision", record.State.CapturePhaseRevision,
		"--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi),
		"--execute=true",
	}, &terminalOutput); err != nil {
		t.Fatalf("capture targeted validator without repository verification evidence: %v\n%s", err, terminalOutput.String())
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
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
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &output); err != nil {
		t.Fatalf("compiled capture-validation: %v\n%s", err, output.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, output.Bytes(), &terminal)
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved ||
		terminal.Action != reviewApprovedLastEventAcknowledgementAction || terminal.Acknowledgement == nil {
		t.Fatalf("compiled validator terminal closure = %#v", terminal)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
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
		"--target", request.CorrectionTargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
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
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
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
	var closer reviewLastEventClosureResult
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
		closer = result
		closers++
	}
	if closers != 1 {
		t.Fatalf("targeted validator closers = %d, want 1; errors=%v", closers, errs)
	}
	pendingRecord, err := store.Load()
	if err != nil {
		t.Fatalf("load approved targeted validator authority: %v", err)
	}
	pending, present := reviewtransaction.PendingApprovedCompactAcknowledgement(pendingRecord)
	if !present || pending.ExpectedRevision != closer.StoreRevision {
		t.Fatalf("pending targeted validator acknowledgement = %#v, closer = %#v", pending, closer)
	}
	assertApprovedAcknowledgementTransition(t, closer.Acknowledgement, repo, lineage, pending.TargetIdentity, pending.ExpectedRevision)

	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &statusOutput); err != nil {
		t.Fatalf("status after concurrent targeted validator capture: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Authority == nil || status.Authority.Revision != closer.StoreRevision ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("status after concurrent targeted validator capture = authority=%#v transition=%#v, want the exact pending acknowledgement", status.Authority, status.NextTransition)
	}
	assertApprovedAcknowledgementTransition(t, status.NextTransition.Execute, repo, lineage, pending.TargetIdentity, pending.ExpectedRevision)
	if status.NextTransition.Execute.Command != closer.Acknowledgement.Command {
		t.Fatalf("status acknowledgement command = %q, want %q", status.NextTransition.Execute.Command, closer.Acknowledgement.Command)
	}

	assertApprovedCompactAuthorityBurned(t, store, lineage)
	if err := RunReviewCaptureValidation(args, &bytes.Buffer{}); err == nil {
		t.Fatal("replayed targeted validator capture resurrected burned authority")
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestTargetedValidatorCaptureIssuesAcknowledgementWithoutFinalize(t *testing.T) {
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
		"--expected-revision", record.State.CapturePhaseRevision,
		"--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi),
		"--execute=true",
	}, &terminalOutput); err != nil {
		t.Fatalf("capture targeted validator: %v\n%s", err, terminalOutput.String())
	}
	var terminal reviewLastEventClosureResult
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Schema != reviewLastEventClosureSchema || terminal.Operation != "review/capture-validation" ||
		terminal.LineageID != lineage || terminal.State != reviewtransaction.StateApproved || terminal.Action != reviewApprovedLastEventAcknowledgementAction {
		t.Fatalf("targeted validator terminal result = %#v", terminal)
	}
	pendingRecord, err := store.Load()
	if err != nil {
		t.Fatalf("load pending targeted validator acknowledgement: %v", err)
	}
	pending, present := reviewtransaction.PendingApprovedCompactAcknowledgement(pendingRecord)
	if !present || pending.ExpectedRevision != terminal.StoreRevision {
		t.Fatalf("pending targeted validator acknowledgement = %#v, terminal = %#v", pending, terminal)
	}
	assertApprovedAcknowledgementTransition(t, terminal.Acknowledgement, repo, lineage, pending.TargetIdentity, pending.ExpectedRevision)
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}

func TestReviewStartWithZeroLensesIssuesAcknowledgement(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lineage := startLowRiskFacadeReview(t, repo)

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil || record.State.State != reviewtransaction.StateApproved {
		t.Fatalf("zero-lens START did not retain its approved authority: %#v, %v", record, err)
	}
	if _, pending := reviewtransaction.PendingApprovedCompactAcknowledgement(record); !pending {
		t.Fatalf("zero-lens START did not issue a pending acknowledgement: %#v", record)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
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
	request, err := reviewtransaction.BuildCorrectionPlanRequest(record.State, record.State.CapturePhaseRevision)
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
		"--target", request.TargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
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
		"--target", request.TargetIdentity, "--expected-revision", record.State.CapturePhaseRevision,
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
		"--expected-revision", record.State.CapturePhaseRevision,
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
		Evidence  json.RawMessage         `json:"targeted_validator_evidence"`
	}
	if err := json.Unmarshal(terminalOutput.Bytes(), &terminal); err != nil {
		t.Fatal(err)
	}
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateEscalated ||
		!strings.Contains(terminal.Action, "rejected") || len(terminal.Evidence) == 0 {
		t.Fatalf("rejected validator terminal result = %#v", terminal)
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateEscalated || after.State.OriginalCriteria == nil || after.State.OriginalCriteria.Passed {
		t.Fatalf("rejected validator authority = %#v, %v", after, err)
	}
	beforeReplay := after.Revision

	var replayOutput bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo,
		"--lineage", lineage,
		"--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision,
		"--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi),
		"--execute=true",
	}, &replayOutput); err != nil {
		t.Fatalf("replay rejected targeted validator: %v\\n%s", err, replayOutput.String())
	}
	var replay reviewLastEventClosureResult
	decodeStrictReviewJSON(t, replayOutput.Bytes(), &replay)
	replayEvidence, err := json.Marshal(replay.TargetedValidatorEvidence)
	if err != nil {
		t.Fatal(err)
	}
	var canonicalEvidence bytes.Buffer
	if err := json.Compact(&canonicalEvidence, terminal.Evidence); err != nil {
		t.Fatal(err)
	}
	if replay.Schema != reviewLastEventClosureSchema || replay.Operation != terminal.Operation || replay.State != terminal.State || replay.Action != terminal.Action || !bytes.Equal(replayEvidence, canonicalEvidence.Bytes()) {
		t.Fatalf("replayed rejected validator closure = %#v, want %#v", replay, terminal)
	}
	afterReplay, err := store.Load()
	if err != nil || afterReplay.Revision != beforeReplay {
		t.Fatalf("replayed rejected validator capture mutated authority = %#v, %v", afterReplay, err)
	}
}

// assertAcknowledgedEnvelope pins the one typed answer the burn prints (#3946):
// an orchestrator reports the most consequential step of the lifecycle from
// the command's own output, not from a later STATUS offering a fresh START.
func assertAcknowledgedEnvelope(t *testing.T, output []byte, lineage, target, revision string) {
	t.Helper()
	var envelope reviewAcknowledgedResult
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("acknowledgement output is not one JSON envelope: %v\n%s", err, output)
	}
	want := reviewAcknowledgedResult{
		Schema: reviewAcknowledgedSchema, Operation: "review/acknowledge-approved", Action: "acknowledged",
		LineageID: lineage, TargetIdentity: target, ConsumedRevision: revision, Authority: "burned",
	}
	if envelope != want {
		t.Fatalf("acknowledgement envelope = %#v, want %#v", envelope, want)
	}
}

// writeZeroLensDocumentationCandidate stages the docs-only candidate that
// START closes with zero lenses (non_executable_only) and a pending
// acknowledgement.
func writeZeroLensDocumentationCandidate(t *testing.T, repo string) {
	t.Helper()
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", strings.Join(lines, "\n")+"\n", 0o644)
}

func runSelectorlessNegotiatedStatus(t *testing.T, repo string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--agent", string(model.AgentClaudeCode), "--next-transition",
	}, &output); err != nil {
		t.Fatalf("selectorless negotiated STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	return status
}

// startZeroLensPendingAcknowledgement runs the START the canonical selectorless
// STATUS offers for a zero-lens candidate, which closes approved with one
// pending acknowledgement under the derived lineage.
func startZeroLensPendingAcknowledgement(t *testing.T) (string, ReviewIntegrationStartResult, reviewtransaction.CompactStore, reviewtransaction.ApprovedCompactAcknowledgement) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeZeroLensDocumentationCandidate(t, repo)
	offered := runSelectorlessNegotiatedStatus(t, repo)
	started := runNegotiatedReviewStart(t, repo, startTransitionArgumentValue(t, offered, "lineage"))
	if started.State != reviewtransaction.StateApproved || len(started.SelectedLenses) != 0 {
		t.Fatalf("zero-lens START = %#v, want approved with no lenses", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	pending, present := reviewtransaction.PendingApprovedCompactAcknowledgement(record)
	if !present {
		t.Fatalf("zero-lens START issued no pending acknowledgement: %#v", record.State)
	}
	return repo, started, store, pending
}

// TestNegotiatedStatusReplaysPendingAcknowledgementWithoutLineageSelector is
// the RED-first proof for #3900: a zero-lens START closes approved with a
// pending acknowledgement, and the canonical selectorless STATUS the
// orchestrator contract prescribes must replay that exact acknowledgement
// instead of calling the candidate unrelated and reoffering a START the store
// then refuses with atomic_start_conflict.
func TestNegotiatedStatusReplaysPendingAcknowledgementWithoutLineageSelector(t *testing.T) {
	repo, started, store, pending := startZeroLensPendingAcknowledgement(t)

	selectorless := runSelectorlessNegotiatedStatus(t, repo)
	if selectorless.Applicability != reviewtransaction.TargetApplicabilityCurrent || selectorless.NextTransition == nil ||
		selectorless.NextTransition.Kind != reviewNextTransitionExecute || selectorless.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("selectorless STATUS = applicability=%q transition=%#v, want the pending acknowledgement", selectorless.Applicability, selectorless.NextTransition)
	}
	assertApprovedAcknowledgementTransition(t, selectorless.NextTransition.Execute, repo, started.LineageID, pending.TargetIdentity, pending.ExpectedRevision)

	var lineageOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID, "--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &lineageOutput); err != nil {
		t.Fatalf("lineage STATUS: %v\n%s", err, lineageOutput.String())
	}
	var byLineage ReviewTargetStatusResult
	decodeStrictReviewJSON(t, lineageOutput.Bytes(), &byLineage)
	if byLineage.NextTransition == nil || byLineage.NextTransition.Execute == nil ||
		byLineage.NextTransition.Execute.Command != selectorless.NextTransition.Execute.Command {
		t.Fatalf("selectorless acknowledgement %q differs from the lineage one %#v", selectorless.NextTransition.Execute.Command, byLineage.NextTransition)
	}

	verb, found := reviewTransitionCommandVerb(selectorless.NextTransition.Execute.Operation)
	if !found {
		t.Fatalf("acknowledgement operation %q has no dispatched verb", selectorless.NextTransition.Execute.Operation)
	}
	acknowledgeArgs := []string{verb}
	for _, argument := range selectorless.NextTransition.Execute.Arguments {
		acknowledgeArgs = append(acknowledgeArgs, argument.Token)
	}
	var acknowledged bytes.Buffer
	if err := RunReview(acknowledgeArgs, &acknowledged); err != nil {
		t.Fatalf("selectorless-issued acknowledgement: %v\n%s", err, acknowledged.String())
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
	assertAcknowledgedEnvelope(t, acknowledged.Bytes(), started.LineageID, pending.TargetIdentity, pending.ExpectedRevision)

	after := runSelectorlessNegotiatedStatus(t, repo)
	if after.NextTransition == nil || after.NextTransition.Kind != reviewNextTransitionExecute ||
		after.NextTransition.ReasonCode != "fresh_target_ready" || after.NextTransition.Execute == nil ||
		after.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("selectorless STATUS after the burn = %#v, want fresh_target_ready START", after.NextTransition)
	}
}

// The admission is exact: an approved record whose current snapshot identity
// is not the live target's stays historical, and STATUS keeps offering a
// fresh START for the changed candidate.
func TestNegotiatedStatusKeepsFreshStartForApprovedRecordOfDifferentTarget(t *testing.T) {
	repo, _, store, _ := startZeroLensPendingAcknowledgement(t)
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", "a different documentation candidate\n", 0o644)

	status := runSelectorlessNegotiatedStatus(t, repo)
	if status.Applicability == reviewtransaction.TargetApplicabilityCurrent || status.NextTransition == nil ||
		status.NextTransition.ReasonCode != "fresh_target_ready" || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("selectorless STATUS for a different target = applicability=%q transition=%#v, want fresh START", status.Applicability, status.NextTransition)
	}
	if record, err := store.Load(); err != nil || record.State.State != reviewtransaction.StateApproved {
		t.Fatalf("approved authority for the earlier target was mutated: %#v, %v", record, err)
	}
}

// TestNegotiatedStatusAfterInBudgetCorrectionExposesValidationWithoutHostRuntime
// is the RED-first proof for #3805: START froze the lineage to its runtime,
// so a negotiated STATUS after an in-budget correction that omits `--agent`
// must collect the targeted validation through review.capture-validation
// bound to that recorded runtime, exactly as the `--agent` form does, instead
// of stopping with manual_intervention_required.
func TestNegotiatedStatusAfterInBudgetCorrectionExposesValidationWithoutHostRuntime(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t, "--agent", string(model.AgentClaudeCode))

	assertValidation := func(label string, args ...string) {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview(append([]string{
			"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition",
		}, args...), &output); err != nil {
			t.Fatalf("status %s: %v\n%s", label, err, output.String())
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		if err := status.Validate(); err != nil {
			t.Fatal(err)
		}
		if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
			status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
			len(status.NextTransition.Collect.Inputs) != 1 {
			t.Fatalf("status %s = %#v, want one targeted validation input", label, status.NextTransition)
		}
		input := status.NextTransition.Collect.Inputs[0]
		arguments, err := reviewTransitionArgumentMap(input.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		if input.CaptureOperation != reviewCaptureValidationCaptureOperation || input.ValidationRequest == nil ||
			input.ValidationRequest.RequestHash != request.RequestHash || arguments["agent"] != string(model.AgentClaudeCode) {
			t.Fatalf("status %s targeted validation input = %#v, want review.capture-validation bound to %s", label, input, model.AgentClaudeCode)
		}
	}
	assertValidation("without a runtime identity")
	assertValidation("with a runtime identity", "--agent", string(model.AgentClaudeCode))
}

// A lineage started before the runtime was recorded keeps the manual route:
// nothing is invented for it.
func TestNegotiatedStatusAfterInBudgetCorrectionWithoutRecordedRuntimeStaysManual(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, _ := providerCorrectionReadyWithoutVerificationEvidence(t)
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &output); err != nil {
		t.Fatalf("status without a recorded runtime: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "manual_intervention_required" {
		t.Fatalf("status without a recorded runtime = %#v", status.NextTransition)
	}
}

// The admission reads exactly one store: the lineage START derives for the
// live target. An unrelated sibling store the process cannot read is not this
// candidate's authority and must not fail the canonical selectorless STATUS.
func TestNegotiatedStatusReplaysPendingAcknowledgementDespiteUnreadableSiblingStore(t *testing.T) {
	repo, started, store, pending := startZeroLensPendingAcknowledgement(t)
	// A directory where the sibling's record file belongs makes every read of
	// that store an operational failure rather than a damaged record.
	if err := os.MkdirAll(filepath.Join(filepath.Dir(store.Dir), "review-unreadable-sibling", filepath.Base(store.StatePath())), 0o755); err != nil {
		t.Fatal(err)
	}

	selectorless := runSelectorlessNegotiatedStatus(t, repo)
	if selectorless.Applicability != reviewtransaction.TargetApplicabilityCurrent || selectorless.NextTransition == nil ||
		selectorless.NextTransition.Kind != reviewNextTransitionExecute || selectorless.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("selectorless STATUS = applicability=%q transition=%#v, want the pending acknowledgement", selectorless.Applicability, selectorless.NextTransition)
	}
	assertApprovedAcknowledgementTransition(t, selectorless.NextTransition.Execute, repo, started.LineageID, pending.TargetIdentity, pending.ExpectedRevision)
}
