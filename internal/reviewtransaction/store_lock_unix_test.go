//go:build unix

package reviewtransaction

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// sameDirectory reports whether the open file descriptor and the directory
// at path refer to the exact same underlying inode, so an anchor comparison
// does not depend on path string equality.
func sameDirectory(t *testing.T, fd int, path string) bool {
	t.Helper()
	var fdStat unix.Stat_t
	if err := unix.Fstat(fd, &fdStat); err != nil {
		t.Fatal(err)
	}
	pathStat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	pathSys, ok := pathStat.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("path stat does not expose syscall.Stat_t")
	}
	return uint64(fdStat.Dev) == uint64(pathSys.Dev) && fdStat.Ino == pathSys.Ino
}

// TestSecureOpenLockParentAnchor proves 1781: the secure open walk anchors
// at an explicit root instead of unconditionally opening the filesystem
// root, narrowing the walk to repository-owned directories, while a target
// outside that root fails safe by falling back to today's root-anchored
// walk verbatim so no working configuration regresses.
func TestSecureOpenLockParentAnchor(t *testing.T) {
	t.Run("under-root walk succeeds", func(t *testing.T) {
		root := canonicalTempDir(t)
		target := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		fd, err := secureOpenLockParent(root, target)
		if err != nil {
			t.Fatalf("secureOpenLockParent(root, under-root target) error = %v", err)
		}
		defer unix.Close(fd)
		if !sameDirectory(t, fd, target) {
			t.Fatal("under-root walk opened the wrong directory")
		}
	})

	t.Run("not-under-root falls back to the root-anchored walk verbatim", func(t *testing.T) {
		root := canonicalTempDir(t)
		outside := canonicalTempDir(t)
		fallbackFD, err := secureOpenLockParent(string(filepath.Separator), outside)
		if err != nil {
			t.Fatalf("secureOpenLockParent(/, outside) error = %v", err)
		}
		defer unix.Close(fallbackFD)

		fd, err := secureOpenLockParent(root, outside)
		if err != nil {
			t.Fatalf("secureOpenLockParent(root, not-under-root target) error = %v", err)
		}
		defer unix.Close(fd)
		if !sameDirectory(t, fd, outside) {
			t.Fatal("not-under-root fallback opened the wrong directory")
		}
	})

	t.Run("symlinked component is refused", func(t *testing.T) {
		root := canonicalTempDir(t)
		real := filepath.Join(root, "real")
		if err := os.MkdirAll(real, 0o755); err != nil {
			t.Fatal(err)
		}
		linked := filepath.Join(root, "linked")
		if err := os.Symlink(real, linked); err != nil {
			t.Fatal(err)
		}
		if _, err := secureOpenLockParent(root, linked); err == nil {
			t.Fatal("secureOpenLockParent(root, symlinked component) succeeded")
		}
	})
}

func TestAcquireLocalStoreLockRejectsSymlinkAndPreservesTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "external-target")
	want := []byte("external data must not be lock metadata\n")
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	if _, err := acquireLocalStoreLock(path); err == nil {
		t.Fatal("acquireLocalStoreLock(symlink) succeeded")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("external target changed: got %q, want %q", got, want)
	}
}

func TestAcquireLocalStoreLockRejectsSymlinkedParentAndPreservesTarget(t *testing.T) {
	externalStore := filepath.Join(t.TempDir(), "external-store")
	if err := os.MkdirAll(externalStore, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(externalStore, "LOCK")
	want := []byte("external data must not be lock metadata\n")
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.Symlink(externalStore, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}

	if _, err := acquireLocalStoreLock(path); err == nil {
		t.Fatal("acquireLocalStoreLock(symlinked parent) succeeded")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("external target changed through parent: got %q, want %q", got, want)
	}
}

func TestAcquireLocalStoreLockRejectsSymlinkSwapAtOpenBoundary(t *testing.T) {
	target := filepath.Join(t.TempDir(), "external-target")
	want := []byte("external data must not be lock metadata\n")
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old lock\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previousHook := secureOpenLocalStoreLockBeforeOpen
	secureOpenLocalStoreLockBeforeOpen = func(openPath string) {
		if openPath != path {
			t.Fatalf("secure open path = %q, want %q", openPath, path)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { secureOpenLocalStoreLockBeforeOpen = previousHook })

	if _, err := acquireLocalStoreLock(path); err == nil {
		t.Fatal("acquireLocalStoreLock(symlink swap) succeeded")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("external target changed after swap: got %q, want %q", got, want)
	}
}

func TestAcquireLocalStoreLockRejectsNonRegularUnixObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-store", "LOCK")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireLocalStoreLock(path); err == nil {
		t.Fatal("acquireLocalStoreLock(directory) succeeded")
	}
}

func TestAcquireLocalStoreLockCreatesAndReopensWithoutChangingPermissions(t *testing.T) {
	path := filepath.Join(canonicalTempDir(t), "review-store", "LOCK")
	first, err := acquireLocalStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.release(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	second, err := acquireLocalStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.release(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("LOCK permissions = %#o, want 0640", got)
	}
}

func TestAcquireLocalStoreLockAllowsSearchOnlyAncestor(t *testing.T) {
	root := canonicalTempDir(t)
	ancestor := filepath.Join(root, "search-only")
	path := filepath.Join(ancestor, "review-store", "LOCK")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ancestor, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(ancestor, 0o700); err != nil {
			t.Error(err)
		}
	})

	lock, err := acquireLocalStoreLock(path)
	if err != nil {
		t.Fatalf("acquireLocalStoreLock(search-only ancestor): %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

func TestBusyStoreLockProbePreservesExistingUnixInodeAndMetadata(t *testing.T) {
	path := filepath.Join(canonicalTempDir(t), "review-store", "LOCK")
	held, err := acquireStoreLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	beforePayload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeStat := beforeInfo.Sys().(*syscall.Stat_t)

	if _, err := acquireStoreLock(path); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("busy acquisition = %v, want ErrConcurrentUpdate", err)
	}
	evidence, exists := inventoryLock(AuthorityVersionCompact, "", path)
	if !exists || evidence.Status != AuthorityLockOwned || evidence.Owner != nil {
		t.Fatalf("busy lock evidence = %#v, exists=%t", evidence, exists)
	}

	afterPayload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	afterStat := afterInfo.Sys().(*syscall.Stat_t)
	if beforeStat.Ino != afterStat.Ino || beforeInfo.ModTime() != afterInfo.ModTime() || !reflect.DeepEqual(beforePayload, afterPayload) {
		t.Fatalf("busy probe mutated LOCK: inode %d/%d mtime %s/%s", beforeStat.Ino, afterStat.Ino, beforeInfo.ModTime(), afterInfo.ModTime())
	}
}
