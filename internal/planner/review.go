package planner

import (
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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

	agentNotes := agentNotesFor(resolved.Agents)

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
		StrictTDD: selection.StrictTDD,
		HasSDD:    hasSDD,
		// Catalog-owned notes for catalog-only agents (Conductor) only.
		AgentNotes: agentNotes,
	}
}

// agentNotesFor collects the catalog review notes of the selected agents, in
// selection order. Writable agents carry no note, so a selection without a
// catalog-only agent yields an empty slice.
func agentNotesFor(agents []model.AgentID) []AgentNote {
	var notes []AgentNote
	for _, agent := range agents {
		for _, candidate := range catalog.AllAgents() {
			if candidate.ID != agent || candidate.ReviewNote == "" {
				continue
			}
			notes = append(notes, AgentNote{Agent: agent, Note: candidate.ReviewNote})
			break
		}
	}
	return notes
}
