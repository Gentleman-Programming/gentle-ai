//go:build linux

package shellinstaller

import (
	"bytes"
	"errors"
	"fmt"
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
	observed := strings.TrimSuffix(membership, "\n")
	member := observed
	if !strings.HasPrefix(member, "0::") || !privateHierarchyPath(strings.TrimPrefix(member, "0::")) {
		return "", privateError("unavailable", fmt.Errorf("membership %q is not one canonical 0:: hierarchy entry", member))
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
					return "", privateError("unavailable", fmt.Errorf("mountinfo line %q repeats the '-' separator", line))
				}
				separator = i
			}
		}
		if separator < 6 || len(fields) != separator+4 {
			return "", privateError("unavailable", fmt.Errorf("mountinfo line %q has %d fields with the '-' separator at %d, want one '-' and four trailing fields", line, len(fields), separator))
		}
		// Only the mountpoint is consumed for every line: it classifies the
		// entry against the trusted cgroup2 hierarchy. The root field is
		// consumed solely by the trusted cgroup2 mount itself; unrelated
		// mounts such as snapd's nsfs entries carry namespace identifiers
		// like "mnt:[4026532518]" there (#5733), so their roots stay unread.
		mountpoint, pointOK := privateMountPath(fields[4])
		if !pointOK {
			return "", privateError("unavailable", fmt.Errorf("mountinfo mountpoint %q is not canonical", fields[4]))
		}
		if strings.HasPrefix(mountpoint, trusted+"/") {
			return "", privateError("unavailable", fmt.Errorf("mountpoint %q is underneath the trusted cgroup2 mountpoint", mountpoint))
		}
		if mountpoint == trusted {
			mountRoot, rootOK := privateMountPath(fields[3])
			if !rootOK {
				return "", privateError("unavailable", fmt.Errorf("mountinfo root %q or mountpoint %q is not canonical", mountRoot, mountpoint))
			}
			if root != "" {
				return "", privateError("unavailable", fmt.Errorf("trusted cgroup2 mount %q appears more than once", mountpoint))
			}
			if fields[separator+1] != "cgroup2" {
				return "", privateError("unavailable", fmt.Errorf("trusted cgroup2 mount has fstype %q, want cgroup2", fields[separator+1]))
			}
			root = mountRoot
		}
	}
	if root == "" || (root != "/" && member != root && !strings.HasPrefix(member, root+"/")) {
		return "", privateError("unavailable", fmt.Errorf("membership %q does not resolve inside the observed cgroup2 mount root %q", observed, root))
	}
	relative := strings.TrimPrefix(member, root)
	return filepath.Join(trusted, relative), nil
}

func privateKernelData(membership, mounts string, limits []string) error {
	if _, err := privateCgroupPath(membership, mounts); err != nil {
		return err
	}
	const want = "3221225472\n0\n100000 100000\n64"
	if len(limits) != 4 || strings.Join(limits, "\n") != want {
		observed := append([]string(nil), limits...)
		for len(observed) < 4 {
			observed = append(observed, "")
		}
		return privateError("unavailable", fmt.Errorf(
			"leaf limits differ: memory.max=%q memory.swap.max=%q cpu.max=%q pids.max=%q want 3221225472/0/\"100000 100000\"/64",
			observed[0], observed[1], observed[2], observed[3]))
	}
	return nil
}

// privateCgroupPhysical refuses every path that is not a canonical cgroup2
// entry and always names the clause plus the observed values, so the cause
// survives even when Lstat, EvalSymlinks and Statfs all succeed.
func privateCgroupPhysical(path string, directory bool) error {
	info, err := os.Lstat(path)
	canonical, canonicalErr := filepath.EvalSymlinks(path)
	var fs unix.Statfs_t
	var cause error
	switch {
	case err != nil || canonicalErr != nil:
		cause = fmt.Errorf("physical cgroup path %q is not inspectable: %w", path, errors.Join(err, canonicalErr))
	case canonical != path:
		cause = fmt.Errorf("physical cgroup path %q is not canonical: symlink resolves to %q", path, canonical)
	case directory && !info.IsDir():
		cause = fmt.Errorf("physical cgroup path %q is not the required directory: mode %v", path, info.Mode())
	case !directory && !info.Mode().IsRegular():
		cause = fmt.Errorf("physical cgroup path %q is not the required regular file: mode %v", path, info.Mode())
	case unix.Statfs(path, &fs) != nil:
		cause = fmt.Errorf("physical cgroup path %q does not report a filesystem type", path)
	case fs.Type != unix.CGROUP2_SUPER_MAGIC:
		cause = fmt.Errorf("physical cgroup path %q has fstype %#x, want cgroup2 %#x", path, uint64(fs.Type), uint64(unix.CGROUP2_SUPER_MAGIC))
	}
	if cause != nil {
		return privateError("unavailable", cause)
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
