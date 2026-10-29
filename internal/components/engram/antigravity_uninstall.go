package engram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// AntigravityPluginDir is the plugin-owned Engram registration directory for
// one Antigravity config home. It is shared by the desktop and CLI variants on
// purpose (see antigravityLockTarget), so it never depends on the variant.
func AntigravityPluginDir(homeDir string) string {
	return filepath.Join(homeDir, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram")
}

// AntigravityPluginAssetPaths returns the Engram plugin assets install writes
// for one Antigravity config home, in install order.
func AntigravityPluginAssetPaths(homeDir string) []string {
	dir := AntigravityPluginDir(homeDir)
	return []string{
		filepath.Join(dir, "plugin.json"),
		filepath.Join(dir, "mcp_config.json"),
		filepath.Join(dir, "hooks.json"),
	}
}

// RemoveManagedAntigravityPluginAsset removes path only when it is a regular
// file byte-identical to the asset install writes there (#779). The MCP
// config embeds the resolved engram command, so it is managed when re-rendering
// the overlay with the file's own command reproduces it exactly. Anything
// else -- an edited asset, a symlink, an unknown file -- is left untouched.
func RemoveManagedAntigravityPluginAsset(path string) (bool, error) {
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
		return bytes.Equal(content, antigravityEngramHooksJSON())
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
