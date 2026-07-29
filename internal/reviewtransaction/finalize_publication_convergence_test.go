package reviewtransaction

import (
	"context"
	"os"
	"testing"
	"time"
)

// releaseShortlyHeldStoreLock simulates the winner's side of post-terminal
// publication contention with the real primitive: the advisory lock is held at
// the instant the completion call starts and released milliseconds later,
// exactly as a concurrent FINALIZE holds it across its own bookkeeping
// critical sections.
func releaseShortlyHeldStoreLock(t *testing.T, path string) {
	t.Helper()
	held, err := AcquireAuthorityFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = held.Release()
	}()
}

// TestWriteReceiptWaitsOutBrieflyHeldAdvisoryLock is the store half of the
// third concurrent-FINALIZE loser shape. Once authority is terminal, the
// receipt bytes are derived deterministically from that authority and
// publication is idempotent (publishImmutable accepts an identical existing
// receipt), so a completer that meets a briefly-held advisory lock cannot
// double-apply anything by waiting — refusing instantly, as the pre-commit
// mutation paths correctly do, turns a milliseconds-long race into a reported
// publication failure for an operation whose outcome already committed.
func TestWriteReceiptWaitsOutBrieflyHeldAdvisoryLock(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	_, store, receipt := approvedCompactRevisionFixture(t, repo, "receipt-convergence")
	if err := os.Remove(store.ReceiptPath()); err != nil {
		t.Fatal(err)
	}

	releaseShortlyHeldStoreLock(t, store.lockPath)
	if err := store.WriteReceipt(context.Background(), receipt); err != nil {
		t.Fatalf("terminal receipt publication refused a briefly-held advisory lock instead of converging: %v", err)
	}
	published, err := os.ReadFile(store.ReceiptPath())
	if err != nil || len(published) == 0 {
		t.Fatalf("converged publication left no receipt: %v", err)
	}
}

// TestFinalizeAttemptCompletionWaitsOutBrieflyHeldAdvisoryLock covers the
// journal half of the same shape: ReceiptPublished and Completed are monotonic
// idempotent flags, so their writers may also wait out transient contention.
// The pre-commit journal writers (Reconcile/Plan/Record) keep the instant
// refusal — nothing here may widen to them.
func TestFinalizeAttemptCompletionWaitsOutBrieflyHeldAdvisoryLock(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	state, store, _ := approvedCompactRevisionFixture(t, repo, "journal-convergence")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	request := FinalizeAttemptRequest{
		LineageID:                state.LineageID,
		ExpectedRevision:         record.Revision,
		CandidateDigest:          FinalizeAttemptValueDigest("candidate", state.CurrentSnapshot),
		ReviewerResultsDigest:    FinalizeAttemptValueDigest("results", nil),
		CorrectionForecastDigest: FinalizeAttemptValueDigest("correction", 0),
		ValidationDigest:         FinalizeAttemptValueDigest("validation", nil),
		RefuterDigest:            FinalizeAttemptValueDigest("refuter", nil),
		EvidenceDigest:           FinalizeAttemptValueDigest("evidence", nil),
		FailedDigest:             FinalizeAttemptValueDigest("failed", false),
	}
	request.RequestDigest = FinalizeAttemptRequestDigest(request)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	releaseShortlyHeldStoreLock(t, store.lockPath)
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err != nil {
		t.Fatalf("receipt-published bookkeeping refused a briefly-held advisory lock instead of converging: %v", err)
	}
	releaseShortlyHeldStoreLock(t, store.lockPath)
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatalf("attempt-completion bookkeeping refused a briefly-held advisory lock instead of converging: %v", err)
	}
	pending, err := store.PendingFinalizeAttempt()
	if err != nil || pending != nil {
		t.Fatalf("converged completion left a pending attempt: %#v, %v", pending, err)
	}
}
