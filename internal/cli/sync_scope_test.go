package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestParseSyncFlagsAcceptsScopeWorkspace verifies that --scope=workspace is
// accepted and stored in the resolved scope field.
func TestParseSyncFlagsAcceptsScopeWorkspace(t *testing.T) {
	t.Parallel()
	flags, err := ParseSyncFlags([]string{"--scope=workspace"})
	if err != nil {
		t.Fatalf("ParseSyncFlags returned error for --scope=workspace: %v", err)
	}
	if flags.Scope != "workspace" {
		t.Errorf("flags.Scope = %q, want %q", flags.Scope, "workspace")
	}
}

// TestParseSyncFlagsAcceptsScopeGlobal verifies that --scope=global is accepted.
func TestParseSyncFlagsAcceptsScopeGlobal(t *testing.T) {
	t.Parallel()
	flags, err := ParseSyncFlags([]string{"--scope=global"})
	if err != nil {
		t.Fatalf("ParseSyncFlags returned error for --scope=global: %v", err)
	}
	if flags.Scope != "global" {
		t.Errorf("flags.Scope = %q, want %q", flags.Scope, "global")
	}
}

// TestParseSyncFlagsDefaultsScopeToGlobal verifies that omitting --scope yields
// the global default (preserving the existing sync behavior).
func TestParseSyncFlagsDefaultsScopeToGlobal(t *testing.T) {
	t.Parallel()
	flags, err := ParseSyncFlags([]string{})
	if err != nil {
		t.Fatalf("ParseSyncFlags returned error when no --scope flag: %v", err)
	}
	if flags.Scope != "global" {
		t.Errorf("flags.Scope = %q, want default %q", flags.Scope, "global")
	}
}

// TestParseSyncFlagsRejectsInvalidScope verifies that an unknown scope value
// is rejected with a clear error so a typo never silently falls back to global.
func TestParseSyncFlagsRejectsInvalidScope(t *testing.T) {
	t.Parallel()
	_, err := ParseSyncFlags([]string{"--scope=invalid"})
	if err == nil {
		t.Fatal("ParseSyncFlags expected error for --scope=invalid, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported scope") {
		t.Errorf("error = %q, want contains %q", err.Error(), "unsupported scope")
	}
}

// TestSyncBackupTargetsWorkspaceScope verifies that with ScopeWorkspace the
// backup targets include paths rooted under workspaceDir, so a workspace-scoped
// sync refreshes only the per-repo config without touching global state.
func TestSyncBackupTargetsWorkspaceScope(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	workspace := t.TempDir()

	selection := model.Selection{
		Agents: []model.AgentID{model.AgentClaudeCode},
	}
	adapters := resolveAdapters(selection.Agents)

	targets := syncBackupTargets(home, workspace, ScopeWorkspace, selection, adapters)

	if len(targets) == 0 {
		t.Fatal("syncBackupTargets returned no targets")
	}
	hasWorkspacePath := false
	for _, p := range targets {
		if strings.HasPrefix(p, workspace) {
			hasWorkspacePath = true
			break
		}
	}
	if !hasWorkspacePath {
		t.Errorf("ScopeWorkspace targets missing workspace-rooted path; got: %v", targets)
	}
}

// TestSyncBackupTargetsGlobalScope verifies that with ScopeGlobal the backup
// targets are rooted under homeDir, matching the existing sync behavior.
func TestSyncBackupTargetsGlobalScope(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	workspace := t.TempDir()

	selection := model.Selection{
		Agents: []model.AgentID{model.AgentClaudeCode},
	}
	adapters := resolveAdapters(selection.Agents)

	targets := syncBackupTargets(home, workspace, ScopeGlobal, selection, adapters)

	if len(targets) == 0 {
		t.Fatal("syncBackupTargets returned no targets")
	}
	// At least one non-backup-root path must be home-rooted; the backup root
	// itself lives under home/.gentle-ai and is excluded by the rel check.
	hasHomeRootedPath := false
	for _, p := range targets {
		rel, err := filepath.Rel(home, p)
		if err == nil && !strings.HasPrefix(rel, ".gentle-ai") {
			hasHomeRootedPath = true
			break
		}
	}
	if !hasHomeRootedPath {
		t.Errorf("ScopeGlobal targets missing home-rooted path; got: %v", targets)
	}
}
