package mutationjournal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// replaceThenFail mimics the one window WriteFileAtomic documents: the rename
// published the new bytes and a later step failed, so the writer reports both
// Changed=true and an error. Only the first write is faulted, so the rollback
// that follows exercises the real writer.
func replaceThenFail(t *testing.T) {
	t.Helper()
	original := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = original })
	faulted := false
	writeFileAtomic = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		result, err := original(path, data, mode)
		if err != nil {
			t.Fatalf("underlying write %q: %v", path, err)
		}
		if faulted {
			return result, nil
		}
		faulted = true
		result.Changed = true
		return result, errors.New("injected parent-directory sync failure")
	}
}

// TestRestoreRollsBackWriteThatFailedAfterReplacement is #1676. WriteWithMode
// used to mark its entry changed only after a nil error, so a failure reported
// after the destination was already replaced left the entry marked unchanged
// and Restore skipped it: a reported rollback that rolled nothing back.
func TestRestoreRollsBackWriteThatFailedAfterReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "opencode.json")
	if err := os.WriteFile(path, []byte("before-image"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	replaceThenFail(t)

	journal := New(root)
	if _, err := journal.WriteWithMode(path, []byte("replacement bytes"), 0o644); err == nil {
		t.Fatal("WriteWithMode: want the injected error, got nil")
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if string(onDisk) != "replacement bytes" {
		t.Fatalf("destination = %q, want %q. The premise is that replacement already landed", onDisk, "replacement bytes")
	}

	if err := journal.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after restore: %v", err)
	}
	if string(restored) != "before-image" {
		t.Errorf("destination after Restore = %q, want %q: Restore skipped an entry whose file had in fact been replaced", restored, "before-image")
	}
}

// TestRestoreRemovesCreatedFileWhenWriteFailedAfterReplacement covers the same
// window for a path that did not exist before: rollback owes the caller an
// absent file, not an orphan created by a write that reported failure.
func TestRestoreRemovesCreatedFileWhenWriteFailedAfterReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "created.json")

	replaceThenFail(t)

	journal := New(root)
	if _, err := journal.WriteWithMode(path, []byte("orphan"), 0o644); err == nil {
		t.Fatal("WriteWithMode: want the injected error, got nil")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat after write: %v. The premise is that the file was created", err)
	}

	if err := journal.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after Restore (stat err = %v), want it removed", err)
	}
}

// TestWriteWithModeStillReportsTheError guards against the fix swallowing the
// failure: recording the change must not turn a failed write into a success.
func TestWriteWithModeStillReportsTheError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "opencode.json")

	replaceThenFail(t)

	journal := New(root)
	owned, err := journal.WriteWithMode(path, []byte("replacement bytes"), 0o644)
	if err == nil {
		t.Fatal("WriteWithMode: want the injected error, got nil")
	}
	if !strings.Contains(err.Error(), "injected parent-directory sync failure") {
		t.Errorf("error = %q, want it to wrap the writer failure", err)
	}
	if owned.After != "" {
		t.Errorf("WriteWithMode returned a populated OwnedFile %+v alongside its error", owned)
	}
}

// TestRestoreSkipsEntryWhenWriteFailedWithoutReplacing is the other side of the
// contract: when the writer reports Changed=false the destination was never
// touched, so Restore must leave it alone rather than rewrite it.
func TestRestoreSkipsEntryWhenWriteFailedWithoutReplacing(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "opencode.json")
	if err := os.WriteFile(path, []byte("before-image"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	original := writeFileAtomic
	t.Cleanup(func() { writeFileAtomic = original })
	restoreCalls := 0
	writeFileAtomic = func(path string, data []byte, mode os.FileMode) (filemerge.WriteResult, error) {
		restoreCalls++
		return filemerge.WriteResult{}, errors.New("injected pre-replacement failure")
	}

	journal := New(root)
	if _, err := journal.WriteWithMode(path, []byte("replacement bytes"), 0o644); err == nil {
		t.Fatal("WriteWithMode: want the injected error, got nil")
	}
	if err := journal.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restoreCalls != 1 {
		t.Errorf("writeFileAtomic called %d times, want 1: Restore rewrote a file that was never replaced", restoreCalls)
	}
}
