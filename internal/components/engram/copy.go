package engram

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CopyDB copies a quiesced Engram SQLite database from src to dst.
// It refuses to copy when SQLite sidecar files exist because a raw file copy
// cannot prove WAL-mode consistency for a live database.
func CopyDB(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("source %q is not a regular file", src)
	}
	if err := requireQuiescedDB(src); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination dir for %q: %w", dst, err)
	}

	tmp := dst + ".tmp"

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %q: %w", src, err)
	}

	dstFile, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		srcFile.Close()
		return fmt.Errorf("create temp file %q: %w", tmp, err)
	}

	_, copyErr := io.Copy(dstFile, srcFile)
	srcCloseErr := srcFile.Close()
	syncErr := dstFile.Sync()
	dstCloseErr := dstFile.Close()

	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("copy %q to %q: %w", src, dst, copyErr)
	}
	if srcCloseErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("close source %q: %w", src, srcCloseErr)
	}
	if syncErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("sync %q: %w", tmp, syncErr)
	}
	if dstCloseErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("close temp file %q: %w", tmp, dstCloseErr)
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

func requireQuiescedDB(src string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := src + suffix
		if _, err := os.Stat(sidecar); err == nil {
			return fmt.Errorf("copy %q requires a quiesced SQLite database; sidecar %q exists", src, sidecar)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat SQLite sidecar %q: %w", sidecar, err)
		}
	}
	return nil
}
