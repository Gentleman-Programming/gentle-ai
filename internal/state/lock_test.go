package state

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTryLockFileReportsBusyForSecondHandle covers the platform primitive
// directly: a second open handle on an already-locked file reports busy rather
// than returning an error or blocking.
func TestTryLockFileReportsBusyForSecondHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")

	first, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open first handle: %v", err)
	}
	defer first.Close()
	locked, err := tryLockFile(first)
	if err != nil {
		t.Fatalf("lock first handle: %v", err)
	}
	if !locked {
		t.Fatal("first handle could not take a free lock")
	}

	second, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	defer second.Close()
	locked, err = tryLockFile(second)
	if err != nil {
		t.Fatalf("lock second handle: %v", err)
	}
	if locked {
		t.Error("second handle took a held lock; the lock does not exclude")
	}

	if err := unlockFile(first); err != nil {
		t.Fatalf("unlock first handle: %v", err)
	}
	locked, err = tryLockFile(second)
	if err != nil {
		t.Fatalf("relock second handle: %v", err)
	}
	if !locked {
		t.Error("second handle could not take the lock after release")
	}
}
