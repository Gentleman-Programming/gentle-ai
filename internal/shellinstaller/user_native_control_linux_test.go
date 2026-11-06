//go:build linux && amd64

package shellinstaller

import (
	"golang.org/x/sys/unix"
	"os"
	"testing"
)

func TestUserNativeControlReadOnlyView(t *testing.T) {
	// The qualified harness supplies the positive read-only case; a normal host
	// with a writable control view must instead receive a refusal.
	var filesystem unix.Statfs_t
	statErr := unix.Statfs("/sys/fs/cgroup", &filesystem)
	mounts, mountErr := userNativeProc("mountinfo")
	refuse := statErr != nil || mountErr != nil || filesystem.Type != unix.CGROUP2_SUPER_MAGIC || filesystem.Flags&unix.ST_RDONLY == 0
	if !refuse {
		refuse = userNativeMountsData(mounts, []string{"/sys/fs/cgroup"}) != nil
	}
	if err := userNativeControlCheck(); (err != nil) != refuse {
		t.Fatal("live control-view disposition differs", err)
	}
}

func TestUserNativeControlMounts(t *testing.T) {
	root := "10 1 0:25 / /sys/fs/cgroup ro,nosuid,nodev,noexec - cgroup2 cgroup2 rw,nsdelegate\n"
	for _, tt := range []struct {
		name, mounts string
		refuse       bool
	}{
		{"readonly", root, false},
		{"missing root", "10 1 0:25 / /other ro - cgroup2 cgroup2 rw\n", true},
		{"writable root", "10 1 0:25 / /sys/fs/cgroup rw - cgroup2 cgroup2 rw\n", true},
		{"writable descendant", root + "11 10 0:25 /slice /sys/fs/cgroup/slice rw - cgroup2 cgroup2 rw\n", true},
		{"readonly descendant", root + "11 10 0:25 /slice /sys/fs/cgroup/slice ro - cgroup2 cgroup2 rw\n", false},
		{"writable sibling", root + "11 1 0:25 /sibling /sys/fs/cgroup-other rw - cgroup2 cgroup2 rw\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := userNativeMountsData(tt.mounts, []string{"/sys/fs/cgroup"})
			if (err != nil) != tt.refuse {
				t.Fatal("control mount disposition differs", err)
			}
		})
	}
}

func TestUserNativeControlExecutionBoundary(t *testing.T) {
	phase := os.Getenv("GENTLE_PRIVATE_GUARD_TEST")
	if testing.Short() || (phase != "sealed" && phase != "plain-rw") {
		t.Skip("requires the isolated native execution fixture")
	}
	err := userNativeExecutionCheck("/fixture/native-worker", "/run/user/1000")
	if (err != nil) != (phase == "plain-rw") {
		t.Fatal("execution boundary control-view disposition differs", phase, err)
	}
}
