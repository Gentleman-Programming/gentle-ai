//go:build !windows

package reviewtransaction

import (
	"io/fs"
	"os"
	"syscall"
)

func compactReviewerFileSingleLink(_ *os.File, info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
