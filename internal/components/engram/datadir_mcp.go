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
		settingsWrite, err := mergeJSONFile(settingsPath, engramOverlayJSONWithDataDir(settingsPath, adapter.Agent(), cmd, dataDir))
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
		overlay := engramOverlayJSONWithDataDir(mcpPath, adapter.Agent(), cmd, dataDir)
		if adapter.Agent() == model.AgentVSCodeCopilot {
			overlay = vsCodeEngramOverlayJSONWithDataDir(mcpPath, cmd, dataDir)
		}
		mcpWrite, err := mergeJSONFile(mcpPath, overlay)
		if err != nil {
			return InjectionResult{}, err
		}
		changed = changed || mcpWrite.Changed
		files = append(files, mcpPath)
		if adapter.Agent() == model.AgentAntigravity {
			pluginPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json")
			pluginWrite, err := mergeJSONFile(pluginPath, engramOverlayJSONWithDataDir(pluginPath, model.AgentAntigravity, cmd, dataDir))
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
		return jsonFileHasEngramServer(adapter.SettingsPath(homeDir), adapter.Agent())
	case model.StrategyMCPConfigFile:
		if jsonFileHasEngramServer(adapter.MCPConfigPath(homeDir, "engram"), adapter.Agent()) {
			return true
		}
		if adapter.Agent() == model.AgentAntigravity {
			pluginPath := filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram", "mcp_config.json")
			return jsonFileHasEngramServer(pluginPath, adapter.Agent())
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

func jsonFileHasEngramServer(path string, agentID model.AgentID) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	_, ok := existingMergedEngramServer(content, agentID)
	return ok
}

func engramServerJSONWithDataDir(cmd, dataDir string) []byte {
	cfg := engramServerConfig(cmd, dataDir)
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

func engramOverlayJSONWithDataDir(path string, agentID model.AgentID, cmd, dataDir string) []byte {
	server := engramServerConfigForPath(path, agentID, cmd, dataDir)
	var cfg map[string]any
	if agentID == model.AgentOpenCode || agentID == model.AgentKilocode {
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

func vsCodeEngramOverlayJSONWithDataDir(path, cmd, dataDir string) []byte {
	cfg := map[string]any{
		"servers": map[string]any{
			"engram": map[string]any{"__replace__": engramServerConfigForPath(path, model.AgentVSCodeCopilot, cmd, dataDir)},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}

func engramServerConfig(cmd, dataDir string) map[string]any {
	return engramServerConfigFromExisting(nil, "", cmd, dataDir)
}

func engramServerConfigForPath(path string, agentID model.AgentID, cmd, dataDir string) map[string]any {
	existing := map[string]any(nil)
	if raw, err := osReadFile(path); err == nil {
		existing, _ = existingMergedEngramServer(raw, agentID)
	}
	return engramServerConfigFromExisting(existing, agentID, cmd, dataDir)
}

func engramServerConfigFromExisting(existing map[string]any, agentID model.AgentID, cmd, dataDir string) map[string]any {
	cfg := cloneJSONMap(existing)
	switch agentID {
	case model.AgentOpenCode, model.AgentKilocode:
		cfg["command"] = []string{cmd, "mcp", "--tools=agent"}
		cfg["type"] = "local"
		delete(cfg, "args")
	case model.AgentAntigravity:
		cfg["command"] = cmd
		cfg["args"] = []string{"mcp"}
	default:
		cfg["command"] = cmd
		cfg["args"] = []string{"mcp", "--tools=agent"}
	}
	upsertDataDirEnv(cfg, dataDir)
	return cfg
}

func upsertDataDirEnv(cfg map[string]any, dataDir string) {
	env, _ := cfg["env"].(map[string]any)
	if dataDir != "" {
		if env == nil {
			env = map[string]any{}
		}
		env[DataDirEnvVar] = dataDir
		cfg["env"] = env
		return
	}
	if env != nil {
		delete(env, DataDirEnvVar)
		if len(env) == 0 {
			delete(cfg, "env")
		}
	}
}

func cloneJSONMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	for k, v := range in {
		out[k] = v
	}
	return out
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
	serverLines := codexEngramServerLines(lines)
	envLines := codexEngramEnvLines(lines)
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

	commandLine := fmt.Sprintf("command = %q", engramCmd)
	for _, line := range serverLines {
		if strings.HasPrefix(strings.TrimSpace(line), "command") {
			commandLine = line
			break
		}
	}
	block := "[mcp_servers.engram]\n" + commandLine + "\nargs = [\"mcp\", \"--tools=agent\"]\n"
	for _, line := range serverLines {
		key := strings.TrimSpace(strings.SplitN(strings.TrimSpace(line), "=", 2)[0])
		if key == "command" || key == "args" {
			continue
		}
		block += line + "\n"
	}
	if dataDir != "" || len(envLines) > 0 {
		block += "\n[mcp_servers.engram.env]\n"
		for _, line := range envLines {
			block += line + "\n"
		}
		if dataDir != "" {
			block += fmt.Sprintf("%s = %q\n", DataDirEnvVar, dataDir)
		}
	}
	base := strings.TrimSpace(strings.Join(kept, "\n"))
	if base == "" {
		return block
	}
	return base + "\n\n" + block
}

func codexEngramServerLines(lines []string) []string {
	var out []string
	inServer := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inServer = trimmed == "[mcp_servers.engram]"
			continue
		}
		if !inServer || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func codexEngramEnvLines(lines []string) []string {
	var out []string
	inEnv := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inEnv = trimmed == "[mcp_servers.engram.env]"
			continue
		}
		if !inEnv || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key := strings.TrimSpace(strings.SplitN(trimmed, "=", 2)[0])
		if key == DataDirEnvVar {
			continue
		}
		out = append(out, line)
	}
	return out
}

func marshalJSON(cfg map[string]any) []byte {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}
