package communitytool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const codeGraphUpstreamVersion = "1.4.1"

type codeGraphStrategy string

const (
	codeGraphNative     codeGraphStrategy = "native"
	codeGraphReconciled codeGraphStrategy = "reconciled"
	codeGraphExcluded   codeGraphStrategy = "excluded"
)

type codeGraphCompatibility struct {
	Agent         model.AgentID
	Strategy      codeGraphStrategy
	Target        string
	OwnedPaths    func(string, agents.Adapter) []string
	Postcondition func(string, agents.Adapter) (string, bool)
}

// codeGraphCompatibilityTable is deliberately exhaustive. Tests compare its
// keys with the registry so a newly added agent cannot silently enter setup.
var codeGraphCompatibilityTable = map[model.AgentID]codeGraphCompatibility{
	model.AgentClaudeCode:    nativeCompatibility(model.AgentClaudeCode, "claude"),
	model.AgentOpenCode:      reconciledCompatibility(model.AgentOpenCode, "opencode"),
	model.AgentKilocode:      excludedCompatibility(model.AgentKilocode),
	model.AgentGeminiCLI:     nativeCompatibility(model.AgentGeminiCLI, "gemini"),
	model.AgentCursor:        nativeCompatibility(model.AgentCursor, "cursor"),
	model.AgentVSCodeCopilot: excludedCompatibility(model.AgentVSCodeCopilot),
	model.AgentCodex:         nativeCompatibility(model.AgentCodex, "codex"),
	model.AgentAntigravity:   nativeCompatibility(model.AgentAntigravity, "antigravity"),
	model.AgentWindsurf:      excludedCompatibility(model.AgentWindsurf),
	model.AgentKimi:          excludedCompatibility(model.AgentKimi),
	model.AgentQwenCode:      excludedCompatibility(model.AgentQwenCode),
	model.AgentKiroIDE:       nativeCompatibility(model.AgentKiroIDE, "kiro"),
	model.AgentOpenClaw:      excludedCompatibility(model.AgentOpenClaw),
	model.AgentPi:            reconciledCompatibility(model.AgentPi, ""),
	model.AgentTrae:          excludedCompatibility(model.AgentTrae),
	model.AgentHermes:        nativeCompatibility(model.AgentHermes, "hermes"),
}

func nativeCompatibility(id model.AgentID, target string) codeGraphCompatibility {
	return codeGraphCompatibility{Agent: id, Strategy: codeGraphNative, Target: target, OwnedPaths: codeGraphOwnedPaths, Postcondition: codeGraphPostcondition}
}

func reconciledCompatibility(id model.AgentID, target string) codeGraphCompatibility {
	return codeGraphCompatibility{Agent: id, Strategy: codeGraphReconciled, Target: target, OwnedPaths: codeGraphOwnedPaths, Postcondition: codeGraphPostcondition}
}

func excludedCompatibility(id model.AgentID) codeGraphCompatibility {
	return codeGraphCompatibility{Agent: id, Strategy: codeGraphExcluded}
}

func codeGraphCompatibilityFor(id model.AgentID) (codeGraphCompatibility, bool) {
	compatibility, ok := codeGraphCompatibilityTable[id]
	return compatibility, ok
}

func isCodeGraphCompatibleAgent(id model.AgentID) bool {
	compatibility, ok := codeGraphCompatibilityFor(id)
	return ok && compatibility.Strategy != codeGraphExcluded
}

func codeGraphOwnedPaths(homeDir string, adapter agents.Adapter) []string {
	paths := codeGraphToolWiringPaths(homeDir, adapter)
	if adapter.SupportsSystemPrompt() && adapter.Agent() != model.AgentPi {
		paths = append(paths, adapter.SystemPromptFile(homeDir))
	}
	return paths
}

func codeGraphPostcondition(homeDir string, adapter agents.Adapter) (string, bool) {
	return hasCodeGraphToolWiring(homeDir, adapter)
}

func codeGraphToolWiringPaths(homeDir string, adapter agents.Adapter) []string {
	switch adapter.Agent() {
	case model.AgentClaudeCode:
		return []string{filepath.Join(homeDir, ".claude.json")}
	case model.AgentAntigravity:
		return []string{
			filepath.Join(homeDir, ".gemini", "config", "mcp_config.json"),
			filepath.Join(homeDir, ".gemini", "antigravity", "mcp_config.json"),
		}
	case model.AgentKiroIDE:
		return []string{filepath.Join(homeDir, ".kiro", "settings", "mcp.json")}
	default:
		paths := []string{adapter.MCPConfigPath(homeDir, "codegraph"), adapter.SettingsPath(homeDir)}
		if _, ok := adapter.(agents.EffectiveCodeGraphWiringDetector); ok && strings.HasSuffix(adapter.SettingsPath(homeDir), ".json") {
			paths = append(paths, strings.TrimSuffix(adapter.SettingsPath(homeDir), ".json")+".jsonc")
		}
		return paths
	}
}

func hasCodeGraphToolWiring(homeDir string, adapter agents.Adapter) (string, bool) {
	if adapter.Agent() == model.AgentClaudeCode {
		path := filepath.Join(homeDir, ".claude.json")
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false
		}
		return path, hasCanonicalCodeGraphServer(data)
	}
	if adapter.Agent() == model.AgentAntigravity {
		path := filepath.Join(homeDir, ".gemini", "antigravity", "mcp_config.json")
		if _, err := os.Stat(filepath.Join(homeDir, ".gemini", "config", ".migrated")); err == nil {
			path = filepath.Join(homeDir, ".gemini", "config", "mcp_config.json")
		}
		data, err := os.ReadFile(path)
		return path, err == nil && hasCanonicalCodeGraphServer(data)
	}
	if detector, ok := adapter.(agents.EffectiveCodeGraphWiringDetector); ok {
		return detector.EffectiveCodeGraphWiring(homeDir)
	}
	for _, path := range codeGraphToolWiringPaths(homeDir, adapter) {
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(strings.ToLower(string(data)), "codegraph") {
			return path, true
		}
	}
	return "", false
}

func hasCanonicalCodeGraphServer(data []byte) bool {
	var config struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(data, &config) != nil {
		return false
	}
	var server struct {
		Command string `json:"command"`
	}
	raw, ok := config.MCPServers["codegraph"]
	return ok && json.Unmarshal(raw, &server) == nil && server.Command == "codegraph"
}

func detectedCodeGraphTargets(homeDir string) []string {
	reg, err := agents.NewDefaultRegistry()
	if err != nil {
		return nil
	}
	targets := make([]string, 0)
	for _, installed := range agents.DiscoverInstalled(reg, homeDir) {
		compatibility, ok := codeGraphCompatibilityFor(installed.ID)
		if ok && compatibility.Strategy == codeGraphNative && compatibility.Target != "" {
			targets = append(targets, compatibility.Target)
		}
	}
	slices.Sort(targets)
	return slices.Compact(targets)
}

type codeGraphSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func snapshotCodeGraphPaths(paths []string) ([]codeGraphSnapshot, error) {
	snapshots := make([]codeGraphSnapshot, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			snapshots = append(snapshots, codeGraphSnapshot{path: path})
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot CodeGraph path %q: %w", path, err)
		}
		if info.IsDir() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("snapshot CodeGraph path %q: %w", path, err)
		}
		snapshots = append(snapshots, codeGraphSnapshot{path: path, data: data, mode: info.Mode(), exists: true})
	}
	return snapshots, nil
}

func restoreCodeGraphPaths(snapshots []codeGraphSnapshot) error {
	var restoreErr error
	for _, snapshot := range snapshots {
		if !snapshot.exists {
			if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
				restoreErr = fmt.Errorf("remove candidate-created CodeGraph file %q: %w", snapshot.path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(snapshot.path), 0o755); err != nil {
			restoreErr = err
			continue
		}
		if err := os.WriteFile(snapshot.path, snapshot.data, snapshot.mode.Perm()); err != nil {
			restoreErr = err
			continue
		}
		if err := os.Chmod(snapshot.path, snapshot.mode); err != nil {
			restoreErr = err
		}
	}
	return restoreErr
}
