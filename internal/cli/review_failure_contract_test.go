package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/internal/sddstatus"
)

func TestNegotiatedReviewFailuresUseOneEnvelopeAcrossRoutes(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		operation string
	}{
		{name: "capabilities", args: []string{"capabilities", "unexpected"}, operation: "review.capabilities"},
		{name: "start", args: []string{"start", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.start"},
		{name: "status", args: []string{"status", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.status"},
		{name: "finalize", args: []string{"finalize", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.finalize"},
		{name: "validate", args: []string{"validate", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.validate"},
		{name: "bind sdd", args: []string{"bind-sdd", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.bind_sdd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(tt.args, &output)
			if err == nil {
				t.Fatal("negotiated invalid request succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Operation != tt.operation || failure.Code != "invalid_request" ||
				failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe ||
				failure.Replayability != reviewtransaction.ReplayabilityNotReplayable {
				t.Fatalf("failure = %#v", failure)
			}
			var publicErr *ReviewIntegrationFailureError
			if !errors.As(err, &publicErr) {
				t.Fatalf("error = %T, want *ReviewIntegrationFailureError", err)
			}
			assertNoPrivateReviewOperationFields(t, output.Bytes())
		})
	}
}

func TestNegotiatedReviewContractFailuresArePreMutationAndLegacyErrorsStayCompatible(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "capabilities unsupported", args: []string{"capabilities", "--contract", "gentle-ai.review-integration/v2"}, code: "unsupported_contract"},
		{name: "start empty", args: []string{"start", "--contract="}, code: "empty_contract"},
		{name: "finalize malformed", args: []string{"finalize", "--contract"}, code: "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview(tt.args, &output); err == nil {
				t.Fatal("invalid contract request succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code != tt.code || failure.MutationOutcome != ReviewMutationNotStarted {
				t.Fatalf("failure = %#v", failure)
			}
		})
	}

	var legacy bytes.Buffer
	err := RunReview([]string{"start", "unexpected"}, &legacy)
	if err == nil || legacy.Len() != 0 {
		t.Fatalf("legacy invalid request = output %q, error %v", legacy.String(), err)
	}
	var publicErr *ReviewIntegrationFailureError
	if errors.As(err, &publicErr) {
		t.Fatalf("unnegotiated error became negotiated: %v", err)
	}
}

func TestNegotiatedReviewFailuresPreserveRequestedLineage(t *testing.T) {
	lineage := "review-requested-lineage"
	tests := []struct {
		name     string
		runErr   error
		wantCode string
	}{
		{name: "reviewer preflight", runErr: reviewPreflightError(errors.New("invalid reviewer payload")), wantCode: "invalid_request"},
		{name: "unknown native outcome", runErr: errors.New("transport interrupted"), wantCode: "operation_outcome_unknown"},
		{name: "legacy read only", runErr: reviewtransaction.NewLegacyReadOnlyError("review/finalize", lineage), wantCode: reviewtransaction.LegacyReadOnlyErrorCode},
		{name: "journal request mismatch", runErr: &reviewtransaction.FinalizeAttemptReplayMismatchError{LineageID: lineage}, wantCode: "finalize_request_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure(
				ReviewIntegrationOperationFinalize,
				[]string{"--lineage", lineage},
				tt.runErr,
			)
			if failure.Code != tt.wantCode || failure.LineageID != lineage {
				t.Fatalf("failure = %#v", failure)
			}
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	bindingFailure := newReviewIntegrationFailure(
		ReviewIntegrationOperationBindSDD,
		[]string{"--cwd", ".", "--change", "thin", "--lineage", lineage, "--expected-binding-revision="},
		&sddstatus.ReviewBindingPublicationError{Cause: errors.New("sync")},
	)
	if bindingFailure.Code != "binding_publication_pending" || bindingFailure.LineageID != lineage ||
		bindingFailure.Replayability != reviewtransaction.ReplayabilityExactReplaySafe || bindingFailure.NextAction != ReviewIntegrationOperationBindSDD {
		t.Fatalf("binding publication failure = %#v", bindingFailure)
	}
	if err := bindingFailure.Validate(); err != nil {
		t.Fatal(err)
	}
	receiptConflict := newReviewIntegrationFailure(
		ReviewIntegrationOperationFinalize,
		[]string{"--lineage", lineage},
		newFacadeReceiptPublicationError(lineage, "", &reviewtransaction.ImmutablePublicationConflictError{Cause: errors.New("conflict")}),
	)
	if receiptConflict.Code != "receipt_publication_conflict" || receiptConflict.MutationOutcome != ReviewMutationCommitted ||
		receiptConflict.Replayability != reviewtransaction.ReplayabilityManualActionRequired || receiptConflict.RetrySafe ||
		receiptConflict.NextAction != "explicit-maintainer-action" {
		t.Fatalf("receipt conflict failure = %#v", receiptConflict)
	}
	if err := receiptConflict.Validate(); err != nil {
		t.Fatal(err)
	}

	_, negotiated, routed := reviewIntegrationFailureRoute([]string{
		"finalize", "--contract=", "--lineage", lineage,
	})
	if !negotiated || routed == nil || routed.LineageID != lineage || routed.Code != "empty_contract" {
		t.Fatalf("routed preflight failure = %#v, negotiated %v", routed, negotiated)
	}
}

func TestNegotiatedFailureLineageUsesCanonicalFlagParsing(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		args      []string
		want      string
	}{
		{name: "equals", operation: ReviewIntegrationOperationFinalize, args: []string{"--lineage=review-equals"}, want: "review-equals"},
		{name: "split", operation: ReviewIntegrationOperationFinalize, args: []string{"--lineage", "review-split"}, want: "review-split"},
		{name: "boolean before lineage", operation: ReviewIntegrationOperationFinalize, args: []string{"--failed", "--lineage", "review-after-boolean"}, want: "review-after-boolean"},
		{name: "other flag owns lineage-shaped value", operation: ReviewIntegrationOperationFinalize, args: []string{"--cwd", "--lineage=not-a-lineage"}},
		{name: "double dash stops parsing", operation: ReviewIntegrationOperationFinalize, args: []string{"--", "--lineage=review-after-stop"}},
		{name: "positional stops parsing", operation: ReviewIntegrationOperationFinalize, args: []string{"unexpected", "--lineage=review-after-positional"}},
		{name: "duplicate last wins", operation: ReviewIntegrationOperationFinalize, args: []string{"--lineage", "review-first", "--lineage=review-second"}, want: "review-second"},
		{name: "duplicate malformed last clears", operation: ReviewIntegrationOperationFinalize, args: []string{"--lineage", "review-first", "--lineage=-invalid"}},
		{name: "unknown flag fails closed", operation: ReviewIntegrationOperationFinalize, args: []string{"--unknown", "value", "--lineage", "review-hidden"}},
		{name: "over maximum length", operation: ReviewIntegrationOperationFinalize, args: []string{"--lineage", "r" + strings.Repeat("a", 128)}},
		{name: "start boolean before lineage", operation: "review.start", args: []string{"--committed-only", "--lineage", "review-start"}, want: "review-start"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure(tt.operation, tt.args, errors.New("transport interrupted"))
			if failure.LineageID != tt.want {
				t.Fatalf("lineage_id = %q, want %q", failure.LineageID, tt.want)
			}
		})
	}
}

func TestNegotiatedStartLockFailuresPreservePreMutationRetryTruth(t *testing.T) {
	lineage := "review-lock-boundary"
	for _, tt := range []struct {
		name  string
		err   error
		code  string
		retry bool
		next  string
	}{
		{name: "timeout", err: &reviewtransaction.AuthorityLockTimeoutError{}, code: "authority_lock_timeout", retry: true, next: "retry_with_bounded_backoff"},
		{name: "cancelled", err: &reviewtransaction.AuthorityLockCancelledError{}, code: "authority_lock_cancelled", next: "stop"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure("review.start", []string{"--lineage", lineage}, tt.err)
			if failure.Code != tt.code || failure.Phase != "pre_native" || failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.RetrySafe != tt.retry || failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
				failure.NextAction != tt.next || failure.LineageID != lineage {
				t.Fatalf("lock failure = %#v", failure)
			}
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNegotiatedFacadeAggregateTimeoutPreservesMutationTruth(t *testing.T) {
	originalRunner := reviewFacadeCommandRunner
	originalTimeout := reviewFacadeOperationTimeout
	t.Cleanup(func() {
		reviewFacadeCommandRunner = originalRunner
		reviewFacadeOperationTimeout = originalTimeout
	})
	reviewFacadeOperationTimeout = 25 * time.Millisecond
	for _, tt := range []struct {
		name          string
		args          []string
		phase         string
		mutation      ReviewMutationOutcome
		replayability reviewtransaction.Replayability
		nextAction    string
		lineage       string
	}{
		{
			name: "read only", args: []string{"status", "--contract", ReviewIntegrationContractV1},
			phase: "pre_native", mutation: ReviewMutationNotStarted,
			replayability: reviewtransaction.ReplayabilityManualActionRequired, nextAction: "stop",
		},
		{
			name: "mutating", args: []string{"finalize", "--contract", ReviewIntegrationContractV1, "--lineage", "review-timeout"},
			phase: "native_running", mutation: ReviewMutationUnknown,
			replayability: reviewtransaction.ReplayabilityStatusRequired, nextAction: "review.status", lineage: "review-timeout",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviewFacadeCommandRunner = func(context.Context, []string, io.Writer) error { time.Sleep(100 * time.Millisecond); return nil }
			started := time.Now()
			var output bytes.Buffer
			err := RunReview(tt.args, &output)
			if elapsed := time.Since(started); elapsed > 75*time.Millisecond {
				t.Fatalf("aggregate facade timeout took %s", elapsed)
			}
			if err == nil {
				t.Fatal("timed out operation succeeded")
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.Code != "operation_timeout" || failure.Phase != tt.phase || failure.MutationOutcome != tt.mutation ||
				failure.RetrySafe || failure.Replayability != tt.replayability || failure.NextAction != tt.nextAction || failure.LineageID != tt.lineage {
				t.Fatalf("timeout failure = %#v", failure)
			}
		})
	}
}

func TestNegotiatedBindSDDTimeoutWaitsForPublicationCompletion(t *testing.T) {
	originalRunner, originalTimeout := reviewFacadeCommandRunner, reviewFacadeOperationTimeout
	t.Cleanup(func() { reviewFacadeCommandRunner, reviewFacadeOperationTimeout = originalRunner, originalTimeout })
	reviewFacadeOperationTimeout = 10 * time.Millisecond
	blocked, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	reviewFacadeCommandRunner = func(ctx context.Context, _ []string, stdout io.Writer) error {
		<-ctx.Done()
		close(blocked)
		<-release
		_, err := io.WriteString(stdout, "binding published\n")
		return err
	}
	var output bytes.Buffer
	completed := make(chan error, 1)
	go func() {
		completed <- RunReview([]string{"bind-sdd", "--contract", ReviewIntegrationContractV1}, &output)
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("BIND-SDD worker did not observe timeout")
	}
	select {
	case err := <-completed:
		t.Fatalf("BIND-SDD returned before publication completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	released = true
	select {
	case err := <-completed:
		if err != nil || output.String() != "binding published\n" {
			t.Fatalf("BIND-SDD completion = %v, %q", err, output.String())
		}
	case <-time.After(time.Second):
		t.Fatal("BIND-SDD did not return after publication completed")
	}
}

func TestNegotiatedBindSDDTimeoutBeforePublicationIsRetryable(t *testing.T) {
	originalRunner, originalTimeout := reviewFacadeCommandRunner, reviewFacadeOperationTimeout
	t.Cleanup(func() { reviewFacadeCommandRunner, reviewFacadeOperationTimeout = originalRunner, originalTimeout })
	reviewFacadeOperationTimeout = 10 * time.Millisecond
	reviewFacadeCommandRunner = func(ctx context.Context, _ []string, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	}
	var output bytes.Buffer
	err := RunReview([]string{"bind-sdd", "--contract", ReviewIntegrationContractV1, "--lineage", "review-bind-timeout"}, &output)
	if err == nil {
		t.Fatal("timed out BIND-SDD succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != ReviewIntegrationOperationBindSDD || failure.Code != "operation_timeout" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		failure.NextAction != "retry" || failure.LineageID != "review-bind-timeout" {
		t.Fatalf("BIND-SDD timeout failure = %#v", failure)
	}
}

func TestNegotiatedGitFailuresAreTypedNonAmplifyingAndPreMutation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		err       error
		code      string
		causeText string
	}{
		{name: "timeout", err: &reviewtransaction.GitCommandTimeoutError{Timeout: 15 * time.Second}, code: "git_command_timeout"},
		{name: "exit", err: &reviewtransaction.GitCommandError{ExitCode: 128}, code: "git_command_failed"},
		{
			name: "process control",
			err: &reviewtransaction.GitProcessControlError{
				Args: []string{"read-tree", "--empty"}, Cause: errors.New("job object assignment denied (0xC0000022)"),
			},
			code:      "git_command_failed",
			causeText: "job object assignment denied (0xC0000022)",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure("review.start", []string{"--lineage", "review-git-boundary"}, tt.err)
			if failure.Code != tt.code || failure.Phase != "pre_native" || failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "stop" {
				t.Fatalf("git failure = %#v", failure)
			}
			if tt.causeText != "" && !strings.Contains(failure.Message, tt.causeText) {
				t.Fatalf("git failure message masks cause: %q", failure.Message)
			}
		})
	}
}

func TestNegotiatedStatusProcessControlFailureIsTypedAndDiagnosable(t *testing.T) {
	originalRunner := reviewFacadeCommandRunner
	t.Cleanup(func() { reviewFacadeCommandRunner = originalRunner })
	reviewFacadeCommandRunner = func(context.Context, []string, io.Writer) error {
		return fmt.Errorf("inventory review authority: %w", &reviewtransaction.GitProcessControlError{
			Args: []string{"status", "--porcelain=v2"}, Cause: errors.New("NtResumeProcess status 0xC0000022"),
		})
	}
	var output bytes.Buffer
	err := RunReview([]string{"status", "--contract", ReviewIntegrationContractV1}, &output)
	if err == nil {
		t.Fatal("negotiated status with process-control failure succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != "review.status" || failure.Code != "git_command_failed" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "stop" {
		t.Fatalf("negotiated status process-control failure = %#v", failure)
	}
	if !strings.Contains(failure.Message, "NtResumeProcess status 0xC0000022") {
		t.Fatalf("process-control envelope masks cause: %q", failure.Message)
	}
}

func TestNegotiatedReadOnlyCatchAllStaysContentFreeAndNeverAbsorbsProcessControl(t *testing.T) {
	leaky := fmt.Errorf("assess negotiated review target: %w",
		errors.New("open /home/user/.git/review-authority/receipt.json: permission denied"))
	failure := newReviewIntegrationFailure("review.status", nil, leaky)
	if failure.Code != "operation_failed" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityNotReplayable || failure.NextAction != "retry" {
		t.Fatalf("read-only catch-all failure = %#v", failure)
	}
	if failure.Message != "The negotiated read-only review operation failed safely." {
		t.Fatalf("read-only catch-all message is not content-free: %q", failure.Message)
	}

	control := fmt.Errorf("inventory review authority: %w", &reviewtransaction.GitProcessControlError{
		Args: []string{"status", "--porcelain=v2"}, Cause: errors.New("NtResumeProcess status 0xC0000022"),
	})
	typed := newReviewIntegrationFailure("review.status", nil, control)
	if typed.Code != "git_command_failed" || typed.Phase != "pre_native" || typed.RetrySafe ||
		typed.Replayability != reviewtransaction.ReplayabilityManualActionRequired || typed.NextAction != "stop" {
		t.Fatalf("process-control failure reached the catch-all: %#v", typed)
	}
}

func TestNegotiatedFinalizePostTransitionGitTimeoutRequiresStatus(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	if len(started.SelectedLenses) != 1 {
		t.Fatalf("post-transition fixture lenses = %v", started.SelectedLenses)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Lens: started.SelectedLenses[0],
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"focused differential test failed on candidate"},
	})
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression test passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})

	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	helperDir := t.TempDir()
	helperPath := filepath.Join(helperDir, filepath.Base(realGit))
	copyReviewGitHelperExecutable(t, helperPath)
	oldPath := os.Getenv("PATH")
	t.Setenv(reviewGitHelperModeEnv, "1")
	t.Setenv(reviewGitHelperRealGitEnv, realGit)
	t.Setenv(reviewGitHelperStatePathEnv, store.StatePath())
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })
	oldTimeout := reviewFacadeOperationTimeout
	reviewFacadeOperationTimeout = time.Second
	t.Cleanup(func() { reviewFacadeOperationTimeout = oldTimeout })
	oldTransitionHook := reviewFacadeCommittedTransitionHook
	reviewFacadeCommittedTransitionHook = func(ctx context.Context, hookRepo, operation, _ string) error {
		if operation != "review/begin-fix" {
			return nil
		}
		if err := os.Setenv("PATH", helperDir+string(os.PathListSeparator)+oldPath); err != nil {
			return err
		}
		defer func() { _ = os.Setenv("PATH", oldPath) }()
		_, err := (reviewtransaction.SnapshotBuilder{Repo: hookRepo}).HasDirtyTrackedChanges(ctx)
		return err
	}
	t.Cleanup(func() { reviewFacadeCommittedTransitionHook = oldTransitionHook })

	tracePath := filepath.Join(t.TempDir(), "finalize-trace.jsonl")
	args := []string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
		"--result", resultPath, "--correction-lines", "2", "--validation", validationPath, "--trace", tracePath,
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err == nil {
		t.Fatal("post-transition Git stall succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "git_command_timeout" || failure.Phase != "native_committed" || failure.MutationOutcome != ReviewMutationUnknown ||
		failure.AuthorityApplicability != "current_target" || failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired ||
		failure.NextAction != "review.status" || failure.LineageID != started.LineageID {
		t.Fatalf("post-transition Git timeout failure = %#v", failure)
	}
	committed, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if committed.Revision == initial.Revision || committed.State.State != reviewtransaction.StateCorrectionRequired ||
		committed.State.ProposedCorrectionLines == nil || *committed.State.ProposedCorrectionLines != 2 ||
		committed.State.CorrectionBudget != initial.State.CorrectionBudget || len(committed.State.CorrectionAttempts) != 0 {
		t.Fatalf("post-transition authority = %#v", committed)
	}
	traceBeforeReplay, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(traceBeforeReplay), `"operation":"review/complete-review"`) != 1 ||
		strings.Count(string(traceBeforeReplay), `"operation":"review/begin-fix"`) != 1 {
		t.Fatalf("post-transition trace = %s", traceBeforeReplay)
	}

	if err := os.Setenv("PATH", oldPath); err != nil {
		t.Fatal(err)
	}
	reviewFacadeOperationTimeout = oldTimeout
	reviewFacadeCommittedTransitionHook = oldTransitionHook
	output.Reset()
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatalf("status after post-transition timeout: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
		status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateCorrectionRequired ||
		status.Frozen == nil || status.Frozen.CorrectionBudget != initial.State.CorrectionBudget {
		t.Fatalf("status after post-transition timeout = %#v", status)
	}

	output.Reset()
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("exact replay after committed begin-fix: %v\n%s", err, output.String())
	}
	replayed, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Revision != committed.Revision || replayed.State.CorrectionBudget != committed.State.CorrectionBudget ||
		replayed.State.ProposedCorrectionLines == nil || *replayed.State.ProposedCorrectionLines != 2 || len(replayed.State.CorrectionAttempts) != 0 {
		t.Fatalf("exact replay changed the committed correction budget: before=%#v after=%#v", committed, replayed)
	}
	traceAfterReplay, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(traceAfterReplay, traceBeforeReplay) {
		t.Fatalf("exact replay duplicated a committed transition: before=%s after=%s", traceBeforeReplay, traceAfterReplay)
	}
	if pending, err := store.PendingFinalizeAttempt(); err != nil || pending != nil {
		t.Fatalf("exact replay left pending finalize attempt: %#v, %v", pending, err)
	}
}

const (
	reviewGitHelperModeEnv      = "GENTLE_AI_REVIEW_GIT_HELPER"
	reviewGitHelperRealGitEnv   = "GENTLE_AI_REVIEW_GIT_HELPER_REAL"
	reviewGitHelperStatePathEnv = "GENTLE_AI_REVIEW_GIT_HELPER_STATE"
)

func reviewGitProcessHelperExitCode() (int, bool) {
	if os.Getenv(reviewGitHelperModeEnv) != "1" {
		return 0, false
	}
	if payload, err := os.ReadFile(os.Getenv(reviewGitHelperStatePathEnv)); err == nil && strings.Contains(string(payload), `"proposed_correction_lines":`) {
		time.Sleep(10 * time.Second)
		return 0, true
	}
	command := exec.Command(os.Getenv(reviewGitHelperRealGitEnv), os.Args[1:]...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), true
		}
		return 1, true
	}
	return 0, true
}

func copyReviewGitHelperExecutable(t *testing.T, destination string) {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, payload, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestNegotiatedLegacyReadOnlyFailurePreservesTypedCauseAcrossMutationRoutes(t *testing.T) {
	tests := []struct {
		name              string
		contractOperation string
		legacyOperation   string
	}{
		{name: "start collision", contractOperation: "review.start", legacyOperation: "review/start"},
		{name: "finalize", contractOperation: "review.finalize", legacyOperation: "review/finalize"},
		{name: "review step", contractOperation: "review.finalize", legacyOperation: "review/freeze-findings"},
		{name: "invalidate", contractOperation: "review.finalize", legacyOperation: "review/invalidate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "legacy-negotiated-" + strings.ReplaceAll(tt.name, " ", "-")
			typed := reviewtransaction.NewLegacyReadOnlyError(tt.legacyOperation, lineage)
			secret := "/tmp/private-authority token=secret"
			runErr := fmt.Errorf("%s: %w", secret, typed)
			failure := newReviewIntegrationFailure(tt.contractOperation, []string{"--lineage", lineage}, runErr)
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
			if failure.Code != reviewtransaction.LegacyReadOnlyErrorCode || failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
				failure.NextAction != "stop" || strings.Contains(failure.Message, secret) || strings.Contains(failure.Message, "/tmp/") {
				t.Fatalf("legacy negotiated failure = %#v", failure)
			}
			publicErr := newReviewIntegrationFailureError(failure, runErr)
			var preserved *reviewtransaction.LegacyReadOnlyError
			if !errors.Is(publicErr, reviewtransaction.ErrLegacyReadOnly) || !errors.As(publicErr, &preserved) || preserved != typed {
				t.Fatalf("negotiated wrapper lost typed legacy cause: %#v", publicErr)
			}
		})
	}
}

func TestNegotiatedGateDenialUsesFailureEnvelopeWithoutAuthorityDrift(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeNegotiatedOperationChange(t, repo, "thin")
	lineage := "review-failure-gate"
	_, store := finalizeNegotiatedOperationFixture(t, repo, lineage, true)
	var err error
	beforeAuthority := readReviewOperationFile(t, store.StatePath())
	beforeReceipt := readReviewOperationFile(t, store.ReceiptPath())
	if err := os.WriteFile(filepath.Join(repo, "openspec", "changes", "thin", "proposal.md"), []byte("# Drifted proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		t.Fatal("drifted target passed negotiated validation")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "gate_scope_changed" || failure.MutationOutcome != ReviewMutationNotStarted ||
		failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
		failure.NextAction != "explicit-maintainer-action" {
		t.Fatalf("gate failure = %#v", failure)
	}
	assertScopeChangeRecovery(t, failure, lineage, "openspec/changes/thin/proposal.md")
	if !bytes.Equal(beforeAuthority, readReviewOperationFile(t, store.StatePath())) ||
		!bytes.Equal(beforeReceipt, readReviewOperationFile(t, store.ReceiptPath())) {
		t.Fatal("negotiated gate denial changed authority or receipt bytes")
	}
}

func TestNegotiatedReceiptPublicationFailureIsSanitizedAndExactlyReplayable(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeNegotiatedOperationChange(t, repo, "thin")
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	finalizeArgs := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, started)...)
	if err := RunReviewFacadeFinalize(finalizeArgs, io.Discard); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := writeCompactFacadeReceipt
	secret := "raw provider stderr token=secret /tmp/authority.lock"
	writeCompactFacadeReceipt = func(string, reviewtransaction.CompactReceipt) error { return errors.New(secret) }
	var output bytes.Buffer
	err = RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", started.LineageID, "--evidence", evidence,
	}, &output)
	writeCompactFacadeReceipt = original
	if err == nil {
		t.Fatal("receipt publication interruption succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "receipt_publication_pending" || failure.MutationOutcome != ReviewMutationCommitted ||
		failure.Replayability != reviewtransaction.ReplayabilityExactReplaySafe || failure.RetrySafe ||
		failure.LineageID != started.LineageID || !strings.HasPrefix(failure.RequestDigest, "sha256:") ||
		failure.NextAction != "review.finalize" {
		t.Fatalf("receipt failure = %#v", failure)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(err.Error(), secret) ||
		strings.Contains(output.String(), "/tmp/") || strings.Contains(output.String(), "token=secret") {
		t.Fatalf("negotiated failure leaked private diagnostics: output=%s error=%v", output.String(), err)
	}
	pending, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	pendingAuthority := readReviewOperationFile(t, store.StatePath())
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("failed publication materialized receipt: %v", err)
	}

	output.Reset()
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatalf("exact negotiated receipt replay: %v\n%s", err, output.String())
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != pending.Revision || !bytes.Equal(pendingAuthority, readReviewOperationFile(t, store.StatePath())) {
		t.Fatal("exact receipt replay changed authority identity or bytes")
	}
	if _, err := os.Stat(store.ReceiptPath()); err != nil {
		t.Fatalf("exact receipt replay did not publish receipt: %v", err)
	}
}

func TestReviewIntegrationFailureSchemaAndFixtureAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "failure.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaPayload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != ReviewIntegrationFailureSchemaID || schema["additionalProperties"] != false {
		t.Fatalf("failure schema header = %#v", schema)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "failure.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	failure := decodeReviewIntegrationFailure(t, fixture)
	if failure.Code != "gate_scope_changed" || failure.Context == nil || failure.Context.ScopeChange == nil {
		t.Fatalf("failure fixture = %#v", failure)
	}
	failure.Context.ScopeChange.Expected.PathsDigest = "invalid"
	if err := failure.Validate(); err == nil {
		t.Fatal("failure validation accepted malformed scope-change evidence")
	}
	var raw map[string]any
	if err := json.Unmarshal(fixture, &raw); err != nil {
		t.Fatal(err)
	}
	contextObject := raw["context"].(map[string]any)
	contextObject["scope_change"].(map[string]any)["paths"] = []string{"private/path"}
	malformed, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(malformed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ReviewIntegrationFailure{}); err == nil {
		t.Fatal("strict failure decoder accepted a private scope-change field")
	}
}

func decodeReviewIntegrationFailure(t *testing.T, payload []byte) ReviewIntegrationFailure {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var failure ReviewIntegrationFailure
	if err := decoder.Decode(&failure); err != nil {
		t.Fatalf("decode failure envelope %q: %v", payload, err)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("validate failure envelope: %v\n%s", err, payload)
	}
	return failure
}
