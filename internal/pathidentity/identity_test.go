package pathidentity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aliasedRoots returns two different spellings of one directory: the real
// path and a path reached through a symlinked ancestor. This is the shape
// that /var vs /private/var produces on macOS, and structurally the same
// shape a case-insensitive or Unicode-equivalent spelling produces: two
// distinct strings that the operating system resolves to one inode.
func aliasedRoots(t *testing.T) (real string, aliased string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real = filepath.Join(base, "real", "repo")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	return real, filepath.Join(base, "alias", "repo")
}

// lexicallyWithin is the comparator this package replaces: filepath.Rel over
// two strings. It is reproduced here so the tests can show, in the same run,
// that the string answer and the filesystem answer disagree.
func lexicallyWithin(root, path string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func TestSameDirectoryAcceptsTwoSpellingsOfOneDirectory(t *testing.T) {
	real, aliased := aliasedRoots(t)
	if lexicallyWithin(real, aliased) {
		t.Fatal("precondition failed: the two spellings compare equal lexically, so this test proves nothing")
	}
	if !SameDirectory(real, aliased) {
		t.Fatalf("SameDirectory(%q, %q) = false, want true", real, aliased)
	}
	if !SameDirectory(aliased, real) {
		t.Fatal("SameDirectory is not symmetric")
	}
}

func TestContainsAcceptsAPathAddressedThroughAnAliasedAncestor(t *testing.T) {
	real, aliased := aliasedRoots(t)
	nested := filepath.Join(aliased, "openspec", "changes", "thin")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if lexicallyWithin(real, nested) {
		t.Fatal("precondition failed: the aliased descendant is already lexically within the real root")
	}
	if !Contains(real, nested) {
		t.Fatalf("Contains(%q, %q) = false, want true", real, nested)
	}
	if !Contains(aliased, filepath.Join(real, "openspec")) {
		t.Fatal("Contains did not accept the real descendant under the aliased root")
	}
}

func TestContainsAcceptsTheRootItself(t *testing.T) {
	real, aliased := aliasedRoots(t)
	if !Contains(real, real) || !Contains(real, aliased) {
		t.Fatal("Contains rejected the root addressed as itself")
	}
}

func TestContainsRejectsSiblingsAndAncestors(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	sibling := filepath.Join(base, "sibling")
	for _, dir := range []string{root, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if Contains(root, sibling) {
		t.Fatal("Contains accepted a sibling directory")
	}
	if Contains(root, base) {
		t.Fatal("Contains accepted the parent directory")
	}
}

// A descendant that does not exist yet is still contained, because identity
// is decided at the deepest ancestor that does exist. This keeps the
// comparator usable before a directory is created.
func TestContainsAcceptsADescendantThatDoesNotExistYet(t *testing.T) {
	real, aliased := aliasedRoots(t)
	if !Contains(real, filepath.Join(aliased, "not", "created", "yet")) {
		t.Fatal("Contains rejected a not-yet-created descendant of the aliased root")
	}
}

func TestContainsRejectsEverythingWhenTheRootDoesNotExist(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(base, "missing")
	if Contains(missing, filepath.Join(missing, "child")) {
		t.Fatal("Contains accepted a path under a root that does not exist")
	}
}

// Documented limit: identity needs a lookup, so a path that does not exist is
// never identical to anything, not even to itself.
func TestSameDirectoryRejectsPathsThatDoNotExist(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(base, "missing")
	if SameDirectory(missing, missing) {
		t.Fatal("SameDirectory accepted a directory that does not exist")
	}
}

// Documented limit: a regular file is never a directory identity.
func TestSameDirectoryRejectsRegularFiles(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if SameDirectory(file, file) {
		t.Fatal("SameDirectory accepted a regular file")
	}
}

// The policy pin. Case folding is the volume's decision, never this package's:
// where the volume creates two directories, they are two identities; where the
// volume folds them into one, they are one identity. This test asserts the
// same rule on a case-sensitive volume (Linux ext4, where the second Mkdir
// succeeds) and on a case-insensitive one (default APFS and NTFS, where it
// fails with ErrExist), so a future reader can see exactly what is deferred to
// the operating system and what is not.
func TestSameDirectoryDefersCaseFoldingToTheVolume(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lower := filepath.Join(base, "repo")
	upper := filepath.Join(base, "REPO")
	if err := os.Mkdir(lower, 0o755); err != nil {
		t.Fatal(err)
	}
	switch err := os.Mkdir(upper, 0o755); {
	case err == nil:
		if SameDirectory(lower, upper) {
			t.Fatal("case-sensitive volume created two directories, but SameDirectory folded them into one")
		}
		if Contains(lower, filepath.Join(upper, "child")) {
			t.Fatal("case-sensitive volume created two directories, but Contains folded them into one")
		}
	case os.IsExist(err):
		if !SameDirectory(lower, upper) {
			t.Fatal("case-insensitive volume folded the two spellings, but SameDirectory reported two identities")
		}
		if !Contains(lower, filepath.Join(upper, "child")) {
			t.Fatal("case-insensitive volume folded the two spellings, but Contains reported two identities")
		}
	default:
		t.Fatal(err)
	}
}
