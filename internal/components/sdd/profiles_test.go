package sdd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/assets"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestResolveProfileStrategy_ExplicitWins(t *testing.T) {
	home := t.TempDir()

	profilesDir := filepath.Join(home, ".config", "opencode", "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(profiles): %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "active.json"), []byte(`{"name":"external"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(active.json): %v", err)
	}

	got := ResolveProfileStrategy(home, model.SDDProfileStrategyGeneratedMulti)
	if got != model.SDDProfileStrategyGeneratedMulti {
		t.Fatalf("ResolveProfileStrategy(explicit) = %q, want %q", got, model.SDDProfileStrategyGeneratedMulti)
	}
}

func TestResolveProfileStrategy_AutoDetectsExternalProfiles(t *testing.T) {
	home := t.TempDir()

	profilesDir := filepath.Join(home, ".config", "opencode", "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(profiles): %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "cheap.json"), []byte(`{"name":"cheap"}`), 0o644); err != nil {
		t.Fatalf("WriteFile(cheap.json): %v", err)
	}

	got := ResolveProfileStrategy(home, "")
	if got != model.SDDProfileStrategyExternalSingleActive {
		t.Fatalf("ResolveProfileStrategy(auto) = %q, want %q", got, model.SDDProfileStrategyExternalSingleActive)
	}
}

func TestResolveProfileStrategy_DefaultsGeneratedMultiWithoutExternalProfiles(t *testing.T) {
	home := t.TempDir()

	got := ResolveProfileStrategy(home, "")
	if got != model.SDDProfileStrategyGeneratedMulti {
		t.Fatalf("ResolveProfileStrategy(no external profiles) = %q, want %q", got, model.SDDProfileStrategyGeneratedMulti)
	}
}

// ─── ValidateProfileName ───────────────────────────────────────────────────

func TestValidateProfileName_Valid(t *testing.T) {
	valid := []string{
		"cheap",
		"premium-v2",
		"a",
		"123",
		"my-profile",
		"a1b2",
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateProfileName(name); err != nil {
				t.Errorf("ValidateProfileName(%q) = %v, want nil", name, err)
			}
		})
	}
}

func TestValidateProfileName_Invalid(t *testing.T) {
	tests := []struct {
		name string
		desc string
	}{
		{"", "empty"},
		{"default", "reserved word"},
		{"sdd-orchestrator", "reserved word"},
		{"my profile", "contains space"},
		{"has spaces", "contains spaces"},
		{"has_underscores", "slug convention: lowercase + hyphens only"},
		{"LOUD", "uppercase"},
		{"My-Profile", "mixed case"},
		{"trailing-", "trailing hyphen"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if err := ValidateProfileName(tt.name); err == nil {
				t.Errorf("ValidateProfileName(%q) = nil, want error (%s)", tt.name, tt.desc)
			}
		})
	}
}

// ─── ProfileAgentKeys ─────────────────────────────────────────────────────

func TestProfileAgentKeys_Named(t *testing.T) {
	keys := ProfileAgentKeys("cheap")

	want := []string{
		"sdd-orchestrator-cheap",
		"sdd-init-cheap",
		"sdd-explore-cheap",
		"sdd-propose-cheap",
		"sdd-spec-cheap",
		"sdd-design-cheap",
		"sdd-tasks-cheap",
		"sdd-apply-cheap",
		"sdd-verify-cheap",
		"sdd-archive-cheap",
		"sdd-onboard-cheap",
	}

	if len(keys) != len(want) {
		t.Fatalf("ProfileAgentKeys(\"cheap\") returned %d keys, want %d\ngot: %v", len(keys), len(want), keys)
	}

	// Build maps for order-insensitive comparison
	got := make(map[string]bool, len(keys))
	for _, k := range keys {
		got[k] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing key %q", w)
		}
	}
}

func TestProfileAgentKeys_Default(t *testing.T) {
	keys := ProfileAgentKeys("")

	want := []string{
		"sdd-orchestrator",
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
		"sdd-onboard",
	}

	if len(keys) != len(want) {
		t.Fatalf("ProfileAgentKeys(\"\") returned %d keys, want %d\ngot: %v", len(keys), len(want), keys)
	}

	got := make(map[string]bool, len(keys))
	for _, k := range keys {
		got[k] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing key %q", w)
		}
	}
}

func TestProfileAgentKeys_Count(t *testing.T) {
	if n := len(ProfileAgentKeys("cheap")); n != 11 {
		t.Errorf("ProfileAgentKeys(\"cheap\") = %d keys, want 11", n)
	}
	if n := len(ProfileAgentKeys("")); n != 11 {
		t.Errorf("ProfileAgentKeys(\"\") = %d keys, want 11", n)
	}
}

// ─── DetectProfiles ───────────────────────────────────────────────────────

func TestDetectProfiles_SingleProfile(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "mode": "primary", "prompt": "orchestrator" },
    "sdd-orchestrator-cheap": { "mode": "primary", "model": "anthropic:claude-haiku-3-5" },
    "sdd-init-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-explore-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-propose-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-spec-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-design-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-tasks-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-apply-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-verify-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-archive-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-onboard-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	profiles, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("DetectProfiles() returned %d profiles, want 1", len(profiles))
	}

	p := profiles[0]
	if p.Name != "cheap" {
		t.Errorf("Profile.Name = %q, want %q", p.Name, "cheap")
	}
	if p.OrchestratorModel.ProviderID != "anthropic" {
		t.Errorf("OrchestratorModel.ProviderID = %q, want %q", p.OrchestratorModel.ProviderID, "anthropic")
	}
	if p.OrchestratorModel.ModelID != "claude-haiku-3-5" {
		t.Errorf("OrchestratorModel.ModelID = %q, want %q", p.OrchestratorModel.ModelID, "claude-haiku-3-5")
	}
}

func TestDetectProfiles_DefaultOnly(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "mode": "primary" },
    "sdd-init": { "mode": "subagent" },
    "sdd-apply": { "mode": "subagent" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	profiles, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() error = %v", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("DetectProfiles() returned %d profiles, want 0 (default is not a detected profile)", len(profiles))
	}
}

func TestDetectProfiles_MissingFile(t *testing.T) {
	profiles, err := DetectProfiles("/nonexistent/opencode.json")
	if err != nil {
		t.Fatalf("DetectProfiles() with missing file returned error = %v, want nil", err)
	}
	if len(profiles) != 0 {
		t.Fatalf("DetectProfiles() with missing file returned %d profiles, want 0", len(profiles))
	}
}

func TestDetectProfiles_MalformedJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	if err := os.WriteFile(settingsPath, []byte(`{ not valid json `), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	_, err := DetectProfiles(settingsPath)
	if err == nil {
		t.Fatal("DetectProfiles() with malformed JSON should return error, got nil")
	}
}

func TestDetectProfiles_TwoProfiles(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	content := `{
  "agent": {
    "sdd-orchestrator": { "mode": "primary" },
    "sdd-orchestrator-cheap": { "mode": "primary", "model": "anthropic:claude-haiku-3-5" },
    "sdd-init-cheap": { "mode": "subagent", "model": "anthropic:claude-haiku-3-5" },
    "sdd-explore-cheap": { "mode": "subagent" },
    "sdd-propose-cheap": { "mode": "subagent" },
    "sdd-spec-cheap": { "mode": "subagent" },
    "sdd-design-cheap": { "mode": "subagent" },
    "sdd-tasks-cheap": { "mode": "subagent" },
    "sdd-apply-cheap": { "mode": "subagent" },
    "sdd-verify-cheap": { "mode": "subagent" },
    "sdd-archive-cheap": { "mode": "subagent" },
    "sdd-onboard-cheap": { "mode": "subagent" },
    "sdd-orchestrator-premium": { "mode": "primary", "model": "anthropic:claude-opus-4-5" },
    "sdd-init-premium": { "mode": "subagent", "model": "anthropic:claude-opus-4-5" },
    "sdd-explore-premium": { "mode": "subagent" },
    "sdd-propose-premium": { "mode": "subagent" },
    "sdd-spec-premium": { "mode": "subagent" },
    "sdd-design-premium": { "mode": "subagent" },
    "sdd-tasks-premium": { "mode": "subagent" },
    "sdd-apply-premium": { "mode": "subagent" },
    "sdd-verify-premium": { "mode": "subagent" },
    "sdd-archive-premium": { "mode": "subagent" },
    "sdd-onboard-premium": { "mode": "subagent" }
  }
}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	profiles, err := DetectProfiles(settingsPath)
	if err != nil {
		t.Fatalf("DetectProfiles() error = %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("DetectProfiles() returned %d profiles, want 2; got %v", len(profiles), profiles)
	}

	// Must be sorted by name
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.Name
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)

	for i := range names {
		if names[i] != sorted[i] {
			t.Errorf("profiles not sorted by name: got %v", names)
			break
		}
	}
	if profiles[0].Name != "cheap" {
		t.Errorf("profiles[0].Name = %q, want %q", profiles[0].Name, "cheap")
	}
	if profiles[1].Name != "premium" {
		t.Errorf("profiles[1].Name = %q, want %q", profiles[1].Name, "premium")
	}
}

// ─── GenerateProfileOverlay ───────────────────────────────────────────────

func makeHaikuProfile() model.Profile {
	haikuModel := model.ModelAssignment{ProviderID: "anthropic", ModelID: "claude-haiku-3-5"}
	phases := map[string]model.ModelAssignment{}
	for _, ph := range []string{
		"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec",
		"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify",
		"sdd-archive", "sdd-onboard",
	} {
		phases[ph] = haikuModel
	}
	return model.Profile{
		Name:              "cheap",
		OrchestratorModel: haikuModel,
		PhaseAssignments:  phases,
	}
}

func TestGenerateProfileOverlay_Structure(t *testing.T) {
	home := t.TempDir()

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("overlay is not valid JSON: %v", err)
	}

	agentRaw, ok := root["agent"]
	if !ok {
		t.Fatal("overlay missing 'agent' key")
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		t.Fatal("overlay 'agent' is not an object")
	}

	// Must have 11 agents
	if len(agentMap) != 11 {
		t.Errorf("agent map has %d entries, want 11", len(agentMap))
	}

	// Orchestrator checks
	orchRaw, ok := agentMap["sdd-orchestrator-cheap"]
	if !ok {
		t.Fatal("missing sdd-orchestrator-cheap")
	}
	orch, ok := orchRaw.(map[string]any)
	if !ok {
		t.Fatal("sdd-orchestrator-cheap is not an object")
	}
	if mode, _ := orch["mode"].(string); mode != "primary" {
		t.Errorf("sdd-orchestrator-cheap mode = %q, want %q", mode, "primary")
	}
	if model, _ := orch["model"].(string); model != "anthropic/claude-haiku-3-5" {
		t.Errorf("sdd-orchestrator-cheap model = %q, want %q", model, "anthropic/claude-haiku-3-5")
	}
	if prompt, _ := orch["prompt"].(string); !strings.Contains(prompt, "Agent Teams") && !strings.Contains(prompt, "Orchestrator") {
		t.Errorf("sdd-orchestrator-cheap prompt does not contain orchestrator content; got: %q", prompt[:min(100, len(prompt))])
	}

	// Sub-agent checks — each phase should be hidden subagent with file ref
	for _, phase := range subAgentPhaseOrder {
		key := phase + "-cheap"
		agentRaw, ok := agentMap[key]
		if !ok {
			t.Errorf("missing sub-agent %q", key)
			continue
		}
		agent, ok := agentRaw.(map[string]any)
		if !ok {
			t.Errorf("sub-agent %q is not an object", key)
			continue
		}
		if agentMode, _ := agent["mode"].(string); agentMode != "subagent" {
			t.Errorf("sub-agent %q mode = %q, want %q", key, agentMode, "subagent")
		}
		if hidden, _ := agent["hidden"].(bool); !hidden {
			t.Errorf("sub-agent %q hidden = false, want true", key)
		}
		prompt, _ := agent["prompt"].(string)
		if !strings.HasPrefix(prompt, "{file:") {
			t.Errorf("sub-agent %q prompt = %q, want {file:...} reference", key, prompt)
		}
	}
}

func TestGenerateProfileOverlay_PermissionScoped(t *testing.T) {
	home := t.TempDir()

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("overlay is not valid JSON: %v", err)
	}

	agentMap := root["agent"].(map[string]any)
	orch := agentMap["sdd-orchestrator-cheap"].(map[string]any)

	permRaw, ok := orch["permission"]
	if !ok {
		t.Fatal("sdd-orchestrator-cheap missing 'permission'")
	}
	perm, ok := permRaw.(map[string]any)
	if !ok {
		t.Fatal("sdd-orchestrator-cheap 'permission' is not an object")
	}
	taskRaw, ok := perm["task"]
	if !ok {
		t.Fatal("permission missing 'task'")
	}
	taskWrapper, ok := taskRaw.(map[string]any)
	if !ok {
		t.Fatal("permission.task is not an object")
	}
	taskMap, hasSentinel := taskWrapper["__replace__"].(map[string]any)
	if !hasSentinel {
		t.Fatal("task block must use __replace__ sentinel to discard stale wildcards on sync")
	}

	expected := expectedTaskPermissions("-cheap")
	assertExactTaskPermissions(t, taskMap, expected)
}

func TestDefaultOverlayTaskPermissions_ExplicitAllowlist(t *testing.T) {
	tests := []struct {
		name      string
		assetPath string
	}{
		{name: "single", assetPath: "opencode/sdd-overlay-single.json"},
		{name: "multi", assetPath: "opencode/sdd-overlay-multi.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal([]byte(assets.MustRead(tt.assetPath)), &root); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.assetPath, err)
			}

			agentMap := root["agent"].(map[string]any)
			orch := agentMap["gentle-orchestrator"].(map[string]any)
			permission := orch["permission"].(map[string]any)
			taskWrapper := permission["task"].(map[string]any)

			taskMap, hasSentinel := taskWrapper["__replace__"].(map[string]any)
			if !hasSentinel {
				t.Fatal("task block must use __replace__ sentinel to discard stale wildcards on sync")
			}

			assertExactTaskPermissions(t, taskMap, expectedTaskPermissions(""))
		})
	}
}

func TestGenerateProfileOverlay_TaskPermissionsBlockCrossProfileDelegation(t *testing.T) {
	home := t.TempDir()

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("overlay is not valid JSON: %v", err)
	}

	agentMap := root["agent"].(map[string]any)
	orch := agentMap["sdd-orchestrator-cheap"].(map[string]any)
	permission := orch["permission"].(map[string]any)
	taskWrapper := permission["task"].(map[string]any)

	// The overlay wraps the task block in __replace__ so that deep merge
	// discards old wildcard permissions from existing installations.
	taskMap, hasSentinel := taskWrapper["__replace__"].(map[string]any)
	if !hasSentinel {
		t.Fatal("task block must use __replace__ sentinel to discard stale wildcards on sync")
	}

	for _, phase := range profilePhaseOrder {
		if got := taskMap[phase+"-premium"]; got != nil {
			t.Errorf("unexpected cross-profile permission for %q: %v", phase+"-premium", got)
		}
	}
	if got := taskMap["sdd-*"]; got != nil {
		t.Errorf("unexpected wildcard permission sdd-*: %v", got)
	}
	if got := taskMap["sdd-*-cheap"]; got != nil {
		t.Errorf("unexpected wildcard permission sdd-*-cheap: %v", got)
	}
}

func TestGenerateProfileOverlay_SubAgentFileRefs(t *testing.T) {
	home := t.TempDir()

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	promptDir := SharedPromptDir(home)

	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("overlay is not valid JSON: %v", err)
	}
	agentMap := root["agent"].(map[string]any)

	for _, phase := range subAgentPhaseOrder {
		key := phase + "-cheap"
		agent := agentMap[key].(map[string]any)
		prompt, _ := agent["prompt"].(string)
		expectedRef := "{file:" + filepath.Join(promptDir, phase+".md") + "}"
		if prompt != expectedRef {
			t.Errorf("sub-agent %q prompt = %q, want %q", key, prompt, expectedRef)
		}
	}
}

func TestGenerateProfileOverlay_OrchestratorPromptSuffixed(t *testing.T) {
	home := t.TempDir()

	overlay, err := GenerateProfileOverlay(makeHaikuProfile(), home)
	if err != nil {
		t.Fatalf("GenerateProfileOverlay() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		t.Fatalf("overlay is not valid JSON: %v", err)
	}
	agentMap := root["agent"].(map[string]any)
	orch := agentMap["sdd-orchestrator-cheap"].(map[string]any)
	prompt, _ := orch["prompt"].(string)

	// The orchestrator prompt should reference suffixed sub-agents
	if !strings.Contains(prompt, "sdd-init-cheap") && !strings.Contains(prompt, "-cheap") {
		t.Errorf("orchestrator prompt doesn't contain suffixed sub-agent references; snippet: %q", prompt[:min(200, len(prompt))])
	}

	for _, unwanted := range []string{
		"Agent Teams Lite",
		"| orchestrator | opus |",
		"| sdd-explore | sonnet |",
		"| sdd-archive | haiku |",
	} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("profile orchestrator prompt contains legacy content %q", unwanted)
		}
	}

	for _, wanted := range []string{
		"Gentle AI",
		"| orchestrator | anthropic/claude-haiku-3-5 |",
	} {
		if !strings.Contains(prompt, wanted) {
			t.Fatalf("profile orchestrator prompt missing %q", wanted)
		}
	}
}

// ─── RemoveProfileAgents ─────────────────────────────────────────────────

func buildSettingsWithProfiles(t *testing.T) (path string) {
	t.Helper()
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "opencode.json")

	// Build JSON with default (11 keys) + cheap (11 keys) = 22 total
	agents := make(map[string]any)

	// Default agents (no suffix)
	for _, key := range []string{"sdd-orchestrator", "sdd-init", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"} {
		agents[key] = map[string]any{"mode": "primary"}
	}
	// cheap profile
	for _, key := range []string{"sdd-orchestrator-cheap", "sdd-init-cheap", "sdd-explore-cheap",
		"sdd-propose-cheap", "sdd-spec-cheap", "sdd-design-cheap", "sdd-tasks-cheap",
		"sdd-apply-cheap", "sdd-verify-cheap", "sdd-archive-cheap", "sdd-onboard-cheap"} {
		agents[key] = map[string]any{"mode": "subagent"}
	}

	root := map[string]any{"agent": agents}
	data, _ := json.MarshalIndent(root, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	return settingsPath
}

func TestRemoveProfileAgents_RemovesExactly11(t *testing.T) {
	path := buildSettingsWithProfiles(t)

	if err := RemoveProfileAgents(path, "cheap"); err != nil {
		t.Fatalf("RemoveProfileAgents() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	agentMap := root["agent"].(map[string]any)

	// 11 default keys should remain
	if len(agentMap) != 11 {
		t.Errorf("after RemoveProfileAgents, agent count = %d, want 11; keys: %v", len(agentMap), keysOf(agentMap))
	}

	// No cheap keys remain
	for key := range agentMap {
		if strings.HasSuffix(key, "-cheap") {
			t.Errorf("cheap key %q still present after removal", key)
		}
	}

	// Default keys all preserved
	for _, key := range []string{"sdd-orchestrator", "sdd-init", "sdd-explore",
		"sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard"} {
		if _, ok := agentMap[key]; !ok {
			t.Errorf("default key %q was removed — should be preserved", key)
		}
	}
}

func TestRemoveProfileAgents_NonExistentProfileNoOp(t *testing.T) {
	path := buildSettingsWithProfiles(t)

	// Read original
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if err := RemoveProfileAgents(path, "nonexistent"); err != nil {
		t.Fatalf("RemoveProfileAgents() with non-existent profile should not error; got: %v", err)
	}

	// File should be unchanged (or at least equivalent JSON structure)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}

	var origParsed, afterParsed map[string]any
	_ = json.Unmarshal(original, &origParsed)
	_ = json.Unmarshal(after, &afterParsed)

	origAgents := origParsed["agent"].(map[string]any)
	afterAgents := afterParsed["agent"].(map[string]any)

	if len(origAgents) != len(afterAgents) {
		t.Errorf("agent count changed: before=%d after=%d", len(origAgents), len(afterAgents))
	}
}

func TestRemoveProfileAgents_CannotRemoveDefault(t *testing.T) {
	path := buildSettingsWithProfiles(t)

	if err := RemoveProfileAgents(path, ""); err == nil {
		t.Fatal("RemoveProfileAgents(\"\") should return error for default profile")
	}
}

func TestRemoveProfileAgents_CannotRemoveDefaultByName(t *testing.T) {
	path := buildSettingsWithProfiles(t)

	if err := RemoveProfileAgents(path, "default"); err == nil {
		t.Fatal("RemoveProfileAgents(\"default\") should return error for default profile")
	}
}

// helper
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func expectedTaskPermissions(suffix string) map[string]any {
	permissions := map[string]any{
		"*": "deny",
	}
	for _, phase := range profilePhaseOrder {
		permissions[phase+suffix] = "allow"
	}
	return permissions
}

func assertExactTaskPermissions(t *testing.T, got, want map[string]any) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("permission.task length = %d, want %d; got keys: %v", len(got), len(want), keysOf(got))
	}

	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok {
			t.Errorf("permission.task missing key %q", key)
		} else if gotValue != wantValue {
			t.Errorf("permission.task[%q] = %v, want %v", key, gotValue, wantValue)
		}
	}

	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("permission.task has unexpected key %q", key)
		}
	}
}
