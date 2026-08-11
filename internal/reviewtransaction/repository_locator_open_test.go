package reviewtransaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestOpenReviewRepositoryContextFileRetriesTransientSharingViolation proves
// the final-file open tolerates a brief transient sharing-violation window
// by retrying the open up to the bounded schedule, then succeeding on a
// subsequent attempt. The test drives the retry loop with a fake opener and
// a no-op sleeper, so it is portable across platforms: the isTransient
// predicate is injected and reports true for the synthesized sharing
// violation regardless of OS.
func TestOpenReviewRepositoryContextFileRetriesTransientSharingViolation(t *testing.T) {
	transient := errors.New("transient sharing violation")
	other := errors.New("non-transient failure")
	calls := atomic.Int32{}
	attempts := make([]error, 0, 6)
	var open func(string) (*os.File, error)
	open = func(string) (*os.File, error) {
		call := calls.Add(1)
		switch call {
		case 1, 2, 3, 4:
			attempts = append(attempts, transient)
			return nil, transient
		case 5:
			attempts = append(attempts, nil)
			return os.NewFile(uintptr(call)+0xC0FFEE, "synthetic"), nil
		default:
			attempts = append(attempts, other)
			return nil, other
		}
	}
	slept := []time.Duration{}
	sleep := func(d time.Duration) { slept = append(slept, d) }
	isTransient := func(err error) bool { return errors.Is(err, transient) }
	backoffs := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond}

	file, err := openReviewRepositoryContextFileWith("/synthetic/path", open, backoffs, sleep, isTransient)
	if err != nil {
		t.Fatalf("retry on transient sharing violation: %v", err)
	}
	if file == nil {
		t.Fatal("retry returned a nil file handle on success")
	}
	_ = file.Close()
	if got := calls.Load(); got != 5 {
		t.Fatalf("open calls = %d, want 5 (1 initial + 4 retries before success)", got)
	}
	wantBackoffs := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond}
	if len(slept) != len(wantBackoffs) {
		t.Fatalf("slept %d backoffs, want %d", len(slept), len(wantBackoffs))
	}
	for i, d := range slept {
		if d != wantBackoffs[i] {
			t.Fatalf("backoff[%d] = %v, want %v", i, d, wantBackoffs[i])
		}
	}
	for i, err := range attempts[:len(attempts)-1] {
		if !errors.Is(err, transient) {
			t.Fatalf("attempt[%d] error = %v, want transient sharing violation", i, err)
		}
	}
	if attempts[len(attempts)-1] != nil {
		t.Fatalf("final attempt error = %v, want nil", attempts[len(attempts)-1])
	}
}

// TestOpenReviewRepositoryContextFileFailsImmediatelyOnNonTransientError
// proves the retry loop does not engage for any error that is not the
// transient sharing violation. A non-transient failure on the first attempt
// must surface verbatim with no backoff and no additional open call.
func TestOpenReviewRepositoryContextFileFailsImmediatelyOnNonTransientError(t *testing.T) {
	transient := errors.New("transient sharing violation")
	other := errors.New("non-transient failure")
	calls := atomic.Int32{}
	open := func(string) (*os.File, error) {
		calls.Add(1)
		return nil, other
	}
	slept := []time.Duration{}
	sleep := func(d time.Duration) { slept = append(slept, d) }
	isTransient := func(err error) bool { return errors.Is(err, transient) }
	backoffs := sharingViolationBackoffs

	file, err := openReviewRepositoryContextFileWith("/synthetic/path", open, backoffs, sleep, isTransient)
	if file != nil {
		_ = file.Close()
		t.Fatal("non-transient failure returned a file handle")
	}
	if !errors.Is(err, other) {
		t.Fatalf("err = %v, want %v", err, other)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("open calls = %d, want 1 (no retry on non-transient)", got)
	}
	if len(slept) != 0 {
		t.Fatalf("slept %d backoffs on non-transient failure, want 0", len(slept))
	}
}

// TestOpenReviewRepositoryContextFileBoundedRetryOnPersistentSharingViolation
// proves the retry budget is bounded. When the open keeps returning the
// transient sharing violation past the schedule, the helper exhausts the
// five backoffs (5/10/20/40/80 ms = 155 ms total) and surfaces the last
// transient error verbatim. No further retries, no infinite loop.
func TestOpenReviewRepositoryContextFileBoundedRetryOnPersistentSharingViolation(t *testing.T) {
	transient := errors.New("transient sharing violation")
	calls := atomic.Int32{}
	open := func(string) (*os.File, error) {
		calls.Add(1)
		return nil, transient
	}
	var slept []time.Duration
	sleep := func(d time.Duration) { slept = append(slept, d) }
	isTransient := func(err error) bool { return errors.Is(err, transient) }
	backoffs := sharingViolationBackoffs

	file, err := openReviewRepositoryContextFileWith("/synthetic/path", open, backoffs, sleep, isTransient)
	if file != nil {
		_ = file.Close()
		t.Fatal("bounded retry returned a file handle on persistent failure")
	}
	if !errors.Is(err, transient) {
		t.Fatalf("err = %v, want %v", err, transient)
	}
	if want := int32(1 + len(backoffs)); calls.Load() != want {
		t.Fatalf("open calls = %d, want %d (1 initial + 5 backoffs)", calls.Load(), want)
	}
	if len(slept) != len(backoffs) {
		t.Fatalf("slept %d backoffs, want %d", len(slept), len(backoffs))
	}
	var total time.Duration
	for i, d := range slept {
		if d != backoffs[i] {
			t.Fatalf("backoff[%d] = %v, want %v", i, d, backoffs[i])
		}
		total += d
	}
	if total != 155*time.Millisecond {
		t.Fatalf("total backoff sleep = %v, want 155ms", total)
	}
}

// TestOpenReviewRepositoryContextFileProductionScheduleMatchesIssueBounded
// proves the production sharingViolationBackoffs schedule sums to exactly
// 155 ms with five backoffs in the 5/10/20/40/80 ms shape. The issue pins
// these numbers; if any future change loosens the bound, this guard fails.
func TestOpenReviewRepositoryContextFileProductionScheduleMatchesIssueBounded(t *testing.T) {
	if want := 5; len(sharingViolationBackoffs) != want {
		t.Fatalf("len(sharingViolationBackoffs) = %d, want %d", len(sharingViolationBackoffs), want)
	}
	want := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond}
	var total time.Duration
	for i, d := range sharingViolationBackoffs {
		if d != want[i] {
			t.Fatalf("sharingViolationBackoffs[%d] = %v, want %v", i, d, want[i])
		}
		total += d
	}
	if total != 155*time.Millisecond {
		t.Fatalf("total sharing-violation backoff = %v, want 155ms", total)
	}
}

// TestOpenReviewRepositoryContextFileSwitchesBetweenTransientAndNonTransient
// proves the retry loop surfaces a non-transient error verbatim as soon as
// the open returns one, even if the prior attempts returned the transient
// sharing violation. The retry engages only while every attempt is the
// transient error; a different error halts the loop immediately.
func TestOpenReviewRepositoryContextFileSwitchesBetweenTransientAndNonTransient(t *testing.T) {
	transient := errors.New("transient sharing violation")
	other := errors.New("non-transient failure mid-retry")
	calls := atomic.Int32{}
	open := func(string) (*os.File, error) {
		switch calls.Add(1) {
		case 1, 2:
			return nil, transient
		case 3:
			return nil, other
		default:
			return nil, fmt.Errorf("unexpected call %d", calls.Load())
		}
	}
	var slept []time.Duration
	sleep := func(d time.Duration) { slept = append(slept, d) }
	isTransient := func(err error) bool { return errors.Is(err, transient) }
	backoffs := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond}

	file, err := openReviewRepositoryContextFileWith("/synthetic/path", open, backoffs, sleep, isTransient)
	if file != nil {
		_ = file.Close()
		t.Fatal("non-transient mid-retry returned a file handle")
	}
	if !errors.Is(err, other) {
		t.Fatalf("err = %v, want %v", err, other)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("open calls = %d, want 3 (1 initial + 2 transient retries, halted on non-transient)", got)
	}
	if len(slept) != 2 {
		t.Fatalf("slept %d backoffs before non-transient halt, want 2", len(slept))
	}
}

// TestOpenReviewRepositoryContextFileRealPathSuccess proves the production
// openReviewRepositoryContextFile succeeds against a real, non-flaky path on
// every supported platform. This is the smoke test: if the helper is
// mis-wired (for example if it returns the file from os.Open without going
// through the helper, or if the build-tag shim is broken), the test catches
// the regression on the native open path.
func TestOpenReviewRepositoryContextFileRealPathSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "real-record")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := openReviewRepositoryContextFile(path)
	if err != nil {
		t.Fatalf("real-path open: %v", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != int64(len("payload")) {
		t.Fatalf("stat.Size() = %d, want %d", stat.Size(), len("payload"))
	}
}
