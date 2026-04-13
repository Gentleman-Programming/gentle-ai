package filemerge

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomicReadOnlyDirRelaxesOwnerWritePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 555 semantics differ on Windows")
	}
	base := t.TempDir()
	skillDir := filepath.Join(base, "sdd-init")
	if err := os.Mkdir(skillDir, 0o555); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	path := filepath.Join(skillDir, "SKILL.md")
	content := []byte("# SDD Init\n")

	_, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() error = %v, want successful write with permission relaxation", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile() error = %v", readErr)
	}
	if string(got) != string(content) {
		t.Fatalf("file content = %q, want %q", string(got), string(content))
	}
}

func TestWriteFileAtomicCreatesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json")
	content := []byte("{\"ok\":true}\n")

	first, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() first write error = %v", err)
	}

	if !first.Changed || !first.Created {
		t.Fatalf("WriteFileAtomic() first write result = %+v", first)
	}

	second, err := WriteFileAtomic(path, content, 0o644)
	if err != nil {
		t.Fatalf("WriteFileAtomic() second write error = %v", err)
	}

	if second.Changed || second.Created {
		t.Fatalf("WriteFileAtomic() second write result = %+v", second)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(got) != string(content) {
		t.Fatalf("file content = %q", string(got))
	}
}

func TestWriteFileAtomicRejectsExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(target) error = %v", err)
	}
	path := filepath.Join(dir, "linked.txt")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := WriteFileAtomic(path, []byte("new\n"), 0o644)
	if err == nil || err.Error() == "" {
		t.Fatalf("WriteFileAtomic(symlink) error = %v, want rejection", err)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "old\n" {
		t.Fatalf("target content changed through symlink: got %q err=%v", got, readErr)
	}
}

func TestWriteFileAtomicRejectsOversizedExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	data := make([]byte, maxAtomicFileSize+1)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(big) error = %v", err)
	}

	_, err := WriteFileAtomic(path, []byte("small\n"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic(big) error = nil, want max-size rejection")
	}
}

func TestWriteFileAtomicRejectsSymlinkParentDirectory(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("Mkdir(realDir) error = %v", err)
	}
	linkDir := filepath.Join(base, "linked")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("Symlink(linkDir) error = %v", err)
	}

	path := filepath.Join(linkDir, "config.txt")
	_, err := WriteFileAtomic(path, []byte("value\n"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic() error = nil, want symlink parent rejection")
	}
}
