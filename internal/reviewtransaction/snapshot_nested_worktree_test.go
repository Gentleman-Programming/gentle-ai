package reviewtransaction

// Issue #1881: `git ls-files --others --exclude-standard` reports a nested,
// non-gitignored Git checkout as one opaque directory entry with a trailing
// slash (".wt/test/"), and canonicalPaths rejected that spelling, so a nested
// worktree hard-broke review start. These tests pin the honest behavior: a
// directory `git worktree list --porcelain` names as a linked worktree of this
// repository is excluded from the candidate the same way `.git` is, ordinary
// untracked files (including ones beside the worktree) stay in scope, and an
// opaque nested repository that is NOT a registered worktree is refused with a
// message that names the path and the way out.

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverUnignoredUntrackedExcludesRegisteredNestedWorktree(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	gitSnapshot(t, repo, "worktree", "add", "-q", "-b", "nested-feature", filepath.Join(repo, ".wt", "test"))
	writeSnapshotFile(t, repo, "notes.txt", "untracked\n")
	writeSnapshotFile(t, repo, ".wt/stray.txt", "beside the worktree, not inside it\n")

	intended, err := (SnapshotBuilder{Repo: repo}).DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatalf("DiscoverUnignoredUntracked with a registered nested worktree: %v", err)
	}
	want := []string{".wt/stray.txt", "notes.txt"}
	if !reflect.DeepEqual(intended, want) {
		t.Fatalf("intended untracked = %#v, want %#v (worktree directory excluded, sibling file kept)", intended, want)
	}
}

func TestDiscoverUnignoredUntrackedFromInsideNestedWorktreeStaysScoped(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	nested := filepath.Join(repo, ".wt", "test")
	gitSnapshot(t, repo, "worktree", "add", "-q", "-b", "nested-inside", nested)
	writeSnapshotFile(t, repo, ".wt/test/inner.txt", "untracked inside the nested worktree\n")

	intended, err := (SnapshotBuilder{Repo: nested}).DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatalf("DiscoverUnignoredUntracked from inside the nested worktree: %v", err)
	}
	if !reflect.DeepEqual(intended, []string{"inner.txt"}) {
		t.Fatalf("intended untracked from inside worktree = %#v, want [inner.txt]", intended)
	}
}

func TestDiscoverUnignoredUntrackedIgnoresSiblingLinkedWorktree(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	sibling := filepath.Join(t.TempDir(), "sibling-worktree")
	gitSnapshot(t, repo, "worktree", "add", "-q", "-b", "sibling-feature", sibling)
	writeSnapshotFile(t, repo, "notes.txt", "untracked\n")

	intended, err := (SnapshotBuilder{Repo: repo}).DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatalf("DiscoverUnignoredUntracked with a sibling linked worktree: %v", err)
	}
	if !reflect.DeepEqual(intended, []string{"notes.txt"}) {
		t.Fatalf("intended untracked = %#v, want [notes.txt]", intended)
	}
}

func TestDiscoverUnignoredUntrackedRefusesForeignEmbeddedRepository(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	embedded := filepath.Join(repo, "vendor", "embedded")
	gitSnapshot(t, repo, "init", "-q", embedded)

	_, err := (SnapshotBuilder{Repo: repo}).DiscoverUnignoredUntracked(context.Background())
	if err == nil {
		t.Fatal("DiscoverUnignoredUntracked admitted a foreign embedded repository")
	}
	message := err.Error()
	if !strings.Contains(message, "vendor/embedded") {
		t.Fatalf("embedded-repository refusal does not name the blocking path: %q", message)
	}
	if !strings.Contains(message, ".gitignore") {
		t.Fatalf("embedded-repository refusal does not name a way out: %q", message)
	}
	if strings.Contains(message, "logical path is not canonical") {
		t.Fatalf("embedded-repository refusal still speaks the internal canonicalization error: %q", message)
	}
}
