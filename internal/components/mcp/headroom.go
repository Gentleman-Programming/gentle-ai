package mcp

var defaultHeadroomServerJSON = []byte("{\n  \"command\": \"headroom\",\n  \"args\": [\n    \"mcp\",\n    \"serve\"\n  ]\n}\n")

var defaultHeadroomOverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"headroom\": {\n      \"command\": \"headroom\",\n      \"args\": [\n        \"mcp\",\n        \"serve\"\n      ]\n    }\n  }\n}\n")

// openCodeHeadroomOverlayJSON is the opencode.json overlay using the new MCP format.
// Headroom is a local stdio MCP server.
// The headroom entry must replace atomically so legacy local keys do not survive
// deep merge into OpenCode/KiloCode's strict MCP schema.
var openCodeHeadroomOverlayJSON = []byte("{\n  \"mcp\": {\n    \"headroom\": {\n      \"__replace__\": {\n        \"type\": \"local\",\n        \"command\": \"headroom\",\n        \"args\": [\n          \"mcp\",\n          \"serve\"\n        ],\n        \"enabled\": true\n      }\n    }\n  }\n}\n")

// openClawHeadroomOverlayJSON is the OpenClaw openclaw.json overlay.
var openClawHeadroomOverlayJSON = []byte("{\n  \"mcp\": {\n    \"servers\": {\n      \"headroom\": {\n        \"command\": \"headroom\",\n        \"args\": [\n          \"mcp\",\n          \"serve\"\n        ]\n      }\n    }\n  }\n}\n")

// vsCodeHeadroomOverlayJSON is the VS Code mcp.json overlay using the "servers" key.
var vsCodeHeadroomOverlayJSON = []byte("{\n  \"servers\": {\n    \"headroom\": {\n      \"command\": \"headroom\",\n      \"args\": [\n        \"mcp\",\n        \"serve\"\n      ]\n    }\n  }\n}\n")

// antigravityHeadroomOverlayJSON is the Antigravity mcp_config.json overlay.
// Uses mcpServers key with command/args for local stdio.
var antigravityHeadroomOverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"headroom\": {\n      \"__replace__\": {\n        \"command\": \"headroom\",\n        \"args\": [\n          \"mcp\",\n          \"serve\"\n        ]\n      }\n    }\n  }\n}\n")

// kimiHeadroomOverlayJSON follows Kimi's documented mcp.json format for local servers.
var kimiHeadroomOverlayJSON = []byte("{\n  \"mcpServers\": {\n    \"headroom\": {\n      \"__replace__\": {\n        \"command\": \"headroom\",\n        \"args\": [\n          \"mcp\",\n          \"serve\"\n        ]\n      }\n    }\n  }\n}\n")

func DefaultHeadroomServerJSON() []byte {
	content := make([]byte, len(defaultHeadroomServerJSON))
	copy(content, defaultHeadroomServerJSON)
	return content
}

func DefaultHeadroomOverlayJSON() []byte {
	content := make([]byte, len(defaultHeadroomOverlayJSON))
	copy(content, defaultHeadroomOverlayJSON)
	return content
}

func OpenCodeHeadroomOverlayJSON() []byte {
	content := make([]byte, len(openCodeHeadroomOverlayJSON))
	copy(content, openCodeHeadroomOverlayJSON)
	return content
}

func OpenClawHeadroomOverlayJSON() []byte {
	content := make([]byte, len(openClawHeadroomOverlayJSON))
	copy(content, openClawHeadroomOverlayJSON)
	return content
}

func VSCodeHeadroomOverlayJSON() []byte {
	content := make([]byte, len(vsCodeHeadroomOverlayJSON))
	copy(content, vsCodeHeadroomOverlayJSON)
	return content
}

func AntigravityHeadroomOverlayJSON() []byte {
	content := make([]byte, len(antigravityHeadroomOverlayJSON))
	copy(content, antigravityHeadroomOverlayJSON)
	return content
}

func KimiHeadroomOverlayJSON() []byte {
	content := make([]byte, len(kimiHeadroomOverlayJSON))
	copy(content, kimiHeadroomOverlayJSON)
	return content
}
