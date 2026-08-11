package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestFailedCapturedEvidenceDrivesNegotiatedEscalationWithoutCallerBoolean(t *testing.T) {
	repo, started, _, _, _ := capturedArtifact(t)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	validating, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "verification.txt")
	if err := os.WriteFile(evidencePath, []byte("repository verification failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	captureArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", validating.State.CurrentSnapshot.Identity,
		"--expected-revision", validating.Revision, "--outcome", string(reviewtransaction.VerificationOutcomeFailed), "--input", evidencePath,
	}
	if err := RunReviewCaptureEvidence(captureArgs, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	conflict := append([]string{}, captureArgs...)
	for index := range conflict {
		if conflict[index] == string(reviewtransaction.VerificationOutcomeFailed) {
			conflict[index] = string(reviewtransaction.VerificationOutcomePassed)
		}
	}
	if err := RunReviewCaptureEvidence(conflict, &bytes.Buffer{}); err == nil {
		t.Fatal("conflicting captured outcome replay succeeded")
	}

	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}
	var statusOut bytes.Buffer
	if err := RunReview(statusArgs, &statusOut); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionExecute || transition.Execute == nil ||
		transition.Execute.Operation != "review.finalize" || transition.ReasonCode != "captured_verification_failed" ||
		strings.Contains(transition.Execute.Command, "--failed=false") {
		t.Fatalf("failed evidence transition = %#v", transition)
	}

	forged := []string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence", "--failed=false"}
	if err := RunReviewFacadeFinalize(forged, &bytes.Buffer{}); err == nil {
		t.Fatal("forged --failed=false approved failed captured metadata")
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"finalize", "--cwd", repo}
	for _, argument := range transition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	var finalizedOut bytes.Buffer
	if err := RunReview(args, &finalizedOut); err != nil {
		t.Fatalf("exact failed-evidence transition: %v\n%s", err, finalizedOut.String())
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if before.State.State != reviewtransaction.StateValidating || after.State.State != reviewtransaction.StateEscalated ||
		after.State.EvidenceOutcome != reviewtransaction.VerificationOutcomeFailed {
		t.Fatalf("failed evidence transition state: before=%#v after=%#v", before.State, after.State)
	}
	if _, eligible, err := reviewtransaction.InspectCompactFinalVerificationRetrySource(context.Background(), repo, started.LineageID, after.Revision); err != nil || eligible {
		t.Fatalf("genuine verification failure retry eligibility = %v, %v", eligible, err)
	}
}

func TestLegacyRawEvidenceWithoutMetadataFailsClosed(t *testing.T) {
	repo, started, _, _, _ := capturedArtifact(t)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(store.Dir, reviewtransaction.CompactFinalEvidenceDir)
	if err := os.MkdirAll(legacyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, reviewtransaction.CompactFinalEvidenceFile), []byte("legacy raw says PASS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "verification_evidence_required" {
		t.Fatalf("legacy raw evidence status = %#v", status.NextTransition)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence"}, &bytes.Buffer{}); err == nil {
		t.Fatal("legacy raw evidence without metadata finalized as passed")
	}
}

func TestCorrectionAcceptanceWaitsForMatchingPassedRepositoryEvidence(t *testing.T) {
	t.Parallel()

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "atomic-correction-cli")
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
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nstill wrong\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}
	status := readCorrectionEvidenceStatus(t, statusArgs)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "correction_repository_verification_required" {
		t.Fatalf("pre-validation correction status = %#v", status.NextTransition)
	}
	firstTarget := transitionArgumentValue(t, status.NextTransition, "target")
	failedPath := filepath.Join(t.TempDir(), "failed.txt")
	if err := os.WriteFile(failedPath, []byte("full repository verification failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", firstTarget, "--expected-revision", before.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomeFailed), "--input", failedPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	failedStatus := readCorrectionEvidenceStatus(t, statusArgs)
	if failedStatus.NextTransition == nil || failedStatus.NextTransition.Kind != reviewNextTransitionStop ||
		failedStatus.NextTransition.ReasonCode != "correction_repository_verification_failed" {
		t.Fatalf("failed correction repository status = %#v", failedStatus.NextTransition)
	}
	afterFailure, err := store.Load()
	if err != nil || afterFailure.Revision != before.Revision || len(afterFailure.State.CorrectionAttempts) != 0 ||
		afterFailure.State.CumulativeCorrectionLines != 0 || afterFailure.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("failed repository verification consumed correction: %#v, %v", afterFailure, err)
	}
	firstRaw := filepath.Join(store.Dir, reviewtransaction.CompactFinalEvidenceDir,
		strings.TrimPrefix(firstTarget, "sha256:"), reviewtransaction.CompactFinalEvidenceFile)
	if _, err := os.Stat(firstRaw); err != nil {
		t.Fatalf("first candidate evidence was not retained: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	secondStatus := readCorrectionEvidenceStatus(t, statusArgs)
	if secondStatus.NextTransition == nil || secondStatus.NextTransition.Kind != reviewNextTransitionCollect ||
		secondStatus.NextTransition.ReasonCode != "correction_repository_verification_required" {
		t.Fatalf("changed correction candidate status = %#v", secondStatus.NextTransition)
	}
	secondTarget := transitionArgumentValue(t, secondStatus.NextTransition, "target")
	if secondTarget == firstTarget {
		t.Fatal("changed correction candidate reused the failed candidate identity")
	}
	passedPath := filepath.Join(t.TempDir(), "passed.txt")
	if err := os.WriteFile(passedPath, []byte("targeted and full repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", secondTarget, "--expected-revision", before.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", passedPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	ready := readCorrectionEvidenceStatus(t, statusArgs)
	if ready.NextTransition == nil || ready.NextTransition.Kind != reviewNextTransitionCollect ||
		ready.NextTransition.ReasonCode != "targeted_validation_required" || ready.ValidationRequest == nil ||
		ready.ValidationRequest.CorrectionTargetIdentity != secondTarget {
		t.Fatalf("passed repository evidence did not unlock matching targeted validation: %#v", ready)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		TargetedValidationRequestHash: ready.ValidationRequest.RequestHash,
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"repository policy passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	var finalized bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--validation", validationPath, "--captured-evidence"}, &finalized); err != nil {
		t.Fatalf("atomic correction finalize: %v\n%s", err, finalized.String())
	}
	terminal, err := store.Load()
	if err != nil || terminal.State.State != reviewtransaction.StateApproved || len(terminal.State.CorrectionAttempts) != 1 ||
		terminal.State.EvidenceOutcome != reviewtransaction.VerificationOutcomePassed || terminal.State.EvidenceTargetIdentity != secondTarget {
		t.Fatalf("atomic correction terminal = %#v, %v", terminal, err)
	}
	if _, err := os.Stat(firstRaw); err != nil {
		t.Fatalf("accepted candidate replaced prior failed evidence: %v", err)
	}
}

func TestProceduralCorrectionEvidenceEscalatesBeforeRetryEligibility(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "procedural-correction-cli")
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
	statusArgs := []string{"status", "--contract", ReviewIntegrationContractV1, "--next-transition", "--cwd", repo, "--lineage", started.LineageID}
	waiting := readCorrectionEvidenceStatus(t, statusArgs)
	target := transitionArgumentValue(t, waiting.NextTransition, "target")
	toolingPath := filepath.Join(t.TempDir(), "tooling.txt")
	if err := os.WriteFile(toolingPath, []byte("repository verification runner crashed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", target, "--expected-revision", waiting.Authority.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomeProceduralFailure), "--input", toolingPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	ready := readCorrectionEvidenceStatus(t, statusArgs)
	if ready.NextTransition == nil || ready.NextTransition.Kind != reviewNextTransitionExecute || ready.NextTransition.Execute == nil ||
		ready.NextTransition.ReasonCode != "correction_repository_tooling_failed" || ready.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("procedural correction transition = %#v", ready.NextTransition)
	}
	args := []string{"finalize", "--cwd", repo}
	for _, argument := range ready.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	if err := RunReview(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := store.Load()
	if err != nil || terminal.State.State != reviewtransaction.StateEscalated || len(terminal.State.CorrectionAttempts) != 0 ||
		terminal.State.CumulativeCorrectionLines != 0 || terminal.State.CorrectionVerificationTarget == nil ||
		terminal.State.EvidenceOutcome != reviewtransaction.VerificationOutcomeProceduralFailure {
		t.Fatalf("procedural correction terminal = %#v, %v", terminal, err)
	}
}

func readCorrectionEvidenceStatus(t *testing.T, args []string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("correction status: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}

func capturePassedCorrectionEvidenceForTest(t *testing.T, repo, lineage string) reviewtransaction.TargetedValidationRequest {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	target, err := facadeVerificationEvidenceTarget(context.Background(), repo, record.State, record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "passed-correction-evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("targeted and full repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{
		"--cwd", repo, "--lineage", lineage, "--target", target.Identity,
		"--expected-revision", record.Revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", evidencePath,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequestFromSnapshot(context.Background(), repo, record.State, record.Revision, target)
	if err != nil {
		t.Fatal(err)
	}
	if request.CorrectionTargetIdentity != target.Identity {
		t.Fatalf("targeted validation identity %q != captured target %q", request.CorrectionTargetIdentity, target.Identity)
	}
	return request
}
