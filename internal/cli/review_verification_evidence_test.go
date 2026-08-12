package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// startHighRiskValidatingCLIReview drives a high-risk lineage to the
// StateValidating gate with all four lens results admitted, so the typed
// evidence rules below guard a real gate rather than a synthetic state.
func startHighRiskValidatingCLIReview(t *testing.T, repo string) ReviewFacadeStartResult {
	t.Helper()
	started := startHighRiskCLIReview(t, repo)
	for order := range started.SelectedLenses {
		captureCLIReviewerResult(t, repo, started, order)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results=true"}, io.Discard); err != nil {
		t.Fatalf("finalize high-risk captured results: %v", err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil || record.State.State != reviewtransaction.StateValidating {
		t.Fatalf("high-risk fixture authority = %#v, %v", record.State, err)
	}
	return started
}

// captureVerificationEvidenceForTest publishes a typed captured evidence record
// bound to the current validating snapshot of a lineage, mirroring the exact
// STATUS-emitted capture transition.
func captureVerificationEvidenceForTest(t *testing.T, repo, lineage string, payload []byte, outcome reviewtransaction.VerificationOutcome) {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{
		"--cwd", repo, "--lineage", lineage, "--target", record.State.CurrentSnapshot.Identity,
		"--expected-revision", record.Revision, "--outcome", string(outcome), "--input", evidencePath,
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

// TestHighRiskFinalizeFailsClosedOnRawEvidence proves the raw --evidence path
// can never approve a high-risk candidate: even substantive prose bytes must be
// carried by a typed captured record before they authorize the gate, so the
// authority must stay at StateValidating after the refusal.
func TestHighRiskFinalizeFailsClosedOnRawEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started := startHighRiskValidatingCLIReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused and full repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath}, &bytes.Buffer{}); err == nil {
		t.Fatal("high-risk finalize approved through the raw --evidence path")
	}
	after, err := store.Load()
	if err != nil || after.State.State != reviewtransaction.StateValidating || after.Revision != before.Revision {
		t.Fatalf("raw-evidence refusal advanced authority: before=%#v after=%#v, %v", before.State, after.State, err)
	}
}

// TestHighRiskFinalizeFailsClosedOnSentinelCapturedEvidence proves a typed
// captured record whose outcome is "passed" still cannot authorize a high-risk
// approval when its payload carries no substantive verification content — a
// bare verdict word with a trailing newline, a decorated verdict, whitespace-
// only bytes, or a single opaque token. The gate needs auditable content it can
// inspect, so the authority must stay at StateValidating after every refusal.
func TestHighRiskFinalizeFailsClosedOnSentinelCapturedEvidence(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload string
	}{
		{name: "exact PASS newline", payload: "PASS\n"},
		{name: "decorated PASS bang", payload: "PASS!"},
		{name: "whitespace only", payload: "\n  \n"},
		{name: "opaque single token", payload: "x"},
		{name: "bare PASS", payload: "PASS"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			started := startHighRiskValidatingCLIReview(t, repo)
			captureVerificationEvidenceForTest(t, repo, started.LineageID, []byte(tt.payload), reviewtransaction.VerificationOutcomePassed)
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence=true"}, &bytes.Buffer{}); err == nil {
				t.Fatal("high-risk finalize approved sentinel-only captured evidence")
			}
			after, err := store.Load()
			if err != nil || after.State.State != reviewtransaction.StateValidating || after.Revision != before.Revision {
				t.Fatalf("sentinel refusal advanced authority: before=%#v after=%#v, %v", before.State, after.State, err)
			}
		})
	}
}

// TestHighRiskFinalizeApprovesOnSubstantiveCapturedEvidence proves the typed
// path still reaches approval when the captured payload is real verification
// content, so the fail-closed gate does not lock out legitimate evidence.
func TestHighRiskFinalizeApprovesOnSubstantiveCapturedEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started := startHighRiskValidatingCLIReview(t, repo)
	captureVerificationEvidenceForTest(t, repo, started.LineageID, []byte("go build ./... ok\ngo test ./... ok\n"), reviewtransaction.VerificationOutcomePassed)
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-evidence=true"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("high-risk finalize with substantive captured evidence: %v", err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil || record.State.State != reviewtransaction.StateApproved || record.State.EvidenceOutcome != reviewtransaction.VerificationOutcomePassed {
		t.Fatalf("high-risk substantive approval = %#v, %v", record.State, err)
	}
}

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

// TestHighRiskCorrectionAcceptanceFailsClosedOnSentinelEvidence proves a
// high-risk correction cannot be accepted on sentinel-only captured evidence:
// a passed captured record whose payload is bare verdict bytes ("PASS\n")
// carries no auditable substantive content, so the correction gate must refuse
// and leave the authority at StateCorrectionRequired with the revision intact.
func TestHighRiskCorrectionAcceptanceFailsClosedOnSentinelEvidence(t *testing.T) {
	t.Parallel()

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startHighRiskCLIReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "blocking-result.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "terminal value is incorrect",
			ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
			CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"inspected correction target"},
	})
	args := []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}
	for order := 1; order < len(started.SelectedLenses); order++ {
		cleanPath := filepath.Join(t.TempDir(), "clean-"+strconv.Itoa(order)+".json")
		writeReviewCLIJSON(t, cleanPath, facadeReviewerResult{
			Lens: started.SelectedLenses[order], Findings: []facadeFinding{},
			Evidence: []string{"inspected the complete frozen candidate scope named by the capture binding"},
		})
		args = append(args, "--result", cleanPath)
	}
	if err := finalizeReviewCLIArgs(t, repo, args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
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
	target := transitionArgumentValue(t, status.NextTransition, "target")
	sentinelPath := filepath.Join(t.TempDir(), "sentinel.txt")
	if err := os.WriteFile(sentinelPath, []byte("PASS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--target", target, "--expected-revision", before.Revision,
		"--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", sentinelPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	ready := readCorrectionEvidenceStatus(t, statusArgs)
	if ready.NextTransition == nil || ready.NextTransition.Kind != reviewNextTransitionCollect ||
		ready.NextTransition.ReasonCode != "targeted_validation_required" || ready.ValidationRequest == nil ||
		ready.ValidationRequest.CorrectionTargetIdentity != target {
		t.Fatalf("passed sentinel evidence did not unlock targeted validation: %#v", ready)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		TargetedValidationRequestHash: ready.ValidationRequest.RequestHash,
		CorrectionTargetIdentity:      ready.ValidationRequest.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"repository policy passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--validation", validationPath, "--captured-evidence"}, &bytes.Buffer{}); err == nil {
		t.Fatal("high-risk correction accepted sentinel-only captured evidence")
	}
	terminal, err := store.Load()
	if err != nil || terminal.State.State != reviewtransaction.StateCorrectionRequired ||
		terminal.Revision != before.Revision || len(terminal.State.CorrectionAttempts) != 0 {
		t.Fatalf("high-risk correction refusal advanced authority: before=%#v after=%#v, %v", before.State, terminal.State, err)
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
