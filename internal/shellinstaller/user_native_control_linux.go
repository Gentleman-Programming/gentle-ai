//go:build linux && amd64

package shellinstaller

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// A masked manager socket alone does not protect user-owned controller files.
// This guard reads the live mount view, never caller-supplied limit evidence.
func userNativeControlCheck() error {
	const root = "/sys/fs/cgroup"
	before, err := userNativeProc("mountinfo")
	if err != nil {
		return err
	}
	if err := userNativeMountsData(before, []string{root}); err != nil {
		return err
	}
	if err := privateCgroupPhysical(root, true); err != nil {
		return err
	}
	file, err := os.Open(root)
	if err != nil {
		return err
	}
	var filesystem unix.Statfs_t
	statErr := unix.Fstatfs(int(file.Fd()), &filesystem)
	if err := errors.Join(statErr, file.Close()); err != nil {
		return err
	}
	if filesystem.Type != unix.CGROUP2_SUPER_MAGIC || filesystem.Flags&unix.ST_RDONLY == 0 {
		return privateError("refused", nil)
	}
	after, err := userNativeProc("mountinfo")
	if err != nil || before != after {
		return privateError("preimage", err)
	}
	return nil
}
