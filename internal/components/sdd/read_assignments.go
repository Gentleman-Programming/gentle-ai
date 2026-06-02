package sdd

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

// configurableAgentSet is the set of valid agent names that may appear in
// opencode.json. It includes SDD phases, JD agents, and the gentle-orchestrator coordinator.
var configurableAgentSet = buildConfigurableAgentSet()

func buildConfigurableAgentSet() map[string]bool {
	phases := opencode.ConfigurableAgentPhases() // SDD + JD phases
	set := make(map[string]bool, len(phases)+1)
	for _, p := range phases {
		set[p] = true
	}
	set["gentle-orchestrator"] = true
	// Backward-compatible read alias for configs that have not been synced yet.
	set["sdd-orchestrator"] = true
	return set
}

// ReadCurrentProfiles reads the named SDD profiles from opencode.json at
// settingsPath. It is a thin wrapper around DetectProfiles provided so that
// sync code can import a single symbol from this file.
func ReadCurrentProfiles(settingsPath string) ([]model.Profile, error) {
	return DetectProfiles(settingsPath)
}

// ReadCurrentModelAssignments reads the agent definitions from opencode.json
// at settingsPath and extracts the "model" field for each configurable agent.
//
// Only agents whose names match a configurable agent phase (SDD phases, JD agents
// via opencode.ConfigurableAgentPhases()) or "gentle-orchestrator" are included.
// Agents without a "model" field, or with a malformed model value, are silently
// skipped.
//
// Returns an empty map (no error) when the file does not exist, contains no
// "agent" key, or has no matching phase agents with a valid model field.
func ReadCurrentModelAssignments(settingsPath string) (map[string]model.ModelAssignment, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]model.ModelAssignment{}, nil
		}
		return nil, err
	}

	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		// Unparseable JSON — return empty map, no error.
		return map[string]model.ModelAssignment{}, nil
	}

	agentRaw, ok := root["agent"]
	if !ok {
		return map[string]model.ModelAssignment{}, nil
	}
	agentMap, ok := agentRaw.(map[string]any)
	if !ok {
		return map[string]model.ModelAssignment{}, nil
	}

	result := make(map[string]model.ModelAssignment)
	for name, defRaw := range agentMap {
		if !configurableAgentSet[name] {
			continue
		}
		defMap, ok := defRaw.(map[string]any)
		if !ok {
			continue
		}
		modelStr, ok := defMap["model"].(string)
		if !ok || modelStr == "" {
			continue
		}
		// Try colon first (standard: "anthropic:claude-sonnet-4"), then slash
		// ("zai-coding-plan/glm-5-turbo") for custom providers (issue #152).
		idx := strings.Index(modelStr, ":")
		if idx <= 0 {
			idx = strings.Index(modelStr, "/")
		}
		if idx <= 0 {
			// No separator or separator is the first character — skip malformed value.
			continue
		}
		providerID := modelStr[:idx]
		modelID := modelStr[idx+1:]
		if modelID == "" {
			continue
		}
		assignmentKey := name
		if name == "sdd-orchestrator" {
			assignmentKey = "gentle-orchestrator"
			if _, hasGentleOrchestrator := result[assignmentKey]; hasGentleOrchestrator {
				continue
			}
		}
		effort, _ := defMap["variant"].(string)
		result[assignmentKey] = model.ModelAssignment{
			ProviderID: providerID,
			ModelID:    modelID,
			Effort:     effort,
		}
	}

	return result, nil
}
