package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agents"
	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

// InjectionResult records the outcome of an MCP configuration injection operation.
type InjectionResult struct {
	Changed bool
	Files   []string
}

// Inject configures MCP server settings for the given adapter according to its Strategy.
func Inject(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	if !adapter.SupportsMCP() {
		return InjectionResult{}, nil
	}

	switch adapter.MCPStrategy() {
	case model.StrategySeparateMCPFiles:
		if adapter.Agent() == model.AgentClaudeCode {
			return injectMergeIntoSettings(homeDir, adapter)
		}
		return injectSeparateFile(homeDir, adapter)
	case model.StrategyMergeIntoSettings:
		return injectMergeIntoSettings(homeDir, adapter)
	case model.StrategyMCPConfigFile:
		return injectMCPConfigFile(homeDir, adapter)
	case model.StrategyTOMLFile:
		return injectTOMLFile(homeDir, adapter)
	case model.StrategyMergeIntoYAML:
		return injectYAMLFile(homeDir, adapter)
	default:
		return InjectionResult{}, fmt.Errorf("mcp injector does not support MCP strategy %d for agent %q", adapter.MCPStrategy(), adapter.Agent())
	}
}

// context7Args returns the pinned args slice for the Context7 MCP server.
func context7Args() []string {
	return []string{"-y", "--package=@upstash/context7-mcp@" + versions.Context7MCP, "--", "context7-mcp"}
}

// injectTOMLFile upserts the [mcp_servers.context7] block into a TOML-based
// agent config file (e.g. ~/.codex/config.toml) using Context7's remote MCP
// endpoint. The file is created if it does not yet exist.
func injectTOMLFile(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	configPath := adapter.MCPConfigPath(homeDir, "context7")
	if configPath == "" {
		return InjectionResult{}, nil
	}

	existingBytes, err := osReadFile(configPath)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("read TOML config %q: %w", configPath, err)
	}

	existing := string(existingBytes)
	updated := filemerge.UpsertCodexRemoteMCPServerBlock(existing, "context7", "https://mcp.context7.com/mcp")

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("write TOML config %q: %w", configPath, err)
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}

// injectYAMLFile upserts the context7 MCP server block into a YAML-based agent
// config file (e.g. ~/.hermes/config.yaml) via the filemerge YAML helpers.
// The file is created if it does not yet exist. The upsert is idempotent and
// comment-preserving — user content outside the managed block is untouched.
func injectYAMLFile(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	configPath := adapter.MCPConfigPath(homeDir, "context7")
	if configPath == "" {
		return InjectionResult{}, nil
	}

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
	updated := filemerge.UpsertHermesContext7Block(existing)

	writeResult, err := filemerge.WriteFileAtomic(configPath, []byte(updated), 0o644)
	if err != nil {
		return InjectionResult{}, fmt.Errorf("write YAML config %q: %w", configPath, err)
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{configPath}}, nil
}

// injectSeparateFile writes a standalone JSON file per MCP server.
func injectSeparateFile(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	path := adapter.MCPConfigPath(homeDir, "context7")
	if path == "" {
		return InjectionResult{}, nil
	}
	writeResult, err := filemerge.WriteFileAtomic(path, DefaultContext7ServerJSON(), 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: writeResult.Changed, Files: []string{path}}, nil
}

// injectMergeIntoSettings merges MCP servers into a config file (OpenCode opencode.json, Gemini settings.json, Claude Desktop claude_desktop_config.json).
func injectMergeIntoSettings(homeDir string, adapter agents.Adapter) (InjectionResult, error) {
	settingsPath := adapter.SettingsPath(homeDir)
	if settingsPath == "" {
		return InjectionResult{}, nil
	}

	overlay := DefaultContext7OverlayJSON()
	if adapter.Agent() == model.AgentOpenCode || adapter.Agent() == model.AgentKilocode {
		return injectOpenCodeMergeIntoSettings(settingsPath)
	}
	if adapter.Agent() == model.AgentOpenClaw {
		return injectOpenClawMergeIntoSettings(settingsPath)
	}
	if adapter.Agent() == model.AgentClaudeDesktop {
		return injectClaudeDesktopMergeIntoSettings(settingsPath)
	}

	settingsWrite, err := mergeJSONFile(settingsPath, overlay)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
}

var gentleAILookPath = exec.LookPath

func isGentleAICommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	base := filepath.Base(cmd)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(base, "gentle-ai.exe") || strings.EqualFold(base, "gentle-ai")
	}
	return base == "gentle-ai"
}

func isStableHomebrewGentleAIPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return (clean == "/opt/homebrew/bin/gentle-ai" || clean == "/usr/local/bin/gentle-ai") && isGentleAICommand(clean)
}

func preferredStableGentleAICommand() string {
	p, err := gentleAILookPath("gentle-ai")
	if err == nil && isStableHomebrewGentleAIPath(p) {
		return p
	}
	return "gentle-ai"
}

func resolveGentleAICommand() string {
	return preferredStableGentleAICommand()
}

func isVersionedHomebrewCellarPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.Contains(clean, "/Cellar/")
}

func stableGentleAICommandForExisting(existing string) string {
	if existing != "" && !isVersionedHomebrewCellarPath(existing) {
		return existing
	}
	return preferredStableGentleAICommand()
}

func injectClaudeDesktopMergeIntoSettings(settingsPath string) (InjectionResult, error) {
	cmd := resolveGentleAICommand()
	baseJSON, err := osReadFile(settingsPath)
	if err != nil {
		return InjectionResult{}, err
	}
	fileExisted := baseJSON != nil
	var backupBytes []byte
	backupPath := settingsPath + ".bak"

	if fileExisted {
		backupBytes = baseJSON
		if existingCmd, ok := existingMergedGentleAICommand(baseJSON); ok {
			cmd = stableGentleAICommandForExisting(existingCmd)
		}
		if err := os.WriteFile(backupPath, backupBytes, 0o600); err != nil {
			return InjectionResult{}, fmt.Errorf("write backup settings %q: %w", backupPath, err)
		}
		_ = os.Chmod(backupPath, 0o600)
	}

	overlay := ClaudeDesktopOverlayJSON(cmd)
	settingsWrite, err := mergeJSONFileMode(settingsPath, overlay, 0o600)
	if err != nil {
		if fileExisted {
			if _, restoreErr := filemerge.WriteFileAtomic(settingsPath, backupBytes, 0o600); restoreErr != nil {
				_ = os.Remove(backupPath)
				return InjectionResult{}, fmt.Errorf("merge json error: %w; restore backup failed: %v", err, restoreErr)
			}
			_ = os.Chmod(settingsPath, 0o600)
			_ = os.Remove(backupPath)
		} else {
			if rmErr := os.Remove(settingsPath); rmErr != nil && !os.IsNotExist(rmErr) {
				return InjectionResult{}, fmt.Errorf("merge json error: %w; remove settings failed: %v", err, rmErr)
			}
		}
		return InjectionResult{}, err
	}

	if fileExisted {
		_ = os.Remove(backupPath)
	}

	if err := os.Chmod(settingsPath, 0o600); err != nil && !os.IsNotExist(err) {
		return InjectionResult{}, fmt.Errorf("chmod settings %q: %w", settingsPath, err)
	}

	return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
}

func existingMergedGentleAICommand(baseJSON []byte) (string, bool) {
	if len(baseJSON) == 0 {
		return "", false
	}
	normalized, err := filemerge.MergeJSONObjects(baseJSON, []byte("{}"))
	if err != nil {
		return "", false
	}
	var root map[string]any
	if err := json.Unmarshal(normalized, &root); err != nil {
		return "", false
	}
	mcpServers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return "", false
	}
	server, ok := mcpServers["gentle-ai"].(map[string]any)
	if !ok {
		return "", false
	}
	return executableFromCommandValue(server["command"])
}

func executableFromCommandValue(command any) (string, bool) {
	switch value := command.(type) {
	case string:
		if value == "" {
			return "", false
		}
		return value, true
	case []any:
		if len(value) == 0 {
			return "", false
		}
		first, ok := value[0].(string)
		if !ok || first == "" {
			return "", false
		}
		return first, true
	default:
		return "", false
	}
}

func injectOpenCodeMergeIntoSettings(settingsPath string) (InjectionResult, error) {
	baseJSON, err := osReadFile(settingsPath)
	if err != nil {
		return InjectionResult{}, err
	}

	overlay := OpenCodeContext7OverlayJSON()
	if settings, parseErr := filemerge.UnmarshalJSONObject(baseJSON); parseErr == nil {
		mcp, _ := settings["mcp"].(map[string]any)
		context7, _ := mcp["context7"].(map[string]any)
		if headers, ok := context7["headers"].(map[string]any); ok {
			validHeaders := make(map[string]string, len(headers))
			for name, value := range headers {
				if header, valid := value.(string); valid {
					validHeaders[name] = header
				}
			}
			replacement := map[string]any{
				"type":    "remote",
				"url":     "https://mcp.context7.com/mcp",
				"enabled": true,
			}
			if len(validHeaders) > 0 {
				replacement["headers"] = validHeaders
			}
			overlay, err = json.Marshal(map[string]any{
				"mcp": map[string]any{
					"context7": map[string]any{"__replace__": replacement},
				},
			})
			if err != nil {
				return InjectionResult{}, fmt.Errorf("marshal opencode context7 overlay: %w", err)
			}
		}
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return InjectionResult{}, err
	}

	settingsWrite, err := filemerge.WriteFileAtomic(settingsPath, merged, 0o644)
	if err != nil {
		return InjectionResult{}, err
	}

	return InjectionResult{Changed: settingsWrite.Changed, Files: []string{settingsPath}}, nil
}

func injectOpenClawMergeIntoSettings(settingsPath string) (InjectionResult, error) {
	baseJSON, err := osReadFile(settingsPath)
	if err != nil {
		return InjectionResult{}, err
	}

	normalized, err := migrateOpenClawLegacyMCPServers(baseJSON)
	if err != nil {
		return InjectionResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(normalized, OpenClawContext7OverlayJSON())
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
	return mergeJSONFileMode(path, overlay, 0o644)
}

func mergeJSONFileMode(path string, overlay []byte, mode os.FileMode) (filemerge.WriteResult, error) {
	baseJSON, err := osReadFile(path)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	merged, err := filemerge.MergeJSONObjects(baseJSON, overlay)
	if err != nil {
		return filemerge.WriteResult{}, err
	}

	return filemerge.WriteFileAtomic(path, merged, mode)
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
