package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

// releasedSDDAsset reads a real render of a retired SDD file from a release.
func releasedSDDAsset(t *testing.T, tag, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "components", "legacyassets", "testdata", tag, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// claudeSettingsWithSDDHook is a settings.json a v3.x release left behind:
// the released PreToolUse(Agent) preflight hook beside user configuration.
const claudeSettingsWithSDDHook = `{
  "model": "opus",
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "echo pre && true"}]},
      {"matcher": "Agent", "hooks": [{"type": "command", "command": "gentle-ai sdd-preflight-hook --agent claude-code", "timeout": 30}]}
    ],
    "Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "echo keep"}]}]
  }
}
`

func seedRetiredSDDAssets(t *testing.T, home string) (owned map[string][]byte, edited map[string][]byte) {
	t.Helper()
	owned = map[string][]byte{
		filepath.Join(home, ".claude", "commands", "gentle-sdd-apply.md"):                        releasedSDDAsset(t, "v3.7.0", "claude-gentle-sdd-apply.md"),
		filepath.Join(home, ".claude", "commands", "sdd-init.md"):                                releasedSDDAsset(t, "v2.0.0", "claude-sdd-init.md"),
		filepath.Join(home, ".claude", "skills", "sdd-init", "SKILL.md"):                         releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md"),
		filepath.Join(home, ".claude", "skills", "sdd-verify", "references", "report-format.md"): releasedSDDAsset(t, "v3.7.0", "skill-sdd-verify-report-format.md"),
		filepath.Join(home, ".claude", "skills", "_shared", "sdd-orchestrator-sections.md"):      releasedSDDAsset(t, "v3.7.0", "skill-shared-sdd-orchestrator-sections.md"),
		filepath.Join(home, ".qwen", "commands", "sdd-init.md"):                                  releasedSDDAsset(t, "v3.7.0", "opencode-sdd-init.md"),
		filepath.Join(home, ".qwen", "skills", "sdd-init", "SKILL.md"):                           releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md"),
	}
	edited = map[string][]byte{
		filepath.Join(home, ".claude", "commands", "gentle-sdd-verify.md"):                   []byte("my own verify command\n"),
		filepath.Join(home, ".qwen", "skills", "sdd-apply", "SKILL.md"):                      []byte("---\nname: sdd-apply\n---\nMy apply.\n"),
		filepath.Join(home, ".qwen", "skills", "_shared", "sdd-orchestrator-sections.md"):    releasedSDDAsset(t, "v3.7.0", "skill-shared-sdd-orchestrator-sections.md"),
		filepath.Join(home, ".claude", "skills", "sdd-init", "my-notes.md"):                  []byte("not an SDD file\n"),
		filepath.Join(home, ".claude", "skills", "_shared", "skill-resolver-local-notes.md"): []byte("unrelated\n"),
	}
	for path, data := range owned {
		mustWriteFile(t, path, data)
	}
	for path, data := range edited {
		mustWriteFile(t, path, data)
	}
	mustWriteFile(t, filepath.Join(home, ".claude", "settings.json"), []byte(claudeSettingsWithSDDHook))
	return owned, edited
}

// TestRunSyncRetiresOwnedSDDSkillsCommandsAndHook covers #5157: a sync after
// SDD retirement removes the SDD skills, slash commands, and the Claude Code
// preflight hook a release installed, snapshots them, and preserves and
// reports every file and hook whose bytes no release wrote. Files beside
// them are never touched, and a kept edited skill holds the shared SDD
// references it reads.
func TestRunSyncRetiresOwnedSDDSkillsCommandsAndHook(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	agents := []string{string(model.AgentClaudeCode), string(model.AgentQwenCode)}
	if err := state.Write(home, state.InstallState{
		InstalledAgents: agents, SelectionConfigured: true,
		Components: []model.ComponentID{model.ComponentSDD, model.ComponentSkills},
	}); err != nil {
		t.Fatal(err)
	}
	owned, edited := seedRetiredSDDAssets(t, home)
	settings := filepath.Join(home, ".claude", "settings.json")

	result, err := RunSync([]string{"--agents", strings.Join(agents, ",")})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	snapshot := backupManifestEntries(t, home)
	for path := range owned {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("owned retired SDD file survived sync: %s (%v)", path, err)
		}
		if _, ok := snapshot[path]; !ok {
			t.Errorf("rollback snapshot omitted removed file %s", path)
		}
		if !slices.Contains(result.ChangedFiles, path) {
			t.Errorf("sync did not report removed file %s", path)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude", "skills", "sdd-verify")); !os.IsNotExist(err) {
		t.Errorf("emptied retired skill directory survived: %v", err)
	}
	for path, data := range edited {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("sync changed %s: %q, %v", path, got, err)
		}
	}
	for _, path := range []string{
		filepath.Join(home, ".claude", "commands", "gentle-sdd-verify.md"),
		filepath.Join(home, ".qwen", "skills", "sdd-apply", "SKILL.md"),
	} {
		if !slices.ContainsFunc(result.ManualActions, func(action string) bool {
			return strings.Contains(action, path) && strings.Contains(action, "move or delete it")
		}) {
			t.Errorf("preserved %s not reported with an action: %v", path, result.ManualActions)
		}
	}

	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sdd-preflight-hook") {
		t.Fatalf("released SDD preflight hook survived sync:\n%s", raw)
	}
	var root struct {
		Model string                      `json:"model"`
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	// Retained-hook writers re-encode the document, so compare values.
	commands := func(event string) (found []any) {
		for _, group := range root.Hooks[event] {
			for _, hook := range group["hooks"].([]any) {
				found = append(found, hook.(map[string]any)["command"])
			}
		}
		return found
	}
	if root.Model != "opus" || !slices.Equal(commands("PreToolUse"), []any{"echo pre && true"}) || !slices.Contains(commands("Stop"), any("echo keep")) {
		t.Fatalf("sync lost user settings while retiring the hook:\n%s", raw)
	}
	if _, ok := snapshot[settings]; !ok {
		t.Error("rollback snapshot omitted the Claude settings")
	}

	again, err := RunSync([]string{"--agents", strings.Join(agents, ",")})
	if err != nil {
		t.Fatalf("second RunSync() error = %v", err)
	}
	for _, path := range again.ChangedFiles {
		if strings.Contains(path, "sdd-") {
			t.Errorf("second sync touched SDD path %s", path)
		}
	}
	for path, data := range edited {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("second sync changed %s: %v", path, err)
		}
	}
}

// A sync that fails after retirement restores every removed file and the
// rewritten settings from the pipeline snapshot, byte for byte.
func TestSyncRollbackRestoresRetiredSDDAssets(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	owned, _ := seedRetiredSDDAssets(t, home)
	settings := filepath.Join(home, ".claude", "settings.json")
	selection := BuildSyncSelection(SyncFlags{}, []model.AgentID{model.AgentClaudeCode})
	rt, err := newSyncRuntimeWithScope(home, selection, ScopeGlobal)
	if err != nil {
		t.Fatal(err)
	}
	plan := rt.stagePlan()
	retireIndex := slices.IndexFunc(plan.Apply, func(step pipeline.Step) bool {
		return step.ID() == "sync:agent:retire-sdd-assets:claude-code"
	})
	if retireIndex < 0 {
		t.Fatal("sync plan has no SDD asset retirement step for Claude Code")
	}
	claudeOwned := map[string][]byte{settings: []byte(claudeSettingsWithSDDHook)}
	for path, data := range owned {
		if strings.Contains(path, ".claude") {
			claudeOwned[path] = data
			if !slices.Contains(rt.managedPaths, path) {
				t.Fatalf("sync snapshot does not declare %s", path)
			}
		}
	}
	changedBeforeFailure := false
	plan.Apply = append(plan.Apply[:retireIndex+1:retireIndex+1], sddRetirementFailingStep{observe: func() {
		raw, _ := os.ReadFile(settings)
		_, err := os.Lstat(filepath.Join(home, ".claude", "skills", "sdd-init", "SKILL.md"))
		changedBeforeFailure = os.IsNotExist(err) && !strings.Contains(string(raw), "sdd-preflight-hook")
	}})
	if execution := pipeline.NewOrchestrator(pipeline.DefaultRollbackPolicy()).Execute(plan); execution.Err == nil {
		t.Fatal("injected failure did not fail the sync")
	}
	if !changedBeforeFailure {
		t.Fatal("retirement did not run before the failure")
	}
	for path, data := range claudeOwned {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("rollback did not restore %s byte for byte: %v", path, err)
		}
	}
}

// Windsurf received its SDD workflow in the project, so only a
// workspace-scoped sync retires it, and that sync never reaches home files.
func TestRunSyncRetiresWindsurfSDDWorkflowOnlyInWorkspaceScope(t *testing.T) {
	home := t.TempDir()
	setSyncTestHome(t, home)
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/main\n"))
	workflow := filepath.Join(workspace, ".windsurf", "workflows", "sdd-new.md")
	released := releasedSDDAsset(t, "v3.7.0", "windsurf-sdd-new.md")
	skill := releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md")
	mustWriteFile(t, workflow, released)
	homeSkill := filepath.Join(home, ".codeium", "windsurf", "skills", "sdd-init", "SKILL.md")
	mustWriteFile(t, homeSkill, skill)
	t.Chdir(workspace)

	if _, err := RunSync([]string{"--agents", string(model.AgentWindsurf)}); err != nil {
		t.Fatalf("global RunSync() error = %v", err)
	}
	if got, err := os.ReadFile(workflow); err != nil || string(got) != string(released) {
		t.Fatalf("global sync touched the project workflow: %v", err)
	}
	if _, err := os.Lstat(homeSkill); !os.IsNotExist(err) {
		t.Fatalf("global sync kept the owned home skill: %v", err)
	}

	mustWriteFile(t, homeSkill, skill)
	if _, err := RunSync([]string{"--agents", string(model.AgentWindsurf), "--scope", "workspace"}); err != nil {
		t.Fatalf("workspace RunSync() error = %v", err)
	}
	if _, err := os.Lstat(workflow); !os.IsNotExist(err) {
		t.Fatalf("workspace sync kept the owned SDD workflow: %v", err)
	}
	if _, err := os.Stat(homeSkill); err != nil {
		t.Fatalf("workspace sync touched a home-level SDD skill: %v", err)
	}
}

// Install retires the same inventory as sync: a workspace-scoped install
// never touches home files, and a global one removes what a release wrote.
func TestRunInstallRetiresOwnedSDDAssetsInItsScope(t *testing.T) {
	home := installTestHome(t)
	restoreBackupHome := backup.UserHomeDirFn
	backup.UserHomeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { backup.UserHomeDirFn = restoreBackupHome })
	command := filepath.Join(home, ".claude", "commands", "gentle-sdd-apply.md")
	skill := filepath.Join(home, ".claude", "skills", "sdd-init", "SKILL.md")
	settings := filepath.Join(home, ".claude", "settings.json")
	mustWriteFile(t, command, releasedSDDAsset(t, "v3.7.0", "claude-gentle-sdd-apply.md"))
	mustWriteFile(t, skill, releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md"))
	mustWriteFile(t, settings, []byte(claudeSettingsWithSDDHook))
	workspace := t.TempDir()
	workspaceSkill := filepath.Join(workspace, ".claude", "skills", "sdd-init", "SKILL.md")
	mustWriteFile(t, workspaceSkill, releasedSDDAsset(t, "v3.7.0", "skill-sdd-init.md"))

	t.Chdir(workspace)
	if _, err := RunInstall([]string{"--agent", "claude-code", "--component", "skills", "--scope", "workspace"}, system.DetectionResult{}); err != nil {
		t.Fatalf("workspace RunInstall() error = %v", err)
	}
	if _, err := os.Lstat(workspaceSkill); !os.IsNotExist(err) {
		t.Errorf("workspace install kept the owned workspace skill: %v", err)
	}
	for _, path := range []string{command, skill} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("workspace-scoped install touched home file %s: %v", path, err)
		}
	}
	// Other install steps manage the home Claude settings in every scope;
	// retirement itself must leave the home hook alone here.
	if raw, _ := os.ReadFile(settings); !strings.Contains(string(raw), "sdd-preflight-hook") {
		t.Fatal("workspace-scoped install retired the home Claude hook")
	}

	if _, err := RunInstall([]string{"--agent", "claude-code", "--component", "skills"}, system.DetectionResult{}); err != nil {
		t.Fatalf("RunInstall() error = %v", err)
	}
	snapshot := backupManifestEntries(t, home)
	for _, path := range []string{command, skill} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("install kept the owned retired SDD file %s: %v", path, err)
		}
		if _, ok := snapshot[path]; !ok {
			t.Errorf("install snapshot omitted %s", path)
		}
	}
	if raw, _ := os.ReadFile(settings); strings.Contains(string(raw), "sdd-preflight-hook") {
		t.Fatalf("install kept the released SDD preflight hook:\n%s", raw)
	}
}
