package claude

import "path/filepath"

func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude")
}

// MCPRegistryPath returns ~/.claude.json, the only user-scope location Claude
// Code loads MCP servers from. An mcpServers key written to
// ~/.claude/settings.json is not a recognised setting and is ignored, and
// ~/.claude/mcp/*.json is never read at all.
func MCPRegistryPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude.json")
}
