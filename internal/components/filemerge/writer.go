package filemerge

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("create parent directories for %q: %w", path, err)
	}
	// Ensure the directory is writable — it may have been created with
	// restricted permissions (e.g. 555) by a previous installer version or
	// the target agent itself. MkdirAll succeeds on existing dirs but does
	// not fix their permissions, causing os.CreateTemp to fail below.
	if err := os.Chmod(dir, 0o755); err != nil {
		return WriteResult{}, fmt.Errorf("set write permission on directory for %q: %w", path, err)
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

	dirFD, err := os.Open(dir)
	if err != nil {
		return WriteResult{}, fmt.Errorf("open parent directory for %q: %w", path, err)
	}
	defer dirFD.Close()
	if err := dirFD.Sync(); err != nil {
		return WriteResult{}, fmt.Errorf("sync parent directory for %q: %w", path, err)
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
