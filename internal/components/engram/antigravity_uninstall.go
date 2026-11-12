package engram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// AntigravityPluginDir is the plugin-owned Engram registration directory for
// one Antigravity config home. agy 1.4 loads plugins only from the shared
// ~/.gemini/config root (#5930), so it never depends on the desktop/CLI variant.
func AntigravityPluginDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "config", "plugins", "gentle-ai-engram")
}

// AntigravityLegacyPluginDir is where gentle-ai wrote the Engram plugin before
// agy 1.4 (#5930).
func AntigravityLegacyPluginDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram")
}

// AntigravityPluginAssetPaths returns the Engram plugin assets install writes
// for one Antigravity config home, in install order.
func AntigravityPluginAssetPaths(homeDir string) []string {
	return antigravityPluginAssetPathsIn(AntigravityPluginDir(homeDir))
}

// AntigravityLegacyPluginAssetPaths returns the Engram plugin assets older
// gentle-ai versions wrote under AntigravityLegacyPluginDir. Install migrates
// them away and uninstall removes any that remain.
func AntigravityLegacyPluginAssetPaths(homeDir string) []string {
	return antigravityPluginAssetPathsIn(AntigravityLegacyPluginDir(homeDir))
}

func antigravityPluginAssetPathsIn(dir string) []string {
	return []string{
		filepath.Join(dir, "plugin.json"),
		filepath.Join(dir, "mcp_config.json"),
		filepath.Join(dir, "hooks.json"),
	}
}

// RetireManagedAntigravityPlugins retires any managed plugin assets older gentle-ai
// versions wrote under either AntigravityPluginDir or AntigravityLegacyPluginDir (#5988).
// If the plugin mcp_config.json contains custom env, cwd, or args settings, they are migrated
// to the global MCP config before retiring the manifest. If a conflict prevents safe migration,
// retirement stops and reports the conflict so the edited MCP file is not left inactive.
func RetireManagedAntigravityPlugins(homeDir string) ([]string, error) {
	var removed []string
	for _, dir := range []string{AntigravityPluginDir(homeDir), AntigravityLegacyPluginDir(homeDir)} {
		migratedFiles, err := migrateOrReconcilePluginMCP(homeDir, dir)
		if err != nil {
			return removed, err
		}
		for _, f := range migratedFiles {
			if !slices.Contains(removed, f) {
				removed = append(removed, f)
			}
		}
		for _, path := range antigravityPluginAssetPathsIn(dir) {
			ok, err := RemoveManagedAntigravityPluginAsset(path)
			if err != nil {
				return removed, err
			}
			if ok && !slices.Contains(removed, path) {
				removed = append(removed, path)
			}
		}
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
			if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
				return removed, fmt.Errorf("remove empty Antigravity Engram plugin dir %q: %w", dir, err)
			}
		}
	}
	return removed, nil
}

// removeLegacyAntigravityPlugin retires the managed plugin assets an older
// gentle-ai wrote under AntigravityLegacyPluginDir (#5930).
func removeLegacyAntigravityPlugin(homeDir string) ([]string, error) {
	return RetireManagedAntigravityPlugins(homeDir)
}

// WriteManagedAntigravityPluginForTest writes byte-identical managed plugin assets into dir.
func WriteManagedAntigravityPluginForTest(dir, command string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	assets := map[string][]byte{
		"plugin.json":     []byte(antigravityEngramPluginJSON),
		"mcp_config.json": engramOverlayJSON(model.AgentAntigravity, command),
		"hooks.json":      antigravityEngramHooksJSON(),
	}
	for name, content := range assets {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RemoveManagedAntigravityPluginAsset removes path only when it is a regular
// file byte-identical to the asset install writes there (#779). The MCP
// config embeds the resolved engram command, so it is managed when re-rendering
// the overlay with the file's own command reproduces it exactly. Anything
// else -- an edited asset, a symlink, an unknown file -- is left untouched.
// Before retiring plugin.json, any custom MCP env, cwd, or args settings in the same directory
// are migrated to the global MCP config, or retirement stops if a conflict exists.
func RemoveManagedAntigravityPluginAsset(path string) (bool, error) {
	return removeManagedAntigravityPluginAssetInternal(path, true)
}

// RemoveManagedAntigravityPluginAssetForUninstall removes path during uninstall only
// when it is byte-identical to the asset install writes there (#779). Unlike
// RemoveManagedAntigravityPluginAsset, it performs removal only and never migrates
// settings to the global MCP config, ensuring uninstall does not re-register Engram.
func RemoveManagedAntigravityPluginAssetForUninstall(path string) (bool, error) {
	return removeManagedAntigravityPluginAssetInternal(path, false)
}

func removeManagedAntigravityPluginAssetInternal(path string, migrateIfPluginJSON bool) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect Antigravity Engram plugin asset %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read Antigravity Engram plugin asset %q: %w", path, err)
	}
	if filepath.Base(path) == "plugin.json" {
		dir := filepath.Dir(path)
		mcpPath := filepath.Join(dir, "mcp_config.json")
		if mcpInfo, mcpErr := os.Lstat(mcpPath); mcpErr == nil && mcpInfo.Mode().IsRegular() {
			mcpContent, readErr := os.ReadFile(mcpPath)
			if readErr != nil {
				return false, fmt.Errorf("read sibling plugin MCP config %q: %w", mcpPath, readErr)
			}
			if !isManagedAntigravityPluginAsset("mcp_config.json", mcpContent) {
				if migrateIfPluginJSON {
					homeDir := resolveHomeDirFromPluginDir(dir)
					if _, err := migrateOrReconcilePluginMCP(homeDir, dir); err != nil {
						return false, err
					}
				}
			}
		} else if mcpErr != nil && !os.IsNotExist(mcpErr) {
			return false, fmt.Errorf("inspect sibling plugin MCP config %q: %w", mcpPath, mcpErr)
		}
	}
	if !isManagedAntigravityPluginAsset(filepath.Base(path), content) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, fmt.Errorf("remove Antigravity Engram plugin asset %q: %w", path, err)
	}
	return true, nil
}

func isManagedAntigravityPluginAsset(name string, content []byte) bool {
	switch name {
	case "plugin.json":
		return bytes.Equal(content, []byte(antigravityEngramPluginJSON))
	case "hooks.json":
		if bytes.Equal(content, antigravityEngramHooksJSON()) {
			return true
		}
		for _, legacy := range legacyAntigravityEngramHooksJSON() {
			if bytes.Equal(content, legacy) {
				return true
			}
		}
		return false
	case "mcp_config.json":
		var root struct {
			MCPServers map[string]struct {
				Command string `json:"command"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(content, &root); err != nil {
			return false
		}
		server, ok := root.MCPServers["engram"]
		if !ok || server.Command == "" {
			return false
		}
		return bytes.Equal(content, engramOverlayJSON(model.AgentAntigravity, server.Command))
	default:
		return false
	}
}

// resolveAntigravityGlobalMCPPath returns the global mcp_config.json path for homeDir.
// It checks ~/.gemini/config/mcp_config.json first, and falls back to homeDir/mcp_config.json
// when homeDir is already ~/.gemini/config.
func resolveAntigravityGlobalMCPPath(homeDir string) string {
	candidate := filepath.Join(homeDir, ".gemini", "config", "mcp_config.json")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if filepath.Base(homeDir) == "config" && filepath.Base(filepath.Dir(homeDir)) == ".gemini" {
		return filepath.Join(homeDir, "mcp_config.json")
	}
	return candidate
}

// resolveHomeDirFromPluginDir extracts the home directory from an Antigravity plugin directory path.
func resolveHomeDirFromPluginDir(pluginDir string) string {
	d := filepath.Clean(pluginDir)
	for i := 0; i < 4; i++ {
		d = filepath.Dir(d)
	}
	return d
}

// migrateOrReconcilePluginMCP preserves customized MCP env, cwd, and command settings
// from a plugin's mcp_config.json by migrating them to the global MCP config before retiring the manifest.
// Canonical args ["mcp", "--tools=agent"] are enforced consistently across migration and Inject.
// If conflicting settings exist in the global config or the file cannot be safely migrated,
// it returns an error to stop retirement and prevent leaving the edited MCP file inactive.
// On success, it returns the list of modified and removed paths.
func migrateOrReconcilePluginMCP(homeDir, dir string) ([]string, error) {
	pluginMCP := filepath.Join(dir, "mcp_config.json")
	info, err := os.Lstat(pluginMCP)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugin MCP config %q: %w", pluginMCP, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: mcp_config.json is a symlink; plugin manifest preserved", dir)
	}
	content, err := os.ReadFile(pluginMCP)
	if err != nil {
		return nil, fmt.Errorf("read plugin MCP config %q: %w", pluginMCP, err)
	}
	if isManagedAntigravityPluginAsset("mcp_config.json", content) {
		return nil, nil
	}

	var pluginRoot map[string]any
	if err := json.Unmarshal(content, &pluginRoot); err != nil {
		return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: mcp_config.json is not valid JSON", dir)
	}
	pluginServers, _ := pluginRoot["mcpServers"].(map[string]any)
	if len(pluginServers) == 0 {
		return nil, nil
	}
	if len(pluginServers) > 1 {
		return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: mcp_config.json contains other MCP servers; plugin manifest preserved", dir)
	}
	pluginEngram, ok := pluginServers["engram"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: mcp_config.json has non-Engram servers; plugin manifest preserved", dir)
	}

	for k := range pluginEngram {
		switch k {
		case "command", "args", "env", "cwd":
		default:
			return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: mcp_config.json has unsupported setting %q; plugin manifest preserved", dir, k)
		}
	}

	var customEnv map[string]any
	if e, ok := pluginEngram["env"].(map[string]any); ok && len(e) > 0 {
		customEnv = e
	}
	var customCwd string
	if c, ok := pluginEngram["cwd"].(string); ok && c != "" {
		customCwd = c
	}
	var customCmd string
	if c, ok := pluginEngram["command"].(string); ok && c != "" {
		customCmd = c
	}

	if customEnv == nil && customCwd == "" && (customCmd == "" || customCmd == "engram") {
		return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: mcp_config.json has customized settings; plugin manifest preserved", dir)
	}

	globalPath := resolveAntigravityGlobalMCPPath(homeDir)
	globalRaw, err := os.ReadFile(globalPath)
	var globalRoot map[string]any
	if err == nil && len(globalRaw) > 0 {
		if err := json.Unmarshal(globalRaw, &globalRoot); err != nil {
			return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: global MCP config %q is not valid JSON", dir, globalPath)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read global MCP config %q: %w", globalPath, err)
	} else {
		globalRoot = map[string]any{}
	}

	globalServers, _ := globalRoot["mcpServers"].(map[string]any)
	if globalServers == nil {
		globalServers = map[string]any{}
		globalRoot["mcpServers"] = globalServers
	}
	globalEngram, _ := globalServers["engram"].(map[string]any)
	if globalEngram == nil {
		globalEngram = map[string]any{
			"command": "engram",
			"args":    []any{"mcp", "--tools=agent"},
		}
		if customCmd != "" && engramCommandResolves(customCmd) {
			globalEngram["command"] = customCmd
		}
		globalServers["engram"] = globalEngram
	}
	globalEngram["args"] = []any{"mcp", "--tools=agent"}

	// Check for env conflict without leaking env values in error messages (#5988).
	if customEnv != nil {
		if globalEnv, ok := globalEngram["env"].(map[string]any); ok && len(globalEnv) > 0 {
			for k, v := range customEnv {
				if gv, exists := globalEnv[k]; exists && gv != v {
					return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: plugin env key %q conflicts with global config; plugin manifest preserved", dir, k)
				}
			}
			for k, v := range customEnv {
				globalEnv[k] = v
			}
		} else {
			globalEngram["env"] = customEnv
		}
	}

	// Check for cwd conflict.
	if customCwd != "" {
		if globalCwd, ok := globalEngram["cwd"].(string); ok && globalCwd != "" {
			if globalCwd != customCwd {
				return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: plugin cwd %q conflicts with global cwd %q; plugin manifest preserved", dir, customCwd, globalCwd)
			}
		} else {
			globalEngram["cwd"] = customCwd
		}
	}

	// Check for command conflict (do not treat default "engram" as conflict against global path).
	if customCmd != "" && engramCommandResolves(customCmd) {
		if globalCmd, ok := globalEngram["command"].(string); customCmd != "engram" && ok && globalCmd != "" && globalCmd != "engram" && globalCmd != customCmd {
			return nil, fmt.Errorf("conflict retiring Antigravity plugin in %q: plugin command %q conflicts with global command %q; plugin manifest preserved", dir, customCmd, globalCmd)
		}
		if globalCmd, ok := globalEngram["command"].(string); !ok || globalCmd == "" || globalCmd == "engram" {
			globalEngram["command"] = customCmd
		}
	}

	updatedJSON, err := json.MarshalIndent(globalRoot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal updated global MCP config: %w", err)
	}
	updatedJSON = append(updatedJSON, '\n')
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		return nil, fmt.Errorf("create global config dir: %w", err)
	}

	targetMode := filemerge.ExistingFileMode(globalPath, 0o644)
	if customEnv != nil {
		targetMode = 0o600
		if info, statErr := os.Stat(globalPath); statErr == nil {
			if info.Mode().Perm() != 0o600 {
				if err := os.Chmod(globalPath, 0o600); err != nil {
					return nil, fmt.Errorf("restrict global MCP config permissions before migrating secrets: %w", err)
				}
			}
		} else if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("inspect global MCP config %q: %w", globalPath, statErr)
		}
	}

	if _, err := filemerge.WriteFileAtomicMode(globalPath, updatedJSON, targetMode); err != nil {
		return nil, fmt.Errorf("write migrated global MCP config %q: %w", globalPath, err)
	}

	if err := os.Remove(pluginMCP); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove migrated plugin MCP config %q: %w", pluginMCP, err)
	}
	return []string{globalPath, pluginMCP}, nil
}
