//go:build !windows

package filemerge

import "os"

// moveFileReplace is a plain rename everywhere except Windows. Nothing is added
// here on purpose: POSIX rename(2) already replaces the destination atomically,
// including a destination that is currently being executed, so a fallback chain
// would only add ways to be wrong.
func moveFileReplace(src, dst string) error {
	return os.Rename(src, dst)
}
