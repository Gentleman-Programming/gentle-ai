package planner

import (
	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func BuildReviewPayload(selection model.Selection, resolved ResolvedPlan) ReviewPayload {
	autoAdded := make(map[model.ComponentID]struct{}, len(resolved.AddedDependencies))
	for _, component := range resolved.AddedDependencies {
		autoAdded[component] = struct{}{}
	}

	components := make([]ComponentAction, 0, len(resolved.OrderedComponents))
	hasSDD := false
	for _, component := range resolved.OrderedComponents {
		action := "selected"
		if _, ok := autoAdded[component]; ok {
			action = "auto-dependency"
		}
		if component == model.ComponentSDD {
			hasSDD = true
		}
		components = append(components, ComponentAction{ID: component, Action: action})
	}

	agentNotes := buildAgentNotes(resolved.Agents)

	return ReviewPayload{
		Agents:            resolved.Agents,
		UnsupportedAgents: resolved.UnsupportedAgents,
		Persona:           selection.Persona,
		Preset:            selection.Preset,
		Components:        components,
		AddedDependencies: resolved.AddedDependencies,
		PlatformDecision:  resolved.PlatformDecision,
		// Issue #145: pass skills from selection.
		Skills: selection.Skills,
		// Issue #149: pass StrictTDD and whether SDD is in plan.
		StrictTDD:  selection.StrictTDD,
		HasSDD:     hasSDD,
		AgentNotes: agentNotes,
	}
}

// buildAgentNotes collects catalog notes for the selected agents.
// Only agents that carry a Note in the catalog are included.
func buildAgentNotes(agents []model.AgentID) map[model.AgentID]string {
	notesByID := map[model.AgentID]string{}
	for _, a := range catalog.AllAgents() {
		if a.Note != "" {
			notesByID[a.ID] = a.Note
		}
	}

	result := map[model.AgentID]string{}
	for _, id := range agents {
		if note, ok := notesByID[id]; ok {
			result[id] = note
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
