package sdd

import (
	"encoding/json"
	"fmt"
)

var openCodeSDDPhaseEngramTools = []string{
	"engram_mem_save",
	"engram_mem_search",
	"engram_mem_get_observation",
	"engram_mem_update",
}

func grantOpenCodeSDDPhaseEngramTools(tools map[string]any) {
	for _, tool := range openCodeSDDPhaseEngramTools {
		tools[tool] = true
	}
}

func projectOpenCodeSDDPhaseTools(overlay []byte, suffix string, enabled bool) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(overlay, &root); err != nil {
		return nil, fmt.Errorf("decode OpenCode SDD overlay: %w", err)
	}
	agents, ok := root["agent"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode OpenCode SDD overlay: agent is not an object")
	}
	for _, phase := range profilePhaseOrder {
		key := phase + suffix
		agent, ok := agents[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("decode OpenCode SDD overlay: agent %q is not an object", key)
		}
		tools, ok := agent["tools"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("decode OpenCode SDD overlay: agent %q tools is not an object", key)
		}
		if enabled {
			grantOpenCodeSDDPhaseEngramTools(tools)
		} else {
			for _, tool := range openCodeSDDPhaseEngramTools {
				delete(tools, tool)
			}
		}
	}
	result, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenCode SDD overlay: %w", err)
	}
	return append(result, '\n'), nil
}
