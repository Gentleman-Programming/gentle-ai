package uninstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/codex"
	"github.com/gentleman-programming/gentle-ai/v2/internal/backup"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/engram"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type stubSnapshotter struct{}

func TestBuildPlanSnapshotsPiManifestAndOwnedOverlay(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	packageChild := filepath.Join(homeDir, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	if err := os.MkdirAll(filepath.Dir(packageChild), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageChild, []byte("---\ntools: bash\n---\npackage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: homeDir, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatal(err)
	}
	plan, err := svc.buildPlan([]model.AgentID{model.AgentPi}, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := communitytool.PiCodeGraphPaths(homeDir, "")
	for _, path := range paths {
		if !slices.Contains(plan.backupTargets, path) {
			t.Fatalf("backup targets = %v, missing Pi artifact %q", plan.backupTargets, path)
		}
	}
}

func TestExecutePlanCleansPiBeforeSharedMCPMutation(t *testing.T) {
	home := t.TempDir()
	svc, err := NewService(home, t.TempDir(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	svc.snapshotter = stubSnapshotter{}
	mcpPath := filepath.Join(home, ".pi", "agent", "mcp.json")
	child := filepath.Join(home, ".pi", "agent", "subagents", "worker.md")
	if err := os.MkdirAll(filepath.Dir(child), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("---\ntools: bash\n---\nwork\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: home, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.executePlan(plan{operations: []operation{{path: mcpPath, apply: func(path string) (bool, bool, error) {
		return true, false, os.WriteFile(path, []byte(`{"mcpServers":{"engram":{"command":"engram"}}}`), 0o600)
	}}}}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatal(err)
	}
	if body := string(mustReadServiceFile(t, mcpPath)); strings.Contains(body, `"codegraph"`) {
		t.Fatalf("false drift preserved CodeGraph entry: %s", body)
	}
	if slices.ContainsFunc(result.ManualActions, func(action string) bool { return strings.Contains(action, "CodeGraph MCP drifted") }) {
		t.Fatalf("manual actions = %v, want no false drift", result.ManualActions)
	}
}

func mustReadServiceFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExecutePlanPiUninstallPreservesPreexistingMarkedUserChildAndUserMCP(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}
	mcpPath := filepath.Join(homeDir, ".pi", "agent", "mcp.json")
	childPath := filepath.Join(homeDir, ".pi", "agent", "subagents", "worker.md")
	preexisting := "---\ntools: bash, mcp\n---\nuser instructions\n\n<!-- gentle-ai:pi-codegraph-tool -->\npreexisting tool guidance\n<!-- /gentle-ai:pi-codegraph -->\n\n<!-- gentle-ai:pi-codegraph-guidance -->\npreexisting lazy-init guidance\n<!-- /gentle-ai:pi-codegraph -->\n"
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"user":{"command":"user-server"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(childPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte(preexisting), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: homeDir, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v", err)
	}

	result, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatalf("executePlan() error = %v", err)
	}
	if got, err := os.ReadFile(childPath); err != nil || string(got) != preexisting {
		t.Fatalf("service uninstall child = %q, err = %v; want exact preexisting content", got, err)
	}
	if got, err := os.ReadFile(mcpPath); err != nil || !strings.Contains(string(got), "user-server") || strings.Contains(string(got), `"codegraph"`) {
		t.Fatalf("service uninstall MCP = %q, err = %v", got, err)
	}
	if len(result.ManualActions) != 0 {
		t.Fatalf("service uninstall manual actions = %v, want none", result.ManualActions)
	}
	if _, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi}); err != nil {
		t.Fatalf("repeat service uninstall error = %v", err)
	}
}

func TestExecutePlanPiUninstallPreservesDriftedChildAndGentlePiSource(t *testing.T) {
	homeDir := t.TempDir()
	svc, err := NewService(homeDir, t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}
	childPath := filepath.Join(homeDir, ".pi", "agent", "subagents", "worker.md")
	packageChild := filepath.Join(homeDir, ".pi", "agent", "node_modules", "gentle-pi", "subagents", "package.md")
	packageBody := "---\ntools: bash\n---\npackage instructions\n"
	if err := os.MkdirAll(filepath.Dir(childPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("---\ntools: bash\n---\nuser instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(packageChild), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(packageChild, []byte(packageBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := communitytool.ReconcilePiCodeGraph(communitytool.PiCodeGraphOptions{HomeDir: homeDir, Selected: true, EffectiveMCPProbe: piCodeGraphProbeForServiceTest}); err != nil {
		t.Fatalf("ReconcilePiCodeGraph() error = %v", err)
	}
	if err := os.WriteFile(childPath, append([]byte("user changed after provision\n"), []byte("keep this\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := svc.executePlan(plan{}, []model.AgentID{model.AgentPi})
	if err != nil {
		t.Fatalf("executePlan() error = %v", err)
	}
	if got, err := os.ReadFile(childPath); err != nil || !strings.Contains(string(got), "keep this") {
		t.Fatalf("drifted child was not preserved: %q, err = %v", got, err)
	}
	if got, err := os.ReadFile(packageChild); err != nil || string(got) != packageBody {
		t.Fatalf("gentle-pi package child changed: %q, err = %v", got, err)
	}
	if !slices.ContainsFunc(result.ManualActions, func(action string) bool { return strings.Contains(action, "child drifted") }) {
		t.Fatalf("manual actions = %v, want drift action", result.ManualActions)
	}
}

func piCodeGraphProbeForServiceTest(string) (communitytool.PiCodeGraphMCPProbeResult, error) {
	return communitytool.PiCodeGraphMCPProbeResult{
		AdapterAvailable: true,
		Initialized:      true,
		Tools: []communitytool.PiCodeGraphMCPTool{{
			Name: "codegraph_explore",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":       map[string]any{"type": "string"},
					"maxFiles":    map[string]any{"type": "number"},
					"projectPath": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		}},
	}, nil
}

func TestExpandVisualPolishUninstallComponents(t *testing.T) {
	for _, trigger := range model.VisualPolishComponents() {
		t.Run(string(trigger), func(t *testing.T) {
			got := expandVisualPolishUninstallComponents([]model.ComponentID{trigger})
			for _, want := range model.VisualPolishComponents() {
				if !slices.Contains(got, want) {
					t.Fatalf("expanded visual polish components missing %q: %v", want, got)
				}
			}
		})
	}

	unchanged := expandVisualPolishUninstallComponents([]model.ComponentID{model.ComponentPersona})
	if !slices.Equal(unchanged, []model.ComponentID{model.ComponentPersona}) {
		t.Fatalf("non-polish components should not expand: %v", unchanged)
	}
}

func TestPartialUninstallVisualPolishSelectionRemovesThemeLogoGroup(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	svc.snapshotter = stubSnapshotter{}

	opencodeAdapter, ok := svc.registry.Get(model.AgentOpenCode)
	if !ok {
		t.Fatal("opencode adapter not found in registry")
	}
	settingsPath := opencodeAdapter.SettingsPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(opencode settings dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"theme":"gentleman-kanagawa","keep":true}`), 0o644); err != nil {
		t.Fatalf("WriteFile(opencode settings) error = %v", err)
	}

	logoPath := filepath.Join(homeDir, ".config", "opencode", "tui-plugins", "gentle-logo.tsx")
	if err := os.MkdirAll(filepath.Dir(logoPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(logo dir) error = %v", err)
	}
	if err := os.WriteFile(logoPath, []byte("// managed logo"), 0o644); err != nil {
		t.Fatalf("WriteFile(logo) error = %v", err)
	}

	claudeThemePath := filepath.Join(homeDir, ".claude", "themes", "gentleman.json")
	if err := os.MkdirAll(filepath.Dir(claudeThemePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(claude theme dir) error = %v", err)
	}
	if err := os.WriteFile(claudeThemePath, []byte(`{"name":"Gentleman"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(claude theme) error = %v", err)
	}

	if _, err := svc.PartialUninstall(
		[]model.AgentID{model.AgentOpenCode, model.AgentClaudeCode},
		[]model.ComponentID{model.ComponentTheme},
	); err != nil {
		t.Fatalf("PartialUninstall() error = %v", err)
	}

	settings := readJSONFileForTest(t, settingsPath)
	if _, exists := settings["theme"]; exists {
		t.Fatalf("theme should be removed from OpenCode settings: %#v", settings)
	}
	if got := settings["keep"]; got != true {
		t.Fatalf("user setting should be preserved, got %#v", got)
	}
	if _, err := os.Stat(logoPath); !os.IsNotExist(err) {
		t.Fatalf("OpenCode logo should be removed by visual polish group uninstall, err = %v", err)
	}
	if _, err := os.Stat(claudeThemePath); !os.IsNotExist(err) {
		t.Fatalf("Claude theme should be removed by visual polish group uninstall, err = %v", err)
	}
}

func readJSONFileForTest(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", path, err)
	}
	return root
}

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

func TestComponentOperationsContext7ClaudeRemovesSettingsAndManagedLegacyFile(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("Claude adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	legacyPath := adapter.MCPConfigPath(homeDir, "context7")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy dir) error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"mcpServers":{"context7":{"command":"npx"},"engram":{"command":"engram"}},"theme":"dark"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	legacyManaged := []byte(`{
  "command": "npx",
  "args": [
    "-y",
    "--package=@upstash/context7-mcp@1.0.0",
    "--",
    "context7-mcp"
  ]
}
`)
	if err := os.WriteFile(legacyPath, legacyManaged, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	ops, targets, err := svc.componentOperations(adapter, model.ComponentContext7)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	if !slices.Contains(targets, settingsPath) || !slices.Contains(targets, legacyPath) {
		t.Fatalf("targets = %#v, want settings and legacy paths", targets)
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("operation %v on %q error = %v", op.typeID, op.path, err)
		}
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy managed context7 file should be removed; stat err = %v", err)
	}
	settings := readJSONFileForTest(t, settingsPath)
	mcpServers := settings["mcpServers"].(map[string]any)
	if _, ok := mcpServers["context7"]; ok {
		t.Fatalf("settings still contains mcpServers.context7: %#v", settings)
	}
	if _, ok := mcpServers["engram"]; !ok {
		t.Fatalf("settings lost unrelated mcpServers.engram: %#v", settings)
	}
}

func TestComponentOperationsContext7ClaudePreservesCustomLegacyFile(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentClaudeCode)
	if !ok {
		t.Fatal("Claude adapter not found in registry")
	}

	settingsPath := adapter.SettingsPath(homeDir)
	legacyPath := adapter.MCPConfigPath(homeDir, "context7")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(settings dir) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy dir) error = %v", err)
	}
	custom := []byte(`{"command":"custom-context7"}`)
	if err := os.WriteFile(settingsPath, []byte(`{"mcpServers":{"context7":{"command":"npx"}}}`), 0o644); err != nil {
		t.Fatalf("WriteFile(settings) error = %v", err)
	}
	if err := os.WriteFile(legacyPath, custom, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentContext7)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("operation %v on %q error = %v", op.typeID, op.path, err)
		}
	}

	got, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("ReadFile(legacy) error = %v", err)
	}
	if string(got) != string(custom) {
		t.Fatalf("custom legacy file changed: %s", string(got))
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
	skillRegistryPluginPath := filepath.Join(pluginDir, "skill-registry.ts")
	thirdPartyPluginPath := filepath.Join(pluginDir, "third-party.ts")
	for _, path := range []string{backgroundAgentsPath, modelVariantsPluginPath, skillRegistryPluginPath} {
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
	modelVariantsRandomTempPath := filepath.Join(cacheDir, "model-variants.json.a1b2c3.tmp")
	unrelatedCachePath := filepath.Join(cacheDir, "keep.txt")
	unrelatedTempPaths := []string{
		filepath.Join(cacheDir, "model-variants.json.abc12.tmp"),
		filepath.Join(cacheDir, "model-variants.json.abc1234.tmp"),
		filepath.Join(cacheDir, "model-variants.json.ABC123.tmp"),
		filepath.Join(cacheDir, "model-variants.json.notes.tmp"),
	}
	for _, path := range append([]string{modelVariantsCachePath, modelVariantsTempPath, modelVariantsRandomTempPath, unrelatedCachePath}, unrelatedTempPaths...) {
		if err := os.WriteFile(path, []byte("cache"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}

	applySDDOpenCodeOperations(t, svc, adapter)

	for _, path := range []string{backgroundAgentsPath, modelVariantsPluginPath, skillRegistryPluginPath, modelVariantsCachePath, modelVariantsTempPath, modelVariantsRandomTempPath} {
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
	for _, path := range unrelatedTempPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("unrelated model variants temp-like file should be preserved, stat err = %v", err)
		}
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
	for _, name := range []string{"model-variants.json", "model-variants.json.tmp", "model-variants.json.d4e5f6.tmp"} {
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

// TestComponentOperationsEngram_CodexRemovesConsolidatedProtocolAssetsWithNoOrphans
// is the task 2.9 regression assertion: the canonical-asset consolidation
// (design.md Decision 3) renamed/removed the SOURCE assets
// (internal/assets/claude/engram-protocol.md, codex/engram-instructions.md,
// codex/engram-compact-prompt.md -> internal/assets/engram/protocol.md), but
// the WRITTEN on-disk paths for Codex (~/.codex/engram-instructions.md,
// ~/.codex/engram-compact-prompt.md) MUST stay byte-identical so the
// uninstaller keeps covering them with no orphaned files left behind.
func TestComponentOperationsEngram_CodexRemovesConsolidatedProtocolAssetsWithNoOrphans(t *testing.T) {
	restore := codex.SetRuntimeVersionCommandForTest("codex-cli 0.144.0", nil)
	t.Cleanup(restore)
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentCodex)
	if !ok {
		t.Fatal("codex adapter not found in registry")
	}

	// Actually write the files via the real (post-consolidation) engram
	// injector, instead of hand-crafting fixtures, so this test fails if the
	// renderer ever drifts from the on-disk paths the uninstaller expects.
	if _, err := engram.InjectWithOptions(homeDir, adapter, engram.InjectOptions{}); err != nil {
		t.Fatalf("engram.InjectWithOptions(codex) error = %v", err)
	}

	instructionsPath := filepath.Join(homeDir, ".codex", "engram-instructions.md")
	compactPath := filepath.Join(homeDir, ".codex", "engram-compact-prompt.md")
	for _, path := range []string{instructionsPath, compactPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected engram injection to create %q: %v", path, err)
		}
	}

	svc.SetEngramUninstallScope(model.EngramUninstallScopeGlobal)

	ops, targets, err := svc.componentOperations(adapter, model.ComponentEngram)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}

	for _, want := range []string{instructionsPath, compactPath} {
		if !slices.Contains(targets, want) {
			t.Fatalf("componentOperations() targets missing %q; got: %v", want, targets)
		}
	}

	for _, op := range ops {
		if _, _, err := op.apply(op.path); err != nil {
			t.Fatalf("op.apply(%q) error = %v", op.path, err)
		}
	}

	for _, path := range []string{instructionsPath, compactPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %q to be removed by uninstall, err = %v", path, err)
		}
	}

	// No orphaned directory left behind either.
	if entries, err := os.ReadDir(filepath.Join(homeDir, ".codex")); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "engram-") {
				t.Fatalf("orphaned engram asset left behind after uninstall: %s", entry.Name())
			}
		}
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

func TestComponentOperationsSDD_CodexRemovesSkillRegistryHook(t *testing.T) {
	homeDir := t.TempDir()
	workspaceDir := t.TempDir()

	svc, err := NewService(homeDir, workspaceDir, "dev")
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	adapter, ok := svc.registry.Get(model.AgentCodex)
	if !ok {
		t.Fatal("codex adapter not found in registry")
	}
	hooksPath := filepath.Join(adapter.GlobalConfigDir(homeDir), "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          {"type": "command", "command": "gentle-ai skill-registry refresh --quiet --no-gitignore --cwd \"$PWD\" || true"},
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
	if err := os.WriteFile(hooksPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	ops, _, err := svc.componentOperations(adapter, model.ComponentSDD)
	if err != nil {
		t.Fatalf("componentOperations() error = %v", err)
	}
	for _, op := range ops {
		if op.typeID == opRewriteFile && op.path == hooksPath {
			if _, _, err := op.apply(op.path); err != nil {
				t.Fatalf("Codex hooks rewrite op.apply() error = %v", err)
			}
		}
	}
	raw, err := os.ReadFile(hooksPath)
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
