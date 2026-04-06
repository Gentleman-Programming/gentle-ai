package sdd

import (
	"encoding/json"
	"os"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

// sddPhaseSet is the set of valid SDD phase agent names that may appear in
// opencode.json. It includes the sub-agent phases plus the orchestrator.
var sddPhaseSet = buildSDDPhaseSet()

func buildSDDPhaseSet() map[string]bool {
	phases := opencode.SDDPhases()
	set := make(map[string]bool, len(phases)+1)
	for _, p := range phases {
		set[p] = true
	}
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
// "sdd-orchestrator" are included. Agents without a "model" field, or with a
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
		// Find the FIRST occurrence of either '/' or ':' (whichever comes first).
		// This correctly handles OpenRouter free models like "openrouter/qwen/qwen3.6-plus:free"
		// where the provider is "openrouter" and the model is "qwen/qwen3.6-plus:free".
		idx := -1
		for i, c := range modelStr {
			if c == '/' || c == ':' {
				idx = i
				break
			}
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
		result[name] = model.ModelAssignment{
			ProviderID: providerID,
			ModelID:    modelID,
		}
	}

	return result, nil
}
