package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// aliasedSnapshotRepo builds a repository reachable under two spellings: its
// real path, and the same path addressed through a symlinked ancestor. On
// macOS that shape is not exotic -- /var is a symlink to /private/var and
// $TMPDIR lives under it -- so every temporary repository has two spellings.
func aliasedSnapshotRepo(t *testing.T) (realRepo string, aliasedRepo string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realRepo = filepath.Join(base, "real", "repo")
	if err := os.MkdirAll(realRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	template, err := snapshotRepoTemplate()
	if err != nil {
		t.Fatalf("snapshot repo template: %v", err)
	}
	if err := os.CopyFS(realRepo, os.DirFS(template)); err != nil {
		t.Fatalf("CopyFS(snapshot repo template): %v", err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	return realRepo, filepath.Join(base, "alias", "repo")
}

// TestResolveRepositoryRootAcceptsBothSpellingsOfOneRepository is the
// positive control for 1773 boundary 2: a repository addressed through a
// symlinked ancestor must resolve to the same root as the real spelling, from
// the root itself and from a nested path inside it.
func TestResolveRepositoryRootAcceptsBothSpellingsOfOneRepository(t *testing.T) {
	realRepo, aliasedRepo := aliasedSnapshotRepo(t)
	nested := filepath.Join(aliasedRepo, "nested", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	want, err := (SnapshotBuilder{Repo: realRepo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepositoryRoot(real spelling) error = %v", err)
	}
	for name, repo := range map[string]string{"aliased root": aliasedRepo, "nested aliased path": nested} {
		got, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
		if err != nil {
			t.Fatalf("ResolveRepositoryRoot(%s) error = %v", name, err)
		}
		if got != want {
			t.Fatalf("ResolveRepositoryRoot(%s) = %q, want %q", name, got, want)
		}
	}
}

// TestResolveRepositoryRootStillRejectsAForeignPath keeps the containment
// decision a real decision after it stopped comparing strings.
func TestResolveRepositoryRootStillRejectsAForeignPath(t *testing.T) {
	realRepo, _ := aliasedSnapshotRepo(t)
	foreign := t.TempDir()
	if _, err := (SnapshotBuilder{Repo: filepath.Join(realRepo, "..", "..", filepath.Base(foreign))}).
		ResolveRepositoryRoot(context.Background()); err == nil {
		t.Fatal("a path outside the repository resolved a repository root")
	}
}

// TestAuthoritativeStorePathsAreCanonicalUnderAnAliasedRepository establishes
// which side 1773 boundary 3 falls on.
//
// The reported symptom is that six legacy tests building Store{Dir:
// t.TempDir()} fail on macOS because the root-anchored O_NOFOLLOW walk refuses
// to traverse the /var symlink, while the same battery passes under
// /private/tmp. This test asks whether production can ever hand the secure
// walk a path with a symlinked ancestor. It cannot: every store constructor
// derives its directory from the Git common directory, which is resolved
// through filepath.EvalSymlinks before it is used, so the walk only ever sees
// a fully resolved path. The failing tests bypass that constructor and hand
// the raw, unresolved t.TempDir() straight to the walk.
func TestAuthoritativeStorePathsAreCanonicalUnderAnAliasedRepository(t *testing.T) {
	_, aliasedRepo := aliasedSnapshotRepo(t)

	compact, err := CompactAuthoritativeStore(context.Background(), aliasedRepo, "alias-lineage")
	if err != nil {
		t.Fatalf("CompactAuthoritativeStore(aliased repository) error = %v", err)
	}
	legacy, err := AuthoritativeStore(context.Background(), aliasedRepo, "alias-lineage")
	if err != nil {
		t.Fatalf("AuthoritativeStore(aliased repository) error = %v", err)
	}

	for name, dir := range map[string]string{"compact store": compact.Dir, "legacy store": legacy.Dir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		if resolved != filepath.Clean(dir) {
			t.Fatalf("%s directory %q is not canonical; it resolves to %q, so the secure walk would be asked to traverse a symlink",
				name, dir, resolved)
		}
	}
}
