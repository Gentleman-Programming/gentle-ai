package state

import "github.com/gentleman-programming/gentle-ai/internal/model"

// HydrateSelectionAssignments copies persisted assignment maps into selection
// only when the current selection maps are empty and persisted maps have data.
//
// includePI controls whether PI assignments are in scope for hydration.
// Callers should pass true only when PI agent and SDD component are selected.
func HydrateSelectionAssignments(selection *model.Selection, persisted InstallState, includePI bool) {
	if selection == nil {
		return
	}

	if len(selection.ClaudeModelAssignments) == 0 && len(persisted.ClaudeModelAssignments) > 0 {
		selection.ClaudeModelAssignments = claudeAssignmentsFromState(persisted.ClaudeModelAssignments)
	}

	if len(selection.KiroModelAssignments) == 0 && len(persisted.KiroModelAssignments) > 0 {
		selection.KiroModelAssignments = claudeAssignmentsFromState(persisted.KiroModelAssignments)
	}

	if len(selection.ModelAssignments) == 0 && len(persisted.ModelAssignments) > 0 {
		selection.ModelAssignments = modelAssignmentsFromState(persisted.ModelAssignments)
	}

	if includePI && len(selection.PIModelAssignments) == 0 && len(persisted.PIModelAssignments) > 0 {
		selection.PIModelAssignments = modelAssignmentsFromState(persisted.PIModelAssignments)
	}
}

func claudeAssignmentsFromState(input map[string]string) map[string]model.ClaudeModelAlias {
	out := make(map[string]model.ClaudeModelAlias, len(input))
	for key, value := range input {
		out[key] = model.ClaudeModelAlias(value)
	}
	return out
}

func modelAssignmentsFromState(input map[string]ModelAssignmentState) map[string]model.ModelAssignment {
	out := make(map[string]model.ModelAssignment, len(input))
	for key, value := range input {
		out[key] = model.ModelAssignment{ProviderID: value.ProviderID, ModelID: value.ModelID}
	}
	return out
}
