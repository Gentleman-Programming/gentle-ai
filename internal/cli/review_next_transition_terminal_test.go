package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

func derivedRangeTerminalStatus(t *testing.T, repo string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &output); err != nil {
		t.Fatalf("derived range STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), output.Bytes())
	return status
}

// TestSelectorlessStatusDoesNotRestartEscalatedCandidate reproduces the
// operator's exact emitted START, not a hand-assembled substitute.
func TestSelectorlessStatusDoesNotRestartEscalatedCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n", 0o644)
	// v1 emits a direct START; v2 relays candidate consent before START.
	fresh := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV1)
	started := executeStartTransition(t, repo, fresh)
	lineage := started.LineageID
	if lineage != startTransitionArgumentValue(t, fresh, "lineage") {
		t.Fatalf("START lineage = %q, offered %q", lineage, startTransitionArgumentValue(t, fresh, "lineage"))
	}
	legacyStarted := ReviewFacadeStartResult{
		LineageID: lineage, TargetIdentity: negotiatedStartTarget(started), SelectedLenses: started.SelectedLenses,
	}
	args := cliReviewerCaptureArgs(t, repo, legacyStarted, 0, []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
		ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}})
	if err := RunReviewCaptureResult(args, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
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
		"--cwd", repo, "--lineage", lineage, "--target", plan.TargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", plan.RequestHash, "--correction-lines", "2",
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request, err := reviewtransaction.BuildTargetedValidationRequest(t.Context(), repo, record.State, record.State.CapturePhaseRevision)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash, CorrectionTargetIdentity: request.CorrectionTargetIdentity,
		OriginalCriteria:     facadeValidationCheck{Passed: false, Evidence: []string{"acceptance test failed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"no regression observed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	resultFile := writeReviewCLIRawInput(t, payload)
	if err := RunReviewCaptureValidation([]string{
		"--cwd", repo, "--lineage", lineage, "--target", request.CorrectionTargetIdentity,
		"--expected-revision", record.State.CapturePhaseRevision, "--request-hash", request.RequestHash,
		"--agent", string(model.AgentPi), "--input", resultFile,
	}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	record, err = store.Load()
	if err != nil || record.State.State != reviewtransaction.StateEscalated {
		t.Fatalf("terminal authority = %#v, %v; want escalated", record, err)
	}

	// Correction changed the live candidate. Restore the exact original bytes
	// so selectorless STATUS assesses the candidate that occupied this lineage.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := runSelectorlessNegotiatedStatus(t, repo)
	if status.TargetIdentity != record.State.InitialSnapshot.Identity {
		t.Fatalf("restored target = %q, want frozen initial target %q", status.TargetIdentity, record.State.InitialSnapshot.Identity)
	}
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
		status.Authority.LineageID != lineage || status.Authority.State != reviewtransaction.StateEscalated {
		t.Fatalf("restored original target lost terminal authority: %#v", status)
	}
	if status.NextTransition == nil {
		t.Fatal("selectorless STATUS omitted the terminal transition")
	}
	if status.NextTransition.Execute == nil {
		if status.NextTransition.Kind != reviewNextTransitionStop || status.NextTransition.ReasonCode != "native_stop_required" {
			t.Fatalf("escalated authority did not offer a truthful terminal stop: %#v", status.NextTransition)
		}
		return
	}
	if status.NextTransition.ReasonCode != "fresh_target_ready" || status.NextTransition.Execute.Operation != "review.start" ||
		status.NextTransition.Execute.Binding.LineageID != lineage {
		t.Fatalf("unexpected executable STATUS after escalation: %#v", status.NextTransition)
	}
	var output bytes.Buffer
	emitted := []string{"start"}
	for _, argument := range status.NextTransition.Execute.Arguments {
		emitted = append(emitted, argument.Token)
	}
	if err := RunReview(emitted, &output); err != nil {
		t.Fatalf("emitted START consent relay: %v\n%s", err, output.String())
	}
	question := decodeConsentQuestion(t, output.Bytes())
	if question.Action != "consent_required" || question.TargetIdentity != status.TargetIdentity {
		t.Fatalf("emitted START did not relay consent for the same candidate: %#v", question)
	}
	invocation := ""
	for _, choice := range question.Choices {
		if choice.Answer == "granted" {
			invocation = choice.Invocation
			break
		}
	}
	if invocation == "" {
		t.Fatalf("consent relay omitted granted invocation: %#v", question)
	}
	output.Reset()
	err = RunReview(invocationArgs(t, invocation), &output)
	if err == nil {
		t.Fatalf("granted START unexpectedly succeeded: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "atomic_start_conflict" || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("granted START failure = %#v, want atomic_start_conflict/not_started (error %v)", failure, err)
	}
	t.Fatalf("selectorless STATUS advertised fresh_target_ready for escalated lineage %q and target %q; exact granted START failed %s (%s)", lineage, status.TargetIdentity, failure.Code, failure.MutationOutcome)
}

func TestNextTransitionDerivedRangeAcknowledgementStaysTerminal(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation candidate")
	offered := derivedRangeTerminalStatus(t, repo)
	if offered.NextTransition == nil || offered.NextTransition.Execute == nil {
		t.Fatalf("no committed-range START: %#v", offered)
	}
	args := []string{"start"}
	for _, argument := range offered.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("execute offered START: %v\n%s", err, output.String())
	}
	pending := derivedRangeTerminalStatus(t, repo)
	if pending.TargetIdentity != offered.TargetIdentity || pending.NextTransition == nil || pending.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("derived range lost pending acknowledgement: %#v", pending)
	}
	args = []string{"acknowledge-approved"}
	for _, argument := range pending.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	output.Reset()
	if err := RunReview(args, &output); err != nil {
		t.Fatal(err)
	}
	// A new hook session has no reminder history to hide a duplicate START.
	payload := reviewStopHookTestPayload(t, "derived-range-session", repo, false, nil)
	output.Reset()
	var diagnostics bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &output, &diagnostics); err != nil || output.Len() != 0 {
		t.Fatalf("derived range Stop = %s, %v", output.String(), err)
	}
	after := derivedRangeTerminalStatus(t, repo)
	if after.TargetIdentity != offered.TargetIdentity || after.NextTransition == nil || after.NextTransition.ReasonCode != "target_already_acknowledged" || after.Authority != nil {
		t.Fatalf("derived range not terminal: %#v", after)
	}
	consumed, err := reviewtransaction.CompactTargetConsumed(context.Background(), repo, offered.TargetIdentity)
	if err != nil || !consumed {
		t.Fatalf("derived range consumption = %v, %v", consumed, err)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", "new committed range\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "new range")
	changed := derivedRangeTerminalStatus(t, repo)
	if changed.TargetIdentity == offered.TargetIdentity || changed.NextTransition == nil || changed.NextTransition.Execute == nil || changed.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("new committed range suppressed: %#v", changed)
	}
}
