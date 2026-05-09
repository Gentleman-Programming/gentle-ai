package engram

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// SQLiteArtifactPaths returns canonical SQLite file paths (main DB, WAL, SHM)
// for an Engram data directory.
func SQLiteArtifactPaths(dataDir string) []string {
	db := DBPath(dataDir)
	return []string{db, db + "-wal", db + "-shm"}
}

// ExistingSQLiteArtifacts returns artifact paths under dataDir that exist as regular files.
func ExistingSQLiteArtifacts(dataDir string) ([]string, error) {
	var out []string
	for _, p := range SQLiteArtifactPaths(dataDir) {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// TotalSQLiteArtifactSize returns the combined byte size of existing artifacts, or 0 if none/errors.
func TotalSQLiteArtifactSize(dataDir string) int64 {
	paths, err := ExistingSQLiteArtifacts(dataDir)
	if err != nil || len(paths) == 0 {
		return 0
	}
	var n int64
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			n += info.Size()
		}
	}
	return n
}

// DiskSpaceOKForSQLiteArtifacts reports whether the volume containing dst has strictly more
// free bytes than the sum of existing SQLite artifacts under srcDataDir (same rule as
// DataDirService.DiskSpaceOK: avail > needed). When needed==0, returns ok=true without
// querying dst (nothing to copy).
func DiskSpaceOKForSQLiteArtifacts(srcDataDir, dst string) (ok bool, needed, avail int64, err error) {
	needed = TotalSQLiteArtifactSize(srcDataDir)
	if needed == 0 {
		return true, 0, 0, nil
	}
	avail, err = storage.AvailableBytes(dst)
	if err != nil {
		return false, needed, 0, err
	}
	return avail > needed, needed, avail, nil
}

// CopySQLiteArtifacts copies every existing SQLite artifact from srcDataDir into dstDataDir,
// preserving filenames (engram.db, engram.db-wal, engram.db-shm).
func CopySQLiteArtifacts(srcDataDir, dstDataDir string) error {
	srcDataDir = filepath.Clean(srcDataDir)
	dstDataDir = filepath.Clean(dstDataDir)

	paths, err := ExistingSQLiteArtifacts(srcDataDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no sqlite artifacts under %q", srcDataDir)
	}

	for _, src := range paths {
		rel, err := filepath.Rel(srcDataDir, src)
		if err != nil {
			return fmt.Errorf("artifact %q is not under data dir %q: %w", src, srcDataDir, err)
		}
		dst := filepath.Join(dstDataDir, rel)
		if err := CopyDB(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// RemoveSQLiteArtifacts deletes every existing SQLite artifact under dataDir.
func RemoveSQLiteArtifacts(dataDir string) error {
	paths, err := ExistingSQLiteArtifacts(dataDir)
	if err != nil {
		return err
	}
	var firstErr error
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			firstErr = err
		}
	}
	return firstErr
}
