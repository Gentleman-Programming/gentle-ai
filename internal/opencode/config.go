package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

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

// ResolveEffectiveConfig reads the layered local JSON/JSONC view. WritePath is
// independently selected; Path is the highest-priority file, not a write target.
// This does not emulate OpenCode's remote config, substitutions, plugins, or
// OPENCODE_CONFIG overrides. Assignments are not a full runtime model resolution.
func ResolveEffectiveConfig(projectDir string) (ConfigSnapshot, error) {
	home, _ := os.UserHomeDir()
	return ResolveRuntimeConfigForHome(home, projectDir)
}

// ResolveRuntimeConfigForHome overlays normal global, ancestor/project, then
// additive OPENCODE_CONFIG_DIR files. Within each directory JSONC overrides JSON.
func ResolveRuntimeConfigForHome(homeDir, projectDir string) (ConfigSnapshot, error) {
	snapshot, err := ResolveEffectiveConfigForHome(homeDir, projectDir)
	if err != nil {
		return snapshot, err
	}
	dirs := candidateConfigDirs(homeDir, projectDir)
	if override := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")); filepath.IsAbs(override) {
		dirs = append([]string{override}, dirs[:len(dirs)-1]...)
		if homeDir != "" {
			dirs = append(dirs, filepath.Dir(DefaultSettingsPathForHome(homeDir)))
		}
	}
	root := map[string]any{}
	var paths []string
	for i := len(dirs) - 1; i >= 0; i-- {
		for _, name := range []string{"opencode.json", "opencode.jsonc"} {
			path := filepath.Join(dirs[i], name)
			if !fileExists(path) {
				continue
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return snapshot, err
			}
			layer, err := filemerge.UnmarshalJSONObject(raw)
			if err != nil {
				return snapshot, fmt.Errorf("read OpenCode config %s: %w", path, err)
			}
			overlayConfigFields(root, layer)
			snapshot.Path = path
			paths = append(paths, path)
		}
	}
	snapshot.Providers = configuredProviders(root)
	snapshot.Assignments = configuredAssignments(root)
	if len(paths) > 1 {
		snapshot.Diagnostics = append(snapshot.Diagnostics, fmt.Sprintf("OpenCode layered config (%s): higher-priority overrides are preserved. Model edits to %s may not be effective; file-backed model/profile values are not runtime assignments.", strings.Join(paths, " < "), snapshot.WritePath))
	}
	return snapshot, err
}

// overlayConfigFields overlays local object fields without interpreting installer
// directives such as __replace__; those keys are ordinary data in runtime reads.
func overlayConfigFields(base, layer map[string]any) {
	for key, value := range layer {
		baseMap, baseOK := base[key].(map[string]any)
		layerMap, layerOK := value.(map[string]any)
		if baseOK && layerOK {
			overlayConfigFields(baseMap, layerMap)
		} else {
			base[key] = value
		}
	}
}

// EffectiveSettingsPath returns the shared OpenCode settings write path.
func EffectiveSettingsPath(homeDir, projectDir string) string {
	snapshot, err := ResolveEffectiveConfigForHome(homeDir, projectDir)
	if snapshot.WritePath != "" {
		return snapshot.WritePath
	}
	if err != nil {
		return defaultEffectiveSettingsPath(homeDir)
	}
	return defaultEffectiveSettingsPath(homeDir)
}

// ResolveEffectiveConfigForHome retains the file-backed write authority for
// install, sync restoration, profiles, and deletion; it does not merge reads.
func ResolveEffectiveConfigForHome(homeDir, projectDir string) (ConfigSnapshot, error) {
	path := findEffectiveConfigPath(homeDir, projectDir)
	snapshot := ConfigSnapshot{
		Path:        path,
		WritePath:   path,
		Providers:   map[string]Provider{},
		Assignments: map[string]AssignmentPresence{},
	}
	if snapshot.WritePath == "" {
		snapshot.WritePath = defaultEffectiveSettingsPath(homeDir)
		return snapshot, nil
	}

	return ReadConfigSnapshot(path)
}

// ReadConfigSnapshot reads only the named file, including restoration evidence.
func ReadConfigSnapshot(path string) (ConfigSnapshot, error) {
	snapshot := ConfigSnapshot{Path: path, WritePath: path}
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
		jsonPath := filepath.Join(dir, "opencode.json")
		jsoncPath := filepath.Join(dir, "opencode.jsonc")
		jsonExists := fileExists(jsonPath)
		jsoncExists := fileExists(jsoncPath)

		switch {
		case jsonExists && jsoncExists:
			if managedConfigPriority(jsoncPath) > managedConfigPriority(jsonPath) {
				return jsoncPath
			}
			return jsonPath
		case jsonExists:
			return jsonPath
		case jsoncExists:
			return jsoncPath
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
	if globalDir := effectiveGlobalConfigDir(homeDir); globalDir != "" {
		dirs = append(dirs, globalDir)
	}
	return dirs
}

func effectiveGlobalConfigDir(homeDir string) string {
	if dir := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")); filepath.IsAbs(dir) {
		return dir
	}
	if homeDir == "" {
		return ""
	}
	return filepath.Dir(DefaultSettingsPathForHome(homeDir))
}

func defaultEffectiveSettingsPath(homeDir string) string {
	if dir := effectiveGlobalConfigDir(homeDir); dir != "" {
		return filepath.Join(dir, "opencode.json")
	}
	return ""
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

// managedConfigPriority ranks explicit markers above the legacy shape heuristic.
// The heuristic preserves old installs; it is not proof of ownership.
func managedConfigPriority(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return 0
	}
	agents, _ := root["agent"].(map[string]any)
	for _, raw := range agents {
		def, _ := raw.(map[string]any)
		if def["__managed_by"] == "gentle-ai/sdd" {
			return 2
		}
	}
	for _, key := range managedOpenCodeAgentKeys() {
		def, _ := agents[key].(map[string]any)
		if looksLikeManagedOpenCodeAgent(def) {
			return 1
		}
	}
	return 0
}

func managedOpenCodeAgentKeys() []string {
	keys := []string{"gentle-orchestrator", "sdd-orchestrator", ReviewRefuterAgent, ReviewValidatorAgent}
	keys = append(keys, SDDPhases()...)
	keys = append(keys, JDPhases()...)
	return keys
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
