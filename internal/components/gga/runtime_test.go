package gga

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureRuntimeAssetsCreatesPRModeWhenMissing(t *testing.T) {
	home := t.TempDir()
	path := RuntimePRModePath(home)

	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("EnsureRuntimeAssets() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	text := string(content)
	if !strings.Contains(text, "detect_base_branch") {
		t.Fatalf("runtime pr_mode.sh missing expected content")
	}
}

// TestEnsureRuntimeAssetsOverwritesStalePRMode verifies the always-write behavior:
// when an existing pr_mode.sh has stale content (differs from the embedded asset),
// EnsureRuntimeAssets must overwrite it to keep the runtime current.
// WriteFileAtomic ensures this is a no-op when content already matches.
func TestEnsureRuntimeAssetsOverwritesStalePRMode(t *testing.T) {
	home := t.TempDir()
	path := RuntimePRModePath(home)
	if err := os.MkdirAll(RuntimeLibDir(home), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	const stale = "#!/usr/bin/env bash\n# stale-version\n"
	if err := os.WriteFile(path, []byte(stale), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("EnsureRuntimeAssets() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	// The stale content must have been replaced with the embedded asset.
	if string(content) == stale {
		t.Fatalf("EnsureRuntimeAssets did not overwrite stale pr_mode.sh")
	}
	if !strings.Contains(string(content), "detect_base_branch") {
		t.Fatalf("overwritten pr_mode.sh missing expected embedded content")
	}
}

// TestEnsureRuntimeAssetsIsNoOpWhenContentMatches verifies idempotency:
// when pr_mode.sh already contains the correct embedded content,
// EnsureRuntimeAssets must not modify it (WriteFileAtomic no-op).
func TestEnsureRuntimeAssetsIsNoOpWhenContentMatches(t *testing.T) {
	home := t.TempDir()

	// First call creates the file from the embedded asset.
	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("first EnsureRuntimeAssets() error = %v", err)
	}

	path := RuntimePRModePath(home)
	contentAfterFirst, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Get file mod time to detect if it was re-written.
	stat1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// Second call — should be a no-op because content matches.
	if err := EnsureRuntimeAssets(home); err != nil {
		t.Fatalf("second EnsureRuntimeAssets() error = %v", err)
	}

	contentAfterSecond, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(contentAfterFirst) != string(contentAfterSecond) {
		t.Fatalf("content changed between two calls with identical embedded content")
	}

	stat2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// WriteFileAtomic returns early when content matches, so the file is not
	// replaced and the modification time must not change.
	if stat2.ModTime() != stat1.ModTime() {
		t.Fatalf("EnsureRuntimeAssets re-wrote the file even though content was identical")
	}
}
