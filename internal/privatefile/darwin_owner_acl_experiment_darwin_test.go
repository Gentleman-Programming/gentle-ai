//go:build darwin && cgo && privatefileowneraclexperiment

package privatefile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// This fixture is only for opted-in native experiments. It never touches HOME
// or a report destination, and never puts payload bytes in the child inode.
func newOwnerACLFixture(t *testing.T) (*os.File, string, syscall.Stat_t) {
	t.Helper()
	const root = "/private/tmp"
	for _, ancestor := range []string{"/", "/private", root} {
		info, err := os.Lstat(ancestor)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("unsafe fixture ancestor %s: %v, %v", ancestor, info, err)
		}
		if err := syscall.Access(ancestor, 1); err != nil { // X_OK
			t.Fatalf("fixture ancestor not traversable %s: %v", ancestor, err)
		}
	}
	dir, err := os.MkdirTemp(root, "privatefile-owner-acl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() ||
		info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 {
		t.Fatalf("fixture parent identity/mode: %v, %v", info, err)
	}
	var parent syscall.Stat_t
	if err := syscall.Lstat(dir, &parent); err != nil || parent.Uid != uint32(os.Getuid()) {
		t.Fatalf("fixture parent owner: %+v, %v", parent, err)
	}
	if err := syscall.Access(dir, 1); err != nil { // X_OK
		t.Fatalf("fixture parent not traversable: %v", err)
	}
	if message := experimentParentACL(dir); message != "" {
		t.Fatalf("parent inheritable everyone ACL: %s", message)
	}
	path := filepath.Join(dir, "empty")
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|
		syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		t.Fatalf("exclusive nofollow create: %v", err)
	}
	file := os.NewFile(uintptr(fd), path)
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	before := checkOwnerACLInode(t, file, path, nil)
	return file, path, before
}

func checkOwnerACLInode(t *testing.T, file *os.File, path string, original *syscall.Stat_t) syscall.Stat_t {
	t.Helper()
	var held, linked syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &held); err != nil {
		t.Fatalf("fstat held inode: %v", err)
	}
	if err := syscall.Lstat(path, &linked); err != nil {
		t.Fatalf("lstat child without following: %v", err)
	}
	if held.Mode&syscall.S_IFMT != syscall.S_IFREG || held.Mode&07777 != 0600 ||
		held.Uid != uint32(os.Getuid()) || held.Size != 0 || held.Nlink != 1 ||
		held.Dev != linked.Dev || held.Ino != linked.Ino ||
		linked.Mode&syscall.S_IFMT != syscall.S_IFREG || linked.Mode&07777 != 0600 ||
		linked.Uid != held.Uid || linked.Size != 0 {
		t.Fatalf("held/path identity, owner, mode or empty inode mismatch: fd=%+v path=%+v", held, linked)
	}
	if original != nil && (held.Dev != original.Dev || held.Ino != original.Ino ||
		held.Uid != original.Uid || held.Mode != original.Mode || held.Size != original.Size) {
		t.Fatalf("held inode changed during ACL replacement: before=%+v after=%+v", *original, held)
	}
	return held
}

// Reusable by B, but proves only ACL replacement, not distinct-user denial.
func proveOwnerACLReplacement(t *testing.T, file *os.File, path string, before syscall.Stat_t) {
	t.Helper()
	checkOwnerACLInode(t, file, path, &before)
	if message := experimentInheritedACL(file.Fd()); message != "" {
		t.Fatalf("same-FD inherited everyone readback: %s", message)
	}
	checkOwnerACLInode(t, file, path, &before)
	if message := experimentReplaceOwnerACL(file.Fd(), before.Uid); message != "" {
		t.Fatalf("same-FD owner set/readback: %s", message)
	}
	checkOwnerACLInode(t, file, path, &before)
}

func TestDarwinOwnerOnlyACLReplacesInheritedExperiment(t *testing.T) {
	file, path, before := newOwnerACLFixture(t)
	proveOwnerACLReplacement(t, file, path, before)
}
