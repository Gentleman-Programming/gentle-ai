package sddstatus

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path and every missing parent directory.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const originConfig = `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = https://github.com/<owner>/Example-Repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`

// TestInferEngramProjectOrdinaryCheckout pins the already-working shape: a
// `.git` directory is read directly and the project comes from the remote.
func TestInferEngramProjectOrdinaryCheckout(t *testing.T) {
	t.Setenv("ENGRAM_PROJECT", "")

	root := filepath.Join(t.TempDir(), "some-checkout-dir")
	writeFile(t, filepath.Join(root, ".git", "config"), originConfig)

	if got, want := inferEngramProject(root), "example-repo"; got != want {
		t.Fatalf("inferEngramProject(ordinary checkout) = %q, want %q", got, want)
	}
}

// TestInferEngramProjectLinkedWorktree is the regression for #2682. In a linked
// worktree `.git` is a file holding a `gitdir:` pointer, and `config` lives in
// the common dir — not under the pointed-to worktree gitdir. Inference must
// resolve the same project as the primary checkout instead of degrading to the
// worktree's directory basename.
func TestInferEngramProjectLinkedWorktree(t *testing.T) {
	t.Setenv("ENGRAM_PROJECT", "")

	base := t.TempDir()
	mainRepo := filepath.Join(base, "example-repo")
	writeFile(t, filepath.Join(mainRepo, ".git", "config"), originConfig)

	// Git records the per-worktree gitdir under <main>/.git/worktrees/<name>,
	// with a `commondir` pointing back at the shared .git directory.
	worktreeGitDir := filepath.Join(mainRepo, ".git", "worktrees", "FEATURE-ONE")
	writeFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")

	worktree := filepath.Join(base, "example-repo-worktrees", "FEATURE-ONE")
	writeFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")

	if got, want := inferEngramProject(worktree), "example-repo"; got != want {
		t.Fatalf("inferEngramProject(linked worktree) = %q, want %q", got, want)
	}
}

// TestInferEngramProjectLinkedWorktreeRelativeGitdir covers the relative
// `gitdir:` form, which Git writes for worktrees created with a relative path.
func TestInferEngramProjectLinkedWorktreeRelativeGitdir(t *testing.T) {
	t.Setenv("ENGRAM_PROJECT", "")

	base := t.TempDir()
	mainRepo := filepath.Join(base, "example-repo")
	writeFile(t, filepath.Join(mainRepo, ".git", "config"), originConfig)
	writeFile(t, filepath.Join(mainRepo, ".git", "worktrees", "wt", "commondir"), "../..\n")

	worktree := filepath.Join(base, "wt")
	writeFile(t, filepath.Join(worktree, ".git"), "gitdir: ../example-repo/.git/worktrees/wt\n")

	if got, want := inferEngramProject(worktree), "example-repo"; got != want {
		t.Fatalf("inferEngramProject(relative gitdir) = %q, want %q", got, want)
	}
}

// TestInferEngramProjectNoRepository keeps the basename fallback for paths that
// genuinely belong to no repository.
func TestInferEngramProjectNoRepository(t *testing.T) {
	t.Setenv("ENGRAM_PROJECT", "")

	root := filepath.Join(t.TempDir(), "Loose-Directory")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", root, err)
	}

	if got, want := inferEngramProject(root), "loose-directory"; got != want {
		t.Fatalf("inferEngramProject(no repository) = %q, want %q", got, want)
	}
}

// TestInferEngramProjectEnvOverride keeps the explicit override authoritative
// over any discovered repository identity.
func TestInferEngramProjectEnvOverride(t *testing.T) {
	t.Setenv("ENGRAM_PROJECT", "Explicit-Project")

	root := filepath.Join(t.TempDir(), "example-repo")
	writeFile(t, filepath.Join(root, ".git", "config"), originConfig)

	if got, want := inferEngramProject(root), "explicit-project"; got != want {
		t.Fatalf("inferEngramProject(env override) = %q, want %q", got, want)
	}
}
