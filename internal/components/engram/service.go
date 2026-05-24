package engram

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

type snapshotter interface {
	Create(snapshotDir string, paths []string) (backup.Manifest, error)
}

// DataDirService provides snapshot-guarded operations on the Engram data dir.
type DataDirService struct {
	homeDir     string
	backupRoot  string
	snapshotter snapshotter
}

var dataDirStabilityDelay = 100 * time.Millisecond

// NewDataDirService creates a DataDirService with the default snapshotter.
func NewDataDirService(homeDir string) DataDirService {
	return DataDirService{
		homeDir:     homeDir,
		backupRoot:  filepath.Join(homeDir, ".gentle-ai", "backups"),
		snapshotter: backup.NewSnapshotter(),
	}
}

// CopyTo snapshots the current data directory, then copies it to dst.
func (s DataDirService) CopyTo(currentDir, dst string) (backup.Manifest, error) {
	if samePath(currentDir, dst) {
		return backup.Manifest{}, fmt.Errorf("copy destination is the current Engram data directory: %q", dst)
	}
	if pathInside(currentDir, dst) {
		return backup.Manifest{}, fmt.Errorf("copy destination %q is inside the current Engram data directory %q", dst, currentDir)
	}
	if err := requireEmptyDestination(dst); err != nil {
		return backup.Manifest{}, err
	}
	if ok, needed, avail, err := DiskSpaceOKForDataDir(currentDir, dst); err != nil {
		return backup.Manifest{}, fmt.Errorf("check copy destination free space: %w", err)
	} else if !ok {
		return backup.Manifest{}, fmt.Errorf("copy destination %q has insufficient free space: need %d bytes, available %d bytes", dst, needed, avail)
	}
	snap, err := s.SnapshotStable(currentDir)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before copy: %w", err)
	}
	if err := copyDataDir(currentDir, dst); err != nil {
		return snap, fmt.Errorf("copy: %w", err)
	}
	return snap, nil
}

// MoveTo snapshots the current data directory, copies it to dst, then removes the source.
func (s DataDirService) MoveTo(currentDir, dst string) (backup.Manifest, error) {
	if samePath(currentDir, dst) {
		return backup.Manifest{}, fmt.Errorf("move destination is the current Engram data directory: %q", dst)
	}
	snap, err := s.CopyTo(currentDir, dst)
	if err != nil {
		return snap, err
	}
	if err := s.RemoveSource(currentDir); err != nil {
		return snap, fmt.Errorf("remove source after move: %w", err)
	}
	return snap, nil
}

// RemoveSource removes only Engram-owned SQLite artifacts from a source dir.
func (s DataDirService) RemoveSource(currentDir string) error {
	return removeEngramFiles(currentDir)
}

// Delete snapshots the current data directory, then removes it.
func (s DataDirService) Delete(dataDir string) (backup.Manifest, error) {
	snap, err := s.SnapshotStable(dataDir)
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("snapshot before delete: %w", err)
	}
	if err := removeEngramFiles(dataDir); err != nil {
		return snap, fmt.Errorf("delete: %w", err)
	}
	return snap, nil
}

// Snapshot creates a backup manifest without mutating data files.
func (s DataDirService) Snapshot(dataDir string) (backup.Manifest, error) {
	return s.snapshot(dataDir)
}

// SnapshotStable creates a backup manifest after verifying the data dir is not changing.
func (s DataDirService) SnapshotStable(dataDir string) (backup.Manifest, error) {
	if err := ensureDataDirStable(dataDir); err != nil {
		return backup.Manifest{}, err
	}
	return s.snapshot(dataDir)
}

// DiskSpaceOK reports whether dst has enough free space for srcDB.
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

func (s DataDirService) snapshot(dataDir string) (backup.Manifest, error) {
	if err := os.MkdirAll(s.backupRoot, 0o755); err != nil {
		return backup.Manifest{}, fmt.Errorf("create backup root %q: %w", s.backupRoot, err)
	}
	snapshotDir := filepath.Join(s.backupRoot, time.Now().UTC().Format("20060102150405.000000000"))
	paths, err := regularFiles(dataDir)
	if err != nil {
		return backup.Manifest{}, err
	}
	return s.snapshotter.Create(snapshotDir, paths)
}

func copyDataDir(srcDir, dstDir string) error {
	before, err := engramFileSignatures(srcDir)
	if err != nil {
		return err
	}
	var copied []string
	err = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isEngramDataFile(srcDir, path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dstDir, rel)
		if err := copyFile(path, dst, info.Mode()); err != nil {
			for _, copiedPath := range copied {
				_ = os.Remove(copiedPath)
			}
			return err
		}
		copied = append(copied, dst)
		return nil
	})
	if err != nil {
		for _, copiedPath := range copied {
			_ = os.Remove(copiedPath)
		}
		return err
	}
	after, err := engramFileSignatures(srcDir)
	if err != nil {
		for _, copiedPath := range copied {
			_ = os.Remove(copiedPath)
		}
		return err
	}
	if !sameFileSignatures(before, after) {
		for _, copiedPath := range copied {
			_ = os.Remove(copiedPath)
		}
		return fmt.Errorf("Engram data directory changed while copying; stop Engram and retry")
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func regularFiles(dataDir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dataDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !isEngramDataFile(dataDir, path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return paths, err
}

type fileSignature struct {
	size    int64
	modTime int64
}

func ensureDataDirStable(dataDir string) error {
	if dataDirStabilityDelay <= 0 {
		return nil
	}
	before, err := engramFileSignatures(dataDir)
	if err != nil {
		return err
	}
	time.Sleep(dataDirStabilityDelay)
	after, err := engramFileSignatures(dataDir)
	if err != nil {
		return err
	}
	if !sameFileSignatures(before, after) {
		return fmt.Errorf("Engram data directory is changing; stop Engram and retry")
	}
	return nil
}

func engramFileSignatures(dataDir string) (map[string]fileSignature, error) {
	paths, err := regularFiles(dataDir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]fileSignature, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return nil, err
		}
		out[rel] = fileSignature{size: info.Size(), modTime: info.ModTime().UnixNano()}
	}
	return out, nil
}

func sameFileSignatures(a, b map[string]fileSignature) bool {
	if len(a) != len(b) {
		return false
	}
	for path, sig := range a {
		if b[path] != sig {
			return false
		}
	}
	return true
}

func removeEngramFiles(dataDir string) error {
	var paths []string
	for _, name := range []string{"engram.db", "engram.db-wal", "engram.db-shm"} {
		path := filepath.Join(dataDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil
	}
	trashDir, err := os.MkdirTemp(dataDir, ".gentle-ai-delete-")
	if err != nil {
		return err
	}
	renamed := make(map[string]string, len(paths))
	for _, path := range paths {
		dst := filepath.Join(trashDir, filepath.Base(path))
		if err := os.Rename(path, dst); err != nil {
			restoreRenamedFiles(renamed)
			_ = os.RemoveAll(trashDir)
			return err
		}
		renamed[dst] = path
	}
	_ = os.RemoveAll(trashDir)
	return nil
}

func restoreRenamedFiles(files map[string]string) {
	for tmp, original := range files {
		_ = os.Rename(tmp, original)
	}
}

func requireEmptyDestination(dst string) error {
	entries, err := os.ReadDir(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read copy destination %q: %w", dst, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("copy destination %q already exists and is not empty", dst)
	}
	return nil
}

func isEngramDataFile(dataDir, path string) bool {
	rel, err := filepath.Rel(dataDir, path)
	if err != nil || rel == "." || strings.Contains(rel, string(filepath.Separator)) {
		return false
	}
	switch rel {
	case "engram.db", "engram.db-wal", "engram.db-shm":
		return true
	default:
		return false
	}
}

func samePath(a, b string) bool {
	aAbs, aErr := filepath.Abs(filepath.Clean(a))
	bAbs, bErr := filepath.Abs(filepath.Clean(b))
	if aErr == nil {
		a = aAbs
	}
	if bErr == nil {
		b = bAbs
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathInside(parent, child string) bool {
	parent = comparablePath(parent)
	child = comparablePath(child)
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func comparablePath(path string) string {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err == nil {
		path = abs
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
