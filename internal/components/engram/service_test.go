package engram

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
)

type stubSnapshotter struct {
	err       error
	manifest  backup.Manifest
	seenPaths *[]string
}

func (s stubSnapshotter) Create(_ string, paths []string) (backup.Manifest, error) {
	if s.seenPaths != nil {
		*s.seenPaths = append((*s.seenPaths)[:0], paths...)
	}
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

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
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

func TestDataDirService_CopyTo_CopiesDirectoryFiles(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestDB(t, src)
	writeTestFile(t, filepath.Join(src, "engram.db-wal"), "wal")
	writeTestFile(t, filepath.Join(src, "engram.db-shm"), "shm")

	if _, err := svc.CopyTo(src, dst); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}

	for _, rel := range []string{"engram.db", "engram.db-wal", "engram.db-shm"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("copied file %q missing: %v", rel, err)
		}
	}
}

func TestDataDirService_CopyTo_SnapshotsOnlyEngramFiles(t *testing.T) {
	home := t.TempDir()
	var seen []string
	svc := DataDirService{
		homeDir:     home,
		backupRoot:  filepath.Join(home, ".gentle-ai", "backups"),
		snapshotter: stubSnapshotter{manifest: backup.Manifest{ID: "snap-abc123"}, seenPaths: &seen},
	}
	src := filepath.Join(home, "src-data")
	dst := filepath.Join(home, "dst-data")
	writeTestDB(t, src)
	writeTestFile(t, filepath.Join(src, "engram.db-wal"), "wal")
	writeTestFile(t, filepath.Join(src, "engram.db-shm"), "shm")
	writeTestFile(t, filepath.Join(src, "notes.txt"), "ignore")

	if _, err := svc.CopyTo(src, dst); err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	want := map[string]bool{
		DBPath(src):                         true,
		filepath.Join(src, "engram.db-wal"): true,
		filepath.Join(src, "engram.db-shm"): true,
	}
	if len(seen) != len(want) {
		t.Fatalf("snapshot paths = %v, want %d Engram files", seen, len(want))
	}
	for _, path := range seen {
		if !want[path] {
			t.Fatalf("snapshot path %q should not be included; got %v", path, seen)
		}
	}
}

func TestDataDirService_CopyTo_RejectsSameDir(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)

	if _, err := svc.CopyTo(dir, dir); err == nil {
		t.Fatal("CopyTo same dir error = nil, want error")
	}
}

func TestDataDirService_CopyTo_RejectsDestinationInsideSource(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "data")
	dst := filepath.Join(src, "nested-copy")
	writeTestDB(t, src)

	if _, err := svc.CopyTo(src, dst); err == nil {
		t.Fatal("CopyTo nested destination error = nil, want error")
	}
}

func TestDataDirService_CopyTo_RejectsNonEmptyDestination(t *testing.T) {
	svc, home := newTestService(t)
	src := filepath.Join(home, "src")
	dst := filepath.Join(home, "dst")
	writeTestDB(t, src)
	writeTestFile(t, filepath.Join(dst, "stale"), "old")

	if _, err := svc.CopyTo(src, dst); err == nil {
		t.Fatal("CopyTo non-empty destination error = nil, want error")
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

func TestDataDirService_MoveTo_RejectsSameDir(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)

	if _, err := svc.MoveTo(dir, dir); err == nil {
		t.Fatal("MoveTo same dir error = nil, want error")
	}
	if _, err := os.Stat(DBPath(dir)); err != nil {
		t.Fatalf("source DB should remain after rejected move: %v", err)
	}
}

func TestDataDirService_Delete(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)
	writeTestFile(t, filepath.Join(dir, "engram.db-wal"), "wal")

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
	if _, err := os.Stat(filepath.Join(dir, "engram.db-wal")); !os.IsNotExist(err) {
		t.Error("WAL should not exist after Delete")
	}
}

func TestDataDirService_Delete_PreservesUnrelatedFiles(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "keep me")

	if _, err := svc.Delete(dir); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("unrelated file should remain after Delete: %v", err)
	}
}

func TestDataDirService_Delete_PreservesArtifactNamedDirectories(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)
	if err := os.Mkdir(filepath.Join(dir, "engram.db-wal"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Delete(dir); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "engram.db-wal")); err != nil || !info.IsDir() {
		t.Fatalf("artifact-named directory should remain after Delete, info=%v err=%v", info, err)
	}
}

func TestDataDirService_Delete_AlreadyGone(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")

	if _, err := svc.Delete(dir); err != nil {
		t.Fatalf("Delete on missing DB: %v", err)
	}
}

func TestDataDirService_SnapshotStable(t *testing.T) {
	svc, home := newTestService(t)
	dir := filepath.Join(home, "data")
	writeTestDB(t, dir)

	snap, err := svc.SnapshotStable(dir)
	if err != nil {
		t.Fatalf("SnapshotStable: %v", err)
	}
	if snap.ID != "snap-abc123" {
		t.Errorf("snap.ID = %q, want snap-abc123", snap.ID)
	}
	if _, err := os.Stat(DBPath(dir)); err != nil {
		t.Fatalf("SnapshotStable should not remove DB: %v", err)
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
