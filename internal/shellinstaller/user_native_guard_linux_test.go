//go:build linux && amd64

package shellinstaller

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserNativeGuardNamespace(t *testing.T) {
	status := "Uid:\t1000 1000 1000 1000\nGid:\t1000 1000 1000 1000\nGroups:\t\nNoNewPrivs:\t1\n"
	for _, name := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		status += name + ":\t0000000000000000\n"
	}
	for _, sealed := range []bool{false, true} {
		name, mapping := "coordinator", "         0          0 4294967295\n"
		if sealed {
			name, mapping = "child", "      1000       1000          1\n"
		}
		t.Run(name, func(t *testing.T) {
			if err := userNativeNamespaceData(status, mapping, mapping, "deny\n", 1000, 1000, sealed); err != nil {
				t.Fatal(err)
			}
			spaced := strings.Replace(status, "Gid:\t1000 1000 1000 1000", "Gid:\t1000\t1000\t1000\t1000", 1)
			if err := userNativeNamespaceData(spaced, mapping, mapping, "deny\n", 1000, 1000, sealed); err != nil {
				t.Fatal("kernel tab-separated GID refused", err)
			}
			cases := []struct{ name, status, uidMap, gidMap, setgroups string }{
				{"extra uid entry", status, mapping + "2000 2000 1\n", mapping, "deny\n"},
				{"extra gid entry", status, mapping, mapping + "2000 2000 1\n", "deny\n"},
				{"malformed", status, "not a map", mapping, "deny\n"},
				{"negative", status, "-1 1000 1\n", mapping, "deny\n"},
				{"overflow", status, mapping, "0 0 4294967296\n", "deny\n"},
				{"wrong shape", status, "1001 1000 1\n", mapping, "deny\n"},
				{"uid mismatch", strings.Replace(status, "Uid:\t1000 1000 1000 1000", "Uid:\t1000 1000 0 1000", 1), mapping, mapping, "deny\n"},
				{"gid mismatch", strings.Replace(status, "Gid:\t1000 1000 1000 1000", "Gid:\t1000 0 1000 1000", 1), mapping, mapping, "deny\n"},
				{"supplementary groups", strings.Replace(status, "Groups:\t", "Groups:\t1000", 1), mapping, mapping, "deny\n"},
				{"missing groups", strings.Replace(status, "Groups:\t\n", "", 1), mapping, mapping, "deny\n"},
				{"nnp", strings.Replace(status, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1), mapping, mapping, "deny\n"},
				{"duplicate status", status + "CapBnd:\t0000000000000000\n", mapping, mapping, "deny\n"},
			}
			if sealed {
				cases = append(cases, struct{ name, status, uidMap, gidMap, setgroups string }{"setgroups", status, mapping, mapping, "allow\n"})
			} else {
				cases = append(cases, struct{ name, status, uidMap, gidMap, setgroups string }{"singleton coordinator", status, "1000 1000 1\n", "1000 1000 1\n", "deny\n"})
			}
			for _, cap := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
				cases = append(cases, struct{ name, status, uidMap, gidMap, setgroups string }{cap, strings.Replace(status, cap+":\t0000000000000000", cap+":\t0000000000000001", 1), mapping, mapping, "deny\n"})
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					if err := userNativeNamespaceData(tc.status, tc.uidMap, tc.gidMap, tc.setgroups, 1000, 1000, sealed); err == nil {
						t.Fatal("unsafe namespace/status accepted")
					}
				})
			}
		})
	}
}

func TestUserNativeGuardMounts(t *testing.T) {
	roots := []string{"/producer/runtime/main", "/producer/runtime/main-source", "/run/user/1000"}
	id := 30
	parents := map[string]int{}
	line := func(path, mode string) string {
		id++
		parent := 20
		for root, candidate := range parents {
			if strings.HasPrefix(path, root+"/") {
				parent = candidate
			}
		}
		return fmt.Sprintf("%d %d 0:1 / %s %s - ext4 /dev/private rw\n", id, parent, path, mode)
	}
	base := ""
	lines := []string{}
	for _, root := range roots {
		entry := line(root, "ro,nosuid")
		lines = append(lines, entry)
		base += entry
		parents[root] = id
	}
	if err := userNativeMountsData(base, roots); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, mounts string }{
		{"writable source", strings.Replace(base, roots[0]+" ro,nosuid", roots[0]+" rw,nosuid", 1)},
		{"missing root", strings.Replace(base, lines[1], "", 1)},
		{"writable descendant", base + line(roots[1]+"/nested", "rw")},
		{"writable runtime descendant", base + line(roots[2]+"/systemd", "rw")},
		{"duplicate root", base + line(roots[0], "ro")},
		{"malformed", base + "invalid mountinfo\n"},
		{"unsupported escape", base + line(roots[1]+`/bad\011path`, "ro")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := userNativeMountsData(tc.mounts, roots); err == nil {
				t.Fatal("unsafe mounts accepted")
			}
		})
	}
	if err := userNativeMountsData(base+line(roots[1]+"/nested", "ro"), roots); err != nil {
		t.Fatal("readonly descendant refused", err)
	}
}

func TestUserNativeGuardLiveRefusal(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "preserve")
	if err := os.WriteFile(marker, []byte("private evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userNativeExecutionCheck(root, "/run/user/1000"); err == nil {
		t.Fatal("unsealed temporary directory accepted")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "private evidence" {
		t.Fatal("refused guard modified evidence", err)
	}
}

func TestUserNativeGuardMountStack(t *testing.T) {
	roots := []string{"/producer/runtime/main", "/producer/runtime/main-source", "/run/user/1000"}
	source := "31 20 0:1 / /producer/runtime/main ro - ext4 /dev/private rw\n32 20 0:1 / /producer/runtime/main-source ro - ext4 /dev/private rw\n"
	lower := "487 479 8:48 /runtime /run/user/1000 rw - ext4 /dev/private rw\n"
	upper := "503 487 8:48 /empty-runtime /run/user/1000 ro - ext4 /dev/private rw\n"
	for _, tc := range []struct {
		name, mounts string
		accepted     bool
	}{
		{"authentic order", lower + upper, true},
		{"reverse order", upper + lower, true},
		{"hidden lower child", lower + "504 487 8:48 /hidden /run/user/1000/hidden rw - ext4 /dev/private rw\n" + upper, true},
		{"visible writable root", lower + strings.Replace(upper, " ro -", " rw -", 1), false},
		{"visible writable child", lower + upper + "505 503 8:48 /child /run/user/1000/child rw - ext4 /dev/private rw\n", false},
		{"duplicate ID", lower + upper + upper, false},
		{"ambiguous cover", lower + upper + strings.Replace(upper, "503 487", "506 487", 1), false},
		{"cycle", lower + upper + "700 701 0:1 / /loop ro - tmpfs tmpfs rw\n701 700 0:1 / /loop ro - tmpfs tmpfs rw\n", false},
		{"malformed ID", lower + strings.Replace(upper, "503 487", "bad 487", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := userNativeMountsData(source+tc.mounts, roots)
			if (err == nil) != tc.accepted {
				t.Fatal("mount visibility differs", err)
			}
		})
	}
}
