package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestLoadModelsOrEmptyFileNotFound(t *testing.T) {
	providers, err := LoadModelsOrEmpty("/nonexistent/models.json")
	if err != nil {
		t.Fatalf("LoadModelsOrEmpty() error = %v", err)
	}
	if len(providers) != 0 {
		t.Fatalf("providers = %v, want empty map", providers)
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

func TestModel_EffortLevels(t *testing.T) {
	tests := []struct {
		name     string
		variants []string
		want     []string
	}{
		{"no variants", nil, nil},
		{"claude style", []string{"high", "low", "medium"}, []string{"high", "low", "medium"}},
		{"openai style", []string{"high", "low", "medium", "none", "xhigh"}, []string{"high", "low", "medium", "none", "xhigh"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{Variants: tt.variants}
			got := m.EffortLevels()
			if len(got) != len(tt.want) {
				t.Fatalf("EffortLevels() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("EffortLevels()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestReviewPhasesCompleteRuntimeSet(t *testing.T) {
	want := []string{
		"review-risk",
		"review-readability",
		"review-reliability",
		"review-resilience",
		"review-refuter",
	}
	if got := ReviewPhases(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ReviewPhases() = %v, want %v", got, want)
	}

	configurable := ConfigurableAgentPhases()
	if got := configurable[len(configurable)-len(want):]; !reflect.DeepEqual(got, want) {
		t.Fatalf("ConfigurableAgentPhases() review suffix = %v, want %v", got, want)
	}
}

func TestDefaultVariantsCachePath(t *testing.T) {
	got := DefaultVariantsCachePath()
	if got == "" {
		t.Fatal("DefaultVariantsCachePath() returned empty string")
	}
	if !strings.HasSuffix(got, filepath.Join(".gentle-ai", "cache", "model-variants.json")) {
		t.Fatalf("expected path suffix .gentle-ai/cache/model-variants.json, got %q", got)
	}
	legacy := filepath.Join(".cache", "gentle-ai")
	if strings.Contains(got, legacy) {
		t.Fatalf("path must not contain legacy %s, got %q", legacy, got)
	}
}

func TestLoadVariants(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		tmp := t.TempDir()
		p := tmp + "/variants.json"
		data := `{"openai":{"gpt-5":["high","low","medium"]}}`
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := LoadVariants(p)
		if err != nil {
			t.Fatal(err)
		}
		if levels := got["openai"]["gpt-5"]; len(levels) != 3 || levels[0] != "high" {
			t.Errorf("unexpected levels: %v", levels)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadVariants("/nonexistent/variants.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tmp := t.TempDir()
		p := tmp + "/variants.json"
		if err := os.WriteFile(p, []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadVariants(p)
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
	})
}

func TestEnrichWithVariants(t *testing.T) {
	providers := map[string]Provider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]Model{
				"gpt-5": {ID: "gpt-5", Name: "GPT-5", ToolCall: true, Reasoning: true},
			},
		},
	}

	t.Run("merges variants from file", func(t *testing.T) {
		tmp := t.TempDir()
		p := tmp + "/variants.json"
		data := `{"openai":{"gpt-5":["high","low","medium","none","xhigh"]}}`
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		EnrichWithVariants(providers, p)
		m := providers["openai"].Models["gpt-5"]
		if len(m.Variants) != 5 {
			t.Fatalf("expected 5 variants, got %v", m.Variants)
		}
	})

	t.Run("missing file leaves models unchanged", func(t *testing.T) {
		clean := map[string]Provider{
			"openai": {ID: "openai", Models: map[string]Model{
				"gpt-5": {ID: "gpt-5"},
			}},
		}
		EnrichWithVariants(clean, "/nonexistent/variants.json")
		if clean["openai"].Models["gpt-5"].Variants != nil {
			t.Fatal("expected nil variants for missing file")
		}
	})

	t.Run("non-matching provider ignored", func(t *testing.T) {
		tmp := t.TempDir()
		p := tmp + "/variants.json"
		data := `{"unknown-provider":{"model-x":["low","high"]}}`
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		clean := map[string]Provider{
			"openai": {ID: "openai", Models: map[string]Model{
				"gpt-5": {ID: "gpt-5"},
			}},
		}
		EnrichWithVariants(clean, p)
		if clean["openai"].Models["gpt-5"].Variants != nil {
			t.Fatal("expected nil variants for non-matching provider")
		}
	})
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
	if lm.URL != "http://localhost:1234/v1" {
		t.Fatalf("lmstudio URL = %q, want options.baseURL", lm.URL)
	}
}

func TestLoadConfigProvidersPrefersURLOverOptionsBaseURL(t *testing.T) {
	path := writeConfigFixture(t, `{
		"provider": {
			"lmstudio": {
				"url": "http://direct.example/v1",
				"options": {"baseURL": "http://options.example/v1"}
			}
		}
	}`)

	config, err := LoadConfigProviders(path)
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	if got := config["lmstudio"].URL; got != "http://direct.example/v1" {
		t.Fatalf("lmstudio URL = %q, want direct url", got)
	}
}

func TestLoadConfigProvidersSupportedFallbackFilenamesAndJSONC(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	if err := os.WriteFile(path, []byte(`{
		// Local OpenAI-compatible provider.
		"provider": {
			"lmstudio": {
				"name": "LM Studio",
				"models": {
					"local-model": {"name": "Local Model"}, // simple documented shape
				},
			},
		},
	}`), 0o644); err != nil {
		t.Fatalf("write config.jsonc: %v", err)
	}

	config, err := LoadConfigProviders(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	if got := config["lmstudio"].Models["local-model"].Name; got != "Local Model" {
		t.Fatalf("model name = %q, want Local Model", got)
	}
}

func TestLoadConfigProvidersKeepsCommasInsideStrings(t *testing.T) {
	path := writeConfigFixture(t, `{
		"provider": {
			"local": {
				"name": "Local, Provider",
				"models": {
					"model,with,commas": {"name": "Model, With, Commas"},
				},
			},
		},
	}`)

	config, err := LoadConfigProviders(path)
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	provider := config["local"]
	if provider.Name != "Local, Provider" {
		t.Fatalf("provider name = %q, want Local, Provider", provider.Name)
	}
	if got := provider.Models["model,with,commas"].Name; got != "Model, With, Commas" {
		t.Fatalf("model name = %q, want Model, With, Commas", got)
	}
}

func TestLoadConfigProvidersMergesConfigJSONWhenOpenCodeJSONExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"provider": {
			"test-local-provider-1": {
				"name": "Test Local Provider 1",
				"models": {"neuro-model": {"name": "Neuro Model"}}
			},
			"test-local-provider-2": {
				"name": "Test Local Provider 2",
				"models": {"qwen-local": {"name": "Qwen Local"}}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{
		"provider": {
			"ollama": {
				"name": "Ollama",
				"models": {"llama-local": {"name": "Llama Local"}}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write opencode.json: %v", err)
	}

	config, err := LoadConfigProviders(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	for _, id := range []string{"test-local-provider-1", "test-local-provider-2", "ollama"} {
		if _, ok := config[id]; !ok {
			t.Fatalf("missing provider %q in merged config; got %v", id, config)
		}
	}
}

func TestLoadConfigProvidersMergesAllFilesWithDeterministicPrecedence(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"config.json": `{
			"provider": {"shared": {"name": "config json", "url": "http://config-json.example/v1", "models": {"shared-model": {"name": "config json model"}, "config-only": {"name": "config only"}}}}
		}`,
		"config.jsonc": `{
			// JSONC overrides JSON in the legacy family.
			"provider": {"shared": {"name": "config jsonc", "url": "http://config-jsonc.example/v1", "models": {"shared-model": {"name": "config jsonc model"}, "config-jsonc-only": {"name": "config jsonc only"}}}}
		}`,
		"opencode.json": `{
			"provider": {"shared": {"name": "opencode json", "url": "http://opencode-json.example/v1", "models": {"shared-model": {"name": "opencode json model"}, "opencode-only": {"name": "opencode only"}}}}
		}`,
		"opencode.jsonc": `{
			// Final override.
			"provider": {"shared": {"name": "opencode jsonc", "url": "http://opencode-jsonc.example/v1", "models": {"shared-model": {"name": "opencode jsonc model"}, "opencode-jsonc-only": {"name": "opencode jsonc only"}}}}
		}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	config, err := LoadConfigProviders(filepath.Join(dir, "opencode.json"))
	if err != nil {
		t.Fatalf("LoadConfigProviders() error = %v", err)
	}
	shared := config["shared"]
	if shared.Name != "opencode jsonc" {
		t.Fatalf("shared provider name = %q, want opencode jsonc", shared.Name)
	}
	if shared.URL != "http://opencode-jsonc.example/v1" {
		t.Fatalf("shared provider url = %q, want opencode jsonc URL", shared.URL)
	}
	if got := shared.Models["shared-model"].Name; got != "opencode jsonc model" {
		t.Fatalf("shared model name = %q, want opencode jsonc model", got)
	}
	for _, id := range []string{"config-only", "config-jsonc-only", "opencode-only", "opencode-jsonc-only"} {
		if _, ok := shared.Models[id]; !ok {
			t.Fatalf("missing merged model %q; got %v", id, shared.Models)
		}
	}
}

func TestLoadConfigProvidersKeepsValidProvidersWhenOneFileIsMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{
		"provider": {"valid-before": {"name": "Valid Before", "models": {"before-model": {"name": "Before Model"}}}}
	}`), 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"provider":`), 0o644); err != nil {
		t.Fatalf("write malformed opencode.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte(`{
		"provider": {"valid-after": {"name": "Valid After", "models": {"after-model": {"name": "After Model"}}}}
	}`), 0o644); err != nil {
		t.Fatalf("write opencode.jsonc: %v", err)
	}

	config, err := LoadConfigProviders(filepath.Join(dir, "opencode.json"))
	if err == nil {
		t.Fatal("expected parse warning error for malformed config")
	}
	for _, id := range []string{"valid-before", "valid-after"} {
		if _, ok := config[id]; !ok {
			t.Fatalf("valid provider %q was discarded; got %v", id, config)
		}
	}
}

func TestLoadEffectiveConfigMergesDocumentedSourcesWithPrecedence(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{
		"provider": {
			"shared": {"name": "inline", "models": {"inline-only": {"name": "Inline Only"}}}
		},
		"agent": {"sdd-apply": {"model": "inline/apply", "variant": "max"}}
	}`)

	home := t.TempDir()
	globalDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.json"), []byte(`{
		"provider": {
			"legacy-global": {"name": "Legacy Global", "models": {"global-model": {"name": "Global Model"}}},
			"shared": {"name": "global", "models": {"global-only": {"name": "Global Only"}}}
		},
		"agent": {"sdd-verify": {"model": "global/verify"}}
	}`), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	customPath := filepath.Join(t.TempDir(), "custom.jsonc")
	if err := os.WriteFile(customPath, []byte(`{
		// Custom config is loaded after global config.
		"provider": {"custom-only": {"name": "Custom Only", "models": {"custom-model": {"name": "Custom Model"}}}},
		"agent": {"sdd-spec": {"model": "custom/spec"}}
	}`), 0o644); err != nil {
		t.Fatalf("write custom config: %v", err)
	}
	t.Setenv("OPENCODE_CONFIG", customPath)

	projectRoot := t.TempDir()
	workDir := filepath.Join(projectRoot, "nested")
	if err := os.MkdirAll(filepath.Join(workDir, ".opencode"), 0o755); err != nil {
		t.Fatalf("mkdir project dirs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "opencode.jsonc"), []byte(`{
		"provider": {
			"project-only": {"name": "Project Only", "models": {"project-model": {"name": "Project Model"}}},
			"shared": {"name": "project", "models": {"project-only-model": {"name": "Project Only Model"}}}
		},
		"agent": {"sdd-apply": {"model": "project/apply"}}
	}`), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, ".opencode", "opencode.json"), []byte(`{
		"provider": {"dot-opencode-only": {"name": "Dot OpenCode", "models": {"dot-model": {"name": "Dot Model"}}}}
	}`), 0o644); err != nil {
		t.Fatalf("write .opencode config: %v", err)
	}

	effective, err := LoadEffectiveConfig(ConfigLoadOptions{
		HomeDir:      home,
		SettingsPath: filepath.Join(globalDir, "opencode.json"),
		WorkDir:      workDir,
		WorktreeRoot: projectRoot,
		IncludeEnv:   true,
	})
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}

	for _, id := range []string{"legacy-global", "custom-only", "project-only", "dot-opencode-only", "shared"} {
		if _, ok := effective.Provider[id]; !ok {
			t.Fatalf("missing provider %q; got %v", id, effective.Provider)
		}
	}
	shared := effective.Provider["shared"]
	if shared.Name != "inline" {
		t.Fatalf("shared provider name = %q, want inline", shared.Name)
	}
	for _, modelID := range []string{"global-only", "project-only-model", "inline-only"} {
		if _, ok := shared.Models[modelID]; !ok {
			t.Fatalf("missing merged shared model %q; got %v", modelID, shared.Models)
		}
	}
	if got := effective.Agent["sdd-apply"]; got != (ConfigAgent{Model: "inline/apply", Variant: "max", ModelSet: true}) {
		t.Fatalf("sdd-apply = %+v, want inline override with variant", got)
	}
	if got := effective.Agent["sdd-verify"].Model; got != "global/verify" {
		t.Fatalf("sdd-verify model = %q, want global/verify", got)
	}
	if got := effective.Agent["sdd-spec"].Model; got != "custom/spec" {
		t.Fatalf("sdd-spec model = %q, want custom/spec", got)
	}
}

func TestLoadEffectiveConfigIgnoresMissingEnvConfig(t *testing.T) {
	home := t.TempDir()
	globalDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "opencode.json"), []byte(`{
		"provider": {"global": {"name": "Global", "models": {"global-model": {"name": "Global Model"}}}}
	}`), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("OPENCODE_CONFIG", filepath.Join(t.TempDir(), "missing.json"))

	effective, err := LoadEffectiveConfig(ConfigLoadOptions{
		HomeDir:      home,
		SettingsPath: filepath.Join(globalDir, "opencode.json"),
		IncludeEnv:   true,
	})
	if err != nil {
		t.Fatalf("LoadEffectiveConfig() error = %v", err)
	}
	if _, ok := effective.Provider["global"]; !ok {
		t.Fatalf("global provider was not discovered after missing OPENCODE_CONFIG; got %v", effective.Provider)
	}
}

func TestLoadEffectiveConfigKeepsValidSourcesWhenOneFileIsMalformed(t *testing.T) {
	home := t.TempDir()
	globalDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir global config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.json"), []byte(`{
		"provider": {
			"valid-global": {"name": "Valid Global", "models": {"global-model": {"name": "Global Model"}}}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write valid config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "opencode.json"), []byte(`{"provider":`), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	effective, err := LoadEffectiveConfig(ConfigLoadOptions{
		HomeDir:      home,
		SettingsPath: filepath.Join(globalDir, "opencode.json"),
	})
	if err == nil {
		t.Fatal("expected parse warning error for malformed config")
	}
	if _, ok := effective.Provider["valid-global"]; !ok {
		t.Fatalf("valid provider was discarded; got %v", effective.Provider)
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

func TestMergeCustomProvidersDefaultsToolCallToTrue(t *testing.T) {
	providers := map[string]Provider{}
	config := map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{
			"qwen/qwen3.5": {Name: "Qwen 3.5"},
		}},
	}

	merged := MergeCustomProviders(providers, config)
	if !merged["lmstudio"].Models["qwen/qwen3.5"].ToolCall {
		t.Fatal("custom model should default to ToolCall=true when omitted in config")
	}
}

func TestMergeCustomProvidersModelCollisionCustomWins(t *testing.T) {
	providers := map[string]Provider{
		"lmstudio": {ID: "lmstudio", Name: "LMStudio", Models: map[string]Model{
			"shared-model": {ID: "shared-model", Name: "Cache Name", ToolCall: false, Cost: ModelCost{Input: 5.0}},
		}},
	}
	config := map[string]ConfigProvider{
		"lmstudio": {Models: map[string]ConfigModel{
			"shared-model": {Name: "Config Name", ToolCall: true},
		}},
	}

	merged := MergeCustomProviders(providers, config)
	m := merged["lmstudio"].Models["shared-model"]
	if m.Name != "Config Name" {
		t.Fatalf("model name = %q, want %q (custom should win)", m.Name, "Config Name")
	}
	if !m.ToolCall {
		t.Fatal("custom ToolCall=true should win over cache ToolCall=false on collision")
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

func TestDetectAvailableProvidersCustomProviderModelIsSelectableByDefault(t *testing.T) {
	providers := MergeCustomProviders(map[string]Provider{}, map[string]ConfigProvider{
		"lmstudio": {Name: "LM Studio", Models: map[string]ConfigModel{
			"local-model": {Name: "Local Model"},
		}},
	})

	cleanup := withNoAuth(t)
	defer cleanup()

	original := envLookup
	defer func() { envLookup = original }()
	envLookup = func(string) string { return "" }

	available := DetectAvailableProviders(providers, "lmstudio")

	found := false
	for _, id := range available {
		if id == "lmstudio" {
			found = true
		}
	}
	if !found {
		t.Fatalf("lmstudio should be available from config-defined model, got %v", available)
	}
}

func TestFixOpenRouterModels(t *testing.T) {
	const sourceID = "qwen3.6-plus-free"
	const targetID = "qwen/qwen3.6-plus:free"

	t.Run("existing openrouter provider is preserved", func(t *testing.T) {
		providers := map[string]Provider{
			"opencode": {
				ID:   "opencode",
				Name: "OpenCode Zen",
				Models: map[string]Model{
					sourceID: {
						ID:       sourceID,
						Name:     "Qwen3.6 Plus Free",
						ToolCall: true,
					},
				},
			},
			"openrouter": {
				ID:   "openrouter",
				Name: "Custom OpenRouter",
				Env:  []string{"OPENROUTER_API_KEY"},
				Models: map[string]Model{
					"existing-model": {
						ID:   "existing-model",
						Name: "Existing Model",
					},
				},
			},
		}

		FixOpenRouterModels(providers)

		openrouter := providers["openrouter"]
		if openrouter.Name != "Custom OpenRouter" {
			t.Fatalf("openrouter name = %q, want Custom OpenRouter", openrouter.Name)
		}
		if len(openrouter.Env) != 1 || openrouter.Env[0] != "OPENROUTER_API_KEY" {
			t.Fatalf("openrouter env = %v, want [OPENROUTER_API_KEY]", openrouter.Env)
		}
		if _, ok := openrouter.Models["existing-model"]; !ok {
			t.Fatal("existing openrouter model was removed")
		}
		if _, ok := openrouter.Models[targetID]; !ok {
			t.Fatalf("openrouter missing remapped model %q", targetID)
		}
	})

	t.Run("target collision does not overwrite existing target", func(t *testing.T) {
		existingTarget := Model{
			ID:       targetID,
			Name:     "Authoritative OpenRouter Model",
			ToolCall: false,
		}
		source := Model{
			ID:       sourceID,
			Name:     "Misfiled OpenCode Model",
			ToolCall: true,
		}
		providers := map[string]Provider{
			"opencode": {
				ID: "opencode",
				Models: map[string]Model{
					sourceID: source,
				},
			},
			"openrouter": {
				ID: "openrouter",
				Models: map[string]Model{
					targetID: existingTarget,
				},
			},
		}

		FixOpenRouterModels(providers)

		if got := providers["openrouter"].Models[targetID]; !reflect.DeepEqual(got, existingTarget) {
			t.Fatalf("openrouter target = %+v, want %+v", got, existingTarget)
		}
		if got := providers["opencode"].Models[sourceID]; !reflect.DeepEqual(got, source) {
			t.Fatalf("opencode source = %+v, want %+v", got, source)
		}
	})

	t.Run("missing opencode provider is no-op", func(t *testing.T) {
		providers := map[string]Provider{
			"openrouter": {
				ID:   "openrouter",
				Name: "OpenRouter",
				Models: map[string]Model{
					"existing-model": {ID: "existing-model"},
				},
			},
		}

		FixOpenRouterModels(providers)

		if _, ok := providers["opencode"]; ok {
			t.Fatal("opencode provider should not be created")
		}
		if len(providers["openrouter"].Models) != 1 {
			t.Fatalf("openrouter model count = %d, want 1", len(providers["openrouter"].Models))
		}
	})

	t.Run("missing source model is no-op", func(t *testing.T) {
		providers := map[string]Provider{
			"opencode": {
				ID: "opencode",
				Models: map[string]Model{
					"other-model": {ID: "other-model"},
				},
			},
		}

		FixOpenRouterModels(providers)

		if _, ok := providers["openrouter"]; ok {
			t.Fatal("openrouter provider should not be created")
		}
		if _, ok := providers["opencode"].Models["other-model"]; !ok {
			t.Fatal("opencode other-model was removed")
		}
	})

	t.Run("remap preserves model fields", func(t *testing.T) {
		source := Model{
			ID:        sourceID,
			Name:      "Qwen3.6 Plus Free",
			Family:    "qwen",
			ToolCall:  true,
			Reasoning: true,
			Cost: ModelCost{
				Input:  0.1,
				Output: 0.2,
			},
			Limit: ModelLimit{
				Context: 128000,
				Output:  8192,
			},
			Variants: []string{"free", "latest"},
		}
		providers := map[string]Provider{
			"opencode": {
				ID: "opencode",
				Models: map[string]Model{
					sourceID:      source,
					"other-model": {ID: "other-model"},
				},
			},
		}

		FixOpenRouterModels(providers)

		if _, ok := providers["opencode"].Models[sourceID]; ok {
			t.Fatalf("opencode should not have %q after remap", sourceID)
		}
		if _, ok := providers["opencode"].Models["other-model"]; !ok {
			t.Fatal("opencode should still have other-model")
		}

		got := providers["openrouter"].Models[targetID]
		if got.ID != targetID {
			t.Fatalf("remapped ID = %q, want %q", got.ID, targetID)
		}
		if got.Name != source.Name || got.Family != source.Family || got.ToolCall != source.ToolCall || got.Reasoning != source.Reasoning {
			t.Fatalf("remapped model metadata = %+v, want fields from %+v", got, source)
		}
		if got.Cost != source.Cost || got.Limit != source.Limit {
			t.Fatalf("remapped model pricing/limits = cost %+v limit %+v, want cost %+v limit %+v", got.Cost, got.Limit, source.Cost, source.Limit)
		}
		if len(got.Variants) != 2 || got.Variants[0] != "free" || got.Variants[1] != "latest" {
			t.Fatalf("remapped variants = %v, want [free latest]", got.Variants)
		}
	})
}

func TestEnrichWithVariantsFallbackProviderMismatch(t *testing.T) {
	dir := t.TempDir()
	variantsPath := filepath.Join(dir, "model-variants.json")
	// Plugin cached variants under provider "openai"
	variantsData := `{
		"openai": {
			"gpt-5.6-luna": ["low", "medium", "high", "max"]
		}
	}`
	if err := os.WriteFile(variantsPath, []byte(variantsData), 0o644); err != nil {
		t.Fatalf("write variants fixture: %v", err)
	}

	// Cached providers in gentle-ai has both "openai" (exact match) and "opencode" (built-in/custom alias)
	cached := map[string]Provider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]Model{
				"gpt-5.6-luna": {
					ID:       "gpt-5.6-luna",
					Name:     "GPT-5.6 Luna",
					ToolCall: true,
				},
			},
		},
		"opencode": {
			ID:   "opencode",
			Name: "OpenCode",
			Models: map[string]Model{
				"gpt-5.6-luna": {
					ID:       "gpt-5.6-luna",
					Name:     "GPT-5.6 Luna",
					ToolCall: true,
				},
			},
		},
	}

	EnrichWithVariants(cached, variantsPath)

	wantLevels := []string{"low", "medium", "high", "max"}

	modelOpenAI := cached["openai"].Models["gpt-5.6-luna"]
	if !reflect.DeepEqual(modelOpenAI.Variants, wantLevels) {
		t.Fatalf("exact match openai model variants = %v, want %v", modelOpenAI.Variants, wantLevels)
	}

	modelOpenCode := cached["opencode"].Models["gpt-5.6-luna"]
	if !reflect.DeepEqual(modelOpenCode.Variants, wantLevels) {
		t.Fatalf("fallback opencode model variants = %v, want %v", modelOpenCode.Variants, wantLevels)
	}
}
