//go:build windows

package reviewtransaction

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestWindowsStoreLockUsesExistingInodeAdvisoryTruth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	held, err := acquireStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireStoreLock(path); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("second Windows advisory acquisition = %v, want ErrConcurrentUpdate", err)
	}
	if evidence, exists := inventoryLock(AuthorityVersionCompact, "", path); !exists || evidence.Status != AuthorityLockOwned || evidence.Owner != nil {
		t.Fatalf("busy Windows lock evidence = %#v, exists=%t", evidence, exists)
	}
	if err := held.release(); err != nil {
		t.Fatal(err)
	}
	if evidence, exists := inventoryLock(AuthorityVersionCompact, "", path); !exists || evidence.Status != AuthorityLockReleased || evidence.Owner != nil {
		t.Fatalf("released Windows lock evidence = %#v, exists=%t", evidence, exists)
	}
}
