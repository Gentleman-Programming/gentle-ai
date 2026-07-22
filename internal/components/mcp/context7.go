package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

var defaultContext7ServerJSON = []byte(fmt.Sprintf("{\n  \"command\": \"npx\",\n  \"args\": [\n    \"-y\",\n    \"--package=@upstash/context7-mcp@%s\",\n    \"--\",\n    \"context7-mcp\"\n  ]\n}\n", versions.Context7MCP))

var defaultContext7OverlayJSON = []byte(fmt.Sprintf("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"command\": \"npx\",\n      \"args\": [\n        \"-y\",\n        \"--package=@upstash/context7-mcp@%s\",\n        \"--\",\n        \"context7-mcp\"\n      ]\n    }\n  }\n}\n", versions.Context7MCP))

// openCodeContext7OverlayJSON is the opencode.json overlay using the new MCP format.
// Context7 is a remote MCP server — no npx needed.
// The context7 entry must replace atomically so legacy local keys do not survive
// deep merge into OpenCode/KiloCode's strict MCP schema.
var openCodeContext7OverlayJSON = []byte("{\n  \"mcp\": {\n    \"context7\": {\n      \"__replace__\": {\n        \"type\": \"remote\",\n        \"url\": \"https://mcp.context7.com/mcp\",\n        \"enabled\": true\n      }\n    }\n  }\n}\n")

// openClawContext7OverlayJSON is the OpenClaw openclaw.json overlay.
// OpenClaw rejects top-level mcpServers and expects MCP entries under
// mcp.servers.
var openClawContext7OverlayJSON = []byte(fmt.Sprintf("{\n  \"mcp\": {\n    \"servers\": {\n      \"context7\": {\n        \"command\": \"npx\",\n        \"args\": [\n          \"-y\",\n          \"--package=@upstash/context7-mcp@%s\",\n          \"--\",\n          \"context7-mcp\"\n        ]\n      }\n    }\n  }\n}\n", versions.Context7MCP))

// vsCodeContext7OverlayJSON is the VS Code mcp.json overlay using the "servers" key.
var vsCodeContext7OverlayJSON = []byte("{\n  \"servers\": {\n    \"context7\": {\n      \"type\": \"http\",\n      \"url\": \"https://mcp.context7.com/mcp\"\n    }\n  }\n}\n")

// antigravityContext7OverlayJSON is the Antigravity mcp_config.json overlay.
// Uses mcpServers key (same schema as Claude Code) with serverUrl for HTTP remote.
// The context7 entry must replace atomically so legacy local keys do not survive
// deep merge into this managed MCP server entry.
var antigravityContext7OverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"__replace__\": {\n        \"serverUrl\": \"https://mcp.context7.com/mcp\"\n      }\n    }\n  }\n}\n")

// kimiContext7OverlayJSON follows Kimi's documented mcp.json "well-known MCP
// config format", using mcpServers + explicit http transport for Context7.
// The context7 entry must replace atomically so legacy local keys do not survive
// deep merge into this managed MCP server entry.
var kimiContext7OverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"__replace__\": {\n        \"transport\": \"http\",\n        \"url\": \"https://mcp.context7.com/mcp\"\n      }\n    }\n  }\n}\n")

// DefaultContext7ServerJSON returns the default standalone Context7 server JSON configuration.
func DefaultContext7ServerJSON() []byte {
	content := make([]byte, len(defaultContext7ServerJSON))
	copy(content, defaultContext7ServerJSON)
	return content
}

// DefaultContext7OverlayJSON returns the default mcpServers overlay JSON containing Context7.
func DefaultContext7OverlayJSON() []byte {
	content := make([]byte, len(defaultContext7OverlayJSON))
	copy(content, defaultContext7OverlayJSON)
	return content
}

// OpenCodeContext7OverlayJSON returns the opencode.json overlay JSON for Context7.
func OpenCodeContext7OverlayJSON() []byte {
	content := make([]byte, len(openCodeContext7OverlayJSON))
	copy(content, openCodeContext7OverlayJSON)
	return content
}

// OpenClawContext7OverlayJSON returns the openclaw.json overlay JSON for Context7.
func OpenClawContext7OverlayJSON() []byte {
	content := make([]byte, len(openClawContext7OverlayJSON))
	copy(content, openClawContext7OverlayJSON)
	return content
}

// VSCodeContext7OverlayJSON returns the VS Code mcp.json overlay JSON for Context7.
func VSCodeContext7OverlayJSON() []byte {
	content := make([]byte, len(vsCodeContext7OverlayJSON))
	copy(content, vsCodeContext7OverlayJSON)
	return content
}

// AntigravityContext7OverlayJSON returns the Antigravity mcp_config.json overlay JSON for Context7.
func AntigravityContext7OverlayJSON() []byte {
	content := make([]byte, len(antigravityContext7OverlayJSON))
	copy(content, antigravityContext7OverlayJSON)
	return content
}

// KimiContext7OverlayJSON returns the Kimi mcp.json overlay JSON for Context7.
func KimiContext7OverlayJSON() []byte {
	content := make([]byte, len(kimiContext7OverlayJSON))
	copy(content, kimiContext7OverlayJSON)
	return content
}

// ClaudeDesktopOverlayJSON returns the overlay JSON for claude_desktop_config.json,
// containing context7 and gentle-ai MCP server configurations.
func ClaudeDesktopOverlayJSON(gentleAICmd string) []byte {
	if gentleAICmd == "" {
		gentleAICmd = "gentle-ai"
	}
	cfg := map[string]any{
		"mcpServers": map[string]any{
			"context7": map[string]any{
				"command": "npx",
				"args": []string{
					"-y",
					"--package=@upstash/context7-mcp@" + versions.Context7MCP,
					"--",
					"context7-mcp",
				},
			},
			"gentle-ai": map[string]any{
				"command": gentleAICmd,
				"args": []string{
					"mcp",
				},
			},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return append(b, '\n')
}
