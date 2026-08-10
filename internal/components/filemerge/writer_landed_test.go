package filemerge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteFileAtomicReportsReplacementWhenParentSyncFails pins the half of the
// contract that callers cannot observe any other way: when the rename has
// already published the new bytes and only the parent-directory sync fails, the
// error is real but the file HAS changed. Returning a zero WriteResult there
// tells every caller "nothing happened" about a destination that now holds new
// content, which is what makes rollback skip the entry (#1676).
func TestWriteFileAtomicReportsReplacementWhenParentSyncFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	origGOOS := runtimeGOOS
	origSyncDir := syncDirFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		syncDirFn = origSyncDir
	})
	runtimeGOOS = func() string { return "linux" }
	syncDirFn = func(string) error { return errors.New("injected parent-directory sync failure") }

	result, err := WriteFileAtomic(path, []byte("after"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic: want parent-directory sync error, got nil")
	}
	if !result.Changed {
		t.Errorf("WriteFileAtomic returned Changed=false alongside the error, but the destination was replaced; callers cannot distinguish this from an untouched file")
	}
	if result.Created {
		t.Errorf("WriteFileAtomic returned Created=true for a pre-existing file")
	}

	onDisk, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	if string(onDisk) != "after" {
		t.Fatalf("destination content = %q, want %q. The premise of this test is that the rename already landed", onDisk, "after")
	}
}

// TestWriteFileAtomicReportsCreationWhenParentSyncFails is the same contract for
// a file that did not previously exist: the entry was created, so Created must
// survive the post-replacement error too.
func TestWriteFileAtomicReportsCreationWhenParentSyncFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")

	origGOOS := runtimeGOOS
	origSyncDir := syncDirFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		syncDirFn = origSyncDir
	})
	runtimeGOOS = func() string { return "linux" }
	syncDirFn = func(string) error { return errors.New("injected parent-directory sync failure") }

	result, err := WriteFileAtomic(path, []byte("fresh"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic: want parent-directory sync error, got nil")
	}
	if !result.Changed || !result.Created {
		t.Errorf("WriteFileAtomic = %+v alongside the error, want Changed=true Created=true", result)
	}
}

// TestWriteFileAtomicFailsWhenReplacementDoesNotLand is the hard direction: make
// the underlying replacement silently fail to take effect and require the
// function to report failure. A rename that returns nil without moving anything
// is exactly the shape reported on Windows in #2319, and it is the only way to
// prove the result is read back from disk rather than inferred from the attempt.
func TestWriteFileAtomicFailsWhenReplacementDoesNotLand(t *testing.T) {
	tests := []struct {
		name    string
		rename  func(oldPath, newPath string) error
		seed    string
		wantOld string
	}{
		{
			name:    "rename reports success without moving anything",
			rename:  func(string, string) error { return nil },
			seed:    "before",
			wantOld: "before",
		},
		{
			name: "replacement lands and is immediately truncated",
			rename: func(oldPath, newPath string) error {
				if err := os.Rename(oldPath, newPath); err != nil {
					return err
				}
				return os.WriteFile(newPath, []byte("trunc"), 0o644)
			},
			seed:    "before",
			wantOld: "trunc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(tt.seed), 0o644); err != nil {
				t.Fatalf("seed: %v", err)
			}

			origRename := renameFn
			t.Cleanup(func() { renameFn = origRename })
			renameFn = tt.rename

			result, err := WriteFileAtomic(path, []byte("after"), 0o644)
			if err == nil {
				t.Fatal("WriteFileAtomic reported success for a replacement that never landed on disk")
			}
			if want := fmt.Sprintf("%q", path); !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name the destination representation %s", err, want)
			}
			if result.Changed {
				t.Errorf("WriteFileAtomic = %+v, want Changed=false: the destination does not hold the requested bytes", result)
			}

			onDisk, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("read back: %v", readErr)
			}
			if string(onDisk) != tt.wantOld {
				t.Errorf("destination content = %q, want %q", onDisk, tt.wantOld)
			}
		})
	}
}

// TestWriteFileAtomicLeavesNoTempFileWhenReplacementDoesNotLand guards the
// cleanup half: a rename that silently no-ops leaves the staged temp file
// behind, and the writer owns removing it.
func TestWriteFileAtomicLeavesNoTempFileWhenReplacementDoesNotLand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	origRename := renameFn
	t.Cleanup(func() { renameFn = origRename })
	renameFn = func(string, string) error { return nil }

	if _, err := WriteFileAtomic(path, []byte("after"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic reported success for a replacement that never landed on disk")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gentle-ai-") {
			t.Errorf("staged temp file %q was left behind after a failed replacement", entry.Name())
		}
	}
}

// TestWriteFileAtomicSucceedsOnlyAfterReadBack states the positive half of the
// contract explicitly: a nil error means the bytes were read back from the
// destination, not merely handed to the kernel.
func TestWriteFileAtomicSucceedsOnlyAfterReadBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	result, err := WriteFileAtomic(path, []byte("payload"), 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	if !result.Changed || !result.Created {
		t.Errorf("WriteFileAtomic = %+v, want Changed=true Created=true", result)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(onDisk) != "payload" {
		t.Errorf("destination content = %q, want %q", onDisk, "payload")
	}
}
