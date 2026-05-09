package engram

import (
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

	srcFile, err := os.Open(src)
	if err != nil {
		dstFile.Close()
		os.Remove(tmp)
		return fmt.Errorf("open source %q: %w", src, err)
	}

	_, copyErr := io.Copy(dstFile, srcFile)
	srcFile.Close()
	syncErr := dstFile.Sync()
	dstFile.Close()

	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("copy %q → %q: %w", src, dst, copyErr)
	}
	if syncErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("sync %q: %w", tmp, syncErr)
	}

	dstInfo, err := os.Stat(tmp)
	if err != nil || dstInfo.Size() != srcInfo.Size() {
		os.Remove(tmp)
		if err != nil {
			return fmt.Errorf("stat temp file %q: %w", tmp, err)
		}
		return fmt.Errorf("incomplete copy: wrote %d bytes, expected %d", dstInfo.Size(), srcInfo.Size())
	}

	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %q → %q: %w", tmp, dst, err)
	}
	return nil
}
