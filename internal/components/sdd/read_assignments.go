package sdd

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

// sddPhaseSet is the set of valid base SDD agent names that may appear in
// opencode.json. It includes the sub-agent phases plus the gentle-orchestrator coordinator.
var sddPhaseSet = buildSDDPhaseSet()

func buildSDDPhaseSet() map[string]bool {
	phases := opencode.SDDPhases()
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
// at settingsPath and extracts the "model" field for each SDD phase agent.
//
// Only agents whose names match an SDD phase (from opencode.SDDPhases()) or
// "gentle-orchestrator" are included. Legacy "sdd-orchestrator" entries are read as
// "gentle-orchestrator" until the next sync migrates the config. Agents without a "model" field, or with a
// malformed model value (not in "provider:model-id" format), are silently
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
		if !sddPhaseSet[name] {
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
		result[assignmentKey] = model.ModelAssignment{
			ProviderID: providerID,
			ModelID:    modelID,
		}
	}

	return result, nil
}
