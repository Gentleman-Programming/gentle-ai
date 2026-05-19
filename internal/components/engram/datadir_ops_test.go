package engram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirHasContent(t *testing.T) {
	dir := t.TempDir()
	if DataDirHasContent(dir) {
		t.Fatal("empty dir should not have content")
	}
	if err := os.WriteFile(filepath.Join(dir, "engram.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !DataDirHasContent(dir) {
		t.Fatal("dir with file should have content")
	}
}

func TestDataDirSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b"), []byte("de"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DataDirSize(dir); got != 5 {
		t.Errorf("DataDirSize = %d, want 5", got)
	}
}

func TestDiskSpaceOKForDataDir_EmptySource(t *testing.T) {
	ok, needed, avail, err := DiskSpaceOKForDataDir(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("DiskSpaceOKForDataDir: %v", err)
	}
	if !ok || needed != 0 || avail != 0 {
		t.Fatalf("got ok=%v needed=%d avail=%d, want true/0/0", ok, needed, avail)
	}
}
