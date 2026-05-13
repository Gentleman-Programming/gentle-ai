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

func writeTestArtifacts(t *testing.T, dir string) {
	t.Helper()
	writeTestDB(t, dir)
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(DBPath(dir)+suffix, []byte(suffix), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDataDirService_CopyTo(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestArtifacts(t, src)

	snap, err := svc.CopyTo(src, dst)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(DBPath(src) + suffix); err != nil {
			t.Errorf("source artifact %q removed after CopyTo: %v", suffix, err)
		}
		if _, err := os.Stat(DBPath(dst) + suffix); err != nil {
			t.Errorf("dst artifact %q not created: %v", suffix, err)
		}
	}
}

func TestDataDirService_MoveTo(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestArtifacts(t, src)

	_, err := svc.MoveTo(src, dst)
	if err != nil {
		t.Fatalf("MoveTo: %v", err)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(DBPath(src) + suffix); !os.IsNotExist(err) {
			t.Errorf("source artifact %q should not exist after MoveTo", suffix)
		}
		if _, err := os.Stat(DBPath(dst) + suffix); err != nil {
			t.Errorf("dst artifact %q not created: %v", suffix, err)
		}
	}
}

func TestDataDirService_Delete(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestArtifacts(t, dir)

	snap, err := svc.Delete(dir)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(DBPath(dir) + suffix); !os.IsNotExist(err) {
			t.Errorf("artifact %q should not exist after Delete", suffix)
		}
	}
}

func TestDataDirService_Delete_AlreadyGone(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")

	// Delete on a non-existent DB must not error (os.IsNotExist is tolerated).
	_, err := svc.Delete(dir)
	if err != nil {
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

	_, err := svc.Delete(dir)
	if err == nil {
		t.Fatal("expected error from failing snapshotter, got nil")
	}

	// DB must still exist — no delete when snapshot failed.
	if _, statErr := os.Stat(DBPath(dir)); statErr != nil {
		t.Error("DB was deleted despite snapshot failure")
	}
}
