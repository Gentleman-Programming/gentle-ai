package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestRARLockWaiterSurvivesRotatingQueueBeyondFixedBudget is issue #3239's
// positive property: the bounded waiter's deadline bounds time without an
// ownership change, never total queue length. Five holders each keeping the
// lock ~500ms serialize well past the 2s budget, and every waiter still
// acquires because ownership keeps rotating.
func TestRARLockWaiterSurvivesRotatingQueueBeyondFixedBudget(t *testing.T) {
	repo := initSnapshotRepo(t)
	dir := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "lock-progress")
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := createPrivateRARDirectory(filepath.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := createPrivateRARDirectory(dir); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "authority.lock")

	const workers = 5
	const hold = 500 * time.Millisecond
	var group sync.WaitGroup
	errs := make(chan error, workers)
	start := time.Now()
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			lock, err := acquireRARAuthorityLock(context.Background(), lockPath)
			if err != nil {
				errs <- err
				return
			}
			time.Sleep(hold)
			errs <- lock.release()
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("rotating-queue waiter failed: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < time.Duration(workers)*hold {
		t.Fatalf("holders did not serialize: elapsed %s", elapsed)
	}
}

// TestRARLockWaiterStillTimesOutOnStuckHolder pins the unchanged liveness
// guarantee: with no ownership change at all, the waiter fails in exactly the
// configured budget, never extended by the progress reset.
func TestRARLockWaiterStillTimesOutOnStuckHolder(t *testing.T) {
	repo := initSnapshotRepo(t)
	dir := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "lock-stuck")
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(dir)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := createPrivateRARDirectory(filepath.Dir(dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := createPrivateRARDirectory(dir); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "authority.lock")

	holder, err := acquireRARAuthorityLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := holder.release(); err != nil {
			t.Fatal(err)
		}
	}()

	start := time.Now()
	_, err = acquireRARAuthorityLock(context.Background(), lockPath)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrAuthorityLockTimeout) {
		t.Fatalf("stuck-holder wait error = %v, want authority lock timeout", err)
	}
	if elapsed < maintenanceLockTimeout || elapsed > 3*maintenanceLockTimeout {
		t.Fatalf("stuck-holder timeout fired at %s, want ~%s without progress extensions", elapsed, maintenanceLockTimeout)
	}
	if _, statErr := os.Lstat(lockPath); statErr != nil {
		t.Fatalf("lock record vanished during contention: %v", statErr)
	}
}
