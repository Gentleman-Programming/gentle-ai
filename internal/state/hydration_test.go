package state

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestHydrateSelectionAssignmentsOpenCodeHydratesNilOrEmpty(t *testing.T) {
	persisted := InstallState{
		ModelAssignments: map[string]ModelAssignmentState{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
	}

	t.Run("nil map", func(t *testing.T) {
		selection := model.Selection{}
		HydrateSelectionAssignments(&selection, persisted, false)

		got := selection.ModelAssignments["sdd-apply"]
		if got.ProviderID != "anthropic" || got.ModelID != "claude-sonnet-4" {
			t.Fatalf("ModelAssignments[sdd-apply] = %+v, want anthropic/claude-sonnet-4", got)
		}
	})

	t.Run("empty but non-nil map", func(t *testing.T) {
		selection := model.Selection{ModelAssignments: map[string]model.ModelAssignment{}}
		HydrateSelectionAssignments(&selection, persisted, false)

		got := selection.ModelAssignments["sdd-apply"]
		if got.ProviderID != "anthropic" || got.ModelID != "claude-sonnet-4" {
			t.Fatalf("ModelAssignments[sdd-apply] = %+v, want anthropic/claude-sonnet-4", got)
		}
	})
}

func TestHydrateSelectionAssignmentsDoesNotOverwriteNonEmptyMaps(t *testing.T) {
	selection := model.Selection{
		ModelAssignments: map[string]model.ModelAssignment{
			"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5"},
		},
		ClaudeModelAssignments: map[string]model.ClaudeModelAlias{
			"orchestrator": model.ClaudeModelOpus,
		},
	}

	persisted := InstallState{
		ModelAssignments: map[string]ModelAssignmentState{
			"sdd-apply": {ProviderID: "anthropic", ModelID: "claude-sonnet-4"},
		},
		ClaudeModelAssignments: map[string]string{
			"orchestrator": "haiku",
		},
	}

	HydrateSelectionAssignments(&selection, persisted, false)

	gotModel := selection.ModelAssignments["sdd-apply"]
	if gotModel.ProviderID != "openai" || gotModel.ModelID != "gpt-5" {
		t.Fatalf("ModelAssignments[sdd-apply] overwritten = %+v, want openai/gpt-5", gotModel)
	}
	if gotClaude := selection.ClaudeModelAssignments["orchestrator"]; gotClaude != model.ClaudeModelOpus {
		t.Fatalf("ClaudeModelAssignments[orchestrator] overwritten = %q, want %q", gotClaude, model.ClaudeModelOpus)
	}
}

func TestHydrateSelectionAssignmentsPIGatedByScope(t *testing.T) {
	persisted := InstallState{
		PIModelAssignments: map[string]ModelAssignmentState{
			"sdd-apply": {ProviderID: "openai", ModelID: "gpt-5-mini"},
		},
	}

	t.Run("out of scope", func(t *testing.T) {
		selection := model.Selection{}
		HydrateSelectionAssignments(&selection, persisted, false)
		if len(selection.PIModelAssignments) != 0 {
			t.Fatalf("PIModelAssignments hydrated out of scope: %+v", selection.PIModelAssignments)
		}
	})

	t.Run("in scope", func(t *testing.T) {
		selection := model.Selection{}
		HydrateSelectionAssignments(&selection, persisted, true)
		got := selection.PIModelAssignments["sdd-apply"]
		if got.ProviderID != "openai" || got.ModelID != "gpt-5-mini" {
			t.Fatalf("PIModelAssignments[sdd-apply] = %+v, want openai/gpt-5-mini", got)
		}
	})
}
