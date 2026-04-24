package engram

import (
	"fmt"
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

// DataDirEnvVar is the environment variable that controls where Engram stores
// its persistent SQLite database and related files.
const DataDirEnvVar = "ENGRAM_DATA_DIR"

// DefaultDataDir returns the default Engram data directory.
// It respects the ENGRAM_DATA_DIR environment variable if set;
// otherwise it falls back to ~/.engram.
func DefaultDataDir() string {
	if dir := os.Getenv(DataDirEnvVar); dir != "" {
		abs, err := filepath.Abs(dir)
		if err == nil {
			return abs
		}
		return dir
	}
	home, err := userHomeDir()
	if err != nil {
		// Fallback: use current working directory + .engram as absolute path.
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".engram")
	}
	return filepath.Join(home, ".engram")
}

// HardDefaultDataDir returns the canonical default Engram data directory
// (~/.engram) ignoring any ENGRAM_DATA_DIR environment variable. This is
// used as the migration source so that re-running install after a previous
// migration does not self-copy from the already-migrated location.
func HardDefaultDataDir() string {
	home, err := userHomeDir()
	if err != nil {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".engram")
	}
	return filepath.Join(home, ".engram")
}

// DetectExistingData reports whether an Engram database already exists in the
// given directory. It looks for engram.db and its SQLite companion files.
func DetectExistingData(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "engram.db"))
	return err == nil
}

// ExistingEngramFiles returns the list of Engram SQLite files that exist in
// the given directory.
func ExistingEngramFiles(dir string) []string {
	files := []string{"engram.db", "engram.db-wal", "engram.db-shm"}
	var found []string
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			found = append(found, f)
		}
	}
	return found
}

// MigrateData copies Engram SQLite files from source to target and removes the
// source files only after successful verification.
//
// It uses read/write instead of os.Rename so that cross-device moves work
// (e.g. C:\ → D:\ on Windows or /home → /mnt/data on Linux).
func MigrateData(source, target string) error {
	files := []string{"engram.db", "engram.db-wal", "engram.db-shm"}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create target directory %q: %w", target, err)
	}

	// Calculate total size of source files to verify target has enough space.
	var totalSize uint64
	for _, f := range files {
		srcPath := filepath.Join(source, f)
		info, err := os.Stat(srcPath)
		if err != nil {
			continue // file doesn't exist, skip
		}
		if info.IsDir() {
			continue
		}
		totalSize += uint64(info.Size())
	}

	if totalSize > 0 {
		if err := requireFreeSpace(target, totalSize); err != nil {
			return err
		}
	}

	// Phase 1: copy all files (do NOT remove sources yet).
	var toRemove []string
	for _, f := range files {
		srcPath := filepath.Join(source, f)
		dstPath := filepath.Join(target, f)

		info, err := os.Stat(srcPath)
		if err != nil {
			continue // file doesn't exist, skip
		}
		if info.IsDir() {
			continue
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
			return fmt.Errorf("write %s: %w", f, err)
		}

		// Verify copy succeeded before scheduling removal
		dstInfo, err := os.Stat(dstPath)
		if err != nil || dstInfo.Size() != info.Size() {
			return fmt.Errorf("verify %s failed after copy", f)
		}

		toRemove = append(toRemove, srcPath)
	}

	// Phase 2: only remove sources after all copies verified.
	for _, srcPath := range toRemove {
		if err := os.Remove(srcPath); err != nil {
			return fmt.Errorf("remove source %s: %w", filepath.Base(srcPath), err)
		}
	}

	return nil
}

// DetectLockedData tries to determine whether any Engram SQLite files in dir
// are currently open/locked by another process. It is best-effort: on Windows
// it uses a rename probe (rename fails if the file is open); on Unix it
// attempts lsof and falls back to false if unavailable.
func DetectLockedData(dir string) (bool, error) {
	if runtime.GOOS != "windows" {
		return detectLockedDataUnix(dir)
	}
	files := ExistingEngramFiles(dir)
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
			// Should never happen, but handle defensively.
			return true, fmt.Errorf("failed to restore %s after lock check: %w", f, err)
		}
	}
	return false, nil
}

func detectLockedDataUnix(dir string) (bool, error) {
	files := ExistingEngramFiles(dir)
	for _, f := range files {
		path := filepath.Join(dir, f)
		// Best-effort: lsof returns the PID of any process with the file open.
		out, err := exec.Command("lsof", "-t", path).CombinedOutput()
		if err != nil {
			// lsof not available or other error — fall back to unlocked.
			return false, nil
		}
		if len(strings.TrimSpace(string(out))) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// ExpandDataDir expands a user-provided path, replacing leading ~ with the
// user's home directory and converting to an absolute path.
func ExpandDataDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", fmt.Errorf("path is empty")
	}

	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		// Strip leading separator(s) so filepath.Join appends correctly.
		// e.g. ~/foo -> foo; ~\foo -> foo (on Windows).
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
