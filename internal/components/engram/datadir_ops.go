package engram

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// DataDirHasContent reports whether dataDir exists and contains any files.
func DataDirHasContent(dataDir string) bool {
	entries, err := os.ReadDir(dataDir)
	return err == nil && len(entries) > 0
}

// DataDirSize returns the combined size of all regular files under dataDir.
func DataDirSize(dataDir string) int64 {
	var total int64
	_ = filepath.WalkDir(dataDir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, infoErr := d.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func dataDirCopySize(dataDir string) int64 {
	var total int64
	_ = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !isEngramDataFile(dataDir, path) {
			return nil
		}
		if info, infoErr := d.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// DiskSpaceOKForDataDir reports whether dst has enough free bytes for dataDir.
func DiskSpaceOKForDataDir(srcDataDir, dst string) (ok bool, needed, avail int64, err error) {
	needed = dataDirCopySize(srcDataDir)
	if needed == 0 {
		return true, 0, 0, nil
	}
	avail, err = storage.AvailableBytes(dst)
	if err != nil {
		return false, needed, 0, err
	}
	return avail > needed, needed, avail, nil
}
