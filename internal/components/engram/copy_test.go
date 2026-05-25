package engram

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCopyDB_Basic(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "engram.db")

	content := []byte("fake sqlite database content for testing")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDB(src, dst); err != nil {
		t.Fatalf("CopyDB: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestCopyDB_CreatesDestDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "subdir", "deep", "engram.db")

	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDB(src, dst); err != nil {
		t.Fatalf("CopyDB: %v", err)
	}

	if _, err := os.Stat(dst); err != nil {
		t.Errorf("dst not created: %v", err)
	}
}

func TestCopyDB_NoTempOnSuccess(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "engram.db")

	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyDB(src, dst); err != nil {
		t.Fatalf("CopyDB: %v", err)
	}

	tmp := dst + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp file %q should not exist after success", tmp)
	}
}

func TestCopyDB_SourceNotExist(t *testing.T) {
	src := filepath.Join(t.TempDir(), "nonexistent.db")
	dst := filepath.Join(t.TempDir(), "engram.db")

	err := CopyDB(src, dst)
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestCopyDB_OverwritesExistingDestination(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "engram.db")
	content := []byte("fresh sqlite content")

	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("stale content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyDB(src, dst); err != nil {
		t.Fatalf("CopyDB: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("destination content = %q, want %q", got, content)
	}
}

func TestCopyDB_PreservesSourcePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "engram.db")

	if err := os.WriteFile(src, []byte("private memory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CopyDB(src, dst); err != nil {
		t.Fatalf("CopyDB: %v", err)
	}

	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("dst mode = %o, want 0600", got)
	}
}

func TestCopyDB_RejectsSQLiteSidecars(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "engram.db")

	if err := os.WriteFile(src, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src+"-wal", []byte("pending writes"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := CopyDB(src, dst)
	if err == nil {
		t.Fatal("expected error for live SQLite sidecar, got nil")
	}
	if !strings.Contains(err.Error(), "quiesced SQLite database") {
		t.Fatalf("error = %q, want quiesced SQLite message", err)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("dst should not exist after sidecar rejection, stat err = %v", statErr)
	}
}
