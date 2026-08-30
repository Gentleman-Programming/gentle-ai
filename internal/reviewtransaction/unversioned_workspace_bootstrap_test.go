package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
)

// #3880: a genuinely unversioned local workspace is bootstrappable, not a
// dead end. Ordinary review needs local Git to freeze and diff candidates but
// no remote, no staged files, and no commits — after `git init` the negotiated
// lifecycle proceeds exactly as it does for a manually initialized unborn
// repository. The guards below pin each side of that contract.
func TestResolveRepositoryRootBootstrapsUnversionedWorkspace(t *testing.T) {
	workspace := canonicalTempDir(t)
	if err := os.WriteFile(filepath.Join(workspace, "candidate.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root, err := (SnapshotBuilder{Repo: workspace}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepositoryRoot() on an unversioned workspace error = %v", err)
	}
	if !pathidentity.SameDirectory(root, workspace) {
		t.Fatalf("ResolveRepositoryRoot() = %q, want the bootstrapped workspace %q", root, workspace)
	}

	ctx := context.Background()
	if _, statErr := os.Lstat(filepath.Join(workspace, ".git")); statErr != nil {
		t.Fatalf("bootstrap did not initialize local Git: %v", statErr)
	}

	// No commit: the bootstrapped repository stays unborn, so the lifecycle
	// enters the intended-untracked collect route instead of freezing a
	// bootstrap commit as review content.
	if _, headErr := runGit(ctx, workspace, nil, nil, "rev-parse", "--verify", "HEAD^{commit}"); headErr == nil {
		t.Fatal("bootstrap must not create a commit; HEAD resolves to a commit")
	}

	// No staging: the index stays empty, so nothing the user did not declare
	// becomes review scope by the bootstrap itself.
	staged, stagedErr := runGit(ctx, workspace, nil, nil, "ls-files", "--cached")
	if stagedErr != nil {
		t.Fatalf("ls-files --cached error = %v", stagedErr)
	}
	if len(strings.TrimSpace(string(staged))) != 0 {
		t.Fatalf("bootstrap must not stage files; index holds %q", string(staged))
	}

	// No remote: ordinary review never contacts one, and the bootstrap must
	// not configure remotes the operator never asked for.
	remotes, remoteErr := runGit(ctx, workspace, nil, nil, "remote")
	if remoteErr != nil {
		t.Fatalf("git remote error = %v", remoteErr)
	}
	if len(strings.TrimSpace(string(remotes))) != 0 {
		t.Fatalf("bootstrap must not configure a remote; found %q", string(remotes))
	}

	// The second resolve now flows through the ordinary repository path with
	// no bootstrap decision at all.
	again, err := (SnapshotBuilder{Repo: workspace}).ResolveRepositoryRoot(ctx)
	if err != nil {
		t.Fatalf("second ResolveRepositoryRoot() error = %v", err)
	}
	if !pathidentity.SameDirectory(again, workspace) {
		t.Fatalf("second ResolveRepositoryRoot() = %q, want %q", again, workspace)
	}
}

// An unborn repository (valid .git, no commits) must keep flowing through the
// normal lifecycle untouched: the bootstrap exists only for workspaces with
// no repository at all.
func TestResolveRepositoryRootKeepsUnbornRepositoryUntouched(t *testing.T) {
	workspace := canonicalTempDir(t)
	if _, err := runGit(context.Background(), workspace, nil, nil, "init"); err != nil {
		t.Fatalf("git init fixture error = %v", err)
	}
	created, err := os.Stat(filepath.Join(workspace, ".git"))
	if err != nil {
		t.Fatal(err)
	}

	root, err := (SnapshotBuilder{Repo: workspace}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatalf("ResolveRepositoryRoot() on an unborn repository error = %v", err)
	}
	if !pathidentity.SameDirectory(root, workspace) {
		t.Fatalf("ResolveRepositoryRoot() = %q, want %q", root, workspace)
	}

	after, err := os.Stat(filepath.Join(workspace, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(created, after) {
		t.Fatal("unborn repository .git was recreated; the bootstrap must not fire for existing repositories")
	}
}

// An ancestor's corrupt `.git` entry must not be silently bypassed by a
// bootstrap `git init`. `rev-parse --show-toplevel` fails identically for "no
// repository anywhere" and "ancestor repository metadata corrupt", so the
// guard has to inspect the ancestor chain itself: initializing beneath broken
// metadata would carve an unintended nested repository whose scope boundary
// silently differs from the one the ancestor declares.
func TestResolveRepositoryRootRefusesBootstrapUnderCorruptAncestorMetadata(t *testing.T) {
	root := canonicalTempDir(t)
	workspace := filepath.Join(root, "workspace", "nested")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	ancestorGit := filepath.Join(root, "workspace", ".git")
	if err := os.WriteFile(ancestorGit, []byte("not a git directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ancestorGit)
	if err != nil {
		t.Fatal(err)
	}

	if resRoot, resolveErr := (SnapshotBuilder{Repo: workspace}).ResolveRepositoryRoot(context.Background()); resolveErr == nil {
		t.Fatalf("ResolveRepositoryRoot() = %q, want a refusal under corrupt ancestor metadata", resRoot)
	}
	if _, statErr := os.Lstat(filepath.Join(workspace, ".git")); !os.IsNotExist(statErr) {
		t.Fatal("bootstrap must not initialize a nested repository beneath corrupt ancestor metadata")
	}
	after, err := os.ReadFile(ancestorGit)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("corrupt ancestor .git was modified: before %q after %q", before, after)
	}
}

// A `.git` lookup on an ancestor that fails with anything other than "does
// not exist" must count as present metadata: falling through would let the
// bootstrap run `git init` beneath an ancestor entry it could not inspect.
// The seam stubs EACCES because a real permission failure is invisible to
// root CI containers.
func TestAncestorHoldsGitEntryFailsClosedOnLookupError(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	orig := ancestorGitLstat
	t.Cleanup(func() { ancestorGitLstat = orig })
	ancestorGitLstat = func(name string) (os.FileInfo, error) {
		if filepath.Base(name) == ".git" {
			return nil, &os.PathError{Op: "lstat", Path: name, Err: os.ErrPermission}
		}
		return orig(name)
	}

	if !ancestorHoldsGitEntry(workspace) {
		t.Fatal("ancestorHoldsGitEntry() = false under a non-ENOENT .git lookup error, want fail-closed true")
	}
}

// Present-but-invalid Git metadata must fail with the original error and keep
// the invalid entry byte-for-byte intact: the bootstrap never overwrites.
func TestResolveRepositoryRootRefusesInvalidGitMetadata(t *testing.T) {
	workspace := canonicalTempDir(t)
	control := filepath.Join(workspace, ".git")
	if err := os.WriteFile(control, []byte("not a git directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(control)
	if err != nil {
		t.Fatal(err)
	}

	if root, resolveErr := (SnapshotBuilder{Repo: workspace}).ResolveRepositoryRoot(context.Background()); resolveErr == nil {
		t.Fatalf("ResolveRepositoryRoot() = %q, want a refusal for invalid Git metadata", root)
	}

	after, err := os.ReadFile(control)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("invalid .git metadata was overwritten: %q -> %q", string(before), string(after))
	}
}
