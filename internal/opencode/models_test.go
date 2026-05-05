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

func TestDefaultVariantsCachePath(t *testing.T) {
	got := DefaultVariantsCachePath()
	if got == "" {
		t.Fatal("DefaultVariantsCachePath() returned empty string")
	}
	if !strings.HasSuffix(got, filepath.Join(".gentle-ai", "cache", "model-variants.json")) {
		t.Fatalf("expected path suffix .gentle-ai/cache/model-variants.json, got %q", got)
	}
	if strings.Contains(got, filepath.Join(".cache", "gentle-ai")) {
		t.Fatalf("path must not contain legacy .cache/gentle-ai, got %q", got)
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
