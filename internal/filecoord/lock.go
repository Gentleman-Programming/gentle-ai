package filecoord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrBusy          = errors.New("file coordination lock is busy")
	ErrUnsupported   = errors.New("file coordination lock is unsupported")
	ErrInvalidRoot   = errors.New("file coordination lock root is invalid")
	ErrInvalidTarget = errors.New("file coordination target is invalid")
	ErrOperational   = errors.New("file coordination lock operation failed")
)

type BusyError struct{ Cause error }

func (e *BusyError) Error() string { return ErrBusy.Error() }
func (e *BusyError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrBusy}
	}
	return []error{ErrBusy, e.Cause}
}

type UnsupportedError struct{}

func (*UnsupportedError) Error() string { return ErrUnsupported.Error() }
func (*UnsupportedError) Unwrap() error { return ErrUnsupported }

type InvalidRootError struct{}

func (*InvalidRootError) Error() string { return ErrInvalidRoot.Error() }
func (*InvalidRootError) Unwrap() error { return ErrInvalidRoot }

type InvalidTargetError struct{}

func (*InvalidTargetError) Error() string { return ErrInvalidTarget.Error() }
func (*InvalidTargetError) Unwrap() error { return ErrInvalidTarget }

type OperationalError struct{ Cause error }

func (e *OperationalError) Error() string { return ErrOperational.Error() }
func (e *OperationalError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrOperational}
	}
	return []error{ErrOperational, e.Cause}
}

// Lease is a cooperative lease; Release is concurrent-safe and idempotent.
type Lease struct {
	once   sync.Once
	file   *os.File
	unlock func(*os.File) error
	close  func() error
	err    error
}

// Release attempts unlock and close, classifying either failure as operational.
func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		var failures []error
		if l.unlock != nil {
			if err := l.unlock(l.file); err != nil {
				failures = append(failures, err)
			}
		}
		if l.close != nil {
			if err := l.close(); err != nil {
				failures = append(failures, err)
			}
		}
		l.file = nil
		if len(failures) > 0 {
			l.err = &OperationalError{Cause: errors.Join(failures...)}
		}
	})
	return l.err
}

// LockPath is side-effect-free and hashes the absolute, cleaned target path.
// It coordinates only callers honoring this cooperative API; it provides no
// CAS or protection against arbitrary writers.
func LockPath(lockRoot, target string) (string, error) {
	root, err := cleanRoot(lockRoot)
	if err != nil {
		return "", err
	}
	path, err := cleanTarget(target)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(path))
	return filepath.Join(root, hex.EncodeToString(key[:])+".lock"), nil
}

func cleanRoot(path string) (string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return "", &InvalidRootError{}
	}
	return filepath.Clean(path), nil
}

func cleanTarget(path string) (string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return "", &InvalidTargetError{}
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", &InvalidTargetError{}
	}
	return filepath.Clean(absolute), nil
}

// Acquire validates inputs, honors context cancellation before any
// filesystem work, then takes the shared cooperative lock through the
// hardened authority-lock primitive. Acquisition is one non-blocking attempt:
// contention returns a typed BusyError and the caller owns retry pacing. The
// lease is cooperative and provides no arbitrary-writer CAS. The secure
// no-follow open walk rejects symlinked roots and lock paths, so lock roots
// must be canonical paths.
func Acquire(ctx context.Context, target, lockRoot string) (*Lease, error) {
	path, err := LockPath(lockRoot, target)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, &OperationalError{Cause: err}
	}
	return acquireCooperativeLock(path)
}
