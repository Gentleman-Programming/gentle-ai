//go:build linux

package shellinstaller

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func privateMountPath(field string) (string, bool) {
	// Only the defined space escape is supported. Refuse all other escapes,
	// including encoded separators and control characters, rather than guess.
	path := strings.ReplaceAll(field, `\040`, " ")
	return path, privateHierarchyPath(path)
}

// privateCgroupPath resolves DATA only; production supplies /proc/self bytes.
// Membership is in hierarchy coordinates, not relative to the mountpoint.
func privateCgroupPath(membership, mounts string) (string, error) {
	member := strings.TrimSuffix(membership, "\n")
	if !strings.HasPrefix(member, "0::") || !privateHierarchyPath(strings.TrimPrefix(member, "0::")) {
		return "", privateError("unavailable", nil)
	}
	member = strings.TrimPrefix(member, "0::")
	const trusted = "/sys/fs/cgroup"
	root := ""
	for _, line := range strings.Split(strings.TrimSuffix(mounts, "\n"), "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i, field := range fields {
			if field == "-" {
				if separator != -1 {
					return "", privateError("unavailable", nil)
				}
				separator = i
			}
		}
		if separator < 6 || len(fields) != separator+4 {
			return "", privateError("unavailable", nil)
		}
		mountRoot, rootOK := privateMountPath(fields[3])
		mountpoint, pointOK := privateMountPath(fields[4])
		if !rootOK || !pointOK || strings.HasPrefix(mountpoint, trusted+"/") {
			return "", privateError("unavailable", nil)
		}
		if mountpoint == trusted {
			if root != "" || fields[separator+1] != "cgroup2" {
				return "", privateError("unavailable", nil)
			}
			root = mountRoot
		}
	}
	if root == "" || (root != "/" && member != root && !strings.HasPrefix(member, root+"/")) {
		return "", privateError("unavailable", nil)
	}
	relative := strings.TrimPrefix(member, root)
	return filepath.Join(trusted, relative), nil
}

func privateKernelData(membership, mounts string, limits []string) error {
	if _, err := privateCgroupPath(membership, mounts); err != nil {
		return err
	}
	if len(limits) != 4 || strings.Join(limits, "\n") != "3221225472\n0\n100000 100000\n64" {
		return privateError("unavailable", nil)
	}
	return nil
}

func privateCgroupPhysical(path string, directory bool) error {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	var fs unix.Statfs_t
	if err != nil || canonicalErr != nil || canonical != path || (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) || unix.Statfs(path, &fs) != nil || fs.Type != unix.CGROUP2_SUPER_MAGIC {
		return privateError("unavailable", errors.Join(err, canonicalErr))
	}
	return nil
}

func privateKernel() error {
	membership, e1 := os.ReadFile("/proc/self/cgroup")
	mounts, e2 := os.ReadFile("/proc/self/mountinfo")
	if e1 != nil || e2 != nil {
		return privateError("unavailable", errors.Join(e1, e2))
	}
	current, err := privateCgroupPath(string(membership), string(mounts))
	if err != nil {
		return err
	}
	for _, path := range []string{"/sys/fs/cgroup", current} {
		if err := privateCgroupPhysical(path, true); err != nil {
			return err
		}
	}
	// Exact leaf limits enforce upper bounds even with permissive ancestors.
	// Stricter ancestors can reduce availability; this does not promise capacity.
	var limits []string
	for _, name := range []string{"memory.max", "memory.swap.max", "cpu.max", "pids.max"} {
		path := filepath.Join(current, name)
		if err := privateCgroupPhysical(path, false); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 64 {
			return privateError("unavailable", err)
		}
		if err := privateCgroupPhysical(path, false); err != nil {
			return err
		}
		limits = append(limits, strings.TrimSpace(string(data)))
	}
	freshMembership, e1 := os.ReadFile("/proc/self/cgroup")
	freshMounts, e2 := os.ReadFile("/proc/self/mountinfo")
	if e1 != nil || e2 != nil || !bytes.Equal(membership, freshMembership) || !bytes.Equal(mounts, freshMounts) {
		return privateError("unavailable", errors.Join(e1, e2))
	}
	return privateKernelData(string(membership), string(mounts), limits)
}

const privateColdPath = "/dist/v24.18.0/node-v24.18.0-linux-x64.tar.gz"
const privateColdURL = "https://nodejs.org" + privateColdPath
const privateColdSize int64 = 57224421
const privateColdSHA = "783130984963db7ba9cbd01089eaf2c2efb055c7c1693c943174b967b3050cb8"
