package engram

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

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

func writeTestDB(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := DBPath(dir)
	if err := os.WriteFile(path, []byte("fake-sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDataDirService_CopyTo(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestDB(t, src)

	snap, err := svc.CopyTo(src, dst)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}
	if _, err := os.Stat(DBPath(src)); err != nil {
		t.Errorf("source DB removed after CopyTo: %v", err)
	}
	if _, err := os.Stat(DBPath(dst)); err != nil {
		t.Errorf("dst DB not created: %v", err)
	}
}

func TestDataDirService_MoveTo(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestDB(t, src)

	if _, err := svc.MoveTo(src, dst); err != nil {
		t.Fatalf("MoveTo: %v", err)
	}
	if _, err := os.Stat(DBPath(src)); !os.IsNotExist(err) {
		t.Error("source DB should not exist after MoveTo")
	}
	if _, err := os.Stat(DBPath(dst)); err != nil {
		t.Errorf("dst DB not created: %v", err)
	}
}

func TestDataDirService_Delete(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)

	snap, err := svc.Delete(dir)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}
	if _, err := os.Stat(DBPath(dir)); !os.IsNotExist(err) {
		t.Error("DB should not exist after Delete")
	}
}

func TestDataDirService_Delete_AlreadyGone(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")

	if _, err := svc.Delete(dir); err != nil {
		t.Fatalf("Delete on missing DB: %v", err)
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
	writeTestDB(t, dir)

	if _, err := svc.Delete(dir); err == nil {
		t.Fatal("expected error from failing snapshotter, got nil")
	}
	if _, statErr := os.Stat(DBPath(dir)); statErr != nil {
		t.Error("DB was deleted despite snapshot failure")
	}
}

func TestDataDirService_DiskSpaceOK(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	dbPath := writeTestDB(t, dir)

	ok, needed, avail, err := svc.DiskSpaceOK(dbPath, home)
	if err != nil {
		t.Fatalf("DiskSpaceOK: %v", err)
	}
	if needed <= 0 {
		t.Errorf("needed = %d, want > 0", needed)
	}
	if avail <= 0 {
		t.Errorf("avail = %d, want > 0", avail)
	}
	if !ok {
		t.Errorf("DiskSpaceOK = false for tiny test file, expected true")
	}
}
