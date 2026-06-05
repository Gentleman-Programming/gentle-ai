package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type stubSnapshotter struct{}

func (stubSnapshotter) Create(snapshotDir string, paths []string) (backup.Manifest, error) {
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return backup.Manifest{}, err
	}
	return backup.Manifest{
		ID:        "snapshot-001",
		CreatedAt: time.Now().UTC(),
	}, nil
}

func TestExecutePlanReportsManualCleanupForNonEmptyDirectory(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}
	svc.now = func() time.Time { return time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC) }

	nonEmptyDir := filepath.Join(homeDir, ".config", "opencode", "skills")
	if err := os.MkdirAll(nonEmptyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nonEmptyDir, "user-skill.md"), []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	statePath := filepath.Join(homeDir, ".gentle-ai", "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(state dir) error = %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{"installed_agents":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	result, err := svc.executePlan(plan{
		backupTargets: []string{statePath},
		operations: []operation{
			removeDirIfEmpty(nonEmptyDir),
		},
	}, []model.AgentID{})
	if err != nil {
		t.Fatalf("executePlan() error = %v", err)
	}

	if len(result.ManualActions) != 1 {
		t.Fatalf("ManualActions len = %d, want 1; got %v", len(result.ManualActions), result.ManualActions)
	}
	if !strings.Contains(result.ManualActions[0], nonEmptyDir) {
		t.Fatalf("manual action should mention %q, got %q", nonEmptyDir, result.ManualActions[0])
	}
}

func TestComponentOperationsSDD_RemovesBaseAndProfileAgentsFromSettings(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}

	initial := []byte(`{
	  "agent": {
	    "sdd-orchestrator": {"mode": "primary", "model": "anthropic:claude-sonnet-4"},
	    "sdd-apply": {"mode": "subagent", "model": "anthropic:claude-sonnet-4"},
	    "sdd-onboard": {"mode": "subagent", "model": "anthropic:claude-sonnet-4"},
	    "sdd-verify": {"mode": "subagent", "model": "anthropic:claude-sonnet-4"},
	    "sdd-orchestrator-fast": {"mode": "primary", "model": "openai:gpt-4.1-mini"},
	    "sdd-apply-fast": {"mode": "subagent", "model": "openai:gpt-4.1-mini"},
	    "sdd-onboard-fast": {"mode": "subagent", "model": "openai:gpt-4.1-mini"},
	    "sdd-verify-fast": {"mode": "subagent", "model": "openai:gpt-4.1-mini"},
	    "my-custom-agent": {"mode": "subagent", "model": "custom:model"}
	  },
	  "theme": "my-user-theme"
	}`)
	if err := os.WriteFile(settingsPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	appliedSettingsRewrite := false
	for _, op := range ops {
		if op.typeID != opRewriteFile || op.path != settingsPath {
			continue
		}
		appliedSettingsRewrite = true
		_, _, err := op.apply(op.path)
		if err != nil {
			t.Fatalf("settings rewrite op.apply() error = %v", err)
		}
	}
	if !appliedSettingsRewrite {
		t.Fatalf("expected settings rewrite operation for %q", settingsPath)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("json.Unmarshal(settings) error = %v", err)
	}

	agentMap, ok := root["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent object missing or invalid: %#v", root["agent"])
	}

	for _, removedKey := range []string{
		"sdd-orchestrator",
		"sdd-apply",
		"sdd-onboard",
		"sdd-verify",
		"sdd-orchestrator-fast",
		"sdd-apply-fast",
		"sdd-onboard-fast",
		"sdd-verify-fast",
	} {
		if _, exists := agentMap[removedKey]; exists {
			t.Fatalf("managed SDD key %q should be removed, got agent map: %#v", removedKey, agentMap)
		}
	}

	if _, exists := agentMap["my-custom-agent"]; !exists {
		t.Fatalf("user-defined agent key should be preserved, got agent map: %#v", agentMap)
	}
	if gotTheme, ok := root["theme"].(string); !ok || gotTheme != "my-user-theme" {
		t.Fatalf("theme should be preserved, got %#v", root["theme"])
	}
}

func TestComponentOperationsSDD_RemovesOnlySelectedProfilesFromSettings(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}

	initial := []byte(`{
	  "agent": {
	    "sdd-orchestrator": {"mode": "primary", "model": "anthropic:claude-sonnet-4"},
	    "sdd-apply": {"mode": "subagent", "model": "anthropic:claude-sonnet-4"},
	    "sdd-orchestrator-cheap": {"mode": "primary", "model": "openai:gpt-4.1-mini"},
	    "sdd-apply-cheap": {"mode": "subagent", "model": "openai:gpt-4.1-mini"},
	    "sdd-orchestrator-gemini": {"mode": "primary", "model": "google:gemini-2.5-pro"},
	    "sdd-apply-gemini": {"mode": "subagent", "model": "google:gemini-2.5-pro"}
	  }
	}`)
	if err := os.WriteFile(settingsPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	svc.SetProfileNamesToRemove([]string{"cheap"})

	ops, _, err := svc.componentOperations(adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == settingsPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("settings rewrite op.apply() error = %v", err)
			}
		}
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("json.Unmarshal(settings) error = %v", err)
	}

	agentMap := root["agent"].(map[string]any)

	if _, exists := agentMap["sdd-orchestrator-cheap"]; exists {
		t.Fatalf("selected profile orchestrator should be removed, got: %#v", agentMap)
	}
	if _, exists := agentMap["sdd-apply-cheap"]; exists {
		t.Fatalf("selected profile sub-agent should be removed, got: %#v", agentMap)
	}
	if _, exists := agentMap["sdd-orchestrator-gemini"]; !exists {
		t.Fatalf("unselected profile should be preserved, got: %#v", agentMap)
	}
	if _, exists := agentMap["sdd-apply-gemini"]; !exists {
		t.Fatalf("unselected profile sub-agent should be preserved, got: %#v", agentMap)
	}
}

func TestComponentOperationsSDD_ClaudeRemovesManagedCommandFiles(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("claude adapter not found in registry")
	}

	commandsDir := adapter.CommandsDir(homeDir)
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(commands dir) error = %v", err)
	}

	managed := []string{"sdd-init.md", "sdd-explore.md", "sdd-onboard.md"}
	for _, name := range managed {
		if err := os.WriteFile(filepath.Join(commandsDir, name), []byte(name), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	customPath := filepath.Join(commandsDir, "my-custom-command.md")
	if err := os.WriteFile(customPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(custom command) error = %v", err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, op := range ops {
		if op.typeID == opRemoveFile {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("remove file op.apply(%q) error = %v", op.path, err)
			}
		}
	}

	for _, name := range managed {
		if _, err := os.Stat(filepath.Join(commandsDir, name)); !os.IsNotExist(err) {
			t.Fatalf("managed command %q should be removed, stat err = %v", name, err)
		}
	}
	if _, err := os.Stat(customPath); err != nil {
		t.Fatalf("custom command should be preserved, stat err = %v", err)
	}
}

func TestComponentOperationsSDD_OpenCodeRemovesManagedPluginSourcesAndModelVariantsCache(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	pluginDir := filepath.Join(homeDir, ".config", "opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(pluginDir) error = %v", err)
	}
	backgroundAgentsPath := filepath.Join(pluginDir, "background-agents.ts")
	modelVariantsPluginPath := filepath.Join(pluginDir, "model-variants.ts")
	thirdPartyPluginPath := filepath.Join(pluginDir, "third-party.ts")
	for _, path := range []string{backgroundAgentsPath, modelVariantsPluginPath} {
		if err := os.WriteFile(path, []byte("managed"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	if err := os.WriteFile(thirdPartyPluginPath, []byte("third-party"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", thirdPartyPluginPath, err)
	}

	cacheDir := filepath.Join(homeDir, ".gentle-ai", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(cacheDir) error = %v", err)
	}
	modelVariantsCachePath := filepath.Join(cacheDir, "model-variants.json")
	modelVariantsTempPath := filepath.Join(cacheDir, "model-variants.json.tmp")
	unrelatedCachePath := filepath.Join(cacheDir, "keep.txt")
	for _, path := range []string{modelVariantsCachePath, modelVariantsTempPath, unrelatedCachePath} {
		if err := os.WriteFile(path, []byte("cache"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	applySDDOpenCodeOperations(t, svc, adapter)

	for _, path := range []string{backgroundAgentsPath, modelVariantsPluginPath, modelVariantsCachePath, modelVariantsTempPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed file %q should be removed; stat err = %v", path, err)
		}
	}
	if _, err := os.Stat(cacheDir); err != nil {
		t.Fatalf("cache directory should be preserved, stat err = %v", err)
	}
	if _, err := os.Stat(unrelatedCachePath); err != nil {
		t.Fatalf("unrelated cache file should be preserved, stat err = %v", err)
	}
	if _, err := os.Stat(thirdPartyPluginPath); err != nil {
		t.Fatalf("third-party plugin should be preserved, stat err = %v", err)
	}
}

func TestComponentOperationsSDD_OpenCodePreservesEmptyModelVariantsCacheDirectory(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	cacheDir := filepath.Join(homeDir, ".gentle-ai", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(cacheDir) error = %v", err)
	}
	for _, name := range []string{"model-variants.json", "model-variants.json.tmp"} {
		path := filepath.Join(cacheDir, name)
		if err := os.WriteFile(path, []byte("cache"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	applySDDOpenCodeOperations(t, svc, adapter)

	if _, err := os.Stat(cacheDir); err != nil {
		t.Fatalf("empty cache directory should be preserved, stat err = %v", err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("ReadDir(cacheDir) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache directory should be empty after managed cleanup, got %d entries", len(entries))
	}
}

func TestComponentOperationsSDD_OpenCodeMissingManagedModelVariantFilesAreNonFatal(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	applySDDOpenCodeOperations(t, svc, adapter)
}

func applySDDOpenCodeOperations(t *testing.T, svc *Service, adapter agents.Adapter) {
	t.Helper()
	ops, _, err := svc.componentOperations(adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}
}

func TestComponentOperationsEngram_ProjectScopeRemovesWorkspaceDataOnly(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"mcp":{"engram":{"command":["engram"]}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	projectDataDir := filepath.Join(workspaceDir, ".engram")
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDataDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDataDir, "memory.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("WriteFile(memory.db) error = %v", err)
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeProject)

	ops, _, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	if _, err := os.Stat(projectDataDir); !os.IsNotExist(err) {
		t.Fatalf("project .engram dir should be removed; err = %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile(settings) error = %v", err)
	}
	if !strings.Contains(string(raw), `"engram"`) {
		t.Fatalf("global engram config should be preserved in project scope, got: %s", string(raw))
	}
}

func TestComponentOperationsEngram_GlobalScopeKeepsWorkspaceProjectData(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	adapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("openCode adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"mcp":{"engram":{"command":["engram"]}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}

	projectDataDir := filepath.Join(workspaceDir, ".engram")
	if err := os.MkdirAll(projectDataDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectDataDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDataDir, "memory.db"), []byte("db"), 0o644); err != nil {
		t.Fatalf("WriteFile(memory.db) error = %v", err)
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeGlobal)

	ops, _, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	if _, err := os.Stat(projectDataDir); err != nil {
		t.Fatalf("project .engram dir should be preserved in global scope, err = %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("ReadFile(settings) error = %v", err)
		}
		return
	}
	if strings.Contains(string(raw), `"engram"`) {
		t.Fatalf("global engram config should be removed in global scope, got: %s", string(raw))
	}
}

func TestComponentOperationsSDD_ClaudeRemovesSkillRegistryHook(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("claude adapter not found in registry")
	}
	settingsPath := adapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {"type": "command", "command": "gentle-ai skill-registry refresh --quiet --no-gitignore --cwd \"${CLAUDE_PROJECT_DIR:-$PWD}\" || true"},
          {"type": "command", "command": "echo keep"}
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "echo pre"}]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == settingsPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("settings rewrite op.apply() error = %v", err)
			}
		}
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "gentle-ai skill-registry refresh") {
		t.Fatalf("managed hook should be removed:\n%s", text)
	}
	if !strings.Contains(text, "echo keep") || !strings.Contains(text, "echo pre") {
		t.Fatalf("unrelated hooks should be preserved:\n%s", text)
	}
}
