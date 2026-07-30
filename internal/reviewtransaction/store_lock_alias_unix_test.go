//go:build unix

package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// TestSecureOpenLockParentRefusesASymlinkedAncestor reproduces, on any unix,
// the exact mechanism behind the 1773 boundary 3 report. On macOS the walk
// starts at / and the second component is var, which is a symlink to
// /private/var, so O_NOFOLLOW refuses it and every lock path under $TMPDIR
// fails. Here the symlinked ancestor is built explicitly instead of being
// inherited from the platform, and the refusal is identical.
//
// This is the no-follow protection doing its job, not a defect in it. The
// same target reached through its resolved spelling is accepted, which is
// what every production store path looks like -- see
// TestAuthoritativeStorePathsAreCanonicalUnderAnAliasedRepository.
func TestSecureOpenLockParentRefusesASymlinkedAncestor(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(base, "real", "store")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	aliased := filepath.Join(base, "alias", "store")

	if _, err := secureOpenLockParent(string(filepath.Separator), aliased); err == nil {
		t.Fatal("the root-anchored walk traversed a symlinked ancestor; O_NOFOLLOW protection is gone")
	}

	fd, err := secureOpenLockParent(string(filepath.Separator), real)
	if err != nil {
		t.Fatalf("the root-anchored walk refused a fully resolved path: %v", err)
	}
	defer unix.Close(fd)
	if !sameDirectory(t, fd, real) {
		t.Fatal("the root-anchored walk opened the wrong directory")
	}
}

// TestAcquireLocalStoreLockUnderAnAliasedRepository proves the production
// shape works where the raw shape does not. The store is built by the real
// constructor from a repository addressed through a symlinked ancestor, and
// its lock is acquired successfully, because the constructor resolved the Git
// common directory before the secure walk ever saw it.
func TestAcquireLocalStoreLockUnderAnAliasedRepository(t *testing.T) {
	_, aliasedRepo := aliasedSnapshotRepo(t)
	store, err := CompactAuthoritativeStore(context.Background(), aliasedRepo, "alias-lock-lineage")
	if err != nil {
		t.Fatalf("CompactAuthoritativeStore(aliased repository) error = %v", err)
	}
	lock, err := acquireLocalStoreLock(filepath.Join(store.Dir, "LOCK"))
	if err != nil {
		t.Fatalf("acquireLocalStoreLock under an aliased repository: %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

// TestAcquireLocalStoreLockRefusesARawAliasedDirectory pins the harness side
// of 1773 boundary 3. A directory handed straight to the lock, without the
// canonicalization every store constructor performs, is refused whenever any
// of its ancestors is a symlink. That is the shape Store{Dir: t.TempDir()}
// produces on macOS, and it is the shape no production caller produces.
func TestAcquireLocalStoreLockRefusesARawAliasedDirectory(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "real", "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}

	if _, err := acquireLocalStoreLock(filepath.Join(base, "alias", "store", "LOCK")); err == nil {
		t.Fatal("a lock path with a symlinked ancestor was accepted")
	}
	lock, err := acquireLocalStoreLock(filepath.Join(base, "real", "store", "LOCK"))
	if err != nil {
		t.Fatalf("the resolved spelling of the same lock path was refused: %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

// TestStoreLockEnvironmentalRefusalIsNotContention pins the classification
// boundary that TestConcurrentStoreLockRecoverersCannotBothWin leans on: a
// contender may only ever see victory or ErrConcurrentUpdate, so an
// environmental failure of the secure walk — here the by-design refusal of a
// symlinked ancestor, the shape the first Darwin CI run surfaced under
// $TMPDIR — must surface as the typed pre-acquisition error and must never
// enter the contention chain. A caller that misread this as contention would
// retry a lock that can never be acquired instead of reporting a not-started
// environmental refusal (1781).
func TestStoreLockEnvironmentalRefusalIsNotContention(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "real", "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}

	_, err = acquireStoreLock(filepath.Join(base, "alias", "store", "LOCK"))
	if err == nil {
		t.Fatal("a lock path with a symlinked ancestor was accepted")
	}
	var preAcquisition *StoreLockPreAcquisitionError
	if !errors.As(err, &preAcquisition) {
		t.Fatalf("environmental refusal = %T %v, want StoreLockPreAcquisitionError", err, err)
	}
	if errors.Is(err, ErrConcurrentUpdate) || errors.Is(err, ErrStoreLockContended) {
		t.Fatalf("environmental refusal claims contention: %v", err)
	}
}
