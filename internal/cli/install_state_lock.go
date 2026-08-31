package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// installStateLockPath is the install-state lock under the real home
// directory. The lock primitive walks every path component below its root
// with O_NOFOLLOW, so a home whose path crosses a symlink (macOS `/var` ->
// `/private/var`, a `mktemp -d` HOME) failed with "not a directory" before
// any state existed (#3926). Resolving the symlinks first keeps the no-follow
// walk, now over the real path, and lands the lock beside the state file it
// guards. A path that does not resolve yet keeps today's shape verbatim.
func installStateLockPath(homeDir string) string {
	statePath := state.Path(homeDir)
	parent := filepath.Dir(statePath)
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Join(resolved, filepath.Base(statePath)) + ".lock"
	}
	if resolvedHome, err := filepath.EvalSymlinks(homeDir); err == nil {
		if relative, relErr := filepath.Rel(homeDir, statePath); relErr == nil {
			return filepath.Join(resolvedHome, relative) + ".lock"
		}
	}
	return statePath + ".lock"
}

func withInstallStateLock(homeDir string, operation func() error) (err error) {
	lock, err := reviewtransaction.AcquireAuthorityFileLock(installStateLockPath(homeDir))
	if err != nil {
		return fmt.Errorf("acquire install state lock: %w", err)
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release install state lock: %w", releaseErr))
		}
	}()
	return operation()
}
