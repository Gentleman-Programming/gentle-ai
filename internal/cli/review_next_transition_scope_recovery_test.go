package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestNegotiatedStatusRoutesHistoricalScopeChangeToRecovery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/attempt.md", "# reviewed\n", 0o644)
	historical := seedHistoricalApprovalForRepo(t, repo, "scope-recovery-historical")
	writeReviewStartCandidate(t, repo, "docs/attempt.md", "# changed\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", historical.Record.State.LineageID, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityUnrelated || status.Action != reviewtransaction.TargetStatusActionStart ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "fresh_target_ready" {
		t.Fatalf("historical scope status = %#v", status)
	}
}

func TestRejectedTargetedValidatorCaptureRoutesEscalatedRecovery(t *testing.T) {
	reviewEnabledHome(t)
	repo, lineage, request := providerCorrectionReadyWithoutVerificationEvidence(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonicalProviderRoleResult(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: false, Evidence: []string{"inspected the correction candidate: the critical finding remains"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"inspected the correction candidate: no unrelated regression"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, closure, err := reviewProviderCloseTargetedValidatorRaw(t.Context(), repo, store, record.State, record.State.CapturePhaseRevision, payload)
	if err != nil || closure == nil || closure.State != reviewtransaction.StateEscalated {
		t.Fatalf("rejected targeted validator closure = %#v, %v", closure, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "recovery.go"), []byte("package candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "recovery.go")

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
		"--lineage", lineage, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Action != reviewtransaction.TargetStatusActionRecover || status.ActionDisposition != reviewtransaction.RecoveryEscalated ||
		status.NextTransition == nil || status.NextTransition.ReasonCode != "recovery_authorization_required" {
		t.Fatalf("escalated status = %#v", status)
	}
}
