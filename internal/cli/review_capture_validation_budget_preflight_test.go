package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// providerCorrectionReadyOverBudget freezes a one-lens correction whose
// accepted forecast (the full frozen budget) fits, but whose actual applied
// fix changes every remaining line -- well past that same budget. It mirrors
// providerCorrectionReadyWithoutVerificationEvidence's fixture shape, tuned so
// the returned request's actual correction size exceeds CorrectionBudget.
func providerCorrectionReadyOverBudget(t *testing.T) (string, string, reviewtransaction.TargetedValidationRequest) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStartWith(t, repo, "provider-targeted-validator-over-budget")
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
	// Forecast the full frozen budget: accepted because a forecast is never
	// compared to anything (issue #4080's contributing factor), even though
	// the actual fix below spends far more than it.
	if err := RunReviewCaptureCorrectionPlan([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", plan.TargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", plan.RequestHash,
		"--correction-lines", strconv.Itoa(record.State.CorrectionBudget),
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nONE\nTWO\nTHREE\nFOUR\n"), 0o644); err != nil {
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

// TestOverBudgetPassedValidatorRefusesBeforeAdmittingAnyRole is issue #4080:
// a targeted validator that reports "passed" on a correction whose actual
// changed lines exceed the frozen budget must be refused before its admission
// is durably written, leaving zero admitted validator roles behind -- not
// admitted first and refused on a later, separate write, which used to spend
// the sixth admitted-role slot on a result the budget then rejected and wedge
// the lineage on retry.
func TestOverBudgetPassedValidatorRefusesBeforeAdmittingAnyRole(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyOverBudget(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	beforeAdmitted := len(record.State.AdmittedRoleResults)

	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapter{raw: providerTargetedValidationPayload(t, request)}, nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })

	var output bytes.Buffer
	err = RunReviewCaptureValidation([]string{
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &output)
	if err == nil {
		t.Fatalf("over-budget passed validator capture succeeded, want a refused not_started preflight; output=%s", output.String())
	}
	if !strings.Contains(err.Error(), "exceeding the frozen budget") {
		t.Fatalf("over-budget passed validator capture error = %v, want the budget-exceeded refusal", err)
	}

	afterRecord, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRecord.State.AdmittedRoleResults) != beforeAdmitted {
		t.Fatalf("admitted role results after refused over-budget capture = %d, want unchanged %d (zero admitted validator roles)",
			len(afterRecord.State.AdmittedRoleResults), beforeAdmitted)
	}
	if _, found := afterRecord.State.AdmittedRoleResult(reviewtransaction.CompactRoleTargetedValidator,
		afterRecord.State.CapturePhaseRevision, request.CorrectionTargetIdentity, request.RequestHash); found {
		t.Fatal("over-budget passed validator was admitted despite the budget refusal")
	}

	// Shrinking the correction back within budget and retrying succeeds: the
	// refused attempt above consumed neither the correction attempt nor an
	// admitted-role slot.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err = reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	retryRequest, err := reviewtransaction.BuildTargetedValidationRequest(context.Background(), repo, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapter{raw: providerTargetedValidationPayload(t, retryRequest)}, nil
	}
	var retryOutput bytes.Buffer
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo, "--lineage", lineage, "--target", retryRequest.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", retryRequest.RequestHash,
		"--agent", string(model.AgentClaudeCode), "--execute=true",
	}, &retryOutput); err != nil {
		t.Fatalf("in-budget retry after over-budget refusal failed: %v\n%s", err, retryOutput.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, retryOutput.Bytes(), &terminal)
	if terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("in-budget retry terminal state = %q, want approved", terminal.State)
	}
	assertApprovedCompactAuthorityBurned(t, store, lineage)
}
