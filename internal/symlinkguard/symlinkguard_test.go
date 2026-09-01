package symlinkguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func createTestSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == 1314 {
			t.Skipf("symlinks unavailable on this Windows build: %v", err)
		}
		t.Fatalf("Symlink(%q, %q): %v", target, link, err)
	}
}

// A path spelled through a symlinked ancestor points at the same location as
// the allowed root. Containment must recognise that, otherwise legitimate
// layouts (macOS /var -> /private/var and /tmp, Linux /home -> /export/home)
// read as escapes.
func TestEnsureWithinRootAcceptsSymlinkedAncestorSpelling(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	root := filepath.Join(real, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	link := filepath.Join(base, "link")
	createTestSymlink(t, real, link)

	target := filepath.Join(link, "root", "file.txt")
	if err := EnsureWithinRoot(target, root, target); err != nil {
		t.Fatalf("in-root target spelled through a symlinked ancestor must be accepted: %v", err)
	}
}

// The widened comparison must not weaken the guard: a target that genuinely
// resolves outside the allowed root is still rejected.
func TestEnsureWithinRootStillRejectsEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	target := filepath.Join(outside, "file.txt")
	if err := EnsureWithinRoot(target, root, target); err == nil {
		t.Fatal("a target resolving outside the allowed root must still be rejected")
	}
}

// A symlinked ancestor must not become a way out of the root either. This
// exercises the real contract: resolvePath walks and checks one link at a
// time, so the escape has to be caught during resolution.
func TestResolveExistingRejectsEscapeThroughSymlinkedAncestor(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	createTestSymlink(t, outside, filepath.Join(root, "escape"))

	if _, _, err := ResolveExisting(filepath.Join(root, "escape", "file.txt")); err == nil {
		t.Fatal("an escape through a symlinked ancestor must still be rejected")
	}
}

// Dangling targets stay comparable: only the existing prefix is resolved.
func TestCanonicalizeExistingKeepsMissingComponents(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "does", "not", "exist.txt")

	got, err := canonicalPath(missing)
	if err != nil {
		t.Fatalf("canonicalPath(%q): %v", missing, err)
	}
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("eval base: %v", err)
	}
	want := filepath.Join(resolvedBase, "does", "not", "exist.txt")
	if got != want {
		t.Fatalf("canonicalPath(%q) = %q, want %q", missing, got, want)
	}
}

func TestSafeRemovalPathCanonicalizesInRootSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("Mkdir(): %v", err)
	}
	linkDir := filepath.Join(root, "linked")
	createTestSymlink(t, realDir, linkDir)

	path := filepath.Join(linkDir, "config.json")
	if err := os.WriteFile(filepath.Join(realDir, "config.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	safePath, err := SafeRemovalPath(path, root)
	if err != nil {
		t.Fatalf("SafeRemovalPath() error = %v", err)
	}
	if safePath != filepath.Join(realDir, "config.json") {
		t.Fatalf("SafeRemovalPath() = %q, want %q", safePath, filepath.Join(realDir, "config.json"))
	}
}

func TestSafeRemovalPathPreservesFinalSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	link := filepath.Join(root, "config.json")
	createTestSymlink(t, target, link)

	safePath, err := SafeRemovalPath(link, root)
	if err != nil {
		t.Fatalf("SafeRemovalPath() error = %v", err)
	}
	if safePath != link {
		t.Fatalf("SafeRemovalPath() = %q, want final symlink %q", safePath, link)
	}
}

func TestSafeRemovalPathRejectsExternalSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	linkDir := filepath.Join(root, "linked")
	createTestSymlink(t, outside, linkDir)

	_, err := SafeRemovalPath(filepath.Join(linkDir, "config.json"), root)
	if err == nil {
		t.Fatal("SafeRemovalPath() error = nil, want allowed-root rejection")
	}
	if !strings.Contains(err.Error(), "outside allowed root") {
		t.Fatalf("SafeRemovalPath() error = %v, want allowed-root rejection", err)
	}
}

func TestSafeRemovalPathAllowsConfiguredRootSymlink(t *testing.T) {
	base := t.TempDir()
	rootTarget := t.TempDir()
	root := filepath.Join(base, "workspace")
	createTestSymlink(t, rootTarget, root)

	path := filepath.Join(root, "config.json")
	if err := os.WriteFile(filepath.Join(rootTarget, "config.json"), []byte("managed\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	safePath, err := SafeRemovalPath(path, root)
	if err != nil {
		t.Fatalf("SafeRemovalPath() error = %v", err)
	}
	if safePath != filepath.Join(rootTarget, "config.json") {
		t.Fatalf("SafeRemovalPath() = %q, want %q", safePath, filepath.Join(rootTarget, "config.json"))
	}
}
