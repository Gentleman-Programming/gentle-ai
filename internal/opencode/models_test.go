package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureJSON = `{
  "anthropic": {
    "id": "anthropic",
    "env": ["ANTHROPIC_API_KEY"],
    "name": "Anthropic",
    "models": {
      "claude-sonnet-4-20250514": {
        "id": "claude-sonnet-4-20250514",
        "name": "Claude Sonnet 4",
        "family": "claude",
        "tool_call": true,
        "reasoning": false,
        "cost": {"input": 3.0, "output": 15.0},
        "limit": {"context": 200000, "output": 8192}
      },
      "claude-haiku-3-20240307": {
        "id": "claude-haiku-3-20240307",
        "name": "Claude Haiku 3",
        "family": "claude",
        "tool_call": true,
        "reasoning": false,
        "cost": {"input": 0.25, "output": 1.25},
        "limit": {"context": 200000, "output": 4096}
      }
    }
  },
  "openai": {
    "id": "openai",
    "env": ["OPENAI_API_KEY"],
    "name": "OpenAI",
    "models": {
      "gpt-4o": {
        "id": "gpt-4o",
        "name": "GPT-4o",
        "family": "gpt",
        "tool_call": true,
        "reasoning": false,
        "cost": {"input": 2.5, "output": 10.0},
        "limit": {"context": 128000, "output": 4096}
      },
      "o1-mini": {
        "id": "o1-mini",
        "name": "o1-mini",
        "family": "o1",
        "tool_call": false,
        "reasoning": true,
        "cost": {"input": 3.0, "output": 12.0},
        "limit": {"context": 128000, "output": 65536}
      }
    }
  },
  "opencode": {
    "id": "opencode",
    "env": ["OPENCODE_API_KEY"],
    "name": "OpenCode",
    "models": {
      "gpt-5-codex": {
        "id": "gpt-5-codex",
        "name": "GPT-5 Codex",
        "family": "gpt",
        "tool_call": true,
        "reasoning": true,
        "cost": {"input": 5.0, "output": 20.0},
        "limit": {"context": 200000, "output": 16384}
      }
    }
  },
  "notools": {
    "id": "notools",
    "env": [],
    "name": "No Tools Provider",
    "models": {
      "basic": {
        "id": "basic",
        "name": "Basic Model",
        "family": "basic",
        "tool_call": false,
        "reasoning": false,
        "cost": {"input": 0.1, "output": 0.1},
        "limit": {"context": 4096, "output": 1024}
      }
    }
  },
  "empty": {
    "id": "empty",
    "env": [],
    "name": "Empty Provider",
    "models": {}
  }
}`

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, []byte(fixtureJSON), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func writeAuthFixture(t *testing.T, providers map[string]bool) string {
	t.Helper()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	authData := make(map[string]map[string]string)
	for id := range providers {
		authData[id] = map[string]string{"type": "oauth"}
	}
	data, _ := json.Marshal(authData)
	if err := os.WriteFile(authPath, data, 0o644); err != nil {
		t.Fatalf("write auth fixture: %v", err)
	}
	return authPath
}

func TestLoadModels(t *testing.T) {
	path := writeFixture(t)

	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	if len(providers) != 5 {
		t.Fatalf("provider count = %d, want 5", len(providers))
	}

	anthropic, ok := providers["anthropic"]
	if !ok {
		t.Fatal("missing anthropic provider")
	}
	if anthropic.Name != "Anthropic" {
		t.Fatalf("anthropic name = %q", anthropic.Name)
	}
	if len(anthropic.Models) != 2 {
		t.Fatalf("anthropic model count = %d, want 2", len(anthropic.Models))
	}
	if len(anthropic.Env) != 1 || anthropic.Env[0] != "ANTHROPIC_API_KEY" {
		t.Fatalf("anthropic env = %v", anthropic.Env)
	}
}

func TestLoadModelsFileNotFound(t *testing.T) {
	_, err := LoadModels("/nonexistent/models.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func withAuthFixture(t *testing.T, providers map[string]bool) func() {
	t.Helper()
	path := writeAuthFixture(t, providers)
	original := authPath
	authPath = func() string { return path }
	return func() { authPath = original }
}

func withNoAuth(t *testing.T) func() {
	t.Helper()
	original := authPath
	authPath = func() string { return "/nonexistent/auth.json" }
	return func() { authPath = original }
}

func TestDetectAvailableProvidersWithAuth(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	cleanup := withAuthFixture(t, map[string]bool{"anthropic": true, "openai": true})
	defer cleanup()

	// No env vars needed — auth provides access.
	origEnv := envLookup
	defer func() { envLookup = origEnv }()
	envLookup = func(string) string { return "" }

	available := DetectAvailableProviders(providers)
	found := make(map[string]bool)
	for _, id := range available {
		found[id] = true
	}
	if !found["anthropic"] {
		t.Fatal("expected anthropic (OAuth auth)")
	}
	if !found["openai"] {
		t.Fatal("expected openai (OAuth auth)")
	}
	if !found["opencode"] {
		t.Fatal("expected opencode (always included)")
	}
	if found["notools"] {
		t.Fatal("notools should NOT be available")
	}
}

func TestDetectAvailableProvidersViaEnvVars(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	cleanup := withNoAuth(t)
	defer cleanup()

	original := envLookup
	defer func() { envLookup = original }()

	envLookup = func(key string) string {
		if key == "ANTHROPIC_API_KEY" {
			return "sk-test"
		}
		return ""
	}

	available := DetectAvailableProviders(providers)

	found := make(map[string]bool)
	for _, id := range available {
		found[id] = true
	}
	if !found["anthropic"] {
		t.Fatal("expected anthropic (env var set)")
	}
	if !found["opencode"] {
		t.Fatal("expected opencode (always included)")
	}
	if found["openai"] {
		t.Fatal("openai should NOT be available (no auth, no env var)")
	}
	if found["notools"] {
		t.Fatal("notools should NOT be available (no tool_call models)")
	}
}

func TestDetectAvailableProvidersOpenCodeAlwaysIncluded(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	cleanup := withNoAuth(t)
	defer cleanup()

	original := envLookup
	defer func() { envLookup = original }()
	envLookup = func(string) string { return "" }

	available := DetectAvailableProviders(providers)

	// Only opencode should be available (built-in).
	if len(available) != 1 || available[0] != "opencode" {
		t.Fatalf("expected only [opencode], got %v", available)
	}
}

func TestDetectExcludesNoToolCallProviders(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	available := DetectAvailableProviders(providers)
	for _, id := range available {
		if id == "notools" || id == "empty" {
			t.Fatalf("provider %q should not be in available list", id)
		}
	}
}

func TestFilterModelsForSDD(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	// OpenAI has 2 models, but o1-mini has tool_call=false.
	openai := providers["openai"]
	sddModels := FilterModelsForSDD(openai)
	if len(sddModels) != 1 {
		t.Fatalf("openai SDD model count = %d, want 1", len(sddModels))
	}
	if sddModels[0].ID != "gpt-4o" {
		t.Fatalf("filtered model = %q, want gpt-4o", sddModels[0].ID)
	}

	// Anthropic has 2 models, both with tool_call=true.
	anthropic := providers["anthropic"]
	sddModels = FilterModelsForSDD(anthropic)
	if len(sddModels) != 2 {
		t.Fatalf("anthropic SDD model count = %d, want 2", len(sddModels))
	}
}

func TestLoadAuthProviders(t *testing.T) {
	authPath := writeAuthFixture(t, map[string]bool{
		"anthropic":      true,
		"google":         true,
		"github-copilot": true,
		"openai":         true,
	})

	result := loadAuthProviders(authPath)
	if len(result) != 4 {
		t.Fatalf("auth provider count = %d, want 4", len(result))
	}
	for _, id := range []string{"anthropic", "google", "github-copilot", "openai"} {
		if !result[id] {
			t.Fatalf("missing auth provider %q", id)
		}
	}
}

func TestLoadAuthProvidersMissingFile(t *testing.T) {
	result := loadAuthProviders("/nonexistent/auth.json")
	if result != nil {
		t.Fatalf("expected nil for missing file, got %v", result)
	}
}

// ─── LoadConfigProviders ──────────────────────────────────────────────────────

func writeConfigFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return path
}

func TestLoadConfigProviders(t *testing.T) {
	path := writeConfigFixture(t, `{
		"provider": {
			"lmstudio": {
				"npm": "@ai-sdk/openai-compatible",
				"name": "LM Studio (local)",
				"options": {"baseURL": "http://localhost:1234/v1"},
				"models": {
					"qwen/qwen3.5-35b-a3b": {"name": "qwen3.5-35b-a3b", "tool_call": true}
				}
			}
		}
	}`)

	config, err := LoadConfigProviders(path)
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	if len(config) != 1 {
		t.Fatalf("config provider count = %d, want 1", len(config))
	}

	lm, ok := config["lmstudio"]
	if !ok {
		t.Fatal("missing lmstudio provider")
	}
	if lm.Name != "LM Studio (local)" {
		t.Fatalf("lmstudio name = %q, want %q", lm.Name, "LM Studio (local)")
	}
	if len(lm.Models) != 1 {
		t.Fatalf("lmstudio model count = %d, want 1", len(lm.Models))
	}
	if _, ok := lm.Models["qwen/qwen3.5-35b-a3b"]; !ok {
		t.Fatal("missing model qwen/qwen3.5-35b-a3b")
	}
	if !lm.Models["qwen/qwen3.5-35b-a3b"].ToolCall {
		t.Fatal("expected tool_call metadata to be loaded from config")
	}
}

func TestLoadConfigProvidersMissingFile(t *testing.T) {
	config, err := LoadConfigProviders("/nonexistent/opencode.json")
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	if len(config) != 0 {
		t.Fatalf("expected empty map for missing file, got %v", config)
	}
}

func TestLoadConfigProvidersNoProviderKey(t *testing.T) {
	path := writeConfigFixture(t, `{"agent": {"foo": {}}}`)
	config, err := LoadConfigProviders(path)
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	if len(config) != 0 {
		t.Fatalf("expected empty map when no provider key, got %v", config)
	}
}

func TestLoadConfigProvidersInvalidJSON(t *testing.T) {
	path := writeConfigFixture(t, `{"provider":`)
	config, err := LoadConfigProviders(path)
	if err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}
	if !strings.Contains(err.Error(), filepath.Base(path)) {
		t.Fatalf("expected parse error to include file name %q, got %v", filepath.Base(path), err)
	}
	if len(config) != 0 {
		t.Fatalf("expected empty map on parse error, got %v", config)
	}
}

// MergeCustomProviders

func TestMergeCustomProvidersNewProvider(t *testing.T) {
	providers := map[string]Provider{
		"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{
			"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o", ToolCall: true},
		}},
	}
	config := map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{
			"qwen/qwen3.5": {Name: "Qwen 3.5", ToolCall: true},
		}},
	}

	merged := MergeCustomProviders(providers, config)

	if len(merged) != 2 {
		t.Fatalf("merged count = %d, want 2", len(merged))
	}
	lm, ok := merged["lmstudio"]
	if !ok {
		t.Fatal("missing lmstudio in merged")
	}
	if lm.Name != "LM Studio" {
		t.Fatalf("lmstudio name = %q", lm.Name)
	}
	m, ok := lm.Models["qwen/qwen3.5"]
	if !ok {
		t.Fatal("missing model qwen/qwen3.5")
	}
	if !m.ToolCall {
		t.Fatal("custom model should have ToolCall=true")
	}
	if m.Name != "Qwen 3.5" {
		t.Fatalf("model name = %q, want %q", m.Name, "Qwen 3.5")
	}
}

func TestMergeCustomProvidersExistingProviderNewModel(t *testing.T) {
	providers := map[string]Provider{
		"lmstudio": {ID: "lmstudio", Name: "LMStudio", Env: []string{"LMSTUDIO_API_KEY"}, Models: map[string]Model{
			"gpt-oss-20b": {ID: "gpt-oss-20b", Name: "GPT OSS 20B", ToolCall: true, Cost: ModelCost{Input: 1.0}},
		}},
	}
	config := map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio (local)", Models: map[string]ConfigModel{
			"qwen/qwen3.5": {Name: "Qwen 3.5", ToolCall: true},
		}},
	}

	merged := MergeCustomProviders(providers, config)
	lm := merged["lmstudio"]

	if len(lm.Models) != 2 {
		t.Fatalf("model count = %d, want 2", len(lm.Models))
	}
	// Cache model preserved
	if lm.Models["gpt-oss-20b"].Cost.Input != 1.0 {
		t.Fatal("cache model metadata should be preserved")
	}
	// New model added with explicit ToolCall=true from config
	if !lm.Models["qwen/qwen3.5"].ToolCall {
		t.Fatal("new custom model should have ToolCall=true")
	}
}

func TestMergeCustomProvidersDefaultsToolCallToFalse(t *testing.T) {
	providers := map[string]Provider{}
	config := map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{
			"qwen/qwen3.5": {Name: "Qwen 3.5"},
		}},
	}

	merged := MergeCustomProviders(providers, config)
	if merged["lmstudio"].Models["qwen/qwen3.5"].ToolCall {
		t.Fatal("custom model should default to ToolCall=false when omitted in config")
	}
}

func TestMergeCustomProvidersModelCollisionCacheWins(t *testing.T) {
	providers := map[string]Provider{
		"lmstudio": {ID: "lmstudio", Name: "LMStudio", Models: map[string]Model{
			"shared-model": {ID: "shared-model", Name: "Cache Name", ToolCall: true, Cost: ModelCost{Input: 5.0}},
		}},
	}
	config := map[string]ConfigProvider{
		"lmstudio": {Models: map[string]ConfigModel{
			"shared-model": {Name: "Config Name"},
		}},
	}

	merged := MergeCustomProviders(providers, config)
	m := merged["lmstudio"].Models["shared-model"]
	if m.Name != "Cache Name" {
		t.Fatalf("model name = %q, want %q (cache should win)", m.Name, "Cache Name")
	}
	if m.Cost.Input != 5.0 {
		t.Fatal("cache cost metadata should be preserved on collision")
	}
}

func TestMergeCustomProvidersEmptyConfig(t *testing.T) {
	providers := map[string]Provider{
		"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{}},
	}

	merged := MergeCustomProviders(providers, nil)
	if len(merged) != 1 {
		t.Fatalf("expected unchanged providers, got %d", len(merged))
	}
}

func TestMergeCustomProvidersDoesNotMutateInput(t *testing.T) {
	providers := map[string]Provider{
		"openai": {ID: "openai", Name: "OpenAI", Models: map[string]Model{
			"gpt-4o": {ID: "gpt-4o", ToolCall: true},
		}},
	}
	config := map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{
			"local-model": {Name: "Local"},
		}},
	}

	_ = MergeCustomProviders(providers, config)

	if _, ok := providers["lmstudio"]; ok {
		t.Fatal("MergeCustomProviders mutated the input map")
	}
}

func TestMergeCustomProvidersDoesNotAliasEnvSlice(t *testing.T) {
	providers := map[string]Provider{
		"openai": {ID: "openai", Name: "OpenAI", Env: []string{"OPENAI_API_KEY"}, Models: map[string]Model{}},
	}

	merged := MergeCustomProviders(providers, map[string]ConfigProvider{"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{}}})
	merged["openai"].Env[0] = "CHANGED"

	if providers["openai"].Env[0] != "OPENAI_API_KEY" {
		t.Fatal("MergeCustomProviders aliased the input Env slice")
	}
}

func TestMergeCustomProvidersDefaultsEmptyModelNameToID(t *testing.T) {
	merged := MergeCustomProviders(map[string]Provider{}, map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{
			"qwen/qwen3.5": {ToolCall: true},
		}},
	})

	if got := merged["lmstudio"].Models["qwen/qwen3.5"].Name; got != "qwen/qwen3.5" {
		t.Fatalf("merged model name = %q, want fallback to model ID", got)
	}
}

// ─── DetectAvailableProviders with custom IDs ─────────────────────────────────

func TestDetectAvailableProvidersWithCustomIDs(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	cleanup := withNoAuth(t)
	defer cleanup()

	original := envLookup
	defer func() { envLookup = original }()
	envLookup = func(string) string { return "" }

	// Pass "anthropic" as custom — should be available without auth/env
	available := DetectAvailableProviders(providers, "anthropic")

	found := make(map[string]bool)
	for _, id := range available {
		found[id] = true
	}
	if !found["anthropic"] {
		t.Fatal("expected anthropic (custom provider ID)")
	}
	if !found["opencode"] {
		t.Fatal("expected opencode (always included)")
	}
	if found["openai"] {
		t.Fatal("openai should NOT be available (not custom, no auth, no env)")
	}
}

func TestDetectAvailableProvidersCustomStillNeedsToolCall(t *testing.T) {
	path := writeFixture(t)
	providers, err := LoadModels(path)
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}

	cleanup := withNoAuth(t)
	defer cleanup()

	original := envLookup
	defer func() { envLookup = original }()
	envLookup = func(string) string { return "" }

	// "notools" has no tool_call models — custom ID should not bypass that check
	available := DetectAvailableProviders(providers, "notools")

	for _, id := range available {
		if id == "notools" {
			t.Fatal("notools should NOT be available even as custom (no tool_call models)")
		}
	}
}
