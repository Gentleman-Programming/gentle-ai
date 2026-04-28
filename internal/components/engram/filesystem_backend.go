package engram

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// requireFreeSpace is a test hook for disk-space validation in MigrateData.
var requireFreeSpace = storage.RequireFreeSpace

// userHomeDir is a test hook for DefaultDataDir and HardDefaultDataDir.
// Tests can replace this to control the home directory without modifying
// the real filesystem.
var userHomeDir = os.UserHomeDir

// SetUserHomeDirForTest sets the userHomeDir test hook. It is intended for
// use by tests in other packages that need to control the Engram data
// directory location.
func SetUserHomeDirForTest(fn func() (string, error)) func() {
	old := userHomeDir
	userHomeDir = fn
	return func() { userHomeDir = old }
}

// LocalDataBackend implements DataBackend for the local filesystem.
type LocalDataBackend struct{}

// NewLocalDataBackend creates a new local filesystem backend.
func NewLocalDataBackend() *LocalDataBackend {
	return &LocalDataBackend{}
}

// DefaultDataDir returns the default Engram data directory.
// It respects the ENGRAM_DATA_DIR environment variable if set;
// otherwise it falls back to ~/.engram.
func (b *LocalDataBackend) DefaultDataDir() string {
	if dir := getDataDirEnv(); dir != "" {
		abs, err := filepath.Abs(dir)
		if err == nil {
			return abs
		}
		return dir
	}
	home, err := userHomeDir()
	if err != nil {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".engram")
	}
	return filepath.Join(home, ".engram")
}

// HardDefaultDataDir returns the canonical default Engram data directory
// (~/.engram) ignoring any ENGRAM_DATA_DIR environment variable.
func (b *LocalDataBackend) HardDefaultDataDir() string {
	home, err := userHomeDir()
	if err != nil {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".engram")
	}
	return filepath.Join(home, ".engram")
}

// ExpandPath expands a user-provided path, replacing leading ~ with the
// user's home directory and converting to an absolute path.
func (b *LocalDataBackend) ExpandPath(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("path is empty")
	}

	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		remainder := dir[1:]
		remainder = strings.TrimPrefix(remainder, "/")
		remainder = strings.TrimPrefix(remainder, "\\")
		dir = filepath.Join(home, remainder)
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve absolute path: %w", err)
	}

	return abs, nil
}

// DetectExistingData reports whether an Engram database already exists in the
// given directory. It looks for engram.db and its SQLite companion files.
func (b *LocalDataBackend) DetectExistingData(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "engram.db"))
	return err == nil
}

// ExistingFiles returns the list of Engram SQLite files that exist in
// the given directory.
func (b *LocalDataBackend) ExistingFiles(dir string) []string {
	files := []string{"engram.db", "engram.db-wal", "engram.db-shm"}
	var found []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			found = append(found, f)
		}
	}
	return found
}

// CleanData deletes all Engram SQLite files in the given directory.
// It returns an error if the directory does not exist or if any file cannot be removed.
func (b *LocalDataBackend) CleanData(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("directory %q does not exist", dir)
	}

	files := []string{"engram.db", "engram.db-wal", "engram.db-shm"}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove %s: %w", f, err)
			}
		}
	}
	return nil
}

// EstimateMigration returns the list of files that would be migrated and their
// total size, without actually copying anything.
func (b *LocalDataBackend) EstimateMigration(source string) ([]FileInfo, uint64, error) {
	files := []string{"engram.db", "engram.db-wal", "engram.db-shm"}
	var infos []FileInfo
	var total uint64
	for _, f := range files {
		srcPath := filepath.Join(source, f)
		info, err := os.Stat(srcPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		infos = append(infos, FileInfo{Name: f, Size: uint64(info.Size())})
		total += uint64(info.Size())
	}
	return infos, total, nil
}

// MigrateData copies Engram SQLite files from source to target.
// It does NOT remove source files — the caller (DataDirService) is responsible
// for deleting the source only after the configuration has been persisted.
//
// This ordering guarantees that if config persistence fails, the user's data
// is still intact in the original location.
//
// It uses read/write instead of os.Rename so that cross-device moves work
// (e.g. C:\ → D:\ on Windows or /home → /mnt/data on Linux).
func (b *LocalDataBackend) MigrateData(source, target string) (Result, error) {
	files := []string{"engram.db", "engram.db-wal", "engram.db-shm"}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return Result{}, fmt.Errorf("create target directory %q: %w", target, err)
	}

	// Calculate total size of source files to verify target has enough space.
	var totalSize uint64
	for _, f := range files {
		srcPath := filepath.Join(source, f)
		info, err := os.Stat(srcPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		totalSize += uint64(info.Size())
	}

	if totalSize > 0 {
		if err := requireFreeSpace(target, totalSize); err != nil {
			return Result{}, err
		}
	}

	var copied []string
	var result Result
	for _, f := range files {
		srcPath := filepath.Join(source, f)
		dstPath := filepath.Join(target, f)

		info, err := os.Stat(srcPath)
		if err != nil {
			continue
		}
		if info.IsDir() {
			continue
		}

		if err := copyFileBuffered(srcPath, dstPath, info.Mode()); err != nil {
			// Best-effort cleanup: remove any files we already copied so the
			// target is not left in a partial state.
			for _, cp := range copied {
				_ = os.Remove(cp)
			}
			return Result{}, fmt.Errorf("copy %s: %w", f, err)
		}

		// Verify copy succeeded
		dstInfo, err := os.Stat(dstPath)
		if err != nil || dstInfo.Size() != info.Size() {
			for _, cp := range copied {
				_ = os.Remove(cp)
			}
			_ = os.Remove(dstPath)
			return Result{}, fmt.Errorf("verify %s failed after copy", f)
		}

		result.FilesMoved++
		result.BytesMoved += uint64(info.Size())
		copied = append(copied, dstPath)
	}

	return result, nil
}

// DetectLockedData tries to determine whether any Engram SQLite files in dir
// are currently open/locked by another process. It is best-effort: on Windows
// it uses a rename probe (rename fails if the file is open); on Unix it
// attempts lsof and falls back to false if unavailable.
func (b *LocalDataBackend) DetectLockedData(dir string) (bool, error) {
	if runtime.GOOS != "windows" {
		return b.detectLockedDataUnix(dir)
	}
	files := b.ExistingFiles(dir)
	for _, f := range files {
		src := filepath.Join(dir, f)
		tmp := src + ".lockcheck"
		// Defensive: if a previous lock-check crashed mid-way, restore the original file.
		if _, err := os.Stat(tmp); err == nil {
			_ = os.Rename(tmp, src)
		}
		if err := os.Rename(src, tmp); err != nil {
			return true, fmt.Errorf("file appears locked (%s): %w", f, err)
		}
		if err := os.Rename(tmp, src); err != nil {
			return true, fmt.Errorf("failed to restore %s after lock check: %w", f, err)
		}
	}
	return false, nil
}

func (b *LocalDataBackend) detectLockedDataUnix(dir string) (bool, error) {
	files := b.ExistingFiles(dir)
	for _, f := range files {
		path := filepath.Join(dir, f)
		// Best-effort: lsof returns the PID of any process with the file open.
		out, err := exec.Command("lsof", "-t", path).CombinedOutput()
		if err != nil {
			return false, nil
		}
		if len(strings.TrimSpace(string(out))) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// AvailableSpace returns the available disk space at the given directory.
func (b *LocalDataBackend) AvailableSpace(dir string) (uint64, error) {
	return storage.CheckAvailableSpace(dir)
}

// EnsureDir creates the directory and any parents if they don't exist.
func (b *LocalDataBackend) EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// DetectPartialMigration checks if there's existing data in both source and target,
// indicating an interrupted migration. It returns a warning message if detected.
func (b *LocalDataBackend) DetectPartialMigration(source, target string) string {
	srcFiles := b.ExistingFiles(source)
	dstFiles := b.ExistingFiles(target)

	srcHasData := len(srcFiles) > 0
	dstHasData := len(dstFiles) > 0

	if srcHasData && dstHasData {
		return fmt.Sprintf("Found data in both locations. Using %s. To recover old data, set ENGRAM_DATA_DIR manually.", target)
	}

	return ""
}

// copyFileBuffered copies src to dst using a fixed-size buffer so that large
// files (e.g. multi-GB SQLite databases) don't OOM the process.
func copyFileBuffered(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	// Use a deferred close with error capture so that a delayed write error
	// (e.g. disk full) is not silently swallowed.
	closeOut := func() error {
		return out.Close()
	}
	defer func() { _ = closeOut() }()

	const bufSize = 64 * 1024 // 64 KiB
	buf := make([]byte, bufSize)
	if _, err := io.CopyBuffer(out, in, buf); err != nil {
		return err
	}

	return closeOut()
}
