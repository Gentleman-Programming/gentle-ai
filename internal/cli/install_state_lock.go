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
// guards. The home is resolved once, so every process given the same home
// derives the same lock whether or not the state directory exists yet.
func installStateLockPath(homeDir string) (string, error) {
	resolvedHome, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return "", fmt.Errorf("resolve install state home: %w", err)
	}
	return state.Path(resolvedHome) + ".lock", nil
}

func withInstallStateLock(homeDir string, operation func() error) (err error) {
	lockPath, err := installStateLockPath(homeDir)
	if err != nil {
		return fmt.Errorf("acquire install state lock: %w", err)
	}
	lock, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
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
