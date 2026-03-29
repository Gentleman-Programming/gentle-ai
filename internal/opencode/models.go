package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DefaultCachePath returns the default path to the OpenCode models cache file.
func DefaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "opencode", "models.json")
}

// DefaultSettingsPath returns the default path to the OpenCode settings file.
func DefaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

// DefaultAuthPath returns the default path to the OpenCode auth credentials file.
func DefaultAuthPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json")
}

// ModelCost holds the per-million-token pricing.
type ModelCost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// ModelLimit holds context and output token limits.
type ModelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// Model represents a single model within a provider.
type Model struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Family    string     `json:"family"`
	ToolCall  bool       `json:"tool_call"`
	Reasoning bool       `json:"reasoning"`
	Cost      ModelCost  `json:"cost"`
	Limit     ModelLimit `json:"limit"`
}

// Provider represents a model provider with its env vars and model catalog.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Env    []string         `json:"env"`
	Models map[string]Model `json:"models"`
}

// LoadModels parses the OpenCode models cache JSON file and returns providers keyed by ID.
func LoadModels(cachePath string) (map[string]Provider, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("read models cache %q: %w", cachePath, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse models cache: %w", err)
	}

	providers := make(map[string]Provider, len(raw))
	for id, providerJSON := range raw {
		var p Provider
		if err := json.Unmarshal(providerJSON, &p); err != nil {
			// Skip malformed providers.
			continue
		}
		p.ID = id
		providers[id] = p
	}

	return providers, nil
}

// loadAuthProviders reads the OpenCode auth.json and returns authenticated provider IDs.
func loadAuthProviders(authPath string) map[string]bool {
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	result := make(map[string]bool, len(raw))
	for id := range raw {
		result[id] = true
	}
	return result
}

// envLookup is a package-level variable for testing.
var envLookup = os.Getenv

// authPath is a package-level variable for testing.
var authPath = DefaultAuthPath

// DetectAvailableProviders returns provider IDs that the user has access to and
// that have at least one model with tool_call support. Detection sources:
//  1. OAuth credentials in ~/.local/share/opencode/auth.json
//  2. Environment variables (e.g. ANTHROPIC_API_KEY)
//  3. The "opencode" provider is always included if present (built-in subscription)
//
// Results are sorted alphabetically.
func DetectAvailableProviders(providers map[string]Provider) []string {
	authProviders := loadAuthProviders(authPath())

	var available []string
	for id, provider := range providers {
		if !hasToolCallModel(provider) {
			continue
		}

		// Check: authenticated via OAuth?
		if authProviders[id] {
			available = append(available, id)
			continue
		}

		// Check: built-in "opencode" provider (always available with subscription)
		if id == "opencode" {
			available = append(available, id)
			continue
		}

		// Check: env vars set?
		if len(provider.Env) > 0 && allEnvVarsSet(provider.Env) {
			available = append(available, id)
			continue
		}
	}

	sort.Strings(available)
	return available
}

func hasToolCallModel(provider Provider) bool {
	for _, m := range provider.Models {
		if m.ToolCall {
			return true
		}
	}
	return false
}

func allEnvVarsSet(envVars []string) bool {
	for _, v := range envVars {
		if envLookup(v) == "" {
			return false
		}
	}
	return true
}

// FilterModelsForSDD returns models from a provider that support tool_call (required for SDD phases).
// Results are sorted by model name.
func FilterModelsForSDD(provider Provider) []Model {
	var models []Model
	for _, m := range provider.Models {
		if m.ToolCall {
			models = append(models, m)
		}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models
}

// SDDPhases returns the ordered list of SDD phase sub-agent names.
func SDDPhases() []string {
	return []string{
		"sdd-init",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-design",
		"sdd-tasks",
		"sdd-apply",
		"sdd-verify",
		"sdd-archive",
	}
}
