package filecoord

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReadSnapshotNonExistent(t *testing.T) {
	dir := canonicalBackendTempDir(t)
	target := filepath.Join(dir, "nonexistent.txt")

	snap, err := ReadSnapshot(target)
	if err != nil {
		t.Fatalf("ReadSnapshot(nonexistent) = %v, want nil", err)
	}
	if snap == nil || snap.Exists {
		t.Fatalf("ReadSnapshot(nonexistent) Exists = %v, want false", snap)
	}
}

func TestReadSnapshotRegularFile(t *testing.T) {
	dir := canonicalBackendTempDir(t)
	target := filepath.Join(dir, "regular.txt")
	content := []byte("hello cooperative snapshot\n")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}

	snap1, err := ReadSnapshot(target)
	if err != nil {
		t.Fatalf("ReadSnapshot(regular) = %v", err)
	}
	if !snap1.Exists {
		t.Fatal("snap1.Exists = false, want true")
	}
	if string(snap1.Bytes) != string(content) {
		t.Fatalf("snap1.Bytes = %q, want %q", snap1.Bytes, content)
	}
	if snap1.Identity.Size != int64(len(content)) {
		t.Fatalf("snap1.Identity.Size = %d, want %d", snap1.Identity.Size, len(content))
	}

	snap2, err := ReadSnapshot(target)
	if err != nil {
		t.Fatalf("second ReadSnapshot() = %v", err)
	}
	if !snap1.Identity.Matches(snap2.Identity) {
		t.Fatalf("snap1.Identity (%+v) does not match snap2.Identity (%+v)", snap1.Identity, snap2.Identity)
	}

	// Mutate content and verify identity changes
	if err := os.WriteFile(target, []byte("mutated bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap3, err := ReadSnapshot(target)
	if err != nil {
		t.Fatalf("third ReadSnapshot() = %v", err)
	}
	if snap1.Identity.Matches(snap3.Identity) {
		t.Fatal("snap1.Identity matches snap3.Identity after file mutation")
	}
}

func TestReadSnapshotRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures unavailable on windows")
	}
	dir := canonicalBackendTempDir(t)
	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("real content"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(dir, "link.txt")
	if err := os.Symlink(realFile, symlink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := ReadSnapshot(symlink)
	if !errors.Is(err, ErrSymlinkTarget) {
		t.Fatalf("ReadSnapshot(symlink) = %v, want ErrSymlinkTarget", err)
	}
}

func TestReadSnapshotRejectsDirectory(t *testing.T) {
	dir := canonicalBackendTempDir(t)
	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ReadSnapshot(subDir)
	if !errors.Is(err, ErrNonRegularTarget) {
		t.Fatalf("ReadSnapshot(directory) = %v, want ErrNonRegularTarget", err)
	}
}

func TestReadSnapshotRejectsInvalidTarget(t *testing.T) {
	_, err := ReadSnapshot("")
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("ReadSnapshot(\"\") = %v, want ErrInvalidTarget", err)
	}
}
