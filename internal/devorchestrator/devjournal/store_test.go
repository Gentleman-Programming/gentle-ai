package devjournal

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path and every missing parent directory, mirroring the
// helper in internal/sddstatus/engram_project_test.go.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestOpenResolvesJournalRoot covers the three common-dir resolution shapes:
// ordinary checkout, linked worktree (fixture pattern from
// internal/sddstatus/engram_project_test.go:53-55), and no `.git` at all —
// where the fallback must be explicitly observable (design D1: "reported
// explicitly, never silently"), not merely inferable from the path string.
func TestOpenResolvesJournalRoot(t *testing.T) {
	cases := []struct {
		name         string
		setup        func(t *testing.T) (workspaceRoot, wantPath string)
		wantFallback bool
	}{
		{
			name: "ordinary checkout: .git directory is the common dir",
			setup: func(t *testing.T) (string, string) {
				root := filepath.Join(t.TempDir(), "checkout")
				if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
					t.Fatalf("mkdir .git: %v", err)
				}
				return root, filepath.Join(root, ".git", "gentle-ai", "dev-orchestrator", "v1", "example-change", "journal.json")
			},
			wantFallback: false,
		},
		{
			name: "linked worktree: .git is a gitdir pointer, common dir via commondir",
			setup: func(t *testing.T) (string, string) {
				base := t.TempDir()
				mainRepo := filepath.Join(base, "example-repo")
				if err := os.MkdirAll(filepath.Join(mainRepo, ".git"), 0o755); err != nil {
					t.Fatalf("mkdir main .git: %v", err)
				}
				worktreeGitDir := filepath.Join(mainRepo, ".git", "worktrees", "FEATURE-ONE")
				writeFile(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")
				worktree := filepath.Join(base, "example-repo-worktrees", "FEATURE-ONE")
				writeFile(t, filepath.Join(worktree, ".git"), "gitdir: "+worktreeGitDir+"\n")
				return worktree, filepath.Join(mainRepo, ".git", "gentle-ai", "dev-orchestrator", "v1", "example-change", "journal.json")
			},
			wantFallback: false,
		},
		{
			name: "no .git: falls back to workspace-local directory",
			setup: func(t *testing.T) (string, string) {
				root := t.TempDir()
				return root, filepath.Join(root, ".gentle-ai", "dev-orchestrator", "v1", "example-change", "journal.json")
			},
			wantFallback: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspaceRoot, wantPath := tc.setup(t)

			store, err := Open(workspaceRoot, "example-change")
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if store.UsesFallback() != tc.wantFallback {
				t.Fatalf("UsesFallback() = %v, want %v", store.UsesFallback(), tc.wantFallback)
			}
			if store.Path() != wantPath {
				t.Fatalf("Path() = %q, want %q", store.Path(), wantPath)
			}
		})
	}
}
