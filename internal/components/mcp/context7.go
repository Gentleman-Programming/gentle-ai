package mcp

var defaultContext7ServerJSON = []byte("{\n  \"command\": \"npx\",\n  \"args\": [\n    \"-y\",\n    \"@upstash/context7-mcp\"\n  ]\n}\n")

var defaultContext7OverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"command\": \"npx\",\n      \"args\": [\n        \"-y\",\n        \"@upstash/context7-mcp\"\n      ]\n    }\n  }\n}\n")

// openCodeContext7OverlayJSON is the opencode.json overlay using the new MCP format.
// Context7 is a remote MCP server — no npx needed.
var openCodeContext7OverlayJSON = []byte("{\n  \"mcp\": {\n    \"context7\": {\n      \"type\": \"remote\",\n      \"url\": \"https://mcp.context7.com/mcp\",\n      \"enabled\": true\n    }\n  }\n}\n")

// vsCodeContext7OverlayJSON is the VS Code mcp.json overlay using the "servers" key.
var vsCodeContext7OverlayJSON = []byte("{\n  \"servers\": {\n    \"context7\": {\n      \"type\": \"http\",\n      \"url\": \"https://mcp.context7.com/mcp\"\n    }\n  }\n}\n")

// antigravityContext7OverlayJSON is the Antigravity mcp_config.json overlay.
// Uses mcpServers key (same schema as Claude Code) with serverUrl for HTTP remote.
var antigravityContext7OverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"serverUrl\": \"https://mcp.context7.com/mcp\"\n    }\n  }\n}\n")

// kimiContext7OverlayJSON follows Kimi's documented mcp.json "well-known MCP
// config format", using mcpServers + explicit http transport for Context7.
var kimiContext7OverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"transport\": \"http\",\n      \"url\": \"https://mcp.context7.com/mcp\"\n    }\n  }\n}\n")

func DefaultContext7ServerJSON() []byte {
	content := make([]byte, len(defaultContext7ServerJSON))
	copy(content, defaultContext7ServerJSON)
	return content
}

func DefaultContext7OverlayJSON() []byte {
	content := make([]byte, len(defaultContext7OverlayJSON))
	copy(content, defaultContext7OverlayJSON)
	return content
}

func OpenCodeContext7OverlayJSON() []byte {
	content := make([]byte, len(openCodeContext7OverlayJSON))
	copy(content, openCodeContext7OverlayJSON)
	return content
}

func VSCodeContext7OverlayJSON() []byte {
	content := make([]byte, len(vsCodeContext7OverlayJSON))
	copy(content, vsCodeContext7OverlayJSON)
	return content
}

func AntigravityContext7OverlayJSON() []byte {
	content := make([]byte, len(antigravityContext7OverlayJSON))
	copy(content, antigravityContext7OverlayJSON)
	return content
}

func KimiContext7OverlayJSON() []byte {
	content := make([]byte, len(kimiContext7OverlayJSON))
	copy(content, kimiContext7OverlayJSON)
	return content
}
