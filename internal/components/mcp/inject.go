package mcp

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

func Inject(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error) {
	if !adapter.SupportsMCP() {
		return InjectionResult{}, nil
	}

	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		return injectSeparateFile(homeDir, adapter, componentID)
	case model.StrategyMergeIntoSettings:
		return injectMergeIntoSettings(homeDir, adapter, componentID)
	case model.StrategyMCPConfigFile:
		return injectMCPConfigFile(homeDir, adapter, componentID)
	case model.StrategyTOMLFile:
		return injectTOMLFile(homeDir, adapter, componentID)
	case model.StrategyMergeIntoYAML:
		return injectYAMLFile(homeDir, adapter, componentID)
	default:
		return InjectionResult{}, fmt.Errorf("mcp injector does not support MCP strategy %d for agent %q", adapter.MCPStrategy(), adapter.Agent())
	}
}

// context7Args returns the pinned args slice for the Context7 MCP server.
func context7Args() []string {
	return []string{"-y", "--package=@upstash/context7-mcp@" + versions.Context7MCP, "--", "context7-mcp"}
}

// injectTOMLFile upserts the [mcp_servers.<server>] block into a TOML-based
// agent config file (e.g. ~/.codex/config.toml). The file is created if it
// does not yet exist.
func injectTOMLFile(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error) {
	serverName := string(componentID)
	configPath := adapter.MCPConfigPath(homeDir, serverName)

	existingBytes, err := osReadFile(configPath)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("read TOML config %q: %w", configPath, err)
	}

	existing := string(existingBytes)
	var updated string
	switch componentID {
	case model.ComponentHeadroom:
		updated = filemerge.UpsertCodexMCPServerBlock(existing, serverName, "headroom", []string{"mcp", "serve"})
	default:
		updated = filemerge.UpsertCodexRemoteMCPServerBlock(existing, serverName, "https://mcp.context7.com/mcp")
	}

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("write TOML config %q: %w", configPath, err)
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}

// injectYAMLFile upserts the <server> MCP server block into a YAML-based agent
// config file (e.g. ~/.hermes/config.yaml) via the filemerge YAML helpers.
func injectYAMLFile(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error) {
	serverName := string(componentID)
	configPath := adapter.MCPConfigPath(homeDir, serverName)

	raw, err := os.ReadFile(configPath)
	var existingBytes []byte
	switch {
	case err == nil:
		existingBytes = raw
	case os.IsNotExist(err):
		existingBytes = nil
	default:
		return InjectionResult{}, fmt.Errorf("read YAML config %q: %w", configPath, err)
	}

	existing := string(existingBytes)
	var updated string
	switch componentID {
	case model.ComponentHeadroom:
		updated = filemerge.UpsertYAMLMCPServerBlock(existing, serverName, "headroom", []string{"mcp", "serve"}, nil)
	default:
		updated = filemerge.UpsertHermesContext7Block(existing)
	}

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("write YAML config %q: %w", configPath, err)
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}

// injectSeparateFile writes a standalone JSON file per MCP server (Claude Code pattern).
func injectSeparateFile(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error) {
	serverName := string(componentID)
	path := adapter.MCPConfigPath(homeDir, serverName)

	var serverJSON []byte
	switch componentID {
	case model.ComponentHeadroom:
		serverJSON = DefaultHeadroomServerJSON()
	default:
		serverJSON = DefaultContext7ServerJSON()
	}

	writeResult, err := filemerge.WriteFileAtomic(path, serverJSON, 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{path}}, nil
}

// injectMergeIntoSettings merges MCP servers into a config file (OpenCode opencode.json, Gemini settings.json).
func injectMergeIntoSettings(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error) {
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	switch componentID {
	case model.ComponentHeadroom:
		if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
			settingsWrite, err := mergeJSONFile(settingsPath, OpenCodeHeadroomOverlayJSON())
			if err != nil {
				return InjectionResult{}, err
			}
			return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
		}
		if adapter.Agent() == model.AgentOpenClaw {
			return injectOpenClawMergeIntoSettings(settingsPath, componentID)
		}

		settingsWrite, err := mergeJSONFile(settingsPath, DefaultHeadroomOverlayJSON())
		if err != nil {
			return InjectionResult{}, err
		}
		return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
	default: // ComponentContext7
		if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
			settingsWrite, err := mergeJSONFile(settingsPath, OpenCodeContext7OverlayJSON())
			if err != nil {
				return InjectionResult{}, err
			}
			return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
		}
		if adapter.Agent() == model.AgentOpenClaw {
			return injectOpenClawMergeIntoSettings(settingsPath, componentID)
		}

		settingsWrite, err := mergeJSONFile(settingsPath, DefaultContext7OverlayJSON())
		if err != nil {
			return InjectionResult{}, err
		}
		return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
	}
}

func injectOpenClawMergeIntoSettings(settingsPath string, componentID model.ComponentID) (InjectionResult, error) {
	baseJSON, err := osReadFile(settingsPath)
	if err != nil {
		return InjectionResult{}, err
	}

	normalized, err := migrateOpenClawLegacyMCPServers(baseJSON)
	if err != nil {
		return InjectionResult{}, err
	}

	var overlay []byte
	switch componentID {
	case model.ComponentHeadroom:
		overlay = OpenClawHeadroomOverlayJSON()
	default:
		overlay = OpenClawContext7OverlayJSON()
	}

	merged, err := filemerge.MergeJSONObjects(normalized, overlay)
	if err != nil {
		return InjectionResult{}, err
	}

	settingsWrite, err := filemerge.WriteFileAtomic(settingsPath, merged, 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
}

func migrateOpenClawLegacyMCPServers(baseJSON []byte) ([]byte, error) {
	normalized, err := filemerge.MergeJSONObjects(baseJSON, []byte("{}"))
	if err != nil {
		return nil, err
	}

	root := map[string]any{}
	if err := json.Unmarshal(normalized, &root); err != nil {
		return nil, fmt.Errorf("unmarshal openclaw settings json: %w", err)
	}

	legacyServers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return normalized, nil
	}

	mcp, ok := root["mcp"].(map[string]any)
	if !ok {
		mcp = map[string]any{}
		root["mcp"] = mcp
	}

	servers, ok := mcp["servers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		mcp["servers"] = servers
	}

	for name, server := range legacyServers {
		if _, exists := servers[name]; !exists {
			servers[name] = server
		}
	}
	delete(root, "mcpServers")

	migrated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal migrated openclaw settings json: %w", err)
	}

	return append(migrated, '\n'), nil
}

// injectMCPConfigFile writes to a dedicated mcp.json config file (Cursor pattern).
func injectMCPConfigFile(homeDir string, adapter agents.Adapter, componentID model.ComponentID) (InjectionResult, error) {
	serverName := string(componentID)
	path := adapter.MCPConfigPath(homeDir, serverName)
	if path == "" {
		return InjectionResult{}, nil
	}

	var overlay []byte
	switch componentID {
	case model.ComponentHeadroom:
		switch adapter.Agent() {
		case model.AgentVSCodeCopilot:
			overlay = VSCodeHeadroomOverlayJSON()
		case model.AgentAntigravity:
			overlay = AntigravityHeadroomOverlayJSON()
		case model.AgentKimi:
			overlay = KimiHeadroomOverlayJSON()
		default:
			overlay = DefaultHeadroomOverlayJSON()
		}
	default: // ComponentContext7
		switch adapter.Agent() {
		case model.AgentVSCodeCopilot:
			overlay = VSCodeContext7OverlayJSON()
		case model.AgentAntigravity:
			overlay = AntigravityContext7OverlayJSON()
		case model.AgentKimi:
			overlay = KimiContext7OverlayJSON()
		default:
			overlay = DefaultContext7OverlayJSON()
		}
	}

	settingsWrite, err := mergeJSONFile(path, overlay)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: settingsWrite.Changed, Files: []string{path}}, nil
}

func mergeJSONFile(path string, overlay []byte) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, 0o644)
}

var osReadFile = func(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read json file %q: %w", path, err)
	}

	return content, nil
}
