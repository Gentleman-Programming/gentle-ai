//go:build linux && amd64

package shellinstaller

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// A full map is a qualified coordinator shape, not proof of an initial namespace.
func userNativeCoordinatorCheck() error {
	return userNativeCurrentNamespace(false)
}

func userNativeCurrentNamespace(sealed bool) error {
	if err := UserKernelCheck(); err != nil {
		return err
	}
	values := make([]string, 4)
	for i, name := range []string{"status", "uid_map", "gid_map", "setgroups"} {
		data, err := userNativeProc(name)
		if err != nil {
			return err
		}
		values[i] = data
	}
	return userNativeNamespaceData(values[0], values[1], values[2], values[3], os.Getuid(), os.Getgid(), sealed)
}

// Fixed procfs paths are not caller-supplied evidence or namespace identifiers.
func userNativeProc(name string) (string, error) {
	file, err := os.Open("/proc/self/" + name)
	if err != nil {
		return "", err
	}
	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &filesystem); err != nil || filesystem.Type != unix.PROC_SUPER_MAGIC {
		return "", errors.Join(privateError("refused", err), file.Close())
	}
	data, err := io.ReadAll(io.LimitReader(file, 1048577))
	err = errors.Join(err, file.Close())
	if err != nil || len(data) > 1048576 {
		return "", privateError("refused", err)
	}
	return string(data), nil
}

func userNativeNamespaceData(status, uidMap, gidMap, setgroups string, uid, gid int, sealed bool) error {
	if err := userKernelStatus(status, uid); err != nil {
		return err
	}
	fields := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			fields[key] = strings.TrimSpace(value)
		}
	}
	groups, present := fields["Groups"]
	if gid <= 0 || strings.Join(strings.Fields(fields["Gid"]), " ") != strings.Repeat(strconv.Itoa(gid)+" ", 3)+strconv.Itoa(gid) || !present || groups != "" || fields["CapBnd"] != "0000000000000000" {
		return privateError("refused", nil)
	}
	expectedUID, expectedGID := "0 0 4294967295", "0 0 4294967295"
	if sealed {
		expectedUID = strconv.Itoa(uid) + " " + strconv.Itoa(uid) + " 1"
		expectedGID = strconv.Itoa(gid) + " " + strconv.Itoa(gid) + " 1"
		if setgroups != "deny\n" {
			return privateError("refused", nil)
		}
	}
	for i, mapping := range []string{uidMap, gidMap} {
		lines := strings.Split(strings.TrimSuffix(mapping, "\n"), "\n")
		expected := expectedUID
		if i == 1 {
			expected = expectedGID
		}
		if len(lines) != 1 || strings.Join(strings.Fields(lines[0]), " ") != expected {
			return privateError("refused", nil)
		}
	}
	return nil
}

// The caller separately binds and compares the complete fixed source snapshot.
// These checks establish current mount/process conditions, never execution consent.
func userNativeExecutionCheck(root, managerRuntime string) error {
	if err := userNativeCurrentNamespace(true); err != nil {
		return err
	}
	if err := userNativeControlCheck(); err != nil {
		return err
	}
	if !userSelectionPath(root) || managerRuntime != "/run/user/"+strconv.Itoa(os.Getuid()) || userPathsOverlap(root, managerRuntime) {
		return privateError("refused", nil)
	}
	for _, path := range []string{root, filepath.Join(root, "runtime")} {
		if _, err := privateDirectory(path); err != nil {
			return err
		}
	}
	protected := []string{filepath.Join(root, "runtime/main"), filepath.Join(root, "runtime/main-source"), managerRuntime}
	for _, path := range protected {
		if _, err := privateDirectory(path); err != nil {
			return err
		}
		var filesystem unix.Statfs_t
		if err := unix.Statfs(path, &filesystem); err != nil || filesystem.Flags&unix.ST_RDONLY == 0 {
			return privateError("refused", err)
		}
	}
	if _, err := privatePhysical(filepath.Join(protected[0], "main.zip")); err != nil {
		return err
	}
	entries, err := os.ReadDir(managerRuntime)
	if err != nil || len(entries) != 0 {
		return privateError("refused", err)
	}
	for _, endpoint := range []string{"bus", "systemd/private"} {
		if _, err := os.Lstat(filepath.Join(managerRuntime, endpoint)); !os.IsNotExist(err) {
			return privateError("refused", err)
		}
	}
	mounts, err := userNativeProc("mountinfo")
	if err != nil {
		return err
	}
	return userNativeMountsData(mounts, protected)
}

func userNativeMountsData(mounts string, roots []string) error {
	type mount struct {
		parent   int
		point    string
		readonly bool
	}
	nodes := map[int]mount{}
	for _, line := range strings.Split(strings.TrimSuffix(mounts, "\n"), "\n") {
		sides := strings.Split(line, " - ")
		if len(sides) != 2 {
			return privateError("refused", nil)
		}
		fields, right := strings.Fields(sides[0]), strings.Fields(sides[1])
		if len(fields) < 6 || len(right) != 3 {
			return privateError("refused", nil)
		}
		id, idErr := strconv.Atoi(fields[0])
		parent, parentErr := strconv.Atoi(fields[1])
		_, rootOK := privateMountPath(fields[3])
		point, pointOK := privateMountPath(fields[4])
		if idErr != nil || parentErr != nil || id <= 0 || parent <= 0 || !rootOK || !pointOK || len(nodes) >= 16384 {
			return privateError("refused", nil)
		}
		if _, duplicate := nodes[id]; duplicate {
			return privateError("refused", nil)
		}
		nodes[id] = mount{parent, point, strings.Contains(","+fields[5]+",", ",ro,")}
	}
	// An immediate same-point child covers its parent, independently of line order.
	covers := map[int]int{}
	for id, node := range nodes {
		if parent, exists := nodes[node.parent]; exists && parent.point == node.point {
			if covers[node.parent] != 0 {
				return privateError("refused", nil)
			}
			covers[node.parent] = id
		}
	}
	// Branch visibility ignores its own cover so the covering child can stay visible.
	state := map[int]int{}
	var branch func(int) (bool, error)
	branch = func(id int) (bool, error) {
		if state[id] == 1 {
			return false, privateError("refused", nil)
		}
		if state[id] > 1 {
			return state[id] == 2, nil
		}
		state[id] = 1
		node, visible := nodes[id], true
		if _, exists := nodes[node.parent]; exists {
			var err error
			visible, err = branch(node.parent)
			if err != nil {
				return false, err
			}
			if cover := covers[node.parent]; cover != 0 && cover != id {
				visible = false
			}
		}
		state[id] = 3
		if visible {
			state[id] = 2
		}
		return visible, nil
	}
	seen := map[string]bool{}
	for id, node := range nodes {
		visible, err := branch(id)
		if err != nil {
			return err
		}
		if !visible || covers[id] != 0 {
			continue
		}
		for _, root := range roots {
			if node.point == root {
				if seen[root] || !node.readonly {
					return privateError("refused", nil)
				}
				seen[root] = true
			} else if strings.HasPrefix(node.point, root+"/") && !node.readonly {
				return privateError("refused", nil)
			}
		}
	}
	for _, root := range roots {
		if !seen[root] {
			return privateError("refused", nil)
		}
	}
	return nil
}
