// Package statecoord serializes mutations of the shared install state.
package statecoord

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// LockPath returns the canonical install-state lock path for homeDir.
func LockPath(homeDir string) (string, error) {
	resolvedHome, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return "", fmt.Errorf("resolve install state home: %w", err)
	}
	return state.Path(resolvedHome) + ".lock", nil
}

// WithLock runs operation while holding the canonical install-state lock.
func WithLock(homeDir string, operation func() error) (err error) {
	lockPath, err := LockPath(homeDir)
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
