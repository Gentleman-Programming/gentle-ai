package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSnapshotBuilderDiscoversTrackedAndUnignoredPathsInLinkedWorktree(t *testing.T) {
	requireSnapshotGit(t)
	parent := t.TempDir()
	repo := filepath.Join(parent, "main")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "init", "-q")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, ".gitignore", "git-ignored.txt\n")
	writeSnapshotFile(t, repo, "tracked.txt", "tracked\n")
	gitSnapshot(t, repo, "add", "--", ".gitignore", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-qm", "base")

	linked := filepath.Join(parent, "linked")
	gitSnapshot(t, repo, "worktree", "add", "-q", "-b", "inventory-test", linked)
	exclude := filepath.Join(repo, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, []byte("info-ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, linked, "untracked.txt", "untracked\n")
	writeSnapshotFile(t, linked, "git-ignored.txt", "ignored\n")
	writeSnapshotFile(t, linked, "info-ignored.txt", "ignored\n")
	for _, nested := range []string{"vendor", "openspec/changes/thin"} {
		path := filepath.Join(linked, filepath.FromSlash(nested))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		gitSnapshot(t, path, "init", "-q")
	}

	got, err := (SnapshotBuilder{Repo: linked}).DiscoverTrackedAndUnignoredPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "openspec/changes/thin", "tracked.txt", "untracked.txt", "vendor"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverTrackedAndUnignoredPaths() = %v, want %v", got, want)
	}
}
