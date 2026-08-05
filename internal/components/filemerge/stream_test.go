package filemerge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// faultyStagedFile wraps a real staged file and fails one durability step. Real
// *os.File only fails Chmod/Sync/Close under disk-level conditions, which is
// precisely why those branches would otherwise ship unexercised.
type faultyStagedFile struct {
	stagedFile
	failChmod error
	failSync  error
	failClose error
}

func (f *faultyStagedFile) Chmod(mode fs.FileMode) error {
	if f.failChmod != nil {
		return f.failChmod
	}
	return f.stagedFile.Chmod(mode)
}

func (f *faultyStagedFile) Sync() error {
	if f.failSync != nil {
		return f.failSync
	}
	return f.stagedFile.Sync()
}

func (f *faultyStagedFile) Close() error {
	err := f.stagedFile.Close()
	if f.failClose != nil {
		return f.failClose
	}
	return err
}

func faultStagedFile(t *testing.T, fault func(*faultyStagedFile)) {
	t.Helper()
	original := createStagedFile
	t.Cleanup(func() { createStagedFile = original })
	createStagedFile = func(dir string) (stagedFile, error) {
		inner, err := original(dir)
		if err != nil {
			return nil, err
		}
		wrapped := &faultyStagedFile{stagedFile: inner}
		fault(wrapped)
		return wrapped, nil
	}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestWriteStreamAtomicReportsTheDigestOfTheFileOnDisk states the contract the
// download paths depend on: the returned digest describes the destination, so a
// caller comparing it against a release manifest is checking disk.
func TestWriteStreamAtomicReportsTheDigestOfTheFileOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "archive.tar.gz")
	payload := []byte(strings.Repeat("release-archive-bytes", 1024))

	result, err := WriteStreamAtomic(path, strings.NewReader(string(payload)), 0o644)
	if err != nil {
		t.Fatalf("WriteStreamAtomic: %v", err)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if result.Digest != digestOf(onDisk) {
		t.Errorf("Digest = %s, want %s (sha256 of the bytes at %s)", result.Digest, digestOf(onDisk), path)
	}
	if result.Bytes != int64(len(onDisk)) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(onDisk))
	}
	if string(onDisk) != string(payload) {
		t.Error("on-disk content differs from the streamed payload")
	}
}

// TestWriteStreamAtomicPreservesRequestedMode covers the executable case the
// binary installers rely on.
func TestWriteStreamAtomicPreservesRequestedMode(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("mode bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "gentle-ai")

	if _, err := WriteStreamAtomic(path, strings.NewReader("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteStreamAtomic: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
}

// TestDurableWriteFailsOnStagedFileFaults exercises the three durability steps a
// real filesystem only fails when something is genuinely wrong. Each one must
// abort before publication rather than let a partially written file be renamed
// over the destination (#2216).
func TestDurableWriteFailsOnStagedFileFaults(t *testing.T) {
	tests := []struct {
		name    string
		fault   func(*faultyStagedFile)
		wantMsg string
	}{
		{
			name:    "chmod on the staged file fails",
			fault:   func(f *faultyStagedFile) { f.failChmod = errors.New("injected chmod failure") },
			wantMsg: "set permissions on temp file",
		},
		{
			name:    "sync on the staged file fails",
			fault:   func(f *faultyStagedFile) { f.failSync = errors.New("injected sync failure") },
			wantMsg: "sync temp file",
		},
		{
			name:    "close reports a deferred write-back failure",
			fault:   func(f *faultyStagedFile) { f.failClose = errors.New("injected close failure") },
			wantMsg: "close temp file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" (stream)", func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "archive.tar.gz")
			faultStagedFile(t, tt.fault)

			result, err := WriteStreamAtomic(path, strings.NewReader("payload"), 0o644)
			if err == nil {
				t.Fatalf("WriteStreamAtomic reported success (%+v) after %s", result, tt.name)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantMsg)
			}
			if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("destination %s exists after a failed write (stat err = %v)", path, statErr)
			}
			assertNoStagedLeftovers(t, dir)
		})

		t.Run(tt.name+" (bytes)", func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			faultStagedFile(t, tt.fault)

			result, err := WriteFileAtomic(path, []byte("payload"), 0o644)
			if err == nil {
				t.Fatalf("WriteFileAtomic reported success (%+v) after %s", result, tt.name)
			}
			if result.Changed || result.Created {
				t.Errorf("WriteFileAtomic = %+v, want a zero result: nothing was published", result)
			}
			assertNoStagedLeftovers(t, dir)
		})
	}
}

// TestWriteStreamAtomicFailsWhenReplacementDoesNotLand is the streaming half of
// the read-back property: a rename that silently no-ops must not produce a
// digest, because that digest would certify bytes that are not at the
// destination (#1998, #2319).
func TestWriteStreamAtomicFailsWhenReplacementDoesNotLand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")
	stale := []byte("previous archive")
	if err := os.WriteFile(path, stale, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	original := renameFn
	t.Cleanup(func() { renameFn = original })
	renameFn = func(string, string) error { return nil }

	payload := "brand new archive contents"
	result, err := WriteStreamAtomic(path, strings.NewReader(payload), 0o644)
	if err == nil {
		t.Fatalf("WriteStreamAtomic reported success (%+v) for a replacement that never landed", result)
	}
	if result.Digest == digestOf([]byte(payload)) {
		t.Errorf("returned the digest of the stream (%s) for a destination that holds something else", result.Digest)
	}
	if result.Digest != "" {
		t.Errorf("Digest = %q, want empty on failure", result.Digest)
	}
	onDisk, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	if string(onDisk) != string(stale) {
		t.Errorf("destination = %q, want the untouched %q", onDisk, stale)
	}
	assertNoStagedLeftovers(t, dir)
}

// TestWriteStreamAtomicReportsResultWhenParentSyncFails keeps the streaming
// façade honest in the post-publication window: the bytes are there, so the
// result describes them even though the call failed.
func TestWriteStreamAtomicReportsResultWhenParentSyncFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.tar.gz")

	origGOOS := runtimeGOOS
	origSyncDir := syncDirFn
	t.Cleanup(func() {
		runtimeGOOS = origGOOS
		syncDirFn = origSyncDir
	})
	runtimeGOOS = func() string { return "linux" }
	syncDirFn = func(string) error { return errors.New("injected parent-directory sync failure") }

	payload := "archive"
	result, err := WriteStreamAtomic(path, strings.NewReader(payload), 0o644)
	if err == nil {
		t.Fatal("WriteStreamAtomic: want parent-directory sync error, got nil")
	}
	if result.Digest != digestOf([]byte(payload)) {
		t.Errorf("Digest = %q, want %s: the bytes are on disk, so the result must describe them", result.Digest, digestOf([]byte(payload)))
	}
}

// TestWriteStreamAtomicPropagatesSourceErrors keeps a failing producer from
// publishing a truncated destination.
func TestWriteStreamAtomicPropagatesSourceErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive.tar.gz")

	src := io.MultiReader(strings.NewReader("half"), errReader{errors.New("injected read failure")})
	if _, err := WriteStreamAtomic(path, src, 0o644); err == nil {
		t.Fatal("WriteStreamAtomic: want the source error, got nil")
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("destination %s exists after a failed copy (stat err = %v)", path, statErr)
	}
	assertNoStagedLeftovers(t, dir)
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func assertNoStagedLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gentle-ai-") {
			t.Errorf("staged temp file %q was left behind", entry.Name())
		}
	}
}
