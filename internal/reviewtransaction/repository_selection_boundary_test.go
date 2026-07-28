package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRepositorySelectionAcceptsEveryCwdAliasOfTheSameRepository proves the
// accepting half of the "Git repository selection" threat-matrix row: a
// relative `--cwd`, an absolute `--cwd`, a trailing-separator form, a
// dot-segment form, a subdirectory (the `git -C <subdir>` shape), and a
// symlink alias all denote one repository, so canonical `--cwd` plus
// common-dir authority must collapse them to one identity and one storage key.
// Divergence would shard review authority across spellings of the same
// repository, which is how a second, invisible authority root gets created.
func TestRepositorySelectionAcceptsEveryCwdAliasOfTheSameRepository(t *testing.T) {
	requireSnapshotGit(t)
	t.Parallel()

	repo := initSnapshotRepo(t)
	nested := filepath.Join(repo, "nested", "path")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(workingDirectory, repo)
	if err != nil {
		t.Fatal(err)
	}
	// The symlink alias lives outside the repository so the alias itself can
	// never be mistaken for repository content.
	symlink := filepath.Join(t.TempDir(), "alias")
	symlinkAvailable := os.Symlink(repo, symlink) == nil

	canonical, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		alias string
		skip  bool
	}{
		{name: "absolute path", alias: repo},
		{name: "relative path", alias: relative},
		{name: "trailing separator", alias: repo + string(filepath.Separator)},
		{name: "dot segment", alias: filepath.Join(repo, "nested", "..", ".")},
		{name: "subdirectory", alias: nested},
		{name: "symlink alias", alias: symlink, skip: !symlinkAvailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.skip {
				t.Skip("symlinks are unavailable on this platform")
			}
			root, err := (SnapshotBuilder{Repo: tt.alias}).ResolveRepositoryRoot(context.Background())
			if err != nil {
				t.Fatalf("ResolveRepositoryRoot(%q) error = %v", tt.alias, err)
			}
			if root != canonical.Identity().RepositoryRoot {
				t.Fatalf("ResolveRepositoryRoot(%q) = %q, want %q", tt.alias, root, canonical.Identity().RepositoryRoot)
			}
			lease, err := OpenRepositoryIdentityLease(context.Background(), tt.alias)
			if err != nil {
				t.Fatalf("OpenRepositoryIdentityLease(%q) error = %v", tt.alias, err)
			}
			if lease.Identity() != canonical.Identity() {
				t.Fatalf("identity for %q = %#v, want %#v", tt.alias, lease.Identity(), canonical.Identity())
			}
			if lease.StorageKey() != canonical.StorageKey() {
				t.Fatalf("storage key for %q = %q, want %q", tt.alias, lease.StorageKey(), canonical.StorageKey())
			}
		})
	}
}

// TestRepositorySelectionRejectsAliasesThatAreNotTheSameRepository proves the
// rejecting half of the same row. An alias that escapes the repository, names
// nothing, or names a different repository must never resolve to the requested
// repository's authority, and resolution must not create storage anywhere.
// Rejection before any authority mutation is proven separately by
// TestGitCommonDirectoryMismatchFailsBeforeAuthorityMutation.
func TestRepositorySelectionRejectsAliasesThatAreNotTheSameRepository(t *testing.T) {
	requireSnapshotGit(t)
	t.Parallel()

	repo := initSnapshotRepo(t)
	unrelated := initSnapshotRepo(t)
	// A directory that is deliberately not inside any repository. It is created
	// under the repository's own parent so the escape form stays realistic.
	outside := t.TempDir()
	missing := filepath.Join(repo, "does-not-exist")

	canonical, err := OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("rejected aliases", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name  string
			alias string
		}{
			{name: "escape above the repository", alias: filepath.Join(repo, "..")},
			{name: "unrelated non-repository directory", alias: outside},
			{name: "missing path", alias: missing},
		} {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if root, err := (SnapshotBuilder{Repo: tt.alias}).ResolveRepositoryRoot(context.Background()); err == nil && root == canonical.Identity().RepositoryRoot {
					t.Fatalf("ResolveRepositoryRoot(%q) = %q, want a rejection", tt.alias, root)
				}
				if lease, err := OpenRepositoryIdentityLease(context.Background(), tt.alias); err == nil && lease.StorageKey() == canonical.StorageKey() {
					t.Fatalf("OpenRepositoryIdentityLease(%q) reused the requested repository authority", tt.alias)
				}
				if _, err := os.Lstat(filepath.Join(tt.alias, "gentle-ai")); !os.IsNotExist(err) {
					t.Fatalf("rejected alias %q created storage: %v", tt.alias, err)
				}
			})
		}
	})

	t.Run("different repository keeps a different identity", func(t *testing.T) {
		t.Parallel()
		other, err := OpenRepositoryIdentityLease(context.Background(), unrelated)
		if err != nil {
			t.Fatal(err)
		}
		if other.Identity() == canonical.Identity() || other.StorageKey() == canonical.StorageKey() {
			t.Fatalf("unrelated repository shared authority: %#v", other.Identity())
		}
	})

	if runtime.GOOS == "windows" {
		return
	}
	t.Run("symlink into a different repository is not the requested repository", func(t *testing.T) {
		t.Parallel()
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(unrelated, alias); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		lease, err := OpenRepositoryIdentityLease(context.Background(), alias)
		if err != nil {
			t.Fatal(err)
		}
		if lease.StorageKey() == canonical.StorageKey() {
			t.Fatal("symlink to a different repository resolved to the requested repository authority")
		}
	})
}
