package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// snapshotter is a subset of backup.Snapshotter used for testability.
type snapshotter interface {
	Create(snapshotDir string, paths []string) (backup.Manifest, error)
}

// DataDirService provides safe, snapshot-guarded operations on the Engram
// data directory. Every mutating operation creates a backup before changing
// anything; the snapshot is visible in the Backups TUI screen.
type DataDirService struct {
	homeDir     string
	backupRoot  string
	snapshotter snapshotter
}

// NewDataDirService creates a DataDirService with the default backup.Snapshotter.
func NewDataDirService(homeDir string) DataDirService {
	return DataDirService{
		homeDir:     homeDir,
		backupRoot:  filepath.Join(homeDir, ".gentle-ai", "backups"),
		snapshotter: backup.NewSnapshotter(),
	}
}

// CopyTo copies the Engram DB from currentDir to dst without removing the source.
// A snapshot of the source DB is created first. Returns the snapshot manifest.
func (s DataDirService) CopyTo(currentDir, dst string) (backup.Manifest, error) {
	srcDB := DBPath(currentDir)
	snap, err := s.snapshot(srcDB)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before copy: %w", err)
	}
	if err := CopyDB(srcDB, DBPath(dst)); err != nil {
		return snap, fmt.Errorf("copy: %w", err)
	}
	return snap, nil
}

// MoveTo copies the Engram DB from currentDir to dst, then removes the source DB.
// The source is only removed after the copy is verified. Returns the snapshot manifest.
func (s DataDirService) MoveTo(currentDir, dst string) (backup.Manifest, error) {
	snap, err := s.CopyTo(currentDir, dst)
	if err != nil {
		return snap, err
	}
	if err := os.Remove(DBPath(currentDir)); err != nil && !os.IsNotExist(err) {
		return snap, fmt.Errorf("remove source after move: %w", err)
	}
	return snap, nil
}

// Delete creates a snapshot of the current DB, then removes it.
// Returns the snapshot manifest so the caller can show the backup ID.
func (s DataDirService) Delete(dataDir string) (backup.Manifest, error) {
	dbPath := DBPath(dataDir)
	snap, err := s.snapshot(dbPath)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before delete: %w", err)
	}
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return snap, fmt.Errorf("delete: %w", err)
	}
	return snap, nil
}

// DiskSpaceOK reports whether the volume at dst has enough free space to hold
// a copy of the single DB file at srcDB. For copying all SQLite artifacts (DB+WAL+SHM),
// use DiskSpaceOKForSQLiteArtifacts instead.
func (s DataDirService) DiskSpaceOK(srcDB, dst string) (bool, int64, int64, error) {
	info, err := os.Stat(srcDB)
	if err != nil {
		return false, 0, 0, fmt.Errorf("stat source DB %q: %w", srcDB, err)
	}
	avail, err := storage.AvailableBytes(dst)
	if err != nil {
		return false, 0, 0, fmt.Errorf("check available space at %q: %w", dst, err)
	}
	needed := info.Size()
	return avail > needed, needed, avail, nil
}

// snapshot creates a timestamped backup of dbPath under the backup root.
func (s DataDirService) snapshot(dbPath string) (backup.Manifest, error) {
	if err := os.MkdirAll(s.backupRoot, 0o755); err != nil {
		return backup.Manifest{}, fmt.Errorf("create backup root %q: %w", s.backupRoot, err)
	}
	snapshotDir := filepath.Join(s.backupRoot, time.Now().UTC().Format("20060102150405.000000000"))
	return s.snapshotter.Create(snapshotDir, []string{dbPath})
}
