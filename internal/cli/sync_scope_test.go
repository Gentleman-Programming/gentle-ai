package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/opencoderuntimeplugins"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/state"
	"github.com/gentleman-programming/gentle-ai/v3/internal/verify"
)

// ─── Parser: sync --scope ────────────────────────────────────────────────────

func TestParseSyncFlagsScope(t *testing.T) {
	t.Parallel()
	flags, err := ParseSyncFlags([]string{"--scope", "workspace"})
	if err != nil {
		t.Fatalf("ParseSyncFlags(--scope workspace) error = %v", err)
	}
	if flags.Scope != "workspace" {
		t.Errorf("Scope = %q, want %q", flags.Scope, "workspace")
	}

	flags, err = ParseSyncFlags([]string{"--scope=global"})
	if err != nil {
		t.Fatalf("ParseSyncFlags(--scope=global) error = %v", err)
	}
	if flags.Scope != "global" {
		t.Errorf("Scope = %q, want %q", flags.Scope, "global")
	}

	flags, err = ParseSyncFlags(nil)
	if err != nil {
		t.Fatalf("ParseSyncFlags(nil) error = %v", err)
	}
	if flags.Scope != "" {
		t.Errorf("default Scope = %q, want empty (resolved to global later)", flags.Scope)
	}
}

func TestRunSyncRejectsUnsupportedScope(t *testing.T) {
	_, err := RunSync([]string{"--scope", "repo", "--agents", "opencode"})
	if err == nil {
		t.Fatal("RunSync(--scope repo) error = nil, want unsupported scope refusal")
	}
	if !strings.Contains(err.Error(), "unsupported scope") {
		t.Fatalf("RunSync(--scope repo) error = %v, want it to name the unsupported scope", err)
	}
}

// ─── Workspace scope regression (issue #1074) ────────────────────────────────

// snapshotTree records every file below root as "permissions:sha256" (or its
// symlink target) and every directory as "dir:permissions", keyed by
// root-relative path, so a before/after comparison detects byte, permission,
// creation, and deletion differences for files AND directories.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			info, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			snapshot[rel] = fmt.Sprintf("dir:%o", info.Mode().Perm())
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			snapshot[rel] = "symlink:" + target
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		snapshot[rel] = fmt.Sprintf("%o:%s", info.Mode().Perm(), hex.EncodeToString(sum[:]))
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot tree %q: %v", root, err)
	}
	return snapshot
}

// TestSyncWorkspaceScopeDeclaresAndVerifiesPersonaWorkspaceTarget pins the
// persona half of the #1074 ownership contract: a workspace sync must declare
// the workspace persona prompt in the backup/rollback contract and verify it
// after the run, and it must never declare or mutate the global persona file
// (or create anything else in the global tree, directories included).
func TestSyncWorkspaceScopeDeclaresAndVerifiesPersonaWorkspaceTarget(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()

	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentPersona},
		Persona:    model.PersonaNeutral,
	}
	adapters := resolveAdapters(selection.Agents)
	adapter := adapters[0]
	globalPrompt := adapter.SystemPromptFile(home)
	workspacePrompt := adapter.SystemPromptFile(workspace)

	// Rollback contract: the workspace persona prompt is snapshotted so a
	// failed workspace persona write can be restored; the global one is not
	// part of a workspace sync's transaction.
	declared, err := syncBackupTargetsScoped(home, workspace, ScopeWorkspace, selection, adapters)
	if err != nil {
		t.Fatalf("syncBackupTargetsScoped() error = %v", err)
	}
	if !containsString(declared, workspacePrompt) {
		t.Errorf("workspace persona prompt %q missing from rollback contract; declared = %v", workspacePrompt, declared)
	}
	if containsString(declared, globalPrompt) {
		t.Errorf("global persona prompt %q declared under workspace scope", globalPrompt)
	}

	// Verification contract: the workspace persona prompt is a post-sync
	// verification target; the global one never is.
	verification := runPostSyncVerificationScoped(home, workspace, ScopeWorkspace, selection)
	verifiedDeclaration := false
	for _, check := range verification.Checks {
		if check.ID == "verify:sync:file:"+workspacePrompt {
			verifiedDeclaration = true
		}
		if check.ID == "verify:sync:file:"+globalPrompt {
			t.Errorf("verification targets the global persona prompt %q under workspace scope", globalPrompt)
		}
	}
	if !verifiedDeclaration {
		t.Errorf("workspace persona prompt %q missing from verification targets; checks = %v", workspacePrompt, verification.Checks)
	}

	// End to end: the workspace persona write lands, is verified, and the
	// global tree — files and directories — stays byte-identical.
	userPrompt := "user claude instructions\n"
	mustWriteFile(t, globalPrompt, []byte(userPrompt))
	before := snapshotTree(t, home)

	t.Chdir(workspace)
	result, err := RunSyncWithSelectionScope(home, selection, ScopeWorkspace)
	if err != nil {
		t.Fatalf("RunSyncWithSelectionScope() error = %v", err)
	}
	if result.FilesChanged == 0 {
		t.Errorf("workspace persona sync changed nothing: changedFiles = %v", result.ChangedFiles)
	}
	workspaceVerified := false
	for _, check := range result.Verify.Checks {
		if check.ID == "verify:sync:file:"+workspacePrompt {
			workspaceVerified = true
		}
	}
	if !workspaceVerified {
		t.Errorf("workspace persona prompt %q not verified post-sync; checks = %v", workspacePrompt, result.Verify.Checks)
	}
	if !result.Verify.Ready {
		t.Errorf("workspace persona sync verification not ready: %s", verify.RenderReport(result.Verify))
	}
	got, err := os.ReadFile(globalPrompt)
	if err != nil {
		t.Fatalf("ReadFile(global prompt) error = %v", err)
	}
	if string(got) != userPrompt {
		t.Errorf("global persona prompt mutated by workspace sync: %q", got)
	}
	after := snapshotTree(t, home)
	for rel, beforeHash := range before {
		if after[rel] != beforeHash {
			t.Errorf("global tree entry mutated by workspace sync: %q", rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("global tree gained an entry during workspace sync: %q", rel)
		}
	}
}

// TestRunSyncWorkspaceScopeUpdatesWorkspaceWithoutGlobalMutation is the
// isolated CLI regression for issue #1074: `gentle-ai sync --scope=workspace`
// must refresh workspace-scoped managed files while every global file —
// including persisted state, telemetry counters, backups, and plugins —
// stays byte-identical with identical permissions.
func TestRunSyncWorkspaceScopeUpdatesWorkspaceWithoutGlobalMutation(t *testing.T) {
	// The global baseline must stay global even if the environment selects a
	// different install scope.
	t.Setenv(scopeEnvVar, "")
	home := t.TempDir()
	globalWorkspace := t.TempDir()
	workspace := t.TempDir()

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVersionRunner := opencode.VersionRunnerOverride
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		opencode.VersionRunnerOverride = restoreVersionRunner
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.2.3")}, nil
	}

	// Phase 1: establish the global baseline with a default (global) sync.
	t.Chdir(globalWorkspace)
	globalResult, err := RunSync([]string{"--agents", "opencode"})
	if err != nil {
		t.Fatalf("global sync error = %v", err)
	}
	if !globalResult.Verify.Ready {
		t.Fatalf("global sync verification not ready: %#v", globalResult.Verify)
	}
	statePath := state.Path(home)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("global sync did not persist state: %v", err)
	}

	before := snapshotTree(t, home)

	// Phase 2: workspace-scoped refresh in a different project.
	t.Chdir(workspace)
	result, err := RunSync([]string{"--agents", "opencode", "--scope", "workspace"})
	if err != nil {
		t.Fatalf("workspace sync error = %v", err)
	}
	if result.FilesChanged == 0 {
		t.Fatalf("workspace sync changed nothing: filesChanged = 0, changedFiles = %v", result.ChangedFiles)
	}
	for _, path := range result.ChangedFiles {
		if !pathUnder(path, workspace) {
			t.Errorf("workspace sync reported a change outside the workspace: %q", path)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "opencode.json")); err != nil {
		t.Errorf("workspace sync did not materialize the project OpenCode settings OpenCode actually loads: %v", err)
	}
	foundSkip := false
	for _, action := range result.ManualActions {
		if strings.Contains(action, "gga") {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Errorf("workspace sync did not report skipping the global-only gga component; manual actions = %v", result.ManualActions)
	}

	// The global tree — state, telemetry, backups, plugins, prompts — must be
	// byte-identical with identical permissions.
	after := snapshotTree(t, home)
	for rel, beforeHash := range before {
		if after[rel] != beforeHash {
			t.Errorf("global file mutated by workspace sync: %q", rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("global tree gained a file during workspace sync: %q", rel)
		}
	}
}

// TestRunSyncWorkspaceScopeUpdatesOpenCodeManagedComponentsWithoutGlobalMutation
// is the bounded component-matrix regression for issue #1074: an OpenCode
// workspace sync selecting Engram, Context7, Skills, and Permission (the
// components that resolve a workspace-managed OpenCode settings authority)
// must write and verify their workspace targets while the global tree — files
// and directories — stays byte-identical. GGA, ClaudeTheme, and the OpenCode
// Gentle Logo are deliberately absent: they are global-only and skipped by
// design, not silently expected to work.
func TestRunSyncWorkspaceScopeUpdatesOpenCodeManagedComponentsWithoutGlobalMutation(t *testing.T) {
	// The global baseline must stay global even if the environment selects a
	// different install scope.
	t.Setenv(scopeEnvVar, "")
	home := t.TempDir()
	globalWorkspace := t.TempDir()
	workspace := t.TempDir()

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVersionRunner := opencode.VersionRunnerOverride
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		opencode.VersionRunnerOverride = restoreVersionRunner
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.2.3")}, nil
	}

	// Phase 1: establish the global baseline with a default (global) sync.
	t.Chdir(globalWorkspace)
	if _, err := RunSync([]string{"--agents", "opencode"}); err != nil {
		t.Fatalf("global sync error = %v", err)
	}
	before := snapshotTree(t, home)

	// Phase 2: workspace sync of the OpenCode-supported component matrix.
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentEngram, model.ComponentContext7, model.ComponentSkills, model.ComponentPermission},
		Skills:     []model.SkillID{model.SkillGoTesting},
		Persona:    model.PersonaNeutral,
	}
	t.Chdir(workspace)
	result, err := RunSyncWithSelectionScope(home, selection, ScopeWorkspace)
	if err != nil {
		t.Fatalf("workspace component sync error = %v", err)
	}
	if result.FilesChanged == 0 {
		t.Fatalf("workspace component sync changed nothing: changedFiles = %v", result.ChangedFiles)
	}
	for _, path := range result.ChangedFiles {
		if !pathUnder(path, workspace) {
			t.Errorf("workspace component sync reported a change outside the workspace: %q", path)
		}
	}

	workspaceSettings := filepath.Join(workspace, "opencode.json")
	if _, err := os.Stat(workspaceSettings); err != nil {
		t.Errorf("workspace sync did not materialize workspace OpenCode settings: %v", err)
	}
	var settingsVerified, skillsVerified bool
	for _, check := range result.Verify.Checks {
		if check.ID == "verify:sync:file:"+workspaceSettings {
			settingsVerified = true
		}
		if strings.Contains(check.ID, "/skills/") && strings.Contains(check.ID, workspace) && check.Status == verify.CheckStatusPassed {
			skillsVerified = true
		}
	}
	if !settingsVerified {
		t.Errorf("workspace OpenCode settings %q missing from verification targets; checks = %v", workspaceSettings, result.Verify.Checks)
	}
	if !skillsVerified {
		t.Errorf("workspace skills target missing from passing verification checks; checks = %v", result.Verify.Checks)
	}
	if !result.Verify.Ready {
		t.Errorf("workspace component sync verification not ready: %s", verify.RenderReport(result.Verify))
	}

	// The global tree — files and directories — must stay byte-identical.
	after := snapshotTree(t, home)
	for rel, beforeHash := range before {
		if after[rel] != beforeHash {
			t.Errorf("global tree entry mutated by workspace component sync: %q", rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("global tree gained an entry during workspace component sync: %q", rel)
		}
	}
}

// TestRunSyncWorkspaceScopeUpdatesClaudeEngramWithoutGlobalMutation closes the
// Claude Code half of the #1074 ownership contract. Install's scoped engram
// delivery is InjectWorkspaceWithOptions (userScope=false): it writes the
// adapter's workspace MCP config — Claude Code: <workspace>/.claude/mcp/
// engram.json — while the user-scope API would write <workspace>/.claude.json.
// The workspace sync must dispatch to the scoped API, declare the workspace
// file in the backup/rollback contract, verify it post-sync, and leave the
// global tree byte-identical. Context7 is included to pin the already-correct
// workspace .mcp.json target alongside it.
func TestRunSyncWorkspaceScopeUpdatesClaudeEngramWithoutGlobalMutation(t *testing.T) {
	// The global baseline must stay global even if the environment selects a
	// different install scope.
	t.Setenv(scopeEnvVar, "")
	home := t.TempDir()
	globalWorkspace := t.TempDir()
	workspace := t.TempDir()

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
	})

	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }

	// Phase 1: establish the global baseline with a default (global) sync.
	t.Chdir(globalWorkspace)
	if _, err := RunSync([]string{"--agents", "claude-code"}); err != nil {
		t.Fatalf("global sync error = %v", err)
	}
	before := snapshotTree(t, home)

	// Phase 2: workspace sync of Engram + Context7 for Claude Code.
	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentEngram, model.ComponentContext7},
		Persona:    model.PersonaNeutral,
	}
	t.Chdir(workspace)
	result, err := RunSyncWithSelectionScope(home, selection, ScopeWorkspace)
	if err != nil {
		t.Fatalf("workspace Claude engram sync error = %v", err)
	}
	if result.FilesChanged == 0 {
		t.Fatalf("workspace Claude engram sync changed nothing: changedFiles = %v", result.ChangedFiles)
	}
	for _, path := range result.ChangedFiles {
		if !pathUnder(path, workspace) {
			t.Errorf("workspace Claude engram sync reported a change outside the workspace: %q", path)
		}
	}

	workspaceEngram := filepath.Join(workspace, ".claude", "mcp", "engram.json")
	workspaceContext7 := filepath.Join(workspace, ".mcp.json")
	for _, target := range []string{workspaceEngram, workspaceContext7} {
		if _, err := os.Stat(target); err != nil {
			t.Errorf("workspace Claude sync did not materialize %q: %v", target, err)
		}
	}

	// Rollback contract: the workspace engram MCP file is snapshotted; the
	// global one is not part of a workspace sync's transaction.
	declared, err := syncBackupTargetsScoped(home, workspace, ScopeWorkspace, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatalf("syncBackupTargetsScoped() error = %v", err)
	}
	if !containsString(declared, workspaceEngram) {
		t.Errorf("workspace engram MCP file %q missing from rollback contract; declared = %v", workspaceEngram, declared)
	}
	if containsString(declared, filepath.Join(home, ".claude", "mcp", "engram.json")) {
		t.Errorf("global engram MCP file declared under workspace scope")
	}

	// Verification: both workspace MCP targets are verified and the report is ready.
	engramVerified := false
	for _, check := range result.Verify.Checks {
		if check.ID == "verify:sync:file:"+workspaceEngram {
			engramVerified = true
		}
	}
	if !engramVerified {
		t.Errorf("workspace engram MCP file %q missing from verification targets; checks = %v", workspaceEngram, result.Verify.Checks)
	}
	if !result.Verify.Ready {
		t.Errorf("workspace Claude engram sync verification not ready: %s", verify.RenderReport(result.Verify))
	}

	// The global tree — files and directories — must stay byte-identical.
	after := snapshotTree(t, home)
	for rel, beforeHash := range before {
		if after[rel] != beforeHash {
			t.Errorf("global tree entry mutated by workspace Claude engram sync: %q", rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("global tree gained an entry during workspace Claude engram sync: %q", rel)
		}
	}
}

// ─── PR #5080 follow-up: scope validation, home-only rollback filtering,
// ─── legacy plugin verification, and the project settings authority.

func TestRunSyncWithSelectionScopeRejectsInvalidScope(t *testing.T) {
	home := t.TempDir()
	_, err := RunSyncWithSelectionScope(home, model.Selection{Agents: []model.AgentID{model.AgentOpenCode}}, "repo")
	if err == nil {
		t.Fatal("RunSyncWithSelectionScope(scope \"repo\") error = nil, want unsupported scope refusal")
	}
	if !strings.Contains(err.Error(), "unsupported scope") {
		t.Fatalf("RunSyncWithSelectionScope(scope \"repo\") error = %v, want it to name the unsupported scope", err)
	}
}

// TestSyncWorkspaceBackupTargetsExcludeHomeOnlyPaths pins the rollback
// contract for issue #1074: a workspace sync must never declare a home-only
// path — not from component declarations, not from cleanup declarations — so
// a failed workspace sync can never roll back (restore) global files. The
// settings authority declared for OpenCode is the project file OpenCode
// actually loads, never the global one.
func TestSyncWorkspaceBackupTargetsExcludeHomeOnlyPaths(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	// Pre-existing global OpenCode settings make the global authority real.
	mustWriteFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), []byte(`{}`))

	selection := model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode, model.AgentClaudeCode},
		Components: []model.ComponentID{model.ComponentEngram, model.ComponentContext7, model.ComponentPermission, model.ComponentPersona},
		Persona:    model.PersonaNeutral,
	}
	declared, err := syncBackupTargetsScoped(home, workspace, ScopeWorkspace, selection, resolveAdapters(selection.Agents))
	if err != nil {
		t.Fatalf("syncBackupTargetsScoped() error = %v", err)
	}
	for _, path := range declared {
		if pathUnderHomeOnly(path, home, workspace) {
			t.Errorf("workspace rollback contract declares a home-only path: %q", path)
		}
	}
	if containsString(declared, filepath.Join(home, ".config", "opencode", "opencode.json")) {
		t.Errorf("global OpenCode settings declared under workspace scope")
	}
	if !containsString(declared, filepath.Join(workspace, "opencode.json")) {
		t.Errorf("workspace project settings %q missing from rollback contract; declared = %v", filepath.Join(workspace, "opencode.json"), declared)
	}
}

// TestRunSyncWorkspaceScopeSucceedsWithGlobalLegacyPlugin pins the
// verification contract for issue #1074: the global legacy-plugin check is a
// global-scope concern only. A pre-existing global legacy plugin must not fail
// (or be touched by) a workspace refresh.
func TestRunSyncWorkspaceScopeSucceedsWithGlobalLegacyPlugin(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()

	restoreHome := osUserHomeDir
	restoreBackupHome := backup.UserHomeDirFn
	restoreCommand := runCommand
	restoreLookPath := cmdLookPath
	restoreVersionRunner := opencode.VersionRunnerOverride
	t.Cleanup(func() {
		osUserHomeDir = restoreHome
		backup.UserHomeDirFn = restoreBackupHome
		runCommand = restoreCommand
		cmdLookPath = restoreLookPath
		opencode.VersionRunnerOverride = restoreVersionRunner
	})
	osUserHomeDir = func() (string, error) { return home, nil }
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	runCommand = func(string, ...string) error { return nil }
	cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
		return opencode.CommandOutput{Stdout: []byte("1.2.3")}, nil
	}

	legacy := filepath.Join(home, ".config", "opencode", "plugins", opencoderuntimeplugins.LegacyOpenCodeReviewPluginName)
	mustWriteFile(t, legacy, []byte("legacy plugin"))

	t.Chdir(workspace)
	result, err := RunSyncWithSelectionScope(home, model.Selection{
		Agents:     []model.AgentID{model.AgentOpenCode},
		Components: []model.ComponentID{model.ComponentEngram, model.ComponentContext7},
		Persona:    model.PersonaNeutral,
	}, ScopeWorkspace)
	if err != nil {
		t.Fatalf("workspace sync with global legacy plugin error = %v", err)
	}
	if !result.Verify.Ready {
		t.Errorf("workspace sync verification not ready with a pre-existing global legacy plugin: %s", verify.RenderReport(result.Verify))
	}
	if _, err := os.Lstat(legacy); err != nil {
		t.Errorf("workspace sync must not touch the global legacy plugin: %v", err)
	}
}

// TestRunSyncWorkspaceScopeWritesProjectOpenCodeSettings pins the settings
// authority for issue #1074: OpenCode never reads
// <workspace>/.config/opencode/opencode.json, so a workspace sync must write
// and verify the project file OpenCode actually loads — <workspace>/opencode.json,
// or an existing opencode.jsonc — and never the global authority.
func TestRunSyncWorkspaceScopeWritesProjectOpenCodeSettings(t *testing.T) {
	for _, tc := range []struct {
		name, seed, want string
	}{
		{name: "project opencode.json", seed: "opencode.json", want: "opencode.json"},
		{name: "existing opencode.jsonc", seed: "opencode.jsonc", want: "opencode.jsonc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, globalWorkspace, workspace := t.TempDir(), t.TempDir(), t.TempDir()

			restoreHome := osUserHomeDir
			restoreCommand := runCommand
			restoreLookPath := cmdLookPath
			restoreVersionRunner := opencode.VersionRunnerOverride
			t.Cleanup(func() {
				osUserHomeDir = restoreHome
				runCommand = restoreCommand
				cmdLookPath = restoreLookPath
				opencode.VersionRunnerOverride = restoreVersionRunner
			})
			osUserHomeDir = func() (string, error) { return home, nil }
			runCommand = func(string, ...string) error { return nil }
			cmdLookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
			opencode.VersionRunnerOverride = func(context.Context, opencode.Command) (opencode.CommandOutput, error) {
				return opencode.CommandOutput{Stdout: []byte("1.2.3")}, nil
			}

			// Global baseline.
			t.Chdir(globalWorkspace)
			if _, err := RunSync([]string{"--agents", "opencode"}); err != nil {
				t.Fatalf("global sync error = %v", err)
			}
			before := snapshotTree(t, home)

			projectSettings := filepath.Join(workspace, tc.seed)
			mustWriteFile(t, projectSettings, []byte(`{"theme":"user-theme"}`))

			selection := model.Selection{
				Agents:     []model.AgentID{model.AgentOpenCode},
				Components: []model.ComponentID{model.ComponentEngram, model.ComponentContext7, model.ComponentPermission},
				Persona:    model.PersonaNeutral,
			}
			t.Chdir(workspace)
			result, err := RunSyncWithSelectionScope(home, selection, ScopeWorkspace)
			if err != nil {
				t.Fatalf("workspace project-settings sync error = %v", err)
			}

			// The project file gained the managed overlays (engram/context7).
			data, err := os.ReadFile(projectSettings)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", projectSettings, err)
			}
			if !strings.Contains(string(data), "context7") {
				t.Errorf("project settings %q did not gain the context7 overlay: %s", projectSettings, data)
			}
			// The unreadable workspace config-dir settings file is never created.
			if _, err := os.Stat(filepath.Join(workspace, ".config", "opencode", "opencode.json")); !os.IsNotExist(err) {
				t.Errorf("workspace sync wrote the unreadable config-dir settings file: stat err = %v", err)
			}
			// Verification targets the project file.
			settingsVerified := false
			for _, check := range result.Verify.Checks {
				if check.ID == "verify:sync:file:"+projectSettings {
					settingsVerified = true
				}
			}
			if !settingsVerified {
				t.Errorf("project settings %q missing from verification targets; checks = %v", projectSettings, result.Verify.Checks)
			}
			if !result.Verify.Ready {
				t.Errorf("workspace project-settings sync verification not ready: %s", verify.RenderReport(result.Verify))
			}

			// The global tree — files and directories — must stay byte-identical.
			after := snapshotTree(t, home)
			for rel, beforeHash := range before {
				if after[rel] != beforeHash {
					t.Errorf("global tree entry mutated by workspace project-settings sync: %q", rel)
				}
			}
			for rel := range after {
				if _, ok := before[rel]; !ok {
					t.Errorf("global tree gained an entry during workspace project-settings sync: %q", rel)
				}
			}
		})
	}
}
