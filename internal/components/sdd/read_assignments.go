package sdd

import (
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

// configurableAgentSet is the set of valid agent names that may appear in
// opencode.json. It includes SDD, Judgment Day, review, and coordinator agents.
var configurableAgentSet = buildConfigurableAgentSet()

func buildConfigurableAgentSet() map[string]bool {
	phases := opencode.ConfigurableAgentPhases()
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
	assignments, _, err := ReadCurrentModelAssignmentsWithPresence(settingsPath)
	return assignments, err
}

// ReadCurrentModelAssignmentsWithPresence reads current model assignments and
// reports whether the effective OpenCode config contains any configurable agent
// entry. The presence flag lets callers distinguish "no current assignment
// config" from "current config explicitly contains configurable agents but no
// valid model assignments".
func ReadCurrentModelAssignmentsWithPresence(settingsPath string) (map[string]model.ModelAssignment, bool, error) {
	effectiveConfig, err := opencode.LoadEffectiveConfig(opencode.ConfigLoadOptions{
		SettingsPath: settingsPath,
		IncludeEnv:   true,
	})
	if len(effectiveConfig.Agent) == 0 {
		if err != nil {
			return nil, false, err
		}
		return map[string]model.ModelAssignment{}, false, nil
	}

	result := make(map[string]model.ModelAssignment)
	hasConfigurableAgentConfig := false
	for name, def := range effectiveConfig.Agent {
		if !configurableAgentSet[name] {
			continue
		}
		if def.ModelSet {
			hasConfigurableAgentConfig = true
		}
		modelStr := def.Model
		if modelStr == "" {
			continue
		}
		providerID, modelID, ok := model.SplitModelSpec(modelStr)
		if !ok {
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
			Effort:     def.Variant,
		}
	}

	return result, hasConfigurableAgentConfig, nil
}
