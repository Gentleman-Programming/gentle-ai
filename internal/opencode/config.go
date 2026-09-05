package opencode

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

var supportedConfigNames = []string{"opencode.jsonc", "opencode.json"}

// ConfigSnapshot is the file-backed OpenCode configuration view shared by UI,
// install, and sync flows.
type ConfigSnapshot struct {
	Path        string
	WritePath   string
	Providers   map[string]Provider
	Assignments map[string]AssignmentPresence
	Diagnostics []string
}

// AssignmentPresence distinguishes absent assignment keys from explicit user
// intent in the effective OpenCode config.
type AssignmentPresence struct {
	Present    bool
	Cleared    bool
	Managed    bool
	Assignment model.ModelAssignment
}

// ResolveEffectiveConfig locates and parses the effective OpenCode config for
// projectDir. Existing project configs win over global configs; within the same
// directory, JSONC wins over JSON. Ancestor lookup stops at the nearest Git root;
// this snapshot does not implement OpenCode's full multi-file merge semantics or
// OPENCODE_CONFIG overrides. When no config exists, WritePath points at
// OpenCode's default settings path.
func ResolveEffectiveConfig(projectDir string) (ConfigSnapshot, error) {
	home, _ := os.UserHomeDir()
	return ResolveEffectiveConfigForHome(home, projectDir)
}

// EffectiveSettingsPath returns the shared OpenCode settings write path. Existing
// JSONC wins over JSON so user-owned comments are not silently flattened.
func EffectiveSettingsPath(homeDir, projectDir string) string {
	snapshot, err := ResolveEffectiveConfigForHome(homeDir, projectDir)
	if snapshot.WritePath != "" {
		return snapshot.WritePath
	}
	if err != nil {
		return DefaultSettingsPathForHome(homeDir)
	}
	return DefaultSettingsPathForHome(homeDir)
}

// ResolveEffectiveConfigForHome is ResolveEffectiveConfig with an explicit home
// directory for callers that already operate on a test or installation root.
func ResolveEffectiveConfigForHome(homeDir, projectDir string) (ConfigSnapshot, error) {
	path := findEffectiveConfigPath(homeDir, projectDir)
	snapshot := ConfigSnapshot{
		Path:        path,
		WritePath:   path,
		Providers:   map[string]Provider{},
		Assignments: map[string]AssignmentPresence{},
	}
	if snapshot.WritePath == "" {
		snapshot.WritePath = DefaultSettingsPathForHome(homeDir)
		return snapshot, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return snapshot, err
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return snapshot, err
	}
	snapshot.Providers = configuredProviders(root)
	snapshot.Assignments = configuredAssignments(root)
	return snapshot, nil
}

func findEffectiveConfigPath(homeDir, projectDir string) string {
	for _, dir := range candidateConfigDirs(homeDir, projectDir) {
		for _, name := range supportedConfigNames {
			path := filepath.Join(dir, name)
			if fileExists(path) {
				return path
			}
		}
	}
	return ""
}

func candidateConfigDirs(homeDir, projectDir string) []string {
	var dirs []string
	if projectDir != "" {
		if abs, err := filepath.Abs(projectDir); err == nil {
			for {
				dirs = append(dirs, abs)
				if fileExists(filepath.Join(abs, ".git")) || dirExists(filepath.Join(abs, ".git")) {
					break
				}
				parent := filepath.Dir(abs)
				if parent == abs {
					break
				}
				abs = parent
			}
		}
	}
	if homeDir != "" {
		dirs = append(dirs, filepath.Dir(DefaultSettingsPathForHome(homeDir)))
	}
	return dirs
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func configuredProviders(root map[string]any) map[string]Provider {
	providerRaw, _ := root["provider"].(map[string]any)
	providers := make(map[string]Provider, len(providerRaw))
	for id, raw := range providerRaw {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		provider := Provider{
			ID:     id,
			Name:   stringValue(def["name"], id),
			URL:    providerURL(def),
			Models: configuredModels(def),
		}
		providers[id] = provider
	}
	return providers
}

func providerURL(def map[string]any) string {
	if direct, ok := def["url"].(string); ok && direct != "" {
		return direct
	}
	options, _ := def["options"].(map[string]any)
	if fallback, ok := options["baseURL"].(string); ok {
		return fallback
	}
	return ""
}

func configuredModels(provider map[string]any) map[string]Model {
	modelRaw, _ := provider["models"].(map[string]any)
	models := make(map[string]Model, len(modelRaw))
	for id, raw := range modelRaw {
		def, _ := raw.(map[string]any)
		models[id] = Model{
			ID:        id,
			Name:      stringValue(def["name"], id),
			Family:    stringValue(def["family"], ""),
			ToolCall:  boolValue(def["tool_call"]) || boolValue(def["toolcall"]),
			Reasoning: boolValue(def["reasoning"]),
		}
	}
	return models
}

func configuredAssignments(root map[string]any) map[string]AssignmentPresence {
	agentRaw, _ := root["agent"].(map[string]any)
	assignments := make(map[string]AssignmentPresence, len(agentRaw))
	for name, raw := range agentRaw {
		key := name
		if name == "sdd-orchestrator" {
			key = "gentle-orchestrator"
		}
		def, ok := raw.(map[string]any)
		if !ok {
			assignments[key] = AssignmentPresence{Present: true}
			continue
		}
		modelValue, hasModel := def["model"]
		modelSpec, _ := modelValue.(string)
		if !hasModel || strings.TrimSpace(modelSpec) == "" {
			// Presence is the pre-write evidence that an existing assignment was
			// cleared. Restoration may exempt a managed definition when its active
			// mode intentionally generates agents without model fields.
			assignments[key] = AssignmentPresence{Present: true, Cleared: true, Managed: looksLikeManagedOpenCodeAgent(def)}
			continue
		}
		providerID, modelID, ok := model.SplitModelSpec(strings.TrimSpace(modelSpec))
		if !ok {
			assignments[key] = AssignmentPresence{Present: true}
			continue
		}
		effort, _ := def["variant"].(string)
		assignments[key] = AssignmentPresence{Present: true, Assignment: model.ModelAssignment{ProviderID: providerID, ModelID: modelID, Effort: effort}}
	}
	return assignments
}

func looksLikeManagedOpenCodeAgent(def map[string]any) bool {
	hidden, _ := def["hidden"].(bool)
	if !hidden {
		return false
	}
	if _, ok := def["prompt"].(string); !ok {
		return false
	}
	_, ok := def["permission"].(map[string]any)
	return ok
}

func stringValue(value any, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}

func boolValue(value any) bool {
	flag, _ := value.(bool)
	return flag
}
