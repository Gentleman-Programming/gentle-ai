package engram

import (
	"os"
	"path/filepath"
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

func TestCopyDB_Idempotent(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engram.db")
	dst := filepath.Join(t.TempDir(), "engram.db")
	content := []byte("idempotent copy test")

	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := CopyDB(src, dst); err != nil {
			t.Fatalf("CopyDB iteration %d: %v", i, err)
		}
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch after second copy")
	}
}
