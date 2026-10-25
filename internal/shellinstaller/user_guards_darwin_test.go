//go:build darwin

package shellinstaller

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinKernelAcceptsHostAndRefusesInjectedPlatform(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("host check must run as the target user")
	}
	if err := userDarwinKernelCheck(); err != nil {
		t.Fatalf("arm64 macOS>=14 non-root host refused: %v", err)
	}
	for _, version := range []string{"14", "14.0", "15.7.1", "26.5.2"} {
		if err := userDarwinKernel(501, 501, "arm64", "arm64", 0, version); err != nil {
			t.Fatalf("version %q refused: %v", version, err)
		}
	}
	for _, refused := range []struct {
		uid, euid       int
		goarch, machine string
		translated      uint32
		version, want   string
	}{
		{501, 0, "arm64", "arm64", 0, "26.5", "refuses to run as root"},
		{0, 501, "arm64", "arm64", 0, "26.5", "refuses to run as root"},
		{501, 501, "amd64", "arm64", 0, "26.5", "requires native Apple silicon (arm64)"},
		{501, 501, "arm64", "x86_64", 0, "26.5", "requires native Apple silicon (arm64)"},
		{501, 501, "arm64", "arm64", 1, "26.5", "requires native Apple silicon (arm64)"},
		{501, 501, "arm64", "arm64", 0, "13.6.9", "requires macOS 14 or newer"},
		{501, 501, "arm64", "arm64", 0, "", "requires macOS 14 or newer"},
		{501, 501, "arm64", "arm64", 0, "14.x", "requires macOS 14 or newer"},
		{501, 501, "arm64", "arm64", 0, "99999999999999999999", "requires macOS 14 or newer"},
	} {
		err := userDarwinKernel(refused.uid, refused.euid, refused.goarch, refused.machine, refused.translated, refused.version)
		if err == nil || !strings.Contains(err.Error(), refused.want) {
			t.Fatalf("%+v: error = %v, want %q", refused, err, refused.want)
		}
	}
}

func TestDarwinCanonicalPathResolvesSystemLinksOnly(t *testing.T) {
	if got, err := userCanonicalPath("/tmp/gentle-shell-canonical-probe"); err != nil || got != "/private/tmp/gentle-shell-canonical-probe" {
		t.Fatalf("/tmp leaf canonical = %q, %v", got, err)
	}
	raw := t.TempDir() // $TMPDIR spells /var/folders/... through the /var link.
	owned := filepath.Join(raw, "owned")
	if err := os.Mkdir(owned, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := privateDirectory(owned); err == nil {
		t.Fatal("strict check must keep refusing the uncanonical /var spelling")
	}
	physical, err := filepath.EvalSymlinks(raw)
	if err != nil || !strings.HasPrefix(physical, "/private/") {
		t.Fatalf("fixture physical path = %q, %v", physical, err)
	}
	canonical, err := userCanonicalPath(owned)
	if err != nil || canonical != filepath.Join(physical, "owned") {
		t.Fatalf("canonical = %q, %v; want %q", canonical, err, filepath.Join(physical, "owned"))
	}
	if _, err := privateDirectory(canonical); err != nil {
		t.Fatalf("strict check refused the canonical path: %v", err)
	}
	if got, err := userCanonicalPath(filepath.Join(raw, "OWNED", "leaf")); err != nil || got != filepath.Join(canonical, "leaf") {
		t.Fatalf("case variant canonical = %q, %v; want on-disk spelling %q", got, err, filepath.Join(canonical, "leaf"))
	}
	link := filepath.Join(raw, "link")
	if err := os.Symlink(owned, link); err != nil {
		t.Fatal(err)
	}
	if got, err := userCanonicalPath(filepath.Join(link, "leaf")); err == nil {
		t.Fatalf("operator-owned intermediate symlink resolved to %q", got)
	}
	final, err := userCanonicalPath(link)
	if err != nil || final != filepath.Join(physical, "link") {
		t.Fatalf("final symlink canonical = %q, %v", final, err)
	}
	if _, err := privateDirectory(final); err == nil {
		t.Fatal("a final-component symlink must still be refused after canonicalization")
	}
	for _, refused := range []string{"relative/path", "/private/tmp/../tmp", ""} {
		if got, err := userCanonicalPath(refused); err == nil {
			t.Fatalf("noncanonical input %q accepted as %q", refused, got)
		}
	}
}

func TestDarwinPathsOverlapComparesPhysicalIdentity(t *testing.T) {
	raw := t.TempDir()
	physical, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	prefix, agent := filepath.Join(physical, "Prefix"), filepath.Join(physical, "agent")
	for _, directory := range []string{prefix, agent} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range [][2]string{
		{prefix, prefix},
		{prefix, filepath.Join(physical, "PREFIX")},
		{filepath.Join(physical, "prefix", "agent"), prefix},
		{prefix, filepath.Join(raw, "Prefix", "lib")},
		{filepath.Join(raw, "Prefix", "new"), filepath.Join(physical, "prefix", "new", "deeper")},
		{filepath.Join(physical, "Dest"), filepath.Join(physical, "dest", "sub")},
		{filepath.Join(physical, "caf\u00e9"), filepath.Join(physical, "cafe\u0301")},
	} {
		if !userPathsOverlap(pair[0], pair[1]) || !userPathsOverlap(pair[1], pair[0]) {
			t.Fatalf("overlap not detected for %q and %q", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{
		{prefix, agent},
		{prefix, filepath.Join(physical, "Prefix2")},
		{filepath.Join(physical, "dest"), filepath.Join(physical, "destination")},
	} {
		if userPathsOverlap(pair[0], pair[1]) || userPathsOverlap(pair[1], pair[0]) {
			t.Fatalf("disjoint paths %q and %q reported as overlapping", pair[0], pair[1])
		}
	}
}

func darwinACL(t *testing.T, path, entry string) {
	t.Helper()
	if out, err := exec.Command("/bin/chmod", "+a", entry, path).CombinedOutput(); err != nil {
		t.Fatalf("chmod +a %q: %v %s", entry, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-N", path).Run() })
}

func TestDarwinExtendedMetadataRefusesFlagsACLsAndQuarantine(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fresh := func(name string) (string, string) {
		directory := filepath.Join(base, name)
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(directory, "file")
		if err := os.WriteFile(file, []byte("owned"), 0600); err != nil {
			t.Fatal(err)
		}
		return directory, file
	}
	accepted := func(label, directory, file string) {
		t.Helper()
		if _, err := privateDirectory(directory); err != nil {
			t.Fatalf("%s directory refused: %v", label, err)
		}
		if err := privateNativeDirectory(directory); err != nil {
			t.Fatalf("%s native directory refused: %v", label, err)
		}
		if _, err := privatePhysical(file); err != nil {
			t.Fatalf("%s file refused: %v", label, err)
		}
	}
	xattr := func(name string, paths ...string) {
		t.Helper()
		for _, path := range paths {
			if err := unix.Lsetxattr(path, name, []byte("0081;00000000;gentle;"), 0); err != nil {
				t.Fatalf("setxattr %s: %v", name, err)
			}
		}
	}
	directory, file := fresh("clean")
	accepted("clean", directory, file)

	for _, flag := range []int{unix.UF_IMMUTABLE, unix.UF_APPEND, unix.UF_OPAQUE} {
		directory, file := fresh(fmt.Sprintf("flag-%x", flag))
		for _, path := range []string{file, directory} {
			if err := unix.Chflags(path, flag); err != nil {
				t.Fatalf("chflags %#x: %v", flag, err)
			}
			t.Cleanup(func() { _ = unix.Chflags(path, 0) })
		}
		if _, err := privatePhysical(file); err == nil {
			t.Fatalf("file with BSD flag %#x accepted", flag)
		}
		if _, err := privateDirectory(directory); err == nil {
			t.Fatalf("directory with BSD flag %#x accepted", flag)
		}
		if err := privateNativeDirectory(directory); err == nil {
			t.Fatalf("native directory with BSD flag %#x accepted", flag)
		}
	}
	directory, file = fresh("flag-benign")
	for _, path := range []string{file, directory} {
		if err := unix.Chflags(path, unix.UF_NODUMP|unix.UF_HIDDEN); err != nil {
			t.Fatal(err)
		}
	}
	accepted("nodump/hidden", directory, file)

	directory, file = fresh("provenance")
	xattr("com.apple.provenance", directory, file)
	accepted("provenance", directory, file)

	directory, file = fresh("quarantine")
	xattr("com.apple.quarantine", directory, file)
	if _, err := privatePhysical(file); err == nil {
		t.Fatal("owned file with com.apple.quarantine accepted")
	}
	if err := privateNativeDirectory(directory); err == nil {
		t.Fatal("owned native directory with com.apple.quarantine accepted")
	}
	if _, err := privateDirectory(directory); err != nil {
		t.Fatalf("operator ancestor with com.apple.quarantine refused: %v", err)
	}
	// A Gatekeeper-approved, browser-downloaded gentle-ai keeps its quarantine
	// xattr; it is the operator's file, not an installer-owned entry.
	if _, err := privateForeignPhysical(file); err != nil {
		t.Fatalf("operator supervisor source with com.apple.quarantine refused: %v", err)
	}
	if _, err := privateForeignPhysical(filepath.Join(directory, "missing")); err == nil {
		t.Fatal("foreign physical check accepted a missing file")
	}

	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	directory, file = fresh("acl-allow")
	darwinACL(t, file, "user:"+current.Username+" allow write")
	darwinACL(t, directory, "everyone deny delete")
	darwinACL(t, directory, "everyone allow add_file")
	if _, err := privatePhysical(file); err == nil {
		t.Fatal("file with an allow ACL accepted")
	}
	if _, err := privateDirectory(directory); err == nil {
		t.Fatal("directory with an allow ACL accepted")
	}
	if err := privateNativeDirectory(directory); err == nil {
		t.Fatal("native directory with an allow ACL accepted")
	}
	directory, file = fresh("acl-deny")
	darwinACL(t, file, "everyone deny delete")
	darwinACL(t, directory, "group:everyone deny delete,file_inherit,directory_inherit")
	accepted("deny-only ACL", directory, file)
}

func TestDarwinExtendedMetadataAcceptsTransparentCompression(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, compressed := filepath.Join(base, "source"), filepath.Join(base, "compressed")
	if err := os.WriteFile(source, []byte(strings.Repeat("gentle shell ", 4096)), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/ditto", "--hfsCompression", source, compressed).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, out)
	}
	info, err := os.Lstat(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if info.Sys().(*syscall.Stat_t).Flags&unix.UF_COMPRESSED == 0 {
		t.Skip("filesystem did not apply transparent compression")
	}
	if _, err := privatePhysical(compressed); err != nil {
		t.Fatalf("transparently compressed file refused: %v", err)
	}
}

func TestDarwinDescriptorPathReturnsOnDiskSpelling(t *testing.T) {
	file, err := os.Open("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	path, err := userDescriptorPath(int(file.Fd()))
	if err != nil || path != "/private/tmp" {
		t.Fatalf("descriptor path = %q, %v; want /private/tmp", path, err)
	}
}

func TestDarwinSyncIssuesFullFsync(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := userFullSync
	t.Cleanup(func() { userFullSync = original })
	var synced []uintptr
	userFullSync = func(fd uintptr) error {
		synced = append(synced, fd)
		return original(fd)
	}
	if err := userToolWrite(filepath.Join(base, "receipt.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 2 {
		t.Fatalf("publication F_FULLFSYNC calls = %d, want file and directory", len(synced))
	}
	if err := userDirectorySync(base); err != nil || len(synced) != 3 {
		t.Fatalf("directory sync = %v after %d F_FULLFSYNC calls", err, len(synced))
	}
	injected := errors.New("injected full sync failure")
	userFullSync = func(uintptr) error { return injected }
	if err := userToolWrite(filepath.Join(base, "lost.json"), []byte("{}"), 0600); !errors.Is(err, injected) {
		t.Fatalf("F_FULLFSYNC failure lost: %v", err)
	}
	if err := userDirectorySync(base); !errors.Is(err, injected) {
		t.Fatalf("directory F_FULLFSYNC failure lost: %v", err)
	}
}
