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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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
		{name: "validate", args: []string{"validate", "--contract", ReviewIntegrationContractV1, "unexpected"}, operation: "review.validate"},
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
		{name: "capabilities unsupported", args: []string{"capabilities", "--contract", "gentle-ai.review-integration/v3"}, code: "unsupported_contract"},
		{name: "start empty", args: []string{"start", "--contract="}, code: "empty_contract"},
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

func TestNegotiatedReviewV2FailureUsesSuccessorEnvelope(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"start", "--contract", ReviewIntegrationContractV2, "unexpected"}, &output)
	if err == nil {
		t.Fatal("invalid v2 request succeeded")
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Schema != ReviewIntegrationFailureSchemaV2 || failure.Contract != ReviewIntegrationContractV2 || failure.Code != "invalid_request" {
		t.Fatalf("v2 failure = %#v", failure)
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
		{name: "legacy read only", runErr: reviewtransaction.NewLegacyReadOnlyError("review/start", lineage), wantCode: reviewtransaction.LegacyReadOnlyErrorCode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure(
				"review.start",
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
	_, negotiated, routed := reviewIntegrationFailureRoute([]string{
		"start", "--contract=", "--lineage", lineage,
	})
	if !negotiated || routed == nil || routed.LineageID != lineage || routed.Code != "empty_contract" {
		t.Fatalf("routed preflight failure = %#v, negotiated %v", routed, negotiated)
	}
}

func TestRetiredBindSDDDoesNotProduceANegotiatedFailure(t *testing.T) {
	var output bytes.Buffer
	err := RunReview([]string{"bind-sdd", "--contract", ReviewIntegrationContractV1, "--cwd", t.TempDir()}, &output)
	if err == nil || !strings.Contains(err.Error(), "unknown review command") {
		t.Fatalf("retired bind-sdd error = %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("retired bind-sdd emitted a negotiated envelope: %q", output.String())
	}
}

func TestNegotiatedFailureLineageUsesCanonicalFlagParsing(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		args      []string
		want      string
	}{
		{name: "equals", operation: "review.start", args: []string{"--lineage=review-equals"}, want: "review-equals"},
		{name: "split", operation: "review.start", args: []string{"--lineage", "review-split"}, want: "review-split"},
		{name: "boolean before lineage", operation: "review.start", args: []string{"--committed-only", "--lineage", "review-after-boolean"}, want: "review-after-boolean"},
		{name: "other flag owns lineage-shaped value", operation: "review.start", args: []string{"--cwd", "--lineage=not-a-lineage"}},
		{name: "double dash stops parsing", operation: "review.start", args: []string{"--", "--lineage=review-after-stop"}},
		{name: "positional stops parsing", operation: "review.start", args: []string{"unexpected", "--lineage=review-after-positional"}},
		{name: "duplicate last wins", operation: "review.start", args: []string{"--lineage", "review-first", "--lineage=review-second"}, want: "review-second"},
		{name: "duplicate malformed last clears", operation: "review.start", args: []string{"--lineage", "review-first", "--lineage=-invalid"}},
		{name: "unknown flag fails closed", operation: "review.start", args: []string{"--unknown", "value", "--lineage", "review-hidden"}},
		{name: "unknown flag after lineage fails closed", operation: "review.start", args: []string{"--lineage", "review-hidden", "--unknown", "value"}},
		{name: "over maximum length", operation: "review.start", args: []string{"--lineage", "r" + strings.Repeat("a", 128)}},
		{name: "start boolean before lineage", operation: "review.start", args: []string{"--committed-only", "--lineage", "review-start"}, want: "review-start"},
		{name: "repair boolean before lineage", operation: "review.repair", args: []string{"--preflight=false", "--lineage", "review-repair"}, want: "review-repair"},
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

func TestReviewIntegrationOperationRegistryOwnsPublishedAndFailurePolicy(t *testing.T) {
	wantOperations := []string{
		"review.capabilities", "review.repair", "review.start", "review.status", "review.validate",
	}
	if got := reviewIntegrationOperationNames(); !reflect.DeepEqual(got, wantOperations) ||
		!reflect.DeepEqual(reviewCapabilitiesStaticSurface().Operations, wantOperations) {
		t.Fatalf("operation registry surface = %v", got)
	}
	commands := map[string]struct{}{}
	operations := map[string]struct{}{}
	for _, metadata := range reviewIntegrationOperationRegistry {
		if metadata.Command == "" || metadata.Operation == "" || metadata.Label == "" {
			t.Fatalf("incomplete operation metadata: %#v", metadata)
		}
		if _, duplicate := commands[metadata.Command]; duplicate {
			t.Fatalf("duplicate operation command %q", metadata.Command)
		}
		if _, duplicate := operations[metadata.Operation]; duplicate {
			t.Fatalf("duplicate operation name %q", metadata.Operation)
		}
		commands[metadata.Command], operations[metadata.Operation] = struct{}{}, struct{}{}
		byName, nameOK := reviewIntegrationOperationByName(metadata.Operation)
		if !nameOK || byName.Command != metadata.Command || reviewLockOperationLabel(metadata.Operation) != metadata.Label {
			t.Fatalf("operation metadata does not drive every policy lookup: %#v", metadata)
		}
		byCommand, commandOK := reviewIntegrationOperationByCommand(metadata.Command)
		if metadata.Negotiated && metadata.CollectCapture {
			t.Fatalf("operation %q claims both the negotiated and collect-capture row classes", metadata.Operation)
		}
		if metadata.CollectCapture {
			// A collect-capture row owns a plain-dispatched collect-satisfying
			// capture verb. It stays off the negotiated command route and out
			// of the published capabilities `operations` array (a pinned-length
			// contract), but it DOES join the v2 failure envelope's operation
			// vocabulary: its refusals emit typed envelopes on stdout, and its
			// flag metadata is consumed by safe lineage extraction while
			// MutatesAuthority is consumed by timeout classification.
			if commandOK {
				t.Fatalf("collect-capture operation %q is routed as a negotiated command", metadata.Operation)
			}
			if !validReviewIntegrationFailureOperation(metadata.Operation) {
				t.Fatalf("collect-capture operation %q is missing from the failure operation vocabulary", metadata.Operation)
			}
			if capture, ok := reviewCollectCaptureOperationByCommand(metadata.Command); !ok || capture.Operation != metadata.Operation {
				t.Fatalf("collect-capture operation %q does not resolve its own envelope route", metadata.Operation)
			}
			if len(metadata.ValueFlags) == 0 || !metadata.MutatesAuthority {
				t.Fatalf("collect-capture operation %q is missing the consumed flag or mutation metadata: %#v", metadata.Operation, metadata)
			}
			if metadata.JoinOnTimeout || metadata.TimeoutRetryable {
				t.Fatalf("collect-capture operation %q carries negotiated timeout metadata nothing consumes: %#v", metadata.Operation, metadata)
			}
			continue
		}
		if !metadata.Negotiated {
			// A non-negotiated row exists for exactly one reason: to own the
			// runnable CLI verb for an operation the status schemas publish as
			// an execute transition. It must stay out of every negotiated
			// surface -- the capabilities `operations` array and the failure
			// envelope's `operation` enum are both published contracts with a
			// closed vocabulary and a pinned length -- and it must not carry
			// negotiated flag metadata nothing consumes and nothing verifies,
			// because unverified metadata rots into a confident lie.
			if commandOK {
				t.Fatalf("non-negotiated operation %q is routed as a negotiated command", metadata.Operation)
			}
			if validReviewIntegrationFailureOperation(metadata.Operation) {
				t.Fatalf("non-negotiated operation %q widened the published failure operation enum", metadata.Operation)
			}
			if len(metadata.ValueFlags) != 0 || len(metadata.BoolFlags) != 0 || len(metadata.IntFlags) != 0 ||
				metadata.MutatesAuthority || metadata.JoinOnTimeout || metadata.TimeoutRetryable || metadata.ReadOnlyFlag != "" {
				t.Fatalf("non-negotiated operation %q carries negotiated policy metadata nothing consumes: %#v", metadata.Operation, metadata)
			}
			continue
		}
		if !commandOK || byCommand.Operation != metadata.Operation ||
			!validReviewIntegrationFailureOperation(metadata.Operation) {
			t.Fatalf("operation metadata does not drive every policy lookup: %#v", metadata)
		}
		shape := reviewIntegrationOperationFlagShape(metadata.Operation)
		if shape["contract"] != reviewIntegrationValueFlag {
			t.Fatalf("operation %q omitted contract flag metadata", metadata.Operation)
		}
	}
	repair, ok := reviewIntegrationOperationByName("review.repair")
	if !ok || !repair.MutatesAuthority || !repair.JoinOnTimeout || repair.ReadOnlyFlag != "preflight" ||
		reviewIntegrationOperationMutates(repair, []string{"--preflight=true"}) ||
		!reviewIntegrationOperationMutates(repair, []string{"--preflight=false"}) {
		t.Fatalf("repair operation metadata = %#v", repair)
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

func TestNegotiatedStatusLockFailuresAreTypedOperationSpecificAndPathFree(t *testing.T) {
	privatePath := filepath.Join(t.TempDir(), "REVIEW-MAINTENANCE.lock")
	for _, tt := range []struct {
		name    string
		err     error
		code    string
		message string
		retry   bool
		next    string
	}{
		{
			name: "timeout", err: fmt.Errorf("acquire %s: %w", privatePath, &reviewtransaction.AuthorityLockTimeoutError{Timeout: 2 * time.Second}),
			code: "authority_lock_timeout", message: "Review STATUS could not acquire the authority lock within the bounded wait.",
			retry: true, next: "retry_with_bounded_backoff",
		},
		{
			name: "cancelled", err: fmt.Errorf("acquire %s: %w", privatePath, &reviewtransaction.AuthorityLockCancelledError{Cause: context.Canceled}),
			code: "authority_lock_cancelled", message: "Review STATUS authority lock acquisition was cancelled.", next: "stop",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failure := newReviewIntegrationFailure("review.status", nil, tt.err)
			if failure.Code != tt.code || failure.Message != tt.message || failure.Phase != "pre_native" ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.RetrySafe != tt.retry ||
				failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != tt.next {
				t.Fatalf("STATUS lock failure = %#v", failure)
			}
			payload, err := json.Marshal(failure)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(payload, []byte(privatePath)) || bytes.Contains(payload, []byte("Review START")) {
				t.Fatalf("STATUS lock failure leaked private or wrong-operation detail: %s", payload)
			}
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNegotiatedStatusUsesRealMaintenanceLockTruth(t *testing.T) {
	repo := initReviewCLIRepo(t)
	started := startFacadeReview(t, repo)
	lockContext, cancelLock := context.WithTimeout(context.Background(), time.Second)
	defer cancelLock()
	exclusive, err := reviewtransaction.AcquireReviewMaintenanceExclusive(lockContext, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer exclusive.Release()

	originalTimeout := reviewFacadeOperationTimeout
	t.Cleanup(func() { reviewFacadeOperationTimeout = originalTimeout })
	args := []string{
		"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", started.LineageID,
	}

	reviewFacadeOperationTimeout = 3 * time.Second
	var timeoutOutput bytes.Buffer
	err = RunReview(args, &timeoutOutput)
	if err == nil {
		t.Fatal("STATUS succeeded while exclusive maintenance held")
	}
	timeout := decodeReviewIntegrationFailure(t, timeoutOutput.Bytes())
	if timeout.Code != "authority_lock_timeout" || timeout.MutationOutcome != ReviewMutationNotStarted ||
		!timeout.RetrySafe || timeout.NextAction != "retry_with_bounded_backoff" {
		t.Fatalf("real STATUS lock timeout = %#v", timeout)
	}
	if strings.Contains(timeoutOutput.String(), repo) {
		t.Fatalf("real STATUS lock timeout exposed repository path: %s", timeoutOutput.String())
	}
	assertNoPrivateReviewOperationFields(t, timeoutOutput.Bytes())

	reviewFacadeOperationTimeout = 50 * time.Millisecond
	var deadlineOutput bytes.Buffer
	err = RunReview(args, &deadlineOutput)
	if err == nil {
		t.Fatal("STATUS caller deadline succeeded while exclusive maintenance held")
	}
	deadline := decodeReviewIntegrationFailure(t, deadlineOutput.Bytes())
	if deadline.Code != "operation_timeout" || deadline.MutationOutcome != ReviewMutationNotStarted ||
		deadline.RetrySafe || deadline.NextAction != "stop" {
		t.Fatalf("STATUS caller deadline = %#v", deadline)
	}
	if strings.Contains(deadlineOutput.String(), repo) {
		t.Fatalf("STATUS caller deadline exposed repository path: %s", deadlineOutput.String())
	}
	assertNoPrivateReviewOperationFields(t, deadlineOutput.Bytes())
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
		name           string
		args           []string
		phase          string
		mutation       ReviewMutationOutcome
		replayability  reviewtransaction.Replayability
		nextAction     string
		lineage        string
		waitsForWorker bool
	}{
		{
			name: "read only", args: []string{"status", "--contract", ReviewIntegrationContractV1},
			phase: "pre_native", mutation: ReviewMutationNotStarted,
			replayability: reviewtransaction.ReplayabilityManualActionRequired, nextAction: "stop",
		},
		{
			name: "repair execution", args: []string{"repair", "--contract", ReviewIntegrationContractV1, "--preflight=false", "--lineage", "repair-timeout"},
			phase: "native_running", mutation: ReviewMutationUnknown,
			replayability: reviewtransaction.ReplayabilityStatusRequired, nextAction: "review.status", lineage: "repair-timeout", waitsForWorker: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviewFacadeCommandRunner = func(context.Context, []string, io.Writer) error { time.Sleep(100 * time.Millisecond); return nil }
			started := time.Now()
			var output bytes.Buffer
			err := RunReview(tt.args, &output)
			if elapsed := time.Since(started); tt.waitsForWorker && elapsed < 75*time.Millisecond {
				t.Fatalf("aggregate facade timeout returned before mutating worker joined: %s", elapsed)
			} else if !tt.waitsForWorker && elapsed > 75*time.Millisecond {
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

func TestNegotiatedRepairProgressFailurePreservesOpaqueReplayTruth(t *testing.T) {
	base := reviewtransaction.ClassifiedAuthorityRepairExecution{
		Class:     reviewtransaction.AuthorityRepairClassLegacyV1HistoricalAlias,
		LineageID: "repair-progress", Revision: "sha256:" + strings.Repeat("a", 64),
		ChainIdentity:    "sha256:" + strings.Repeat("b", 64),
		Cause:            reviewtransaction.AuthorityRepairCauseUnsupportedHistoricalV1OperationAlias,
		Disposition:      reviewtransaction.AuthorityRepairDispositionQuarantineHistoricalAlias,
		AssessmentDigest: "sha256:" + strings.Repeat("c", 64), RequestDigest: "sha256:" + strings.Repeat("d", 64),
		RecordIdentity: "sha256:" + strings.Repeat("e", 64),
	}
	for _, tt := range []struct {
		name          string
		status        string
		phase         string
		mutation      ReviewMutationOutcome
		replayability reviewtransaction.Replayability
		nextAction    string
		exactReplay   bool
	}{
		{name: "prepared", status: reviewtransaction.CompactReclaimPrepared, phase: "native_running", mutation: ReviewMutationUnknown, replayability: reviewtransaction.ReplayabilityExactReplaySafe, nextAction: "review.repair", exactReplay: true},
		{name: "committed verified", status: reviewtransaction.CompactReclaimCommitted, phase: "native_committed", mutation: ReviewMutationCommitted, replayability: reviewtransaction.ReplayabilityExactReplaySafe, nextAction: "review.repair", exactReplay: true},
		{name: "committed unverified", status: reviewtransaction.CompactReclaimCommitted, phase: "native_committed", mutation: ReviewMutationCommitted, replayability: reviewtransaction.ReplayabilityStatusRequired, nextAction: "review.status"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			progress := base
			progress.Status = tt.status
			failure := newReviewIntegrationFailure("review.repair", []string{"--lineage", progress.LineageID}, &reviewtransaction.ClassifiedAuthorityRepairProgressError{
				Progress: progress, ExactReplaySafe: tt.exactReplay, Cause: context.DeadlineExceeded,
			})
			if failure.Code != "operation_timeout" || failure.Phase != tt.phase || failure.MutationOutcome != tt.mutation ||
				failure.Replayability != tt.replayability || failure.NextAction != tt.nextAction || failure.RetrySafe != tt.exactReplay ||
				failure.LineageID != progress.LineageID || failure.RequestDigest != progress.RequestDigest || failure.ProgressIdentity != progress.RecordIdentity {
				t.Fatalf("%s repair progress failure = %#v", tt.name, failure)
			}
			if err := failure.Validate(); err != nil {
				t.Fatal(err)
			}
			withoutProgress := failure
			withoutProgress.ProgressIdentity = ""
			if err := withoutProgress.Validate(); err == nil {
				t.Fatal("repair progress failure accepted a request digest without its progress identity")
			}
		})
	}
}

func TestNegotiatedGitFailuresAreTypedNonAmplifyingAndPreMutation(t *testing.T) {
	for _, tt := range []struct {
		name      string
		err       error
		code      string
		causeText string
	}{
		{name: "timeout", err: &reviewtransaction.GitCommandTimeoutError{Timeout: 15 * time.Second}, code: "git_command_timeout", causeText: "15s"},
		{name: "exit", err: &reviewtransaction.GitCommandError{Args: []string{"write-tree"}, ExitCode: 128, Output: "fatal: not a git repository"}, code: "git_command_failed", causeText: "git write-tree failed with exit code 128: fatal: not a git repository"},
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
			if tt.causeText != "" && !strings.Contains(failure.Cause, tt.causeText) {
				t.Fatalf("git failure cause field missing diagnostics %q: %q", tt.causeText, failure.Cause)
			}
			if tt.code == "git_command_failed" && tt.name == "process control" && !strings.Contains(failure.Message, tt.causeText) {
				// Process control includes causeText in Message for immediate diagnosis
				t.Fatalf("git failure message masks cause: %q", failure.Message)
			}
		})
	}
}

// TestNegotiatedStoreLockPreAcquisitionFailureIsNotStarted proves 1781: a
// failure that happens before the authoritative store lock is acquired must
// classify as not-started, exactly like the pre-mutation Git failure family,
// instead of falling through to the generic unknown-mutation catch-all.
func TestNegotiatedStoreLockPreAcquisitionFailureIsNotStarted(t *testing.T) {
	err := &reviewtransaction.StoreLockPreAcquisitionError{Err: errors.New("permission denied")}
	failure := newReviewIntegrationFailure("review.start", []string{"--lineage", "review-lock-boundary"}, err)
	// organic-dx Phase 3b task 3b.5: this is advisory lock contention, not
	// damage — the same wait-and-retry route authority_lock_timeout already
	// names, contrasted here since previously this refusal named nothing.
	if failure.Code != "store_lock_unavailable" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe ||
		failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired || failure.NextAction != "retry_with_bounded_backoff" {
		t.Fatalf("pre-lock-acquisition failure = %#v, want a not-started, retry-with-bounded-backoff classification", failure)
	}
	if !strings.Contains(failure.Message, "permission denied") {
		t.Fatalf("pre-lock-acquisition failure message masks cause: %q", failure.Message)
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

func TestNegotiatedReadOnlyCatchAllScrubsItsCauseAndNeverAbsorbsProcessControl(t *testing.T) {
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
	if strings.Contains(failure.Cause, "/home/user") || !strings.Contains(failure.Cause, "permission denied") {
		t.Fatalf("read-only catch-all cause is not the scrubbed native reason: %q", failure.Cause)
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

func TestNegotiatedLegacyReadOnlyFailurePreservesTypedStartCause(t *testing.T) {
	tests := []struct {
		name              string
		contractOperation string
		legacyOperation   string
	}{
		{name: "start collision", contractOperation: "review.start", legacyOperation: "review/start"},
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
			// organic-dx Phase 3b task 3b.4: the negotiated envelope now
			// carries the same "choose a new lineage for compact authority"
			// route the non-negotiated START collision already names
			// (review_facade.go:1080), instead of a bare stop.
			if failure.Code != reviewtransaction.LegacyReadOnlyErrorCode || failure.MutationOutcome != ReviewMutationNotStarted ||
				failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
				failure.NextAction != "review.start" || !strings.Contains(failure.Message, "choose a new lineage for compact authority") ||
				strings.Contains(failure.Message, secret) || strings.Contains(failure.Message, "/tmp/") {
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
	inputs := schemaStringArray(t, schema["properties"].(map[string]any)["required_inputs"].(map[string]any)["items"].(map[string]any)["enum"])
	if !containsString(inputs, "base_ref") {
		t.Fatalf("failure required_inputs vocabulary = %v", inputs)
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

func TestReviewIntegrationFailureMapsTargetResolutionBeforeGateDenial(t *testing.T) {
	targetErr := &reviewtransaction.GateTargetResolutionError{
		RequiredInput: "base_ref",
		Err:           errors.New("configured upstream is missing; pass --base-ref <remote>/<branch>"),
	}
	failure := newReviewIntegrationFailure(ReviewIntegrationOperationValidate, nil, ReviewGateDeniedError{
		Result: reviewtransaction.GateInvalidated,
		Cause:  targetErr,
	})
	if failure.Phase != "pre_native" || failure.Code != "target_resolution_failed" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" ||
		!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		!reflect.DeepEqual(failure.RequiredInputs, []string{"base_ref"}) || failure.NextAction != "correct_request" ||
		!strings.Contains(failure.Message, "--base-ref") {
		t.Fatalf("target-resolution failure mapping = %#v", failure)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("target-resolution failure validation = %v", err)
	}
}

// TestNewReviewIntegrationFailureCause is the RED-first proof for 1666/1807:
// the operation_outcome_unknown default envelope used to discard the real
// native cause behind a fixed placeholder message. Code, Message, and
// MutationOutcome stay byte-identical; the wrapped cause is now additive.
func TestNewReviewIntegrationFailureCause(t *testing.T) {
	runErr := errors.New("unexpected native repository state: dangling worktree lock file")
	failure := newReviewIntegrationFailure("review.start", nil, runErr)
	if failure.Code != "operation_outcome_unknown" || failure.Phase != "native_running" ||
		failure.MutationOutcome != ReviewMutationUnknown ||
		failure.Message != "The negotiated review operation failed without authoritative mutation evidence." {
		t.Fatalf("operation_outcome_unknown envelope changed byte-identical fields = %#v", failure)
	}
	if failure.Cause != runErr.Error() {
		t.Fatalf("failure.Cause = %q, want the wrapped native cause %q", failure.Cause, runErr.Error())
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("operation_outcome_unknown failure with cause validation = %v", err)
	}

	// The read-only catch-all keeps the canned message and carries the same
	// scrubbed cause (issues #2981 and #3379): a caller with no cause has
	// nothing to change and retries the same deterministic refusal forever.
	readOnly := newReviewIntegrationFailure("review.status", nil, runErr)
	if readOnly.Message != "The negotiated read-only review operation failed safely." || readOnly.Cause != runErr.Error() {
		t.Fatalf("read-only catch-all = %#v, want the canned message with the scrubbed cause", readOnly)
	}
}

func TestEscalatedRecoveryAuthorizationInexactUsesCurrentRepairRoute(t *testing.T) {
	tests := []struct {
		name           string
		repairable     bool
		wantNextAction string
		wantMessage    string
	}{
		{
			name:           "schema-prefixed content mismatch",
			repairable:     true,
			wantNextAction: "review.repair",
			wantMessage:    "run review repair",
		},
		{
			name:           "pre-contract authorization",
			repairable:     false,
			wantNextAction: "stop",
			wantMessage:    "no advertised repair operation",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runErr := fmt.Errorf("invalid compact authority graph: %w", &reviewtransaction.CompactRecoveryAuthorizationInexactError{
				Projection: reviewtransaction.ProjectionWorkspace, TargetIdentity: "sha256:" + strings.Repeat("a", 64), Repairable: tt.repairable,
			})
			failure := newReviewIntegrationFailure(
				"review.start",
				[]string{"--lineage", "review-authorization-route"},
				runErr,
			)
			if failure.Code != "escalated_recovery_authorization_inexact" || failure.Phase != "pre_native" ||
				failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "current_target" ||
				failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityManualActionRequired ||
				failure.NextAction != tt.wantNextAction || !strings.Contains(failure.Message, tt.wantMessage) ||
				failure.NextAction == "reconcile-authority" || failure.Cause == "" {
				t.Fatalf("authorization failure = %#v", failure)
			}
			if err := failure.Validate(); err != nil {
				t.Fatalf("authorization failure validation = %v", err)
			}
		})
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

// TestNewReviewIntegrationFailureCauseIsUniversal is root 8's structural fix
// (#2471): Cause used to be per-branch opt-in, and 17 of 25 return sites
// forgot it, so a caller read a constant Message while the native reason sat
// in the discarded typed error. Cause is now projected once at construction,
// so a branch cannot forget it; this test pins two branches that carried no
// cause before, plus the one branch whose contract REQUIRES staying
// content-free.
func TestNewReviewIntegrationFailureCauseIsUniversal(t *testing.T) {
	// A branch matched via errors.Is: contention keeps its typed code and now
	// carries the wrapped operational context too.
	contention := fmt.Errorf("acquire compact authority for lineage %q: %w", "cause-universal", reviewtransaction.ErrStoreLockContended)
	failure := newReviewIntegrationFailure("review.start", nil, contention)
	if failure.Code != "authority_lock_contention" {
		t.Fatalf("contention envelope code = %q", failure.Code)
	}
	if !strings.Contains(failure.Cause, "cause-universal") {
		t.Fatalf("contention envelope discarded the typed cause: %#v", failure)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("contention envelope with cause validation = %v", err)
	}

	// The legacy read-only refusal: constant Message, and before this change
	// no Cause at all, so the lineage the error names never reached the
	// caller's machine-readable envelope.
	legacy := &reviewtransaction.LegacyReadOnlyError{Operation: "review/start", LineageID: "cause-universal-legacy"}
	failure = newReviewIntegrationFailure("review.start", nil, legacy)
	if failure.Code != reviewtransaction.LegacyReadOnlyErrorCode {
		t.Fatalf("legacy envelope code = %q", failure.Code)
	}
	if !strings.Contains(failure.Cause, "cause-universal-legacy") {
		t.Fatalf("legacy envelope discarded the typed cause: %#v", failure)
	}

	// The read-only catch-all inherits the universal scrubbed cause too
	// (issues #2981 and #3379): a deterministic selector refusal with no cause
	// was retried forever.
	readOnly := newReviewIntegrationFailure("review.status", nil, errors.New("selector refused for this input"))
	if readOnly.Code != "operation_failed" || readOnly.Cause != "selector refused for this input" {
		t.Fatalf("read-only catch-all = %#v, want the scrubbed cause", readOnly)
	}
}

// Issues #2981 and #3379: the read-only catch-all cleared its cause, so an
// unclassified read-only failure was content-free. It keeps the scrubbed
// cause that says what to change; retry stays, because this branch is the
// residue of everything the typed classifier did not recognise.
func TestNegotiatedStatusPreNativeFailurePreservesScrubbedCause(t *testing.T) {
	failure := newReviewIntegrationFailure("review.status", nil, fmt.Errorf("freeze negotiated fresh review target: %w",
		errors.New("staged --workspace-overlay requires exactly --base-ref")))
	if failure.Code != "operation_failed" || failure.Phase != "pre_native" || failure.MutationOutcome != ReviewMutationNotStarted ||
		!failure.RetrySafe || failure.NextAction != "retry" || !strings.Contains(failure.Cause, "requires exactly --base-ref") {
		t.Fatalf("read-only failure = %#v, want retry with the cause", failure)
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("read-only catch-all with cause validation = %v", err)
	}
	scrubbed := newReviewIntegrationFailure("review.status", nil, errors.New("open /home/user/.git/receipt.json: permission denied"))
	if strings.Contains(scrubbed.Cause, "/home/user") || !strings.Contains(scrubbed.Cause, "permission denied") {
		t.Fatalf("read-only catch-all cause is not scrubbed: %q", scrubbed.Cause)
	}
}
