package symlinkguard

import (
	"os"
	"path/filepath"
	"testing"
)

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
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink ancestor: %v", err)
	}

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
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}

	if _, _, err := ResolveExisting(filepath.Join(root, "escape", "file.txt")); err == nil {
		t.Fatal("an escape through a symlinked ancestor must still be rejected")
	}
}

// Dangling targets stay comparable: only the existing prefix is resolved.
func TestCanonicalizeExistingKeepsMissingComponents(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "does", "not", "exist.txt")

	got := canonicalizeExisting(missing)
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("eval base: %v", err)
	}
	want := filepath.Join(resolvedBase, "does", "not", "exist.txt")
	if got != want {
		t.Fatalf("canonicalizeExisting(%q) = %q, want %q", missing, got, want)
	}
}
