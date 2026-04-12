package opencode

import (
	"encoding/json"
	"errors"
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
func DetectAvailableProviders(providers map[string]Provider, customProviderIDs ...string) []string {
	authProviders := loadAuthProviders(authPath())

	customSet := make(map[string]bool, len(customProviderIDs))
	for _, id := range customProviderIDs {
		customSet[id] = true
	}

	var available []string
	for id, provider := range providers {
		if !hasToolCallModel(provider) {
			continue
		}

		// Check: explicitly configured custom provider (always available).
		if customSet[id] {
			available = append(available, id)
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

// ConfigModel represents a model entry in the opencode.json provider section.
type ConfigModel struct {
	Name     string `json:"name"`
	ToolCall bool   `json:"tool_call"`
}

// ConfigProvider represents a custom provider defined in opencode.json.
type ConfigProvider struct {
	Name   string                 `json:"name"`
	Models map[string]ConfigModel `json:"models"`
}

// LoadConfigProviders reads the provider section from an opencode.json settings file.
// Returns an empty map with nil error if the file is missing or has no provider key.
func LoadConfigProviders(path string) (map[string]ConfigProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]ConfigProvider{}, nil
		}
		return map[string]ConfigProvider{}, err
	}

	var raw struct {
		Provider map[string]ConfigProvider `json:"provider"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]ConfigProvider{}, fmt.Errorf("parse opencode settings %q: %w", path, err)
	}
	if raw.Provider == nil {
		return map[string]ConfigProvider{}, nil
	}
	return raw.Provider, nil
}

// MergeCustomProviders merges custom providers from opencode.json into the cache-loaded
// providers map. Custom models use the tool_call value from opencode.json, defaulting to false
// when omitted. Cache models win on ID collision. Returns the original providers map unchanged
// when config is empty; otherwise returns a merged copy without mutating the input.
func MergeCustomProviders(providers map[string]Provider, config map[string]ConfigProvider) map[string]Provider {
	if len(config) == 0 {
		return providers
	}

	merged := make(map[string]Provider, len(providers)+len(config))
	for id, p := range providers {
		clone := Provider{ID: p.ID, Name: p.Name, Env: append([]string(nil), p.Env...), Models: make(map[string]Model, len(p.Models))}
		for mid, m := range p.Models {
			clone.Models[mid] = m
		}
		merged[id] = clone
	}

	for id, cp := range config {
		// Provider-level collision: when a provider ID already exists in the cache,
		// we keep the cache's Name/Env and only merge in the config's models below.
		// The config's provider Name is silently ignored in that case.
		existing, ok := merged[id]
		if !ok {
			existing = Provider{ID: id, Name: cp.Name, Models: make(map[string]Model, len(cp.Models))}
		}
		if existing.Models == nil {
			existing.Models = make(map[string]Model, len(cp.Models))
		}
		for mid, cm := range cp.Models {
			if _, exists := existing.Models[mid]; !exists {
				name := cm.Name
				if name == "" {
					name = mid
				}
				existing.Models[mid] = Model{ID: mid, Name: name, ToolCall: cm.ToolCall}
			}
		}
		merged[id] = existing
	}

	return merged
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
