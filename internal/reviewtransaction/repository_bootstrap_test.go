package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
)

func TestPrepareReviewRepositoryRootInitializesOnlyGenuinelyUnversionedWorkspace(t *testing.T) {
	t.Run("initializes an unversioned workspace", func(t *testing.T) {
		repo := t.TempDir()
		candidate := filepath.Join(repo, "candidate.txt")
		if err := os.WriteFile(candidate, []byte("candidate\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		root, err := PrepareReviewRepositoryRoot(context.Background(), repo)
		if err != nil {
			t.Fatalf("prepare local Git repository: %v", err)
		}
		// Identity, not string equality: on Windows the runner's TEMP is an
		// 8.3 short name that Git reports back in its long spelling.
		if !pathidentity.SameDirectory(root, repo) {
			t.Fatalf("prepared root = %q, want %q", root, repo)
		}
		if info, statErr := os.Stat(filepath.Join(repo, ".git")); statErr != nil || !info.IsDir() {
			t.Fatalf("local Git metadata = %v (%v), want directory", info, statErr)
		}
		if content, readErr := os.ReadFile(candidate); readErr != nil || string(content) != "candidate\n" {
			t.Fatalf("candidate after bootstrap = %q, %v", content, readErr)
		}
	})

	t.Run("reuses a containing worktree", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		nested := filepath.Join(repo, "nested", "workspace")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}

		root, err := PrepareReviewRepositoryRoot(context.Background(), nested)
		if err != nil {
			t.Fatalf("prepare containing repository: %v", err)
		}
		if !pathidentity.SameDirectory(root, repo) {
			t.Fatalf("prepared root = %q, want containing root %q", root, repo)
		}
		if _, statErr := os.Lstat(filepath.Join(nested, ".git")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("nested metadata = %v, want no nested .git", statErr)
		}
	})

	t.Run("refuses unusable metadata without modifying it", func(t *testing.T) {
		repo := t.TempDir()
		metadata := filepath.Join(repo, ".git")
		before := []byte("not a git directory\n")
		if err := os.WriteFile(metadata, before, 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := PrepareReviewRepositoryRoot(context.Background(), repo); err == nil {
			t.Fatal("prepare accepted unusable Git metadata")
		}
		after, err := os.ReadFile(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("Git metadata changed:\nbefore: %q\nafter:  %q", before, after)
		}
	})
}

func TestSnapshotBuilderResolveRepositoryRootDoesNotBootstrap(t *testing.T) {
	repo := t.TempDir()
	if _, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background()); err == nil {
		t.Fatal("ResolveRepositoryRoot accepted an unversioned workspace")
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ResolveRepositoryRoot created metadata: %v", err)
	}
}
