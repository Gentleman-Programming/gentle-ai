package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// captureReviewProcessStderr swaps the process stderr for a pipe so a test can
// assert the machine-readable (negotiated) surface stays byte-silent. It
// catches every writer that resolves os.Stderr at call time: the consent
// console (defaultReviewConsole) and any direct fmt.Fprint(os.Stderr, ...)
// site. The returned function restores stderr and yields the captured bytes.
func captureReviewProcessStderr(t *testing.T) func() string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = write
	var once sync.Once
	var captured string
	stop := func() string {
		once.Do(func() {
			os.Stderr = previous
			_ = write.Close()
			data, readErr := io.ReadAll(read)
			_ = read.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			captured = string(data)
		})
		return captured
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

// TestNegotiatedStatusSuccessIsByteSilentOnStderr pins the pi compatibility
// contract: a successful negotiated STATUS (exit 0, valid JSON on stdout)
// writes zero bytes to stderr. The forecast is carried structurally in the
// JSON `forecast` field, never narrated (gentle-pi fails closed with
// UNEXPECTED_STDERR on any stderr byte a successful operation writes).
func TestNegotiatedStatusSuccessIsByteSilentOnStderr(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/notes.md", "silent status\n", 0o644)
	stderr := captureReviewProcessStderr(t)

	var stdout bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--next-transition",
	}, &stdout); err != nil {
		t.Fatalf("negotiated STATUS: %v\n%s", err, stdout.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, stdout.Bytes(), &status)
	if status.NextTransition == nil || status.Forecast == nil || len(status.Forecast.Steps) == 0 {
		t.Fatalf("forecast must stay structural on stdout: %#v", status.Forecast)
	}
	if got := stderr(); got != "" {
		t.Fatalf("successful negotiated STATUS wrote stderr, want zero bytes:\n%q", got)
	}
}

// TestNegotiatedStatusStopIsByteSilentOnStderr covers the stop-shaped success:
// a negotiated STATUS answering `stop` (here rdd_disabled) still exits 0 with
// a valid envelope, so its Tier C stop narration and the rendered enable
// continuation must not reach stderr either. The reason code stays structural
// in next_transition.reason_code.
func TestNegotiatedStatusStopIsByteSilentOnStderr(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/notes.md", "disabled stop\n", 0o644)
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "global"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	stderr := captureReviewProcessStderr(t)

	var stdout bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--next-transition",
	}, &stdout); err != nil {
		t.Fatalf("negotiated STATUS while disabled: %v\n%s", err, stdout.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, stdout.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionStop ||
		status.NextTransition.ReasonCode != "rdd_disabled" {
		t.Fatalf("disabled transition = %#v", status.NextTransition)
	}
	if got := stderr(); got != "" {
		t.Fatalf("stop-shaped negotiated STATUS wrote stderr, want zero bytes:\n%q", got)
	}
}

// TestNegotiatedStartUndeclaredConsentIsByteSilentOnStderr pins the START half
// of the same contract: a negotiated START without a --consent declaration
// reviews the candidate in the fail-safe direction, exactly like a headless
// start, but announces nothing — the consent-skip notices are a human surface
// and negotiated consent is carried by the typed consent envelope instead.
// Neither the one-time consent latch nor the once-per-clone notice marker is
// consumed, so a later plain (non-negotiated) start still shows the notice.
func TestNegotiatedStartUndeclaredConsentIsByteSilentOnStderr(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	stderr := captureReviewProcessStderr(t)

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-negotiated-silent",
	}))
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.Action != string(reviewtransaction.CompactStartCreated) || len(started.SelectedLenses) != 4 {
		t.Fatalf("undeclared negotiated START stopped reviewing: %#v", started)
	}
	if console.Len() != 0 {
		t.Fatalf("undeclared negotiated START wrote to the console, want silence:\n%s", console.String())
	}
	if got := stderr(); got != "" {
		t.Fatalf("undeclared negotiated START wrote stderr, want zero bytes:\n%q", got)
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("negotiated START consumed the one-time question: asked=%v err=%v", asked, err)
	}
	if shown, err := reviewConsentNoticeAlreadyShown(context.Background(), repo); err != nil || shown {
		t.Fatalf("negotiated START burned the once-per-clone notice marker: shown=%v err=%v", shown, err)
	}
}

// TestNegotiatedStartConsentAnswersStayByteSilentOnStderr proves the three
// declared consent shapes stay silent on success: relay answers with the typed
// question envelope, granted starts the review, and neither touches stderr.
func TestNegotiatedStartConsentAnswersStayByteSilentOnStderr(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	stderr := captureReviewProcessStderr(t)

	relay := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-negotiated-consents", "--consent", "relay",
	}))
	question := decodeConsentQuestion(t, relay.Bytes())
	if question.Action != "consent_required" || !question.Blocking {
		t.Fatalf("relay did not answer with the typed question: %#v", question)
	}

	granted := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-negotiated-consents", "--consent", "granted",
	}))
	started := decodeNegotiatedReviewStart(t, granted.Bytes())
	if started.Action != string(reviewtransaction.CompactStartCreated) {
		t.Fatalf("granted START = %#v", started)
	}

	if console.Len() != 0 {
		t.Fatalf("declared negotiated consent wrote to the console:\n%s", console.String())
	}
	if got := stderr(); got != "" {
		t.Fatalf("declared negotiated consent wrote stderr, want zero bytes:\n%q", got)
	}
}

// TestNegotiatedLifecycleOperationsAreByteSilentOnStderr walks a complete
// low-risk negotiated lifecycle — STATUS, START, FINALIZE, staged VALIDATE —
// and requires zero stderr bytes from every successful operation, the exact
// sequence gentle-pi drives fail-closed.
func TestNegotiatedLifecycleOperationsAreByteSilentOnStderr(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "docs/notes.md", "lifecycle\n", 0o644)
	stderr := captureReviewProcessStderr(t)

	status := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV2, "--agent", "claude-code")
	started := executeStartTransition(t, repo, status)
	if started.RiskLevel != reviewtransaction.RiskLow || len(started.SelectedLenses) != 0 {
		t.Fatalf("low-risk negotiated START = %#v", started)
	}

	var finalizeStatus bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code", "--next-transition",
	}, &finalizeStatus); err != nil {
		t.Fatalf("post-start STATUS: %v", err)
	}
	var ready ReviewTargetStatusResult
	decodeStrictReviewJSON(t, finalizeStatus.Bytes(), &ready)
	if ready.NextTransition == nil || ready.NextTransition.Execute == nil ||
		ready.NextTransition.Execute.Operation != "review.finalize" {
		t.Fatalf("post-start transition = %#v", ready.NextTransition)
	}

	var finalized bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--cwd", repo, "--lineage", started.LineageID,
		"--captured-results=true", "--contract", ReviewIntegrationContractV2,
		"--agent", "claude-code",
	}, &finalized); err != nil {
		t.Fatalf("negotiated FINALIZE: %v\n%s", err, finalized.String())
	}

	runReviewCLIGit(t, repo, "add", "--", "docs/notes.md")
	var validated bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV2, "--gate", "pre-commit", "--cwd", repo,
	}, &validated); err != nil {
		t.Fatalf("negotiated VALIDATE: %v\n%s", err, validated.String())
	}
	if !strings.Contains(validated.String(), `"result": "allow"`) &&
		!strings.Contains(validated.String(), `"result":"allow"`) {
		t.Fatalf("pre-commit gate did not allow: %s", validated.String())
	}

	if got := stderr(); got != "" {
		t.Fatalf("negotiated lifecycle wrote stderr, want zero bytes:\n%q", got)
	}
}

// TestNegotiatedStartUndeclaredInteractiveKeepsConsentCeremony pins the human
// half of the negotiated-silence contract: an undeclared negotiated START at a
// real interactive terminal keeps the exact plain-start one-time ceremony —
// the Tier A consent prompt is written to the console and answer 2 refuses
// this candidate. This is also the production-reachability proof for the
// human narration surface: the prompt bytes below are the registered Tier A
// vocabulary reaching a real console through authorizeReviewStart.
func TestNegotiatedStartUndeclaredInteractiveKeepsConsentCeremony(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, true, "2\n")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-negotiated-interactive",
	}), &output)
	if !errors.Is(err, errReviewDeclinedForCandidate) {
		t.Fatalf("interactive negotiated refusal = %v, want errReviewDeclinedForCandidate\n%s", err, output.String())
	}
	assertReviewConsentPrompt(t, console.String(), "scripts/deploy.sh")
}

// TestNegotiatedStartUndeclaredAskedLatchProceedsSilently proves the machine
// half stays intact: a non-interactive negotiated START with the one-time
// question already answered proceeds, writes nothing to the console, and
// leaves both the consent latch and the once-per-clone notice marker exactly
// as it found them.
func TestNegotiatedStartUndeclaredAskedLatchProceedsSilently(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	if err := recordReviewConsentAsked(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-negotiated-asked-latch",
	}))
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.Action != string(reviewtransaction.CompactStartCreated) {
		t.Fatalf("asked-latch negotiated START = %#v", started)
	}
	if console.Len() != 0 {
		t.Fatalf("asked-latch negotiated START wrote to the console:\n%s", console.String())
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || !asked {
		t.Fatalf("negotiated START changed the consent latch: asked=%v err=%v", asked, err)
	}
	if shown, err := reviewConsentNoticeAlreadyShown(context.Background(), repo); err != nil || shown {
		t.Fatalf("negotiated START burned the notice marker: shown=%v err=%v", shown, err)
	}
}

// TestPlainStartConsentNoticeStaysByteIdentical pins requirement 3 of the
// negotiated-silence contract: non-negotiated (human) invocations keep their
// narration byte-identical. A fresh headless clone with the default mode
// source prints exactly the skip notice and the default-provenance line, each
// as its own line, and nothing else.
func TestPlainStartConsentNoticeStaysByteIdentical(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-plain-notice"}, &output); err != nil {
		t.Fatalf("plain headless start: %v\n%s", err, output.String())
	}
	want := reviewConsentSkippedNotice + "\n" + reviewConsentSkippedDefaultProvenance + "\n"
	if console.String() != want {
		t.Fatalf("plain start notice drifted:\n got %q\nwant %q", console.String(), want)
	}
}
