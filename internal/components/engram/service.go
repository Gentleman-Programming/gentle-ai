package engram

import (
	"errors"
	"fmt"
	"io/fs"
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

// CopyTo copies all files from currentDir to dst without removing source.
func (s DataDirService) CopyTo(currentDir, dst string) (backup.Manifest, error) {
	snap, err := s.snapshot(currentDir)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before copy: %w", err)
	}
	if err := copyDataDir(currentDir, dst); err != nil {
		return snap, fmt.Errorf("copy: %w", err)
	}
	return snap, nil
}

// MoveTo copies all files from currentDir to dst, then removes the source files.
func (s DataDirService) MoveTo(currentDir, dst string) (backup.Manifest, error) {
	snap, err := s.CopyTo(currentDir, dst)
	if err != nil {
		return snap, err
	}
	if err := clearDataDir(currentDir); err != nil {
		return snap, fmt.Errorf("remove source after move: %w", err)
	}
	return snap, nil
}

// Delete creates a snapshot of dataDir, then removes all its files.
func (s DataDirService) Delete(dataDir string) (backup.Manifest, error) {
	snap, err := s.snapshot(dataDir)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before delete: %w", err)
	}
	if err := clearDataDir(dataDir); err != nil {
		return snap, fmt.Errorf("delete: %w", err)
	}
	return snap, nil
}

func (s DataDirService) snapshot(dataDir string) (backup.Manifest, error) {
	if err := os.MkdirAll(s.backupRoot, 0o755); err != nil {
		return backup.Manifest{}, fmt.Errorf("create backup root %q: %w", s.backupRoot, err)
	}
	snapshotDir := filepath.Join(s.backupRoot, time.Now().UTC().Format("20060102150405.000000000"))
	return s.snapshotter.Create(snapshotDir, dataDirFiles(dataDir))
}

// copyDataDir recursively copies all files from src to dst.
func copyDataDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)
	if src == "." || dst == "." {
		return fmt.Errorf("source and destination data directories are required")
	}
	if samePath(src, dst) {
		return fmt.Errorf("source and destination data directories must be different")
	}
	if DataDirHasContent(dst) {
		return fmt.Errorf("destination data directory already contains files: %q", dst)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		return CopyDB(path, dstPath)
	})
}

// clearDataDir removes all entries under dataDir, leaving the directory itself.
// All errors are collected and returned together.
func clearDataDir(dataDir string) error {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var errs []error
	for _, e := range entries {
		p := filepath.Join(dataDir, e.Name())
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		a, b = absA, absB
	}
	infoA, statA := os.Stat(a)
	infoB, statB := os.Stat(b)
	if statA == nil && statB == nil && os.SameFile(infoA, infoB) {
		return true
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
