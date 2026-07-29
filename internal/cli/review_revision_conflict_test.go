package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestFinalizeRevisionConflictIsRetryableNotUnknown is the deterministic
// reproduction of the CI-only loser in TestConcurrentFinalizeElectsExactlyOneWriter.
//
// A concurrent FINALIZE has two distinct ways to lose. It can lose the advisory
// authority lock, which never runs the guarded body at all, and it can win the
// lock and lose the compare-and-set on the compact revision, because the
// expected revision is read long before the lock is taken. The second loser is
// the one CI produced, and folding it into operation_outcome_unknown throws away
// what the product already knows: the CAS is the gate in front of the only write
// in replaceContextGuarded, so a refused CAS proves the authority was not
// mutated by this operation.
//
// The window is driven directly here rather than by fanning out goroutines and
// hoping: reviewFacadePlannedTransitionHook runs immediately before the guarded
// commit, which is exactly the instant a competing writer has to advance the
// authority for this operation to lose its CAS.
func TestFinalizeRevisionConflictIsRetryableNotUnknown(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := startLowRiskFacadeReview(t, repo)

	original := reviewFacadePlannedTransitionHook
	t.Cleanup(func() { reviewFacadePlannedTransitionHook = original })
	var advanced atomic.Bool
	var competingErr error
	reviewFacadePlannedTransitionHook = func(_ context.Context, _ string, _ string, _ string) error {
		// The nested FINALIZE below re-enters this hook; only the outermost
		// call may hand the authority to a competing writer.
		if !advanced.CompareAndSwap(false, true) {
			return nil
		}
		var competing bytes.Buffer
		if err := RunReview([]string{
			"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
		}, &competing); err != nil {
			competingErr = fmt.Errorf("competing FINALIZE: %v\n%s", err, competing.String())
		}
		return nil
	}

	var output bytes.Buffer
	err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &output)
	if competingErr != nil {
		t.Fatal(competingErr)
	}
	if !advanced.Load() {
		t.Fatal("the planned-transition hook never ran; this test proved nothing")
	}
	if err == nil {
		t.Fatalf("FINALIZE with a superseded compact revision succeeded: %s", output.String())
	}
	if !errors.Is(err, reviewtransaction.ErrConcurrentUpdate) {
		t.Fatalf("FINALIZE error = %v, want a lost compare-and-set on the compact revision", err)
	}

	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "authority_revision_conflict" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "current_target" ||
		!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		failure.NextAction != "retry_with_bounded_backoff" {
		t.Fatalf("superseded-revision failure = %#v, want a not-started retryable classification", failure)
	}
	// The refusal knows both revisions; discarding them would be the same
	// information loss the unknown default committed.
	if !strings.Contains(failure.Cause, "expected compact revision") {
		t.Fatalf("superseded-revision failure discarded the revisions it knew: %q", failure.Cause)
	}
	if strings.Contains(failure.Message, "review.status") || strings.Contains(failure.Message, "maintainer") {
		t.Fatalf("superseded-revision message routes to unnecessary work: %q", failure.Message)
	}
	if !strings.Contains(strings.ToLower(failure.Message), "retry") {
		t.Fatalf("superseded-revision message does not name the retry continuation: %q", failure.Message)
	}

	// Retry-safe is a claim, not a wish: the identical request must converge on
	// the terminal receipt the competing writer already published.
	for attempt := 1; attempt <= 5; attempt++ {
		var converged bytes.Buffer
		if err := RunReview([]string{
			"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
		}, &converged); err != nil {
			t.Fatalf("retry %d after the superseded revision: %v\n%s", attempt, err, converged.String())
		}
	}
}

// TestRevisionConflictAfterACommittedTransitionStillReportsUnknown is the guard
// on the carve-out above. operation_outcome_unknown exists for a mutation that
// may or may not have landed, and a caller that retries one of those can
// double-apply it. The identical compare-and-set refusal, once it arrives after
// this operation already committed a native transition, must keep saying
// unknown.
//
// Do not relax this test to widen the reclassification.
func TestRevisionConflictAfterACommittedTransitionStillReportsUnknown(t *testing.T) {
	lineage := "revision-conflict-after-progress"
	superseded := fmt.Errorf("%w: expected compact revision %q, current %q",
		reviewtransaction.ErrConcurrentUpdate, "sha256:"+strings.Repeat("b", 64), "sha256:"+strings.Repeat("c", 64))
	progressed := &reviewFacadeOperationProgressError{
		LineageID:            lineage,
		StoreRevision:        "sha256:" + strings.Repeat("a", 64),
		CommittedTransitions: 1,
		Cause:                superseded,
	}
	failure := newReviewIntegrationFailure(
		ReviewIntegrationOperationFinalize, []string{"--lineage", lineage}, progressed,
	)
	if failure.MutationOutcome != ReviewMutationUnknown || failure.Phase != "native_committed" ||
		failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired ||
		failure.NextAction != "review.status" {
		t.Fatalf("post-transition revision conflict = %#v, want the unknown-mutation classification preserved", failure)
	}
	if failure.Code == "authority_revision_conflict" {
		t.Fatalf("post-transition revision conflict was reclassified as pre-mutation: %#v", failure)
	}
}

// TestUntypedConcurrentUpdateStillReportsUnknown proves the carve-out is keyed
// on the typed compare-and-set refusal, not on the broad ErrConcurrentUpdate
// sentinel. That sentinel is reported by preconditions all over the transaction
// package, including ones that are not the guarded authority write, so widening
// to it would launder genuinely unknown outcomes into retryable ones.
func TestUntypedConcurrentUpdateStillReportsUnknown(t *testing.T) {
	failure := newReviewIntegrationFailure(
		ReviewIntegrationOperationFinalize, []string{"--lineage", "untyped-concurrent-update"},
		fmt.Errorf("compact batch source changed: %w", reviewtransaction.ErrConcurrentUpdate),
	)
	if failure.Code != "operation_outcome_unknown" || failure.MutationOutcome != ReviewMutationUnknown ||
		failure.RetrySafe || failure.NextAction != "review.status" {
		t.Fatalf("untyped concurrent-update failure = %#v, want the unknown-mutation default", failure)
	}
}
