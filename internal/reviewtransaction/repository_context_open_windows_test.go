//go:build windows

package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestReadReviewRepositoryContextRetriesTransientWindowsSharingViolation(t *testing.T) {
	path := privateReviewRepositoryContextPath(t, "repository-context-sharing-violation")
	lock := zeroShareReviewRepositoryContextHandle(t, path)
	locked := true
	defer func() {
		if locked {
			_ = windows.CloseHandle(lock)
		}
	}()

	firstRetry := make(chan time.Duration, 1)
	resume := make(chan struct{})
	type result struct {
		file *os.File
		err  error
	}
	results := make(chan result, 1)
	go func() {
		file, err := openReviewRepositoryContextWithObserver(path, reviewRepositoryContextOpenObserver{
			beforeRetry: func(delay time.Duration) {
				firstRetry <- delay
				<-resume
			},
		})
		results <- result{file: file, err: err}
	}()
	select {
	case delay := <-firstRetry:
		if delay != 5*time.Millisecond {
			t.Fatalf("first backoff = %s, want 5ms", delay)
		}
	case result := <-results:
		t.Fatalf("open returned before its first sharing retry: %v", result.err)
	case <-time.After(time.Second):
		t.Fatal("open did not reach its first sharing retry")
	}
	if err := windows.CloseHandle(lock); err != nil {
		t.Fatal(err)
	}
	locked = false
	close(resume)
	select {
	case outcome := <-results:
		if outcome.err != nil {
			t.Fatalf("read after sharing violation: %v", outcome.err)
		}
		if outcome.file == nil {
			t.Fatal("read after sharing violation returned no file")
		}
		if err := outcome.file.Close(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("open did not converge after releasing the zero-share handle")
	}
}

func TestReadReviewRepositoryContextBoundsPersistentWindowsSharingViolation(t *testing.T) {
	path := privateReviewRepositoryContextPath(t, "repository-context-persistent-sharing-violation")
	lock := zeroShareReviewRepositoryContextHandle(t, path)
	defer windows.CloseHandle(lock)

	opens := 0
	var backoffs []time.Duration
	file, err := openReviewRepositoryContextWithObserver(path, reviewRepositoryContextOpenObserver{
		beforeOpen:  func() { opens++ },
		beforeRetry: func(delay time.Duration) { backoffs = append(backoffs, delay) },
	})
	if file != nil {
		_ = file.Close()
		t.Fatal("persistent sharing violation opened a file")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("persistent sharing error = %v", err)
	}
	want := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond}
	if opens != 6 || !slices.Equal(backoffs, want) {
		t.Fatalf("opens = %d, backoffs = %v; want 6 and %v", opens, backoffs, want)
	}
	total := time.Duration(0)
	for _, delay := range backoffs {
		total += delay
	}
	if total != 155*time.Millisecond {
		t.Fatalf("backoff total = %s, want 155ms", total)
	}
}

func TestReadReviewRepositoryContextStopsAfterNonSharingOpenFailure(t *testing.T) {
	path := privateReviewRepositoryContextPath(t, "repository-context-nonsharing-open-failure")
	lock := zeroShareReviewRepositoryContextHandle(t, path)
	locked := true
	defer func() {
		if locked {
			_ = windows.CloseHandle(lock)
		}
	}()

	opens := 0
	var backoffs []time.Duration
	file, err := openReviewRepositoryContextWithObserver(path, reviewRepositoryContextOpenObserver{
		beforeOpen: func() { opens++ },
		beforeRetry: func(delay time.Duration) {
			backoffs = append(backoffs, delay)
			if err := windows.CloseHandle(lock); err != nil {
				t.Fatal(err)
			}
			locked = false
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		},
	})
	if file != nil {
		_ = file.Close()
		t.Fatal("non-sharing failure opened a file")
	}
	if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Fatalf("open error = %v, want ERROR_FILE_NOT_FOUND", err)
	}
	if opens != 2 || !slices.Equal(backoffs, []time.Duration{5 * time.Millisecond}) {
		t.Fatalf("opens = %d, backoffs = %v; want 2 and [5ms]", opens, backoffs)
	}
}

func privateReviewRepositoryContextPath(t *testing.T, lineage string) string {
	t.Helper()
	repo, binding := reviewRepositoryContextFixture(t, lineage)
	handle, err := PublishReviewRepositoryContext(context.Background(), repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	path, err := reviewRepositoryContextPath(handle)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func zeroShareReviewRepositoryContextHandle(t *testing.T, path string) windows.Handle {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}
