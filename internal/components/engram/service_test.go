package engram

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

// stubSnapshotter satisfies the snapshotter interface without touching disk.
type stubSnapshotter struct {
	err      error
	manifest backup.Manifest
}

func (s stubSnapshotter) Create(_ string, _ []string) (backup.Manifest, error) {
	return s.manifest, s.err
}

func newTestService(t *testing.T) (DataDirService, string) {
	t.Helper()
	home := t.TempDir()
	svc := DataDirService{
		homeDir:     home,
		backupRoot:  filepath.Join(home, ".gentle-ai", "backups"),
		snapshotter: stubSnapshotter{manifest: backup.Manifest{ID: "snap-abc123"}},
	}
	return svc, home
}

// writeTestDataDir writes a small file into dir to simulate a populated data directory.
func writeTestDataDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "engram.db"), []byte("fake-data"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDataDirService_CopyTo(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestDataDir(t, src)

	snap, err := svc.CopyTo(src, dst)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}

	// Source must still exist.
	if !DataDirHasContent(src) {
		t.Error("source dir empty after CopyTo — expected files to remain")
	}
	// Destination must contain the copied file.
	if !DataDirHasContent(dst) {
		t.Error("dst dir empty after CopyTo — expected files to be copied")
	}
}

func TestDataDirService_MoveTo(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestDataDir(t, src)

	_, err := svc.MoveTo(src, dst)
	if err != nil {
		t.Fatalf("MoveTo: %v", err)
	}

	// Source must be empty after move.
	if DataDirHasContent(src) {
		t.Error("source dir not empty after MoveTo")
	}
	// Destination must have the file.
	if !DataDirHasContent(dst) {
		t.Error("dst dir empty after MoveTo — expected files to be moved")
	}
}

func TestDataDirService_Delete(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDataDir(t, dir)

	snap, err := svc.Delete(dir)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}

	if DataDirHasContent(dir) {
		t.Error("dir not empty after Delete")
	}
}

func TestDataDirService_Delete_AlreadyGone(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")

	// Delete on a non-existent dir must not error.
	_, err := svc.Delete(dir)
	if err != nil {
		t.Fatalf("Delete on missing dir: %v", err)
	}
}

func TestDataDirService_SnapshotError_Propagates(t *testing.T) {
	home := t.TempDir()
	svc := DataDirService{
		homeDir:     home,
		backupRoot:  filepath.Join(home, ".gentle-ai", "backups"),
		snapshotter: stubSnapshotter{err: errors.New("disk full")},
	}

	dir := filepath.Join(home, "data")
	writeTestDataDir(t, dir)

	_, err := svc.Delete(dir)
	if err == nil {
		t.Fatal("expected error from failing snapshotter, got nil")
	}

	// Dir must still have content — no delete when snapshot failed.
	if !DataDirHasContent(dir) {
		t.Error("dir was cleared despite snapshot failure")
	}
}
