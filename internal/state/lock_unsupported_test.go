//go:build !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !windows

package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestUnsupportedPlatformFailsClosed proves that a target without a verified
// advisory-lock implementation refuses to lock instead of silently proceeding
// unlocked. Degrading to no lock would reintroduce the lost-update race that
// issue #1809 describes.
//
// This test never executes in CI, and that is deliberate rather than an
// oversight: it is tagged for the same targets as lock_unsupported.go, and CI
// runs linux, darwin, and windows. Its value is compile-time. `GOOS=aix
// GOARCH=ppc64 go test -c ./internal/state/` type-checks this file against the
// fallback, so the contract cannot drift — deleting or renaming
// ErrLockUnsupported, or changing either signature, breaks the cross-compiled
// build. Keep that command in the verification steps; without it this file
// really would assert nothing.
func TestUnsupportedPlatformFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open handle: %v", err)
	}
	defer file.Close()

	locked, err := tryLockFile(file)
	if locked {
		t.Error("tryLockFile reported a lock on a platform with no implementation")
	}
	if !errors.Is(err, ErrLockUnsupported) {
		t.Errorf("tryLockFile error = %v, want ErrLockUnsupported", err)
	}
	if err := unlockFile(file); !errors.Is(err, ErrLockUnsupported) {
		t.Errorf("unlockFile error = %v, want ErrLockUnsupported", err)
	}
}
