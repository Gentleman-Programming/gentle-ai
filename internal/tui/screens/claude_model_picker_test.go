package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestNewClaudeModelPickerStateFromAssignments(t *testing.T) {
	cases := []struct {
		name        string
		assignments map[string]model.ClaudeModelAlias
		wantPreset  ClaudeModelPreset
	}{
		{
			name:        "nil → balanced default",
			assignments: nil,
			wantPreset:  ClaudePresetBalanced,
		},
		{
			name:        "empty → balanced default",
			assignments: map[string]model.ClaudeModelAlias{},
			wantPreset:  ClaudePresetBalanced,
		},
		{
			name:        "balanced match",
			assignments: model.ClaudeModelPresetBalanced(),
			wantPreset:  ClaudePresetBalanced,
		},
		{
			name:        "performance match",
			assignments: model.ClaudeModelPresetPerformance(),
			wantPreset:  ClaudePresetPerformance,
		},
		{
			name:        "economy match",
			assignments: model.ClaudeModelPresetEconomy(),
			wantPreset:  ClaudePresetEconomy,
		},
		{
			name:        "custom assignment",
			assignments: map[string]model.ClaudeModelAlias{"sdd-apply": model.ClaudeModelHaiku},
			wantPreset:  ClaudePresetCustom,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := NewClaudeModelPickerStateFromAssignments(tc.assignments)
			if state.Preset != tc.wantPreset {
				t.Errorf("Preset = %q, want %q", state.Preset, tc.wantPreset)
			}
			if state.InCustomMode {
				t.Error("InCustomMode should be false on initial state")
			}
			if state.CustomAssignments == nil {
				t.Error("CustomAssignments should not be nil")
			}
		})
	}
}

func TestNewClaudeModelPickerStateFromAssignments_CopiesMap(t *testing.T) {
	original := model.ClaudeModelPresetBalanced()
	state := NewClaudeModelPickerStateFromAssignments(original)

	// Mutating original should not affect state.
	original["sdd-apply"] = model.ClaudeModelOpus

	if state.CustomAssignments["sdd-apply"].Model == model.ClaudeModelOpus {
		t.Error("CustomAssignments shares memory with the input map — expected a defensive copy")
	}
}

func TestHandleCustomPhaseNav_EnterPhaseOpensModelSelect(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true

	handled, assignments := HandleClaudeModelPickerNav("enter", &state, 0)
	if !handled {
		t.Fatal("enter on a phase row should be handled")
	}
	if assignments != nil {
		t.Fatal("editing a phase should not confirm the screen")
	}
	if state.Mode != ClaudeModeModelSelect {
		t.Fatalf("Mode = %v, want ClaudeModeModelSelect", state.Mode)
	}
	if state.SelectedPhase != claudePhases[0] {
		t.Fatalf("SelectedPhase = %q, want %q", state.SelectedPhase, claudePhases[0])
	}
}

func TestHandleCustomModelSelect_SelectsModelThenEffort(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true
	state.Mode = ClaudeModeModelSelect
	state.SelectedPhase = claudePhases[0]

	handled, assignments := HandleClaudeModelPickerNav("enter", &state, 1) // opus
	if !handled || assignments != nil {
		t.Fatalf("enter on model row = handled %v assignments %v, want handled with nil assignments", handled, assignments)
	}
	if got := state.CustomAssignments[claudePhases[0]].Model; got != model.ClaudeModelOpus {
		t.Fatalf("selected model = %q, want opus", got)
	}
	if state.Mode != ClaudeModeEffortSelect {
		t.Fatalf("Mode = %v, want ClaudeModeEffortSelect", state.Mode)
	}
}

func TestHandleCustomPhaseNav_ConfirmAndBackRows(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true

	handled, assignments := HandleClaudeModelPickerNav("enter", &state, len(claudePhases))
	if !handled {
		t.Fatal("enter on Confirm row should be handled")
	}
	if assignments == nil {
		t.Fatal("enter on Confirm row should return assignments")
	}
	if !state.InCustomMode {
		t.Fatal("confirming custom assignments should not mutate InCustomMode before caller transitions")
	}

	handled, assignments = HandleClaudeModelPickerNav("enter", &state, len(claudePhases)+1)
	if !handled {
		t.Fatal("enter on Back row should be handled")
	}
	if assignments != nil {
		t.Fatal("enter on Back row should not return assignments")
	}
	if state.InCustomMode {
		t.Fatal("enter on Back row should exit custom mode")
	}
}

func TestHandleCustomEffortSelect_OnlyOffersSupportedEfforts(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true
	phase := claudePhases[0]
	state.SelectedPhase = phase

	state.Mode = ClaudeModeModelSelect
	_, _ = HandleClaudeModelPickerNav("enter", &state, 3) // haiku
	if state.Mode != ClaudeModePhaseList {
		t.Fatalf("haiku should skip effort select, mode = %v", state.Mode)
	}

	state.Mode = ClaudeModeEffortSelect
	state.CustomAssignments[phase] = model.ClaudePhaseAssignment{Model: model.ClaudeModelOpus}
	_, _ = HandleClaudeModelPickerNav("enter", &state, 1) // low
	if got := state.CustomAssignments[phase].Effort; got != model.ClaudeEffortLow {
		t.Fatalf("opus selected effort = %q, want low", got)
	}
	if state.Mode != ClaudeModePhaseList {
		t.Fatalf("effort selection should return to phase list, mode = %v", state.Mode)
	}
}

// TestRenderClaudeModelPicker_CustomModeRendersFable verifies that custom
// mode renders the [fable] badge for a fable-assigned phase and explains the
// explicit model/effort selection flow.
func TestRenderClaudeModelPicker_CustomModeRendersFable(t *testing.T) {
	state := NewClaudeModelPickerStateFromAssignments(map[string]model.ClaudeModelAlias{
		"sdd-propose": model.ClaudeModelFable,
	})
	state.InCustomMode = true

	out := RenderClaudeModelPicker(state, 0)
	if !strings.Contains(out, "[fable]") {
		t.Errorf("expected [fable] tag in custom mode render, got:\n%s", out)
	}
	if !strings.Contains(out, "choose its model, then choose a supported effort") {
		t.Errorf("expected explicit model/effort help text, got:\n%s", out)
	}
}

func TestRenderClaudeModelPicker_ShowsCurrentPreset(t *testing.T) {
	cases := []struct {
		name        string
		assignments map[string]model.ClaudeModelAlias
		wantLabel   string
	}{
		{
			name:        "balanced default shows balanced",
			assignments: nil,
			wantLabel:   "Current: balanced",
		},
		{
			name:        "performance preset shows performance",
			assignments: model.ClaudeModelPresetPerformance(),
			wantLabel:   "Current: performance",
		},
		{
			name:        "economy preset shows economy",
			assignments: model.ClaudeModelPresetEconomy(),
			wantLabel:   "Current: economy",
		},
		{
			name:        "custom assignments shows custom",
			assignments: map[string]model.ClaudeModelAlias{"sdd-apply": model.ClaudeModelHaiku},
			wantLabel:   "Current: custom",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := NewClaudeModelPickerStateFromAssignments(tc.assignments)
			out := RenderClaudeModelPicker(state, 0)
			if !strings.Contains(out, tc.wantLabel) {
				t.Errorf("expected %q in render output, got:\n%s", tc.wantLabel, out)
			}
		})
	}
}
