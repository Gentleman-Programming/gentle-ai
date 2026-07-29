package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// compactAuthorityLockPath returns the one exclusive advisory lock every
// compact authority writer and the read-only delivery gate contend for. It is
// repository-wide (`.../review-transactions/v2/LOCK`), which is why a
// controller fanning out concurrent FINALIZE and gate calls against a single
// healthy authority produces contention at all.
func compactAuthorityLockPath(t *testing.T, repo, lineage string) string {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(store.Dir), "LOCK")
}

// holdCompactAuthorityLock reproduces the loser's side of the reporter's
// oracle with the real primitive rather than a fake: the same non-blocking
// advisory syscall a competing process would hit. Nothing is mutated, so any
// refusal observed under it is transient contention by construction.
func holdCompactAuthorityLock(t *testing.T, repo, lineage string) *reviewtransaction.AuthorityFileLock {
	t.Helper()
	lock, err := reviewtransaction.AcquireAuthorityFileLock(compactAuthorityLockPath(t, repo, lineage))
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

// contendedAuthorityLockError produces a genuine advisory-contention error by
// losing a real race for a held lock.
func contendedAuthorityLockError(t *testing.T, repo, lineage string) error {
	t.Helper()
	held := holdCompactAuthorityLock(t, repo, lineage)
	t.Cleanup(func() { _ = held.Release() })
	lost, err := reviewtransaction.AcquireAuthorityFileLock(compactAuthorityLockPath(t, repo, lineage))
	if err == nil {
		_ = lost.Release()
		t.Fatal("second acquisition of a held advisory authority lock succeeded")
	}
	return err
}

// approvedLowRiskFacadeReview drives the zero-lens low-risk lifecycle to a
// terminal approved receipt so a delivery gate has something real to evaluate.
func approvedLowRiskFacadeReview(t *testing.T, repo string) string {
	t.Helper()
	lineage := startLowRiskFacadeReview(t, repo)
	var finalizeOutput bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &finalizeOutput); err != nil {
		t.Fatalf("low-risk FINALIZE: %v\n%s", err, finalizeOutput.String())
	}
	return lineage
}

// TestNegotiatedAuthorityLockContentionIsRetryableNotUnknown is the FINALIZE
// half of issue 1861. A mutation that never acquired the lock provably never
// ran its guarded body, so `operation_outcome_unknown` — which tells a caller
// retry is unsafe because a mutation may have landed — is a false statement
// about it. The loser must be told the truth: nothing started, retry.
func TestNegotiatedAuthorityLockContentionIsRetryableNotUnknown(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "contention-classification"
	busy := contendedAuthorityLockError(t, repo, lineage)

	failure := newReviewIntegrationFailure(
		ReviewIntegrationOperationFinalize, []string{"--lineage", lineage}, busy,
	)
	if failure.Code != "authority_lock_contention" || failure.Phase != "pre_native" ||
		failure.MutationOutcome != ReviewMutationNotStarted || failure.AuthorityApplicability != "not_evaluated" ||
		!failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityNotReplayable ||
		failure.NextAction != "retry_with_bounded_backoff" {
		t.Fatalf("advisory contention failure = %#v, want a not-started retryable classification", failure)
	}
	// The envelope must survive its own published invariants, which is where
	// the first attempt at this classification failed: `exact_replay_safe`
	// promises a committed mutation can be resumed, and there is none here.
	if err := failure.Validate(); err != nil {
		t.Fatalf("contention envelope violates the published failure contract: %v", err)
	}
	// The governing rule on this branch: name a command only when running it
	// resolves the block. The answer here is literally "retry", so the message
	// must say that instead of routing to a command that adds nothing.
	if !strings.Contains(strings.ToLower(failure.Message), "retry") {
		t.Fatalf("contention message does not name the retry continuation: %q", failure.Message)
	}
	if strings.Contains(failure.Message, "review.status") || strings.Contains(failure.Message, "maintainer") {
		t.Fatalf("contention message routes to unnecessary work: %q", failure.Message)
	}
}

// TestLockContentionAfterACommittedTransitionStillReportsUnknown is the
// regression that matters most. `operation_outcome_unknown` exists for real
// ambiguity, and the narrow reclassification above must never reach a failure
// whose operation already committed a native transition — a caller that
// retries a maybe-committed mutation can double-apply it. The identical
// advisory-contention error, once it arrives after durable progress, must keep
// saying unknown.
//
// Do not relax this test to make a broader reclassification pass. The whole
// point of issue 1861 is that the reclassification stays narrow.
func TestLockContentionAfterACommittedTransitionStillReportsUnknown(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "contention-after-progress"
	busy := contendedAuthorityLockError(t, repo, lineage)

	progressed := &reviewFacadeOperationProgressError{
		LineageID:            lineage,
		StoreRevision:        "sha256:" + strings.Repeat("a", 64),
		CommittedTransitions: 1,
		Cause:                busy,
	}
	failure := newReviewIntegrationFailure(
		ReviewIntegrationOperationFinalize, []string{"--lineage", lineage}, progressed,
	)
	if failure.MutationOutcome != ReviewMutationUnknown || failure.Phase != "native_committed" ||
		failure.RetrySafe || failure.Replayability != reviewtransaction.ReplayabilityStatusRequired ||
		failure.NextAction != "review.status" {
		t.Fatalf("post-transition contention failure = %#v, want the unknown-mutation classification preserved", failure)
	}
	if failure.Code == "authority_lock_contention" {
		t.Fatalf("post-transition contention was reclassified as pre-mutation: %#v", failure)
	}
}

// TestConcurrentDeliveryGateContentionIsNotReportedAsInvalidated is the gate
// half of issue 1861. The delivery gate never mutates anything; losing a race
// for the advisory lock is not damage, so it must not report `invalidated`
// with explicit-maintainer-action. The reporter saw 19 of 20 concurrent
// pre-commit gates report exactly that against a healthy authority.
func TestConcurrentDeliveryGateContentionIsNotReportedAsInvalidated(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := approvedLowRiskFacadeReview(t, repo)

	held := holdCompactAuthorityLock(t, repo, lineage)
	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePostApply),
	}, &output)
	if err == nil {
		_ = held.Release()
		t.Fatalf("contended delivery gate reached a verdict: %s", output.String())
	}
	if bytes.Contains(output.Bytes(), []byte(`"invalidated"`)) {
		_ = held.Release()
		t.Fatalf("contended delivery gate reported damage: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != ReviewIntegrationOperationValidate ||
		failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe ||
		failure.NextAction != "retry_with_bounded_backoff" {
		_ = held.Release()
		t.Fatalf("contended delivery gate failure = %#v, want a retryable non-damage classification", failure)
	}
	if failure.Code == "gate_invalidated" {
		_ = held.Release()
		t.Fatalf("contended delivery gate failure kept the invalidated code: %#v", failure)
	}
	if strings.Contains(err.Error(), "explicit maintainer action") {
		_ = held.Release()
		t.Fatalf("contended delivery gate demanded maintainer action: %v", err)
	}

	// Convergence, mirroring the reporter's 5/5: once contention ends, the
	// identical sequential call reaches the real verdict with no repair, no
	// recovery, and no human in the loop.
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		var converged bytes.Buffer
		if err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePostApply),
		}, &converged); err != nil {
			t.Fatalf("sequential gate retry %d after contention: %v\n%s", attempt, err, converged.String())
		}
		var gate ReviewValidateResult
		decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, converged.Bytes()).Result, &gate)
		if !gate.Allowed || gate.Result != reviewtransaction.GateAllow {
			t.Fatalf("sequential gate retry %d = %#v", attempt, gate)
		}
	}
}

// TestConcurrentFinalizeElectsExactlyOneWriter proves the fix does not weaken
// the single-writer guarantee. The durable trace records one line per
// committed native transition under the authority lock, so a concurrent fan-out
// that commits more transitions than one sequential FINALIZE would be a
// double-apply.
func TestConcurrentFinalizeElectsExactlyOneWriter(t *testing.T) {
	sequentialRepo := initReviewCLIRepo(t)
	sequentialTrace := filepath.Join(t.TempDir(), "sequential-trace.jsonl")
	sequentialLineage := startLowRiskFacadeReview(t, sequentialRepo)
	var sequentialOutput bytes.Buffer
	if err := RunReview([]string{
		"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", sequentialRepo,
		"--lineage", sequentialLineage, "--trace", sequentialTrace,
	}, &sequentialOutput); err != nil {
		t.Fatalf("sequential FINALIZE: %v\n%s", err, sequentialOutput.String())
	}
	wantTransitions := countTraceTransitions(t, sequentialTrace)
	if wantTransitions == 0 {
		t.Fatal("sequential FINALIZE recorded no native transition; the trace oracle is not measuring anything")
	}

	concurrentRepo := initReviewCLIRepo(t)
	concurrentTrace := filepath.Join(t.TempDir(), "concurrent-trace.jsonl")
	concurrentLineage := startLowRiskFacadeReview(t, concurrentRepo)

	const attempts = 8
	outputs := make([]bytes.Buffer, attempts)
	failures := make([]error, attempts)
	var wait sync.WaitGroup
	wait.Add(attempts)
	for index := 0; index < attempts; index++ {
		go func(index int) {
			defer wait.Done()
			failures[index] = RunReview([]string{
				"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", concurrentRepo,
				"--lineage", concurrentLineage, "--trace", concurrentTrace,
			}, &outputs[index])
		}(index)
	}
	wait.Wait()

	if got := countTraceTransitions(t, concurrentTrace); got != wantTransitions {
		t.Fatalf("concurrent FINALIZE committed %d native transitions, want exactly %d", got, wantTransitions)
	}
	succeeded := 0
	for index := 0; index < attempts; index++ {
		if failures[index] == nil {
			succeeded++
			continue
		}
		if outputs[index].Len() == 0 {
			t.Fatalf("concurrent FINALIZE loser %d emitted no negotiated envelope; error: %v", index, failures[index])
		}
		failure := decodeReviewIntegrationFailure(t, outputs[index].Bytes())
		// A loser can be refused at either of the two serialization points, and
		// which one it hits is pure scheduling: a loser that reaches the lock
		// while the winner holds it is refused by the lock, and a loser that
		// arrives after the winner released it wins the lock and is refused by
		// the compare-and-set on the revision it read beforehand. CI reached the
		// second one where this machine reaches the first. Both provably
		// published nothing, so both must carry the not-started retryable shape
		// asserted below; see TestFinalizeRevisionConflictIsRetryableNotUnknown
		// for the deterministic reproduction of the revision-conflict loser.
		switch failure.Code {
		case "authority_lock_contention", "authority_lock_timeout", "authority_revision_conflict":
		default:
			t.Fatalf("concurrent FINALIZE loser %d = %#v (error: %v)", index, failure, failures[index])
		}
		if failure.MutationOutcome != ReviewMutationNotStarted || !failure.RetrySafe {
			t.Fatalf("concurrent FINALIZE loser %d = %#v, want a not-started retryable classification", index, failure)
		}
	}
	if succeeded == 0 {
		t.Fatal("no concurrent FINALIZE reached the terminal receipt")
	}

	// Convergence after contention ends, mirroring the reporter's 5/5. The
	// terminal replay admits only --lineage, so the trace flag is dropped
	// here; the trace assertion above already proved the commit count.
	for attempt := 1; attempt <= 5; attempt++ {
		var converged bytes.Buffer
		if err := RunReview([]string{
			"finalize", "--contract", ReviewIntegrationContractV1, "--cwd", concurrentRepo,
			"--lineage", concurrentLineage,
		}, &converged); err != nil {
			t.Fatalf("sequential FINALIZE retry %d after contention: %v\n%s", attempt, err, converged.String())
		}
	}
	if got := countTraceTransitions(t, concurrentTrace); got != wantTransitions {
		t.Fatalf("sequential retries after contention committed %d native transitions, want exactly %d", got, wantTransitions)
	}
}

func startLowRiskFacadeReview(t *testing.T, repo string) string {
	t.Helper()
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = fmt.Sprintf("ordinary documentation line %03d", index+1)
	}
	path := filepath.Join(repo, "docs", "ordinary-guide.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
	}), &startOutput); err != nil {
		t.Fatalf("low-risk START: %v\n%s", err, startOutput.String())
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)
	return started.LineageID
}

func countTraceTransitions(t *testing.T, path string) int {
	t.Helper()
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(string(payload), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
