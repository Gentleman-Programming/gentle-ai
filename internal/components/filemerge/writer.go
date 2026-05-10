package filemerge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// runtimeGOOS and syncDirFn are package-level vars so tests can override them
// without spawning a real Windows process.
var runtimeGOOS = func() string { return runtime.GOOS }
var syncDirFn = func(dir string) error {
	fd, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent directory %q: %w", dir, err)
	}
	defer fd.Close()
	return fd.Sync()
}

const maxAtomicFileSize = 16 << 20

type WriteResult struct {
	Changed bool
	Created bool
}

func WriteFileAtomic(path string, content []byte, perm fs.FileMode) (WriteResult, error) {
	if perm == 0 {
		perm = 0o644
	}

	created := false
	existing, err := readComparableFile(path)
	if err == nil {
		if bytes.Equal(existing, content) {
			return WriteResult{}, nil
		}
	} else if !os.IsNotExist(err) {
		return WriteResult{}, fmt.Errorf("read existing file %q: %w", path, err)
	} else {
		created = true
	}

	dir := filepath.Dir(path)
	if err := ensureAtomicParentDir(dir, path); err != nil {
		return WriteResult{}, err
	}

	tmp, err := os.CreateTemp(dir, ".gentle-ai-*.tmp")
	if err != nil {
		return WriteResult{}, fmt.Errorf("create temp file for %q: %w", path, err)
	}

	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return WriteResult{}, fmt.Errorf("write temp file for %q: %w", path, err)
	}

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return WriteResult{}, fmt.Errorf("set permissions on temp file for %q: %w", path, err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return WriteResult{}, fmt.Errorf("sync temp file for %q: %w", path, err)
	}

	if err := tmp.Close(); err != nil {
		return WriteResult{}, fmt.Errorf("close temp file for %q: %w", path, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return WriteResult{}, fmt.Errorf("replace %q atomically: %w", path, err)
	}

	// Sync the parent directory to flush the new directory entry to disk.
	// On Windows, NTFS returns ErrPermission when syncing a directory fd — tolerate
	// that specific error only. Any other error (e.g. disk full) is still fatal.
	if err := syncDirFn(dir); err != nil {
		if !(runtimeGOOS() == "windows" && errors.Is(err, os.ErrPermission)) {
			return WriteResult{}, fmt.Errorf("sync parent directory for %q: %w", path, err)
		}
	}

	cleanup = false
	return WriteResult{Changed: true, Created: created}, nil
}

func readComparableFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read symlink %q", path)
	}
	if info.Size() > maxAtomicFileSize {
		return nil, fmt.Errorf("file %q exceeds max atomic compare size %d bytes", path, maxAtomicFileSize)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAtomicFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAtomicFileSize {
		return nil, fmt.Errorf("file %q exceeds max atomic compare size %d bytes", path, maxAtomicFileSize)
	}
	return data, nil
}

func ensureAtomicParentDir(dir, path string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create parent directories for %q: %w", path, err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("stat parent directory for %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink parent directory %q for %q", dir, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent path %q for %q is not a directory", dir, path)
	}
	if info.Mode().Perm()&0o200 == 0 {
		if err := os.Chmod(dir, 0o755); err != nil {
			return fmt.Errorf("relax parent directory permissions for %q: %w", path, err)
		}
	}
	return nil
}
