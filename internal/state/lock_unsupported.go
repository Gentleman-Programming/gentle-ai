// Every target that neither lock_unix.go nor lock_windows.go claims lands here.
// aix and solaris match the `unix` pseudo-tag but do not define syscall.Flock,
// and hurd has no port in this toolchain, so none of them can honour the
// advisory-lock contract. Rather than leaving the symbols undefined and failing
// the build, this file keeps the package compilable everywhere and refuses to
// lock where locking was never verified. Callers surface ErrLockUnsupported
// instead of writing state without serialisation.
//go:build !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !windows

package state

import (
	"errors"
	"os"
	"runtime"
)

// ErrLockUnsupported is returned on targets with no verified advisory-lock
// implementation. It fails closed: state writes stop rather than proceeding
// without the serialisation that install and sync depend on.
var ErrLockUnsupported = errors.New("state: advisory file locking is not implemented on " + runtime.GOOS)

// tryLockFile always fails closed on unsupported targets.
func tryLockFile(_ *os.File) (bool, error) {
	return false, ErrLockUnsupported
}

// unlockFile always fails closed on unsupported targets.
func unlockFile(_ *os.File) error {
	return ErrLockUnsupported
}
