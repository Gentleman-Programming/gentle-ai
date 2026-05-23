package engram

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

const DataDirEnvVar = "ENGRAM_DATA_DIR"

func SyncDataDirEnv(homeDir string, adapter agents.Adapter, dataDir string) (InjectionResult, error) {
	if adapter.Agent() == model.AgentPi || !adapter.SupportsMCP() {
		return InjectionResult{}, nil
	}
	dataDir = cleanDataDirEnv(dataDir)
	if !hasExistingEngramMCPConfig(homeDir, adapter) {
		return InjectionResult{}, nil
	}
	files := make([]string, 0, 2)
	changed := false

	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		mcpPath := adapter.MCPConfigPath(homeDir, "engram")
		cmd := stableEngramCommandForMergedConfig(mcpPath, adapter.Agent())
		content := buildSeparateMCPContent(mcpPath, engramServerJSONWithDataDir(cmd, dataDir))
		mcpWrite, err := filemerge.WriteFileAtomic(mcpPath, content, 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)
	case model.StrategyMergeIntoSettings:
		settingsPath := adapter.SettingsPath(homeDir)
		if settingsPath == "" {
			break
		}
		cmd := stableEngramCommandForMergedConfig(settingsPath, adapter.Agent())
		settingsWrite, err := mergeJSONFile(settingsPath, engramOverlayJSONWithDataDir(adapter.Agent(), cmd, dataDir))
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || settingsWrite.Changed
		files = append(files, settingsPath)
	case model.StrategyMCPConfigFile:
		mcpPath := adapter.MCPConfigPath(homeDir, "engram")
		if mcpPath == "" {
			break
		}
		cmd := stableEngramCommandForMergedConfig(mcpPath, adapter.Agent())
		overlay := engramOverlayJSONWithDataDir(adapter.Agent(), cmd, dataDir)
		if adapter.Agent() == model.AgentVSCodeCopilot {
			overlay = vsCodeEngramOverlayJSONWithDataDir(cmd, dataDir)
		}
		mcpWrite, err := mergeJSONFile(mcpPath, overlay)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)
		if adapter.Agent() == model.AgentAntigravity {
			pluginPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json")
			pluginWrite, err := filemerge.WriteFileAtomic(pluginPath, engramStandaloneOverlayJSONWithDataDir(model.AgentAntigravity, cmd, dataDir), 0o644)
			if err != nil {
				return InjectionResult{}, err
			}
			changed = changed || pluginWrite.Changed
			files = append(files, pluginPath)
		}
	case model.StrategyTOMLFile:
		configPath := adapter.MCPConfigPath(homeDir, "engram")
		if configPath == "" {
			break
		}
		existing, err := readFileOrEmpty(configPath)
		if err != nil {
			return InjectionResult{}, err
		}
		cmd := stableEngramCommandForMergedConfig(configPath, adapter.Agent())
		tomlWrite, err := filemerge.WriteFileAtomic(configPath, []byte(upsertCodexEngramBlockWithDataDir(existing, cmd, dataDir)), 0o644)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || tomlWrite.Changed
		files = append(files, configPath)
	}

	return InjectionResult{Changed: changed, Files: files}, nil
}

func hasExistingEngramMCPConfig(homeDir string, adapter agents.Adapter) bool {
	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		return fileExists(adapter.MCPConfigPath(homeDir, "engram"))
	case model.StrategyMergeIntoSettings:
		return jsonFileHasEngramServer(adapter.SettingsPath(homeDir))
	case model.StrategyMCPConfigFile:
		if jsonFileHasEngramServer(adapter.MCPConfigPath(homeDir, "engram")) {
			return true
		}
		if adapter.Agent() == model.AgentAntigravity {
			pluginPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json")
			return jsonFileHasEngramServer(pluginPath)
		}
	case model.StrategyTOMLFile:
		content, err := os.ReadFile(adapter.MCPConfigPath(homeDir, "engram"))
		return err == nil && strings.Contains(string(content), "[mcp_servers.engram")
	}
	return false
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func jsonFileHasEngramServer(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var root any
	if err := json.Unmarshal(content, &root); err != nil {
		return strings.Contains(string(content), `"engram"`)
	}
	return jsonObjectHasKey(root, "engram")
}

func jsonObjectHasKey(v any, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if k == key || jsonObjectHasKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if jsonObjectHasKey(child, key) {
				return true
			}
		}
	}
	return false
}

func engramServerJSONWithDataDir(cmd, dataDir string) []byte {
	cfg := engramServerConfig(cmd, dataDir)
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

func engramOverlayJSONWithDataDir(agentID model.AgentID, cmd, dataDir string) []byte {
	server := engramServerConfig(cmd, dataDir)
	var cfg map[string]any
	if agentID == model.AgentOpenCode || agentID == model.AgentKilocode {
		server = map[string]any{
			"command": []string{cmd, "mcp", "--tools=agent"},
			"type":    "local",
		}
		addDataDirEnv(server, dataDir)
		cfg = map[string]any{
			"mcp": map[string]any{
				"engram": map[string]any{"__replace__": server},
			},
		}
	} else if agentID == model.AgentOpenClaw {
		cfg = map[string]any{
			"mcp": map[string]any{
				"servers": map[string]any{
					"engram": map[string]any{"__replace__": server},
				},
			},
		}
	} else {
		if agentID == model.AgentAntigravity {
			server["args"] = []string{"mcp"}
		}
		cfg = map[string]any{
			"mcpServers": map[string]any{
				"engram": map[string]any{"__replace__": server},
			},
		}
	}
	return marshalJSON(cfg)
}

func engramStandaloneOverlayJSONWithDataDir(agentID model.AgentID, cmd, dataDir string) []byte {
	server := engramServerConfig(cmd, dataDir)
	if agentID == model.AgentOpenCode || agentID == model.AgentKilocode {
		server = map[string]any{
			"command": []string{cmd, "mcp", "--tools=agent"},
			"type":    "local",
		}
		addDataDirEnv(server, dataDir)
		return marshalJSON(map[string]any{
			"mcp": map[string]any{
				"engram": server,
			},
		})
	}
	if agentID == model.AgentOpenClaw {
		return marshalJSON(map[string]any{
			"mcp": map[string]any{
				"servers": map[string]any{
					"engram": server,
				},
			},
		})
	}
	if agentID == model.AgentAntigravity {
		server["args"] = []string{"mcp"}
	}
	return marshalJSON(map[string]any{
		"mcpServers": map[string]any{
			"engram": server,
		},
	})
}

func vsCodeEngramOverlayJSONWithDataDir(cmd, dataDir string) []byte {
	cfg := map[string]any{
		"servers": map[string]any{
			"engram": map[string]any{"__replace__": engramServerConfig(cmd, dataDir)},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

func engramServerConfig(cmd, dataDir string) map[string]any {
	cfg := map[string]any{
		"command": cmd,
		"args":    []string{"mcp", "--tools=agent"},
	}
	addDataDirEnv(cfg, dataDir)
	return cfg
}

func addDataDirEnv(cfg map[string]any, dataDir string) {
	if dataDir != "" {
		cfg["env"] = map[string]string{DataDirEnvVar: dataDir}
	}
}

func cleanDataDirEnv(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Clean(dataDir)
}

func upsertCodexEngramBlockWithDataDir(content, engramCmd, dataDir string) string {
	if engramCmd == "" {
		engramCmd = "engram"
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	var kept []string
	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "[mcp_servers.engram]" || strings.HasPrefix(trimmed, "[mcp_servers.engram.") {
			i++
			for i < len(lines) {
				next := strings.TrimSpace(lines[i])
				if strings.HasPrefix(next, "[") && strings.HasSuffix(next, "]") {
					break
				}
				i++
			}
			continue
		}
		kept = append(kept, lines[i])
		i++
	}

	block := fmt.Sprintf("[mcp_servers.engram]\ncommand = %q\nargs = [\"mcp\", \"--tools=agent\"]\n", engramCmd)
	if dataDir != "" {
		block += fmt.Sprintf("\n[mcp_servers.engram.env]\n%s = %q\n", DataDirEnvVar, dataDir)
	}
	base := strings.TrimSpace(strings.Join(kept, "\n"))
	if base == "" {
		return block
	}
	return base + "\n\n" + block
}

func marshalJSON(cfg map[string]any) []byte {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}
