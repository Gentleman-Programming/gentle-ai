package engram

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// DataDirHasContent reports whether dataDir exists and contains any files.
// Engram's internal storage layout is intentionally opaque to gentle-ai.
func DataDirHasContent(dataDir string) bool {
	entries, err := os.ReadDir(dataDir)
	return err == nil && len(entries) > 0
}

// DataDirSize returns the combined size in bytes of all regular files under dataDir.
func DataDirSize(dataDir string) int64 {
	var total int64
	_ = filepath.WalkDir(dataDir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// DiskSpaceOKForDataDir reports whether the volume at dst has strictly more
// free bytes than the total size of files under srcDataDir.
// When srcDataDir is empty or inaccessible, returns ok=true.
func DiskSpaceOKForDataDir(srcDataDir, dst string) (ok bool, needed, avail int64, err error) {
	needed = DataDirSize(srcDataDir)
	if needed == 0 {
		return true, 0, 0, nil
	}
	avail, err = storage.AvailableBytes(dst)
	if err != nil {
		return false, needed, 0, err
	}
	return avail > needed, needed, avail, nil
}

// dataDirFiles returns paths of all regular files under dataDir.
// Used by DataDirService.snapshot to enumerate what to back up.
func dataDirFiles(dataDir string) []string {
	var paths []string
	_ = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	return paths
}
