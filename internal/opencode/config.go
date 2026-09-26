package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
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
	snapshot.Providers = map[string]Provider{}
	snapshot.Assignments = map[string]AssignmentPresence{}
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
			overlayConfiguredProviders(snapshot.Providers, layer)
			overlayConfiguredAssignments(snapshot.Assignments, layer)
			snapshot.Path = path
			paths = append(paths, path)
		}
	}
	if len(paths) > 1 {
		snapshot.Diagnostics = append(snapshot.Diagnostics, fmt.Sprintf("OpenCode layered config (%s): higher-priority overrides are preserved. Model edits to %s may not be effective; file-backed model/profile values are not runtime assignments.", strings.Join(paths, " < "), snapshot.WritePath))
	}
	return snapshot, err
}

// EffectiveSettingsPath returns the shared OpenCode settings write path.
func EffectiveSettingsPath(homeDir, projectDir string) string {
	snapshot, err := ResolveEffectiveConfigForHome(homeDir, projectDir)
	if snapshot.WritePath != "" {
		return snapshot.WritePath
	}
	if err != nil {
		return ""
	}
	return defaultEffectiveSettingsPath(homeDir)
}

// ResolveEffectiveConfigForHome retains the file-backed write authority for
// install, sync restoration, profiles, and deletion; it does not merge reads.
func ResolveEffectiveConfigForHome(homeDir, projectDir string) (ConfigSnapshot, error) {
	path, err := findEffectiveConfigPathChecked(homeDir, projectDir)
	if err != nil {
		return ConfigSnapshot{}, err
	}
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
	path, _ := findEffectiveConfigPathChecked(homeDir, projectDir)
	return path
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

func legacyConfiguredProviders(root map[string]any) map[string]Provider {
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
			Cost:      configuredCost(def),
			Limit:     configuredLimit(def),
			Variants:  configuredVariants(def),
		}
	}
	return models
}

func configuredCost(def map[string]any) ModelCost {
	costRaw, ok := def["cost"].(map[string]any)
	if !ok {
		return ModelCost{}
	}
	return ModelCost{
		Input:  floatValue(costRaw["input"]),
		Output: floatValue(costRaw["output"]),
	}
}

func configuredLimit(def map[string]any) ModelLimit {
	limitRaw, ok := def["limit"].(map[string]any)
	if !ok {
		return ModelLimit{}
	}
	contextLimit := intValue(limitRaw["context"])
	if contextLimit == 0 {
		contextLimit = intValue(limitRaw["input"])
	}
	return ModelLimit{
		Context: contextLimit,
		Output:  intValue(limitRaw["output"]),
	}
}

func configuredVariants(def map[string]any) []string {
	extract := func(raw any) []string {
		if raw == nil {
			return nil
		}
		var list []string
		seen := make(map[string]bool)
		add := func(s string) {
			s = strings.TrimSpace(s)
			if s != "" && !seen[s] {
				seen[s] = true
				list = append(list, s)
			}
		}

		var collect func(v any)
		collect = func(v any) {
			if v == nil {
				return
			}
			switch val := v.(type) {
			case string:
				add(val)
			case []string:
				for _, item := range val {
					add(item)
				}
			case []any:
				for _, item := range val {
					collect(item)
				}
			case map[string]any:
				if values, ok := val["values"]; ok {
					if typeStr, okType := val["type"].(string); okType && typeStr != "" && typeStr != "effort" {
						return
					}
					collect(values)
					return
				}
				if id, ok := val["id"].(string); ok && strings.TrimSpace(id) != "" {
					add(id)
					return
				}
				if name, ok := val["name"].(string); ok && strings.TrimSpace(name) != "" {
					add(name)
					return
				}
				for key := range val {
					add(key)
				}
			}
		}

		collect(raw)
		return list
	}

	variants := extract(def["variants"])
	if len(variants) == 0 {
		variants = extract(def["reasoning_options"])
	}
	if len(variants) == 0 {
		return nil
	}
	sortVariants(variants)
	return variants
}

func legacyConfiguredAssignments(root map[string]any) map[string]AssignmentPresence {
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
		assignment, ok := model.ParseModelReference(modelSpec)
		if !ok {
			assignments[key] = AssignmentPresence{Present: true}
			continue
		}
		if assignment.Effort == "" {
			assignment.Effort, _ = def["variant"].(string)
		}
		assignments[key] = AssignmentPresence{Present: true, Assignment: assignment}
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
	keys := []string{
		"gentle-orchestrator", "sdd-orchestrator", ReviewRefuterAgent, ReviewValidatorAgent,
		"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker",
		"review-risk", "review-readability", "review-reliability", "review-resilience",
	}
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

func floatValue(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
		return 0
	default:
		return 0
	}
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case json.Number:
		i, err := v.Int64()
		if err == nil {
			return int(i)
		}
		f, _ := v.Float64()
		return int(f)
	case string:
		s := strings.TrimSpace(v)
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return int(i)
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f)
		}
		return 0
	default:
		return 0
	}
}
