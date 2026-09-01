//go:build !windows

package reviewtransaction

import "io/fs"

// SetRARPrivateDirectoryPrimitivesForTest swaps the two filesystem primitives
// that establish an owner-only RAR directory, so a test can reproduce a mount
// that ignores the mode it is given. It exists only in the test binary: the
// production package must never expose a way to change how a security boundary
// is created.
//
// A nil argument keeps the current primitive. The returned function restores
// both.
func SetRARPrivateDirectoryPrimitivesForTest(
	mkdir func(string, fs.FileMode) error,
	chmod func(string, fs.FileMode) error,
) func() {
	previousMkdir, previousChmod := rarPrivateDirectoryMkdir, rarPrivateDirectoryChmod
	if mkdir != nil {
		rarPrivateDirectoryMkdir = mkdir
	}
	if chmod != nil {
		rarPrivateDirectoryChmod = chmod
	}
	return func() {
		rarPrivateDirectoryMkdir, rarPrivateDirectoryChmod = previousMkdir, previousChmod
	}
}
