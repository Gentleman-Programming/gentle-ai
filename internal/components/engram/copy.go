package engram

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyDB copies the Engram SQLite database from src to dst.
//
// Strategy: write to a temp file (dst+".tmp") first, verify the byte count,
// then rename atomically. The temp file is removed on any error so dst is
// never left in a partial state.
//
// io.Copy is used without an explicit buffer: when both src and dst are
// *os.File, Go 1.21+ calls (*os.File).ReadFrom which invokes sendfile(2) on
// Linux and the equivalent on macOS. On Windows it falls back to efficient
// buffered I/O. No manual buffer allocation needed.
func CopyDB(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir for %q: %w", dst, err)
	}

	tmp := dst + ".tmp"

	dstFile, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file %q: %w", tmp, err)
	}

	// removeTmp cleans up the temp file in error paths. Failures other than
	// "already gone" are joined onto the returned error so they are not lost.
	removeTmp := func(primary error) error {
		if rmErr := os.Remove(tmp); rmErr != nil && !os.IsNotExist(rmErr) {
			return errors.Join(primary, fmt.Errorf("clean up temp file %q: %w", tmp, rmErr))
		}
		return primary
	}

	srcFile, err := os.Open(src)
	if err != nil {
		dstFile.Close()
		return removeTmp(fmt.Errorf("open source %q: %w", src, err))
	}

	_, copyErr := io.Copy(dstFile, srcFile)
	srcFile.Close()
	syncErr := dstFile.Sync()
	dstFile.Close()

	if copyErr != nil {
		return removeTmp(fmt.Errorf("copy %q → %q: %w", src, dst, copyErr))
	}
	if syncErr != nil {
		return removeTmp(fmt.Errorf("sync %q: %w", tmp, syncErr))
	}

	dstInfo, err := os.Stat(tmp)
	if err != nil {
		return removeTmp(fmt.Errorf("stat temp file %q: %w", tmp, err))
	}
	if dstInfo.Size() != srcInfo.Size() {
		return removeTmp(fmt.Errorf("incomplete copy: wrote %d bytes, expected %d", dstInfo.Size(), srcInfo.Size()))
	}

	if err := os.Rename(tmp, dst); err != nil {
		return removeTmp(fmt.Errorf("rename %q → %q: %w", tmp, dst, err))
	}
	return nil
}
