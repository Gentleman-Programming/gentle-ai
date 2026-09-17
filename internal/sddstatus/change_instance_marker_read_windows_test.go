//go:build windows

package sddstatus

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestReadChangeInstanceMarkerRetriesTransientWindowsSharingViolation(t *testing.T) {
	path := writeChangeInstanceMarkerFixture(t)
	lock := zeroShareChangeInstanceMarkerHandle(t, path)
	locked := true
	defer func() {
		if locked {
			_ = windows.CloseHandle(lock)
		}
	}()

	firstRetry := make(chan time.Duration, 1)
	resume := make(chan struct{})
	type result struct {
		payload []byte
		err     error
	}
	results := make(chan result, 1)
	go func() {
		payload, err := readChangeInstanceMarkerFileWithObserver(path, changeInstanceMarkerReadObserver{
			beforeRetry: func(delay time.Duration) {
				firstRetry <- delay
				<-resume
			},
		})
		results <- result{payload: payload, err: err}
	}()
	select {
	case delay := <-firstRetry:
		if delay != 5*time.Millisecond {
			t.Fatalf("first backoff = %s, want 5ms", delay)
		}
	case outcome := <-results:
		t.Fatalf("read returned before its first sharing retry: %v", outcome.err)
	case <-time.After(time.Second):
		t.Fatal("read did not reach its first sharing retry")
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
		if string(outcome.payload) != "marker\n" {
			t.Fatalf("read payload = %q", outcome.payload)
		}
	case <-time.After(time.Second):
		t.Fatal("read did not converge after releasing the zero-share handle")
	}
}

func TestReadChangeInstanceMarkerBoundsPersistentWindowsSharingViolation(t *testing.T) {
	path := writeChangeInstanceMarkerFixture(t)
	lock := zeroShareChangeInstanceMarkerHandle(t, path)
	defer windows.CloseHandle(lock)

	reads := 0
	var backoffs []time.Duration
	payload, err := readChangeInstanceMarkerFileWithObserver(path, changeInstanceMarkerReadObserver{
		beforeRead:  func() { reads++ },
		beforeRetry: func(delay time.Duration) { backoffs = append(backoffs, delay) },
	})
	if payload != nil {
		t.Fatalf("persistent sharing violation read payload %q", payload)
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("persistent sharing error = %v", err)
	}
	want := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond}
	if reads != 6 || !slices.Equal(backoffs, want) {
		t.Fatalf("reads = %d, backoffs = %v; want 6 and %v", reads, backoffs, want)
	}
}

func TestReadChangeInstanceMarkerStopsAfterNonSharingReadFailure(t *testing.T) {
	path := writeChangeInstanceMarkerFixture(t)
	lock := zeroShareChangeInstanceMarkerHandle(t, path)
	locked := true
	defer func() {
		if locked {
			_ = windows.CloseHandle(lock)
		}
	}()

	reads := 0
	var backoffs []time.Duration
	payload, err := readChangeInstanceMarkerFileWithObserver(path, changeInstanceMarkerReadObserver{
		beforeRead: func() { reads++ },
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
	if payload != nil {
		t.Fatalf("non-sharing failure read payload %q", payload)
	}
	if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Fatalf("read error = %v, want ERROR_FILE_NOT_FOUND", err)
	}
	if reads != 2 || !slices.Equal(backoffs, []time.Duration{5 * time.Millisecond}) {
		t.Fatalf("reads = %d, backoffs = %v; want 2 and [5ms]", reads, backoffs)
	}
}

func writeChangeInstanceMarkerFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), changeInstanceMarkerFile)
	if err := os.WriteFile(path, []byte("marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func zeroShareChangeInstanceMarkerHandle(t *testing.T, path string) windows.Handle {
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
