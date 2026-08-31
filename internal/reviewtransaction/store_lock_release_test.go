package reviewtransaction

import (
	"os"
	"path/filepath"
	"testing"
)

// #2504: release clears the owner payload so a later observer never reads an
// exited process as the holder; kernel advisory ownership stays the truth.
func TestStoreLockReleaseClearsOwnerPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "LOCK")
	held, err := acquireStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if before, err := os.ReadFile(path); err != nil || len(before) == 0 {
		t.Fatalf("held LOCK payload = %q err=%v", before, err)
	}
	if err := held.release(); err != nil {
		t.Fatal(err)
	}
	if after, err := os.ReadFile(path); err != nil || len(after) != 0 {
		t.Fatalf("released LOCK payload = %q err=%v, want empty", after, err)
	}
	evidence, exists := inventoryLock(AuthorityVersionCompact, "", path)
	if !exists || evidence.Status != AuthorityLockReleased || evidence.Owner != nil || evidence.Problem != "" {
		t.Fatalf("released lock evidence = %#v exists=%t", evidence, exists)
	}
}
