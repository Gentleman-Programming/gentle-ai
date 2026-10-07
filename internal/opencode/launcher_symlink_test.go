package opencode

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"testing"
)

// symlinkOrSkip creates a test symlink, skipping only when Windows lacks the
// required privilege. All other errors remain test failures.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if symlinkPrivilegeUnavailable(runtime.GOOS, err) {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

// symlinkPrivilegeUnavailable distinguishes Windows privilege error 1314
// from ordinary permission, path, or filesystem errors.
func symlinkPrivilegeUnavailable(goos string, err error) bool {
	var linkErr *os.LinkError
	return goos == "windows" && errors.As(err, &linkErr) &&
		errors.Is(linkErr.Err, syscall.Errno(1314)) // ERROR_PRIVILEGE_NOT_HELD
}

func TestSymlinkPrivilegeUnavailable(t *testing.T) {
	privilege := &os.LinkError{Op: "symlink", Old: "target", New: "link", Err: syscall.Errno(1314)}
	for _, tt := range []struct {
		name, goos string
		err        error
		want       bool
	}{
		{"Windows missing privilege", "windows", privilege, true},
		{"Windows wrapped missing privilege", "windows", errors.Join(errors.New("setup"), privilege), true},
		{"Linux error with same number", "linux", privilege, false},
		{"macOS error with same number", "darwin", privilege, false},
		{"Windows access denied", "windows", &os.LinkError{Err: syscall.Errno(5)}, false},
		{"Windows missing path", "windows", &os.LinkError{Err: os.ErrNotExist}, false},
		{"Windows other failure", "windows", errors.New("filesystem failure"), false},
		{"Windows success", "windows", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := symlinkPrivilegeUnavailable(tt.goos, tt.err); got != tt.want {
				t.Fatalf("privilege unavailable = %t, want %t", got, tt.want)
			}
		})
	}
}
