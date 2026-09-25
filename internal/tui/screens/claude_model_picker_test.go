package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

func TestClaudeShortTerminalShowsFocusedRDDRowsAndConfirm(t *testing.T) {
	picker := NewClaudeModelPickerState()
	picker.InCustomMode = true
	picker.Mode = ClaudeModePhaseList
	for _, role := range []string{"risk", "readability", "reliability", "resilience", "refuter", "validator"} {
		for row, phase := range claudePhases {
			if phase != role {
				continue
			}
			view := RenderClaudeModelPicker(picker, row, 12)
			if !strings.Contains(view, claudePhaseLabels[role]) || len(strings.Split(view, "\n")) > 12 {
				t.Errorf("role %s not visible within 12 lines: %q", role, view)
			}
		}
	}
	view := RenderClaudeModelPicker(picker, len(claudePhases), 12)
	if !strings.Contains(view, "Confirm") || len(strings.Split(view, "\n")) > 12 {
		t.Errorf("Confirm not visible within 12 lines: %q", view)
	}
}

func TestClaudePickerPreservesLegacyAssignmentsWithoutShowingRows(t *testing.T) {
	legacy := model.ClaudePhaseAssignment{Model: model.ClaudeModelFable, Effort: model.ClaudeEffortHigh}
	picker := NewClaudeModelPickerStateFromPhaseAssignments(map[string]model.ClaudePhaseAssignment{"sdd-propose": legacy})
	picker.InCustomMode = true
	if strings.Contains(RenderClaudeModelPicker(picker, 0), "Propose") {
		t.Fatal("legacy SDD role is visible")
	}
	_, saved := HandleClaudeModelPickerNav("enter", &picker, ClaudeModelPickerOptionCount(picker)-2)
	if saved["sdd-propose"] != legacy {
		t.Fatalf("legacy assignment changed: %+v", saved["sdd-propose"])
	}
}

func TestClaudePickerOffersOnlyActiveRoles(t *testing.T) {
	picker := NewClaudeModelPickerState()
	picker.InCustomMode = true
	rows := RenderClaudeModelPicker(picker, 0)
	for _, role := range []string{"ODD Explorer", "ODD Worker", "ODD Verify", "RDD Risk", "RDD Validator", "General delegation"} {
		if !strings.Contains(rows, role) {
			t.Errorf("missing active role %q: %s", role, rows)
		}
	}
	for _, role := range []string{"sdd-", "SDD phase", "Explore   ", "Apply   "} {
		if strings.Contains(rows, role) {
			t.Errorf("retired SDD role %q visible: %s", role, rows)
		}
	}
}

func TestClaudePickerSavesAndReopensNativeReviewRoles(t *testing.T) {
	roles := []string{"risk", "readability", "reliability", "resilience", "refuter", "validator"}
	picker := NewClaudeModelPickerState()
	picker.InCustomMode = true
	for _, role := range roles {
		row := -1
		for i, key := range claudePhases {
			if key == role {
				row = i
				break
			}
		}
		if row < 0 || claudePhaseLabels[role] == "" {
			t.Fatalf("missing custom picker row for %s", role)
		}
		HandleClaudeModelPickerNav("enter", &picker, row)
		if picker.SelectedPhase != role {
			t.Fatalf("selected %q, want %q", picker.SelectedPhase, role)
		}
		HandleClaudeModelPickerNav("enter", &picker, 3) // haiku
	}
	_, saved := HandleClaudeModelPickerNav("enter", &picker, len(claudePhases))
	if saved == nil {
		t.Fatal("confirm did not return assignments")
	}
	reopened := NewClaudeModelPickerStateFromPhaseAssignments(saved)
	if reopened.Preset != ClaudePresetCustom {
		t.Fatalf("reopened preset = %s, want custom", reopened.Preset)
	}
	for _, role := range roles {
		if reopened.CustomAssignments[role].Model != model.ClaudeModelHaiku {
			t.Errorf("reopened %s = %v, want haiku", role, reopened.CustomAssignments[role])
		}
	}
	if _, present := model.ClaudeModelPresetBalanced()["risk"]; present {
		t.Fatal("named preset policy changed")
	}
}

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
		"odd-explorer": model.ClaudeModelFable,
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
