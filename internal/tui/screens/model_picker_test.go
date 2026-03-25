package screens

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/opencode"
)

// TestHandleModelNav_Space_TogglesSelection tests that space toggles model selection.
func TestHandleModelNav_Space_TogglesSelection(t *testing.T) {
	// Create a minimal state with mock data
	state := ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-opus", Name: "Claude Opus"},
				{ID: "claude-sonnet", Name: "Claude Sonnet"},
			},
		},
		SelectedModels:    make(map[string][]model.ModelReference),
		SelectedPhaseIdx: 1, // Index 1 = "sdd-init" (first phase after "Set all phases")
		ModelCursor:       0,
	}

	assignments := make(model.ModelAssignments)

	// Press space to select the first model (should become Primary)
	handled, updatedAssignments := HandleModelPickerNav(" ", &state, assignments)
	if !handled {
		t.Fatal("expected space to be handled")
	}
	if len(state.SelectedModels["sdd-init"]) != 1 {
		t.Fatalf("expected 1 selected model for sdd-init, got %d", len(state.SelectedModels["sdd-init"]))
	}
	if state.SelectedModels["sdd-init"][0] != "anthropic/claude-opus" {
		t.Fatalf("expected anthropic/claude-opus, got %s", state.SelectedModels["sdd-init"][0])
	}
	// Check that assignments wasn't modified yet (only modified on enter)
	if len(updatedAssignments) != 0 {
		t.Fatal("expected assignments to be empty until enter is pressed")
	}

	// Press space again to deselect
	handled, _ = HandleModelPickerNav(" ", &state, assignments)
	if !handled {
		t.Fatal("expected space to be handled")
	}
	if len(state.SelectedModels["sdd-init"]) != 0 {
		t.Fatalf("expected 0 selected models after toggle-off, got %d", len(state.SelectedModels["sdd-init"]))
	}
}

// TestHandleModelNav_MultipleSelections tests selecting multiple models.
func TestHandleModelNav_MultipleSelections(t *testing.T) {
	state := ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-opus", Name: "Claude Opus"},
				{ID: "claude-sonnet", Name: "Claude Sonnet"},
				{ID: "claude-haiku", Name: "Claude Haiku"},
			},
		},
		SelectedModels:    make(map[string][]model.ModelReference),
		SelectedPhaseIdx: 1, // "sdd-init"
		ModelCursor:       0,
	}

	assignments := make(model.ModelAssignments)

	// Select first model
	HandleModelPickerNav(" ", &state, assignments)
	if state.SelectedModels["sdd-init"][0] != "anthropic/claude-opus" {
		t.Fatalf("expected first selection to be Primary")
	}

	// Move cursor and select second model
	state.ModelCursor = 1
	HandleModelPickerNav(" ", &state, assignments)
	if len(state.SelectedModels["sdd-init"]) != 2 {
		t.Fatalf("expected 2 selected models, got %d", len(state.SelectedModels["sdd-init"]))
	}
	if state.SelectedModels["sdd-init"][1] != "anthropic/claude-sonnet" {
		t.Fatalf("expected second selection to be Fallback #1")
	}

	// Move cursor and select third model
	state.ModelCursor = 2
	HandleModelPickerNav(" ", &state, assignments)
	if len(state.SelectedModels["sdd-init"]) != 3 {
		t.Fatalf("expected 3 selected models, got %d", len(state.SelectedModels["sdd-init"]))
	}
	if state.SelectedModels["sdd-init"][2] != "anthropic/claude-haiku" {
		t.Fatalf("expected third selection to be Fallback #2")
	}
}

// TestHandleModelNav_EnterBuildsModelPool tests that enter builds the ModelPool.
func TestHandleModelNav_EnterBuildsModelPool(t *testing.T) {
	state := ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-opus", Name: "Claude Opus"},
				{ID: "claude-sonnet", Name: "Claude Sonnet"},
			},
		},
		SelectedModels: map[string][]model.ModelReference{
			"sdd-init": {"anthropic/claude-opus", "anthropic/claude-sonnet"},
		},
		SelectedPhaseIdx: 1, // "sdd-init"
		ModelCursor:       0,
	}

	assignments := make(model.ModelAssignments)

	// Press enter to confirm
	handled, updatedAssignments := HandleModelPickerNav("enter", &state, assignments)
	if !handled {
		t.Fatal("expected enter to be handled")
	}

	// Check that assignments now contains the pool
	pool, ok := updatedAssignments["sdd-init"]
	if !ok {
		t.Fatal("expected sdd-init to have a pool")
	}
	if pool.Primary != "anthropic/claude-opus" {
		t.Fatalf("expected Primary to be anthropic/claude-opus, got %s", pool.Primary)
	}
	if len(pool.Fallbacks) != 1 {
		t.Fatalf("expected 1 fallback, got %d", len(pool.Fallbacks))
	}
	if pool.Fallbacks[0] != "anthropic/claude-sonnet" {
		t.Fatalf("expected Fallback to be anthropic/claude-sonnet, got %s", pool.Fallbacks[0])
	}

	// Check that mode changed back to phase list
	if state.Mode != ModePhaseList {
		t.Fatalf("expected mode to be ModePhaseList, got %d", state.Mode)
	}
}

// TestHandleModelNav_EnterConfirmClearsSelection tests that enter clears the selection state.
func TestHandleModelNav_EnterConfirmClearsSelection(t *testing.T) {
	state := ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-opus", Name: "Claude Opus"},
			},
		},
		SelectedModels: map[string][]model.ModelReference{
			"sdd-init": {"anthropic/claude-opus"},
		},
		SelectedPhaseIdx: 1, // "sdd-init"
		ModelCursor:       0,
	}

	_ = make(model.ModelAssignments)

	// Press enter to confirm
	HandleModelPickerNav("enter", &state, make(model.ModelAssignments))

	// Check that selection was cleared
	if len(state.SelectedModels["sdd-init"]) != 0 {
		t.Fatalf("expected selection to be cleared, got %d items", len(state.SelectedModels["sdd-init"]))
	}
}

// TestHandleModelNav_SetAllPhases tests setting all phases at once.
func TestHandleModelNav_SetAllPhases(t *testing.T) {
	state := ModelPickerState{
		Mode:             ModeModelSelect,
		SelectedProvider: "anthropic",
		SDDModels: map[string][]opencode.Model{
			"anthropic": {
				{ID: "claude-opus", Name: "Claude Opus"},
				{ID: "claude-sonnet", Name: "Claude Sonnet"},
			},
		},
		SelectedModels: map[string][]model.ModelReference{
			// When "Set all phases" (idx 0), we use the selection for ALL phases
			// The selection state is keyed by phase name
			"sdd-init":    {"anthropic/claude-opus", "anthropic/claude-sonnet"},
			"sdd-explore": {"anthropic/claude-opus", "anthropic/claude-sonnet"},
			// etc... (in real usage, the selection would be propagated to all phases)
		},
		SelectedPhaseIdx: 0, // Set all phases
		ModelCursor:       0,
	}

	assignments := make(model.ModelAssignments)

	// First set up selection for all phases manually (simulating the space key behavior for "Set all phases")
	// In production, when space is pressed with SelectedPhaseIdx=0, it adds to ALL phases
	for _, phase := range opencode.SDDPhases() {
		state.SelectedModels[phase] = []model.ModelReference{"anthropic/claude-opus", "anthropic/claude-sonnet"}
	}

	// Press enter to confirm
	handled, updatedAssignments := HandleModelPickerNav("enter", &state, assignments)
	if !handled {
		t.Fatal("expected enter to be handled")
	}

	// Check that all phases have the same pool
	phases := opencode.SDDPhases()
	for _, phase := range phases {
		pool, ok := updatedAssignments[phase]
		if !ok {
			t.Fatalf("expected %s to have a pool", phase)
		}
		if pool.Primary != "anthropic/claude-opus" {
			t.Fatalf("expected %s Primary to be anthropic/claude-opus, got %s", phase, pool.Primary)
		}
		if len(pool.Fallbacks) != 1 {
			t.Fatalf("expected %s to have 1 fallback, got %d", phase, len(pool.Fallbacks))
		}
	}
}

// TestFormatPoolLabel tests the pool label formatting.
func TestFormatPoolLabel(t *testing.T) {
	state := ModelPickerState{
		Providers: map[string]opencode.Provider{
			"anthropic": {
				Name: "Anthropic",
				Models: map[string]opencode.Model{
					"claude-opus":   {Name: "Claude Opus"},
					"claude-sonnet": {Name: "Claude Sonnet"},
				},
			},
		},
	}

	tests := []struct {
		name      string
		pool      model.ModelPool
		wantLabel string
	}{
		{
			name: "primary only",
			pool: model.ModelPool{
				Primary: "anthropic/claude-opus",
			},
			// %-20s pads the phase name to 20 characters
			wantLabel: "sdd_init             Anthropic / Claude Opus",
		},
		{
			name: "primary with one fallback",
			pool: model.ModelPool{
				Primary:   "anthropic/claude-opus",
				Fallbacks: []model.ModelReference{"openai/gpt-4"},
			},
			wantLabel: "sdd_init             Anthropic / Claude Opus (+1 fallbacks)",
		},
		{
			name: "primary with multiple fallbacks",
			pool: model.ModelPool{
				Primary: "anthropic/claude-opus",
				Fallbacks: []model.ModelReference{
					"openai/gpt-4",
					"openai/gpt-3.5-turbo",
				},
			},
			wantLabel: "sdd_init             Anthropic / Claude Opus (+2 fallbacks)",
		},
		{
			name:      "empty pool",
			pool:      model.ModelPool{},
			wantLabel: "sdd_init             (default)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPoolLabel("sdd_init", tt.pool, state)
			if got != tt.wantLabel {
				t.Fatalf("formatPoolLabel() = %q, want %q", got, tt.wantLabel)
			}
		})
	}
}