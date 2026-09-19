package opencode

import "strings"

// ConfigAssignments exposes the bounded file-backed assignment view to readers
// that already enforce their own input size and path boundaries.
func ConfigAssignments(root map[string]any) map[string]AssignmentPresence {
	return configuredAssignments(root)
}

// NativeConfig recognizes unambiguous V2 fields. A historical plural agents
// alias with only V1 fields remains eligible for the existing V1 migration.
func NativeConfig(root map[string]any) bool {
	for _, key := range []string{"providers", "plugins", "commands", "permissions"} {
		if _, ok := root[key]; ok {
			return true
		}
	}
	mcp, _ := root["mcp"].(map[string]any)
	if _, ok := mcp["servers"]; ok {
		return true
	}
	agents, hasAgents := root["agents"].(map[string]any)
	legacyOnly := false
	for _, raw := range agents {
		entry, _ := raw.(map[string]any)
		for _, key := range []string{"prompt", "permission", "variant", "disable", "tools"} {
			if _, exists := entry[key]; exists {
				legacyOnly = true
			}
		}
		for _, key := range []string{"system", "permissions", "disabled", "request"} {
			if _, ok := entry[key]; ok {
				return true
			}
		}
		switch value := entry["model"].(type) {
		case map[string]any:
			return true
		case string:
			if strings.Contains(value, "#") {
				return true
			}
		}
	}
	// With no V1-only evidence, preserving the plural map is safer than migrating it.
	return hasAgents && !legacyOnly
}

// MCPEntry chooses a native server over its legacy counterpart without allowing
// a disabled native entry to resurrect the stale legacy server.
func MCPEntry(root map[string]any, name string) (map[string]any, bool) {
	mcp, _ := root["mcp"].(map[string]any)
	servers, _ := mcp["servers"].(map[string]any)
	if native, ok := servers[name].(map[string]any); ok {
		return native, true
	}
	legacy, _ := mcp[name].(map[string]any)
	return legacy, false
}
