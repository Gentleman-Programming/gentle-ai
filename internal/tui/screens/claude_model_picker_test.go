package screens

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
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
	// The built-in Explore row is active; only the retired SDD Explore row is not.
	retired := strings.ReplaceAll(rows, claudePhaseLabels[model.ClaudeBuiltInExploreRole], "")
	for _, role := range []string{"sdd-", "SDD phase", "Explore   ", "Apply   "} {
		if strings.Contains(retired, role) {
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

func TestSonnetXHighSelectionAndRendering(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true
	state.Mode = ClaudeModeEffortSelect
	phase := claudePhases[0]
	state.SelectedPhase = phase
	state.CustomAssignments[phase] = model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet}

	out := RenderClaudeModelPicker(state, 0)
	if !strings.Contains(out, "xhigh") {
		t.Fatalf("Sonnet effort picker does not offer xhigh:\n%s", out)
	}
	handled, assignments := HandleClaudeModelPickerNav("enter", &state, 4) // xhigh
	if !handled || assignments != nil || state.Mode != ClaudeModePhaseList {
		t.Fatalf("effort selection = handled %v assignments %v mode %v", handled, assignments, state.Mode)
	}
	if got := state.CustomAssignments[phase]; got.Model != model.ClaudeModelSonnet || got.Effort != model.ClaudeEffortXHigh {
		t.Fatalf("assignment = %+v, want sonnet/xhigh", got)
	}
	if out := RenderClaudeModelPicker(state, 0); !strings.Contains(out, "[xhigh]") {
		t.Fatalf("saved Sonnet xhigh effort not rendered:\n%s", out)
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

// #5612 review: no preset sets the ODD rows, so their agents inherit the
// session model until the user picks one.
func TestClaudePresetsLeaveODDRowsInherit(t *testing.T) {
	for cursor, preset := range claudePresetOrder {
		if preset == ClaudePresetCustom {
			continue
		}
		state := NewClaudeModelPickerState()
		_, saved := HandleClaudeModelPickerNav("enter", &state, cursor)
		if saved == nil {
			t.Fatalf("%s: preset not confirmed", preset)
		}
		for _, role := range model.ClaudeODDRoles {
			if value, ok := saved[role]; ok {
				t.Errorf("%s seeds %s = %+v", preset, role, value)
			}
		}
	}
}

// #5612 review: an ODD row shows [inherit] without a pick, and its model list
// offers inherit to clear a pick. Other rows keep their model-only list.
func TestClaudePickerODDRowsOfferInherit(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true
	state.Mode = ClaudeModePhaseList
	rows := RenderClaudeModelPicker(state, 0)
	for _, role := range model.ClaudeODDRoles {
		line := ""
		for _, candidate := range strings.Split(rows, "\n") {
			if strings.Contains(candidate, claudePhaseLabels[role]) {
				line = candidate
			}
		}
		if !strings.Contains(line, "[inherit]") {
			t.Errorf("%s row = %q, want [inherit]", role, line)
		}
	}

	state.Mode = ClaudeModeModelSelect
	state.SelectedPhase = "odd-worker"
	if got, want := ClaudeModelPickerOptionCount(state), len(claudeAliasOrder)+2; got != want {
		t.Fatalf("ODD model options = %d, want %d (aliases, inherit, Back)", got, want)
	}
	if view := RenderClaudeModelPicker(state, 0); !strings.Contains(view, "inherit") {
		t.Fatalf("ODD model list does not offer inherit: %s", view)
	}
	HandleClaudeModelPickerNav("enter", &state, 1) // opus
	if state.CustomAssignments["odd-worker"].Model != model.ClaudeModelOpus {
		t.Fatalf("odd-worker pick = %+v", state.CustomAssignments["odd-worker"])
	}
	state.Mode = ClaudeModeModelSelect
	HandleClaudeModelPickerNav("enter", &state, len(claudeAliasOrder)) // inherit
	if value, ok := state.CustomAssignments["odd-worker"]; ok {
		t.Fatalf("inherit kept odd-worker = %+v", value)
	}
	if state.Mode != ClaudeModePhaseList {
		t.Fatalf("Mode = %v after inherit, want the phase list", state.Mode)
	}

	state.Mode = ClaudeModeModelSelect
	state.SelectedPhase = "risk"
	if got, want := ClaudeModelPickerOptionCount(state), len(claudeAliasOrder)+1; got != want {
		t.Fatalf("risk model options = %d, want %d (aliases, Back)", got, want)
	}
}

// #5612 review: a preset an older picker saved with Sonnet-seeded ODD rows
// still opens as that preset, with the ODD rows on inherit.
func TestClaudePickerTreatsLegacyODDSeedAsInherit(t *testing.T) {
	seeded := model.ClaudePhaseAssignmentsFromModelPreset(model.ClaudeModelPresetPerformance())
	for key := range seeded {
		if strings.HasPrefix(key, "sdd-") {
			delete(seeded, key)
		}
	}
	for _, role := range model.ClaudeODDRoles {
		seeded[role] = model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet}
	}
	state := NewClaudeModelPickerStateFromPhaseAssignments(seeded)
	if state.Preset != ClaudePresetPerformance {
		t.Fatalf("Preset = %q, want performance", state.Preset)
	}
	for _, role := range model.ClaudeODDRoles {
		if value, ok := state.CustomAssignments[role]; ok {
			t.Errorf("legacy seed kept %s = %+v", role, value)
		}
	}
}

func claudePhaseRow(t *testing.T, role string) int {
	t.Helper()
	for i, key := range claudePhases {
		if key == role {
			return i
		}
	}
	t.Fatalf("missing custom picker row for %s", role)
	return -1
}

// #5611: Claude Code's built-in Explore and general-purpose agents get their
// own rows, which inherit the session model until the user picks one.
func TestClaudePickerBuiltInRowsInheritUntilPicked(t *testing.T) {
	picker := NewClaudeModelPickerState()
	picker.InCustomMode = true
	rows := RenderClaudeModelPicker(picker, 0)
	for _, label := range []string{"Built-in Explore", "Built-in general-purpose"} {
		line := ""
		for _, candidate := range strings.Split(rows, "\n") {
			if strings.Contains(candidate, label) {
				line = candidate
			}
		}
		if !strings.Contains(line, "[inherit]") || strings.Contains(line, "[sonnet]") {
			t.Errorf("unpicked row %q does not show inherit: %q", label, line)
		}
	}
	for _, preset := range []map[string]model.ClaudeModelAlias{model.ClaudeModelPresetBalanced(), model.ClaudeModelPresetPerformance(), model.ClaudeModelPresetEconomy(), model.ClaudeModelPresetDiversity()} {
		assignments := claudePickerPresetAssignments(preset)
		for _, role := range []string{model.ClaudeBuiltInExploreRole, model.ClaudeBuiltInGeneralPurposeRole} {
			if _, ok := assignments[role]; ok {
				t.Fatalf("preset assigns built-in row %s", role)
			}
		}
	}
}

// #5611: picking a model and effort for a built-in row saves and reopens it,
// and choosing inherit removes the pick again.
func TestClaudePickerBuiltInRowPickAndInherit(t *testing.T) {
	picker := NewClaudeModelPickerState()
	picker.InCustomMode = true
	row := claudePhaseRow(t, model.ClaudeBuiltInExploreRole)
	HandleClaudeModelPickerNav("enter", &picker, row)
	if got := ClaudeModelPickerOptionCount(picker); got != len(claudeAliasOrder)+2 {
		t.Fatalf("built-in model options = %d, want aliases + inherit + back", got)
	}
	if view := RenderClaudeModelPicker(picker, 0); !strings.Contains(view, "inherit") {
		t.Fatalf("model select does not offer inherit: %s", view)
	}
	HandleClaudeModelPickerNav("enter", &picker, 2) // sonnet
	HandleClaudeModelPickerNav("enter", &picker, 1) // low
	_, saved := HandleClaudeModelPickerNav("enter", &picker, len(claudePhases))
	want := model.ClaudePhaseAssignment{Model: model.ClaudeModelSonnet, Effort: model.ClaudeEffortLow}
	if saved[model.ClaudeBuiltInExploreRole] != want {
		t.Fatalf("saved built-in Explore = %+v, want %+v", saved[model.ClaudeBuiltInExploreRole], want)
	}
	reopened := NewClaudeModelPickerStateFromPhaseAssignments(saved)
	if reopened.Preset != ClaudePresetCustom || reopened.CustomAssignments[model.ClaudeBuiltInExploreRole] != want {
		t.Fatalf("reopened = %s %+v", reopened.Preset, reopened.CustomAssignments[model.ClaudeBuiltInExploreRole])
	}

	reopened.InCustomMode = true
	HandleClaudeModelPickerNav("enter", &reopened, row)
	HandleClaudeModelPickerNav("enter", &reopened, len(claudeAliasOrder)) // inherit
	if reopened.Mode != ClaudeModePhaseList {
		t.Fatalf("inherit did not return to the role list: mode %v", reopened.Mode)
	}
	_, cleared := HandleClaudeModelPickerNav("enter", &reopened, len(claudePhases))
	if _, ok := cleared[model.ClaudeBuiltInExploreRole]; ok {
		t.Fatalf("inherit kept the pick: %+v", cleared)
	}
}
