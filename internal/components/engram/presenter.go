package engram

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/storage"
)

// FormatDirLine returns a single display line for a data-directory option.
func FormatDirLine(dir string, dbSize int64) string {
	if dbSize <= 0 {
		return dir
	}
	return fmt.Sprintf("%s  (%s used)", dir, storage.FormatBytes(dbSize))
}

// FormatSpaceWarning returns a human-readable warning for insufficient disk.
func FormatSpaceWarning(needed, avail int64) string {
	return fmt.Sprintf("Need %s, only %s free", storage.FormatBytes(needed), storage.FormatBytes(avail))
}
