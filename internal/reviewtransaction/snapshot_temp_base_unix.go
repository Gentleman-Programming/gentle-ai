//go:build !windows

package reviewtransaction

import (
	"os"
	"path/filepath"
	"syscall"
)

func snapshotTempBaseSafe(path string) bool {
	for {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || rarPathUnsafe(path, info) {
			return false
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (owner.Uid != 0 && owner.Uid != uint32(os.Geteuid())) {
			return false
		}
		// Qualify every ancestor, not just the private leaf: a writable,
		// nonsticky parent lets another user substitute our scratch directory.
		if info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return true
		}
		path = parent
	}
}
