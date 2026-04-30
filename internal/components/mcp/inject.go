package mcp

import (
	"fmt"
	"os"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

type InjectionResult struct {
	Changed bool
	Files   []string
}

func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if !adapter.SupportsMCP() {
		return InjectionResult{}, nil
	}

	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		return injectSeparateFile(homeDir, adapter)
	case model.StrategyMergeIntoSettings:
		return injectMergeIntoSettings(homeDir, adapter)
	case model.StrategyMCPConfigFile:
		return injectMCPConfigFile(homeDir, adapter)
	case model.StrategyTOMLFile:
		// Context7 injection is not supported for TOML-based agents (Codex).
		// Codex receives Context7 through its agents.md system prompt, not via MCP config.
		return InjectionResult{}, nil
	default:
		return InjectionResult{}, fmt.Errorf("mcp injector does not support MCP strategy %d for agent %q", adapter.MCPStrategy(), adapter.Agent())
	}
}

// injectSeparateFile writes a standalone JSON file per MCP server (Claude Code pattern).
func injectSeparateFile(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	path := adapter.MCPConfigPath(homeDir, "context7")
	writeResult, err := filemerge.WriteFileAtomic(path, DefaultContext7ServerJSON(), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{path}}, nil
}

// injectMergeIntoSettings merges MCP servers into a config file (OpenCode opencode.json, Gemini settings.json).
func injectMergeIntoSettings(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	overlay := DefaultContext7OverlayJSON()
	if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
		overlay = OpenCodeContext7OverlayJSON()
	}

	settingsWrite, err := mergeJSONFile(settingsPath, overlay)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
}

// injectMCPConfigFile writes to a dedicated mcp.json config file (Cursor pattern).
func injectMCPConfigFile(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	path := adapter.MCPConfigPath(homeDir, "context7")
	if path == "" {
		return InjectionResult{}, nil
	}

	overlay := DefaultContext7OverlayJSON()
	if adapter.Agent() == model.AgentVSCodeCopilot {
		overlay = VSCodeContext7OverlayJSON()
	}
	if adapter.Agent() == model.AgentAntigravity {
		overlay = AntigravityContext7OverlayJSON()
	}
	if adapter.Agent() == model.AgentKimi {
		overlay = KimiContext7OverlayJSON()
	}

	// For mcp.json pattern, merge the server config as a named entry.
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
