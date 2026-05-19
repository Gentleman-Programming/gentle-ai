package engram

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyDB copies the Engram SQLite database from src to dst.
//
// It writes to a temp file first, verifies the byte count, then renames the
// temp file into place so dst is never left with a partial copy.
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
		return fmt.Errorf("copy %q to %q: %w", src, dst, copyErr)
	}
	if syncErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("sync %q: %w", tmp, syncErr)
	}

	dstInfo, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("stat temp file %q: %w", tmp, err)
	}
	if dstInfo.Size() != srcInfo.Size() {
		os.Remove(tmp)
		return fmt.Errorf("incomplete copy: wrote %d bytes, expected %d", dstInfo.Size(), srcInfo.Size())
	}

	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %q to %q: %w", tmp, dst, err)
	}
	return nil
}
