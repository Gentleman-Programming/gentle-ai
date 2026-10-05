package reviewtransaction

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// #2504: release clears the owner payload so a later observer never reads an
// exited process as the holder; kernel advisory ownership stays the truth.
func TestStoreLockReleaseClearsOwnerPayload(t *testing.T) {
	// canonicalTempDir, not t.TempDir: the secure open walk refuses a symlinked
	// component, and macOS keeps its per-user temp root behind /var -> /private/var.
	path := filepath.Join(canonicalTempDir(t), "LOCK")
	held, err := acquireStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows byte-range locks are mandatory: while the coordination byte is
	// held, every read of it from another handle fails, so the held payload is
	// read through the holder's own handle, which is legal on every platform.
	if _, err := held.file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if before, err := io.ReadAll(held.file); err != nil || len(before) == 0 {
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
