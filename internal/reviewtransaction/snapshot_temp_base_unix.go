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
		// guard:population snapshot-temp-ancestor-shape fail-closed: Only existing real directory ancestors qualify scratch storage; unresolved or symlink ancestors retain Git-local allocation.
		if err != nil || !info.IsDir() || rarPathUnsafe(path, info) {
			return false
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		// guard:population snapshot-temp-ancestor-owner too-loose: Root/caller-owned ancestors qualify; another owner must not be able to substitute private scratch.
		if !ok || (owner.Uid != 0 && owner.Uid != uint32(os.Geteuid())) {
			return false
		}
		// Qualify every ancestor, not just the private leaf: a writable,
		// nonsticky parent lets another user substitute our scratch directory.
		// guard:population snapshot-temp-ancestor-write too-loose: Nonwritable or owned/system sticky ancestors qualify; shared nonsticky writable ancestors retain Git-local allocation.
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
