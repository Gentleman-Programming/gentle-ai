package engram

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
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

// CopyTo copies all existing Engram SQLite artifacts from currentDir to dst
// without removing the source. A snapshot is created first.
func (s DataDirService) CopyTo(currentDir, dst string) (backup.Manifest, error) {
	snap, err := s.snapshot(currentDir)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before copy: %w", err)
	}
	if err := CopySQLiteArtifacts(currentDir, dst); err != nil {
		return snap, fmt.Errorf("copy: %w", err)
	}
	return snap, nil
}

// MoveTo copies all existing Engram SQLite artifacts from currentDir to dst,
// then removes the source artifacts. The source is only removed after the copy succeeds.
func (s DataDirService) MoveTo(currentDir, dst string) (backup.Manifest, error) {
	snap, err := s.CopyTo(currentDir, dst)
	if err != nil {
		return snap, err
	}
	if err := RemoveSQLiteArtifacts(currentDir); err != nil {
		return snap, fmt.Errorf("remove source after move: %w", err)
	}
	return snap, nil
}

// Delete creates a snapshot of current SQLite artifacts, then removes them.
func (s DataDirService) Delete(dataDir string) (backup.Manifest, error) {
	snap, err := s.snapshot(dataDir)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before delete: %w", err)
	}
	if err := RemoveSQLiteArtifacts(dataDir); err != nil {
		return snap, fmt.Errorf("delete: %w", err)
	}
	return snap, nil
}

// snapshot creates a timestamped backup of the data directory's SQLite artifacts.
func (s DataDirService) snapshot(dataDir string) (backup.Manifest, error) {
	if err := os.MkdirAll(s.backupRoot, 0o755); err != nil {
		return backup.Manifest{}, fmt.Errorf("create backup root %q: %w", s.backupRoot, err)
	}
	snapshotDir := filepath.Join(s.backupRoot, time.Now().UTC().Format("20060102150405.000000000"))
	return s.snapshotter.Create(snapshotDir, SQLiteArtifactPaths(dataDir))
}
