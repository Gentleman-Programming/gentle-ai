//go:build windows

package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAtomicReplaceWindowsReadbackCatchesAFaultedRename is the Windows pin for
// the negative path. The cross-platform TestAtomicReplace (no build tag) already
// exercises the happy path on Windows after the old skip was removed; here the
// read-back is what catches a rename that returns nil without taking effect,
// and the test has to hold on the platform that reported the bug.
func TestAtomicReplaceWindowsReadbackCatchesAFaultedRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gentle-ai.exe.new")
	dst := filepath.Join(dir, "gentle-ai.exe")
	installed := []byte("the binary that is already installed")

	if err := os.WriteFile(src, []byte("the replacement binary"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, installed, 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	original := renameFn
	t.Cleanup(func() { renameFn = original })
	renameFn = func(string, string) error { return nil }

	err := atomicReplace(src, dst)
	if err == nil {
		t.Fatal("atomicReplace reported success for a swap that never took effect")
	}
	if !strings.Contains(err.Error(), "the binary was not replaced") {
		t.Errorf("atomicReplace error = %q, want it to say the binary was not replaced", err)
	}

	onDisk, readErr := os.ReadFile(dst)
	if readErr != nil {
		t.Fatalf("read back dst: %v", readErr)
	}
	if string(onDisk) != string(installed) {
		t.Errorf("installed binary = %q, want the untouched %q", onDisk, installed)
	}
}
