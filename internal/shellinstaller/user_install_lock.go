//go:build linux || darwin || windows

package shellinstaller

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
)

// UserShellOperationInProgressError reports that another process holds the
// operation lock for the same destination. Install, update, reconfigure and
// uninstall each hold it for their whole run, so a second concurrent run
// fails immediately instead of racing on stages, backups and temporaries.
type UserShellOperationInProgressError struct{ Destination string }

func (e *UserShellOperationInProgressError) Error() string {
	return fmt.Sprintf("another Gentle Shell operation is in progress for %s; wait for it to finish and retry", e.Destination)
}

// IsUserShellOperationInProgress reports whether err is a lock contention.
func IsUserShellOperationInProgress(err error) bool {
	var locked *UserShellOperationInProgressError
	return errors.As(err, &locked)
}

// userOperationLockPath names the lock beside the destination, in its private
// parent. The leading dot and the fixed suffix keep it outside every stage,
// replaced/removed backup and registry candidate pattern; it never carries an
// installation manifest, so discovery can never treat it as an installation.
func userOperationLockPath(destination string) string {
	return filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".gentle-shell.lock")
}

// userAcquireOperationLock takes the cross-process exclusive lock for one
// destination without waiting. The operating system releases it when the
// returned release runs or the holding process exits; the lock file itself is
// left in place, because deleting it could split two holders onto different
// files. Acquire it once at the top of an operation: a nested acquire for the
// same destination in the same process reports contention, never deadlocks.
func userAcquireOperationLock(destination string) (release func() error, err error) {
	if destination == "" || !filepath.IsAbs(destination) || filepath.Dir(destination) == destination {
		return nil, errors.New("Gentle Shell operation lock requires an absolute destination")
	}
	destination = filepath.Clean(destination)
	unlock, err := userLockFile(userOperationLockPath(destination), destination)
	if err != nil {
		return nil, err
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() { releaseErr = unlock() })
		return releaseErr
	}, nil
}

// userWithOperationLock runs operation while holding the destination lock and
// joins a release failure into its error. The operation's own error keeps its
// identity whenever the release succeeds.
func userWithOperationLock[T any](destination string, operation func() (T, error)) (result T, err error) {
	release, err := userAcquireOperationLock(destination)
	if err != nil {
		return result, err
	}
	defer userReleaseOperationLock(release, &err)
	return operation()
}

// userReleaseOperationLock is the deferred release of an operation that
// returns through err: a release failure joins it, a successful release keeps
// the operation's own error identity.
func userReleaseOperationLock(release func() error, err *error) {
	if releaseErr := release(); releaseErr != nil {
		*err = errors.Join(*err, releaseErr)
	}
}
