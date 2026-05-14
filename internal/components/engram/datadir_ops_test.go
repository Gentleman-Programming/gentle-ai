package engram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataDirHasContent_emptyDir(t *testing.T) {
	dir := t.TempDir()
	if DataDirHasContent(dir) {
		t.Error("DataDirHasContent = true for empty dir, want false")
	}
}

func TestDataDirHasContent_dirWithFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "engram.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !DataDirHasContent(dir) {
		t.Error("DataDirHasContent = false for dir with file, want true")
	}
}

func TestDataDirHasContent_nonExistentDir(t *testing.T) {
	if DataDirHasContent(filepath.Join(t.TempDir(), "nonexistent")) {
		t.Error("DataDirHasContent = true for non-existent dir, want false")
	}
}

func TestDataDirSize_knownFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("hello world") // 11 bytes
	if err := os.WriteFile(filepath.Join(dir, "file.db"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	got := DataDirSize(dir)
	if got != int64(len(content)) {
		t.Errorf("DataDirSize = %d, want %d", got, len(content))
	}
}

func TestDataDirSize_emptyDir(t *testing.T) {
	dir := t.TempDir()
	if got := DataDirSize(dir); got != 0 {
		t.Errorf("DataDirSize = %d, want 0 for empty dir", got)
	}
}

func TestDiskSpaceOKForDataDir_emptyDir(t *testing.T) {
	dir := t.TempDir()
	ok, needed, avail, err := DiskSpaceOKForDataDir(dir, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || needed != 0 || avail != 0 {
		t.Fatalf("got ok=%v needed=%d avail=%d, want ok=true needed=0 avail=0", ok, needed, avail)
	}
}

func TestDiskSpaceOKForDataDir_withContent(t *testing.T) {
	src := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "engram.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	ok, needed, avail, err := DiskSpaceOKForDataDir(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if needed != 1 {
		t.Errorf("needed = %d, want 1", needed)
	}
	if avail <= 0 {
		t.Errorf("avail = %d, want > 0", avail)
	}
	if !ok {
		t.Fatal("expected ok=true for one-byte file on same volume")
	}
}

func TestCopyDataDir_RejectsSameSourceAndDestination(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.bin"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := copyDataDir(dir, dir)
	if err == nil || !strings.Contains(err.Error(), "must be different") {
		t.Fatalf("copyDataDir same dir error = %v, want 'must be different'", err)
	}
}

func TestCopyDataDir_RejectsPopulatedDestination(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(home, "src")
	dst := filepath.Join(home, "dst")
	for _, dir := range []string{src, dst} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "data.bin"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "data.bin"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := copyDataDir(src, dst)
	if err == nil || !strings.Contains(err.Error(), "already contains files") {
		t.Fatalf("copyDataDir populated dst error = %v, want 'already contains files'", err)
	}
	// Destination must be untouched.
	got, readErr := os.ReadFile(filepath.Join(dst, "data.bin"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "existing" {
		t.Fatalf("destination was modified: %q", got)
	}
}
