package screens_test

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/screens"
)

func TestNewCodexModelPickerState(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	if state.Preset != screens.CodexPresetRecommended {
		t.Errorf("NewCodexModelPickerState().Preset = %q, want %q", state.Preset, screens.CodexPresetRecommended)
	}
}

func TestNewCodexModelPickerStateFromAssignments_KnownPreset(t *testing.T) {
	tests := []struct {
		name        string
		assignments map[string]model.CodexEffort
		wantPreset  screens.CodexModelPreset
	}{
		{
			name:        "Recommended map → Recommended preset",
			assignments: model.CodexModelPresetRecommended(),
			wantPreset:  screens.CodexPresetRecommended,
		},
		{
			name:        "Powerful map → Powerful preset",
			assignments: model.CodexModelPresetPowerful(),
			wantPreset:  screens.CodexPresetPowerful,
		},
		{
			name:        "LowCost map → LowCost preset",
			assignments: model.CodexModelPresetLowCost(),
			wantPreset:  screens.CodexPresetLowCost,
		},
		{
			name:        "unknown map → Recommended (no Custom fallback)",
			assignments: map[string]model.CodexEffort{"sdd-apply": model.CodexEffortXHigh},
			wantPreset:  screens.CodexPresetRecommended,
		},
		{
			name:        "nil → Recommended",
			assignments: nil,
			wantPreset:  screens.CodexPresetRecommended,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := screens.NewCodexModelPickerStateFromAssignments(tc.assignments)
			if state.Preset != tc.wantPreset {
				t.Errorf("NewCodexModelPickerStateFromAssignments().Preset = %q, want %q", state.Preset, tc.wantPreset)
			}
		})
	}
}

func TestCodexModelPickerOptionCount(t *testing.T) {
	// Must return 5: 3 presets + 1 Custom + 1 Back row (default/main mode)
	state := screens.NewCodexModelPickerState()
	count := screens.CodexModelPickerOptionCount(state)
	if count != 5 {
		t.Errorf("CodexModelPickerOptionCount() = %d, want 5", count)
	}
}

func TestCodexModelPickerOptionCount_PhaseListMode(t *testing.T) {
	// Phase-list sub-mode: 13 phases + 1 Confirm = 14
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModePhaseList
	count := screens.CodexModelPickerOptionCount(state)
	if count != 14 {
		t.Errorf("CodexModelPickerOptionCount(phase-list) = %d, want 14", count)
	}
}

func TestCodexModelPickerOptionCount_EffortMode(t *testing.T) {
	// Effort-select mode: 4 effort levels
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeEffortSelect
	count := screens.CodexModelPickerOptionCount(state)
	if count != 4 {
		t.Errorf("CodexModelPickerOptionCount(effort-select) = %d, want 4", count)
	}
}

func TestHandleCodexModelPickerNav_SelectsPreset(t *testing.T) {
	tests := []struct {
		name       string
		cursor     int
		wantPreset screens.CodexModelPreset
	}{
		{"idx 0 → LowCost", 0, screens.CodexPresetLowCost},
		{"idx 1 → Recommended", 1, screens.CodexPresetRecommended},
		{"idx 2 → Powerful", 2, screens.CodexPresetPowerful},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := screens.NewCodexModelPickerState()
			handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, tc.cursor)
			if !handled {
				t.Errorf("HandleCodexModelPickerNav(enter, %d) handled = false, want true", tc.cursor)
			}
			if assignments == nil {
				t.Errorf("HandleCodexModelPickerNav(enter, %d) assignments = nil, want non-nil", tc.cursor)
			}
			if state.Preset != tc.wantPreset {
				t.Errorf("state.Preset = %q after enter at %d, want %q", state.Preset, tc.cursor, tc.wantPreset)
			}
		})
	}
}

func TestHandleCodexModelPickerNav_BackRow(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	// Back row is at index 4 (3 presets + 1 Custom = index 4)
	handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, 4)
	if !handled {
		t.Error("HandleCodexModelPickerNav(enter, Back) handled = false, want true")
	}
	if assignments != nil {
		t.Errorf("HandleCodexModelPickerNav(enter, Back) assignments = %v, want nil", assignments)
	}
}

func TestHandleCodexModelPickerNav_OtherKey(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	handled, assignments := screens.HandleCodexModelPickerNav("j", &state, 0)
	if handled {
		t.Error("HandleCodexModelPickerNav(j) handled = true, want false")
	}
	if assignments != nil {
		t.Errorf("HandleCodexModelPickerNav(j) assignments = %v, want nil", assignments)
	}
}

func TestRenderCodexModelPicker_ContainsTitle(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	out := screens.RenderCodexModelPicker(state, 0)
	if !strings.Contains(out, "Codex Model Assignments") {
		t.Errorf("RenderCodexModelPicker missing title 'Codex Model Assignments': %s", out)
	}
}

func TestRenderCodexModelPicker_HasCustomRow(t *testing.T) {
	// Custom row must appear alongside the 3 presets.
	state := screens.NewCodexModelPickerState()
	out := screens.RenderCodexModelPicker(state, 0)
	if !strings.Contains(out, "Custom") {
		t.Errorf("RenderCodexModelPicker missing 'Custom' row: %s", out)
	}
}

func TestRenderCodexModelPicker_ContainsBack(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	out := screens.RenderCodexModelPicker(state, 0)
	if !strings.Contains(out, "Back") {
		t.Errorf("RenderCodexModelPicker missing '← Back' row: %s", out)
	}
}

func TestRenderCodexModelPicker_ContainsAllLabels(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	out := screens.RenderCodexModelPicker(state, 0)
	presets := []screens.CodexModelPreset{
		screens.CodexPresetLowCost,
		screens.CodexPresetRecommended,
		screens.CodexPresetPowerful,
	}
	for _, preset := range presets {
		label := screens.CodexPresetLabel(preset)
		if !strings.Contains(out, label[:10]) { // check first 10 chars of label
			t.Errorf("RenderCodexModelPicker missing label for preset %q (expected %q): %s", preset, label, out)
		}
	}
}

// ─── WU-4 RED: self-describing labels ────────────────────────────────────────

// TestCodexPickerLabels_SelfDescribing verifies that each preset label contains
// the correct model and effort for each carril independently.
// Each carril is verified separately so a wrong effort in ONE carril is caught
// even if another carril's effort happens to match.
func TestCodexPickerLabels_SelfDescribing(t *testing.T) {
	tests := []struct {
		preset           screens.CodexModelPreset
		wantStrongEffort string // Razonamiento/sdd-strong effort
		wantMidEffort    string // Código/sdd-mid effort
		wantCheapEffort  string // Liviano/sdd-cheap effort
	}{
		{
			preset:           screens.CodexPresetLowCost,
			wantStrongEffort: "medium",
			wantMidEffort:    "medium",
			wantCheapEffort:  "low",
		},
		{
			preset:           screens.CodexPresetRecommended,
			wantStrongEffort: "high",
			wantMidEffort:    "medium",
			wantCheapEffort:  "low",
		},
		{
			preset:           screens.CodexPresetPowerful,
			wantStrongEffort: "xhigh",
			wantMidEffort:    "high",
			wantCheapEffort:  "low",
		},
	}
	for _, tc := range tests {
		t.Run(string(tc.preset), func(t *testing.T) {
			label := screens.CodexPresetLabel(tc.preset)

			// Model must appear at least once.
			if !strings.Contains(label, "gpt-5.5") {
				t.Errorf("CodexPresetLabel(%q) = %q: missing gpt-5.5", tc.preset, label)
			}

			// Verify each carril by anchoring the model/effort token to its OWN
			// carril segment. This catches a regression in one carril even when
			// another carril shares the same model+effort (e.g. LowCost strong
			// and mid are both gpt-5.5/medium).
			strongToken := "Razonamiento gpt-5.5/" + tc.wantStrongEffort
			if !strings.Contains(label, strongToken) {
				t.Errorf("CodexPresetLabel(%q) = %q: Razonamiento carril missing %q", tc.preset, label, strongToken)
			}

			midToken := "Código gpt-5.5/" + tc.wantMidEffort
			if !strings.Contains(label, midToken) {
				t.Errorf("CodexPresetLabel(%q) = %q: Código carril missing %q", tc.preset, label, midToken)
			}

			cheapToken := "Liviano gpt-5.4-mini/" + tc.wantCheapEffort
			if !strings.Contains(label, cheapToken) {
				t.Errorf("CodexPresetLabel(%q) = %q: Liviano carril missing %q", tc.preset, label, cheapToken)
			}
		})
	}
}

// ─── WU-3: Custom per-phase picker ───────────────────────────────────────────

// TestHandleCodexModelPickerNav_CustomRowEntersPhaseList verifies that pressing
// enter on the Custom row (index 3) transitions the state to ModeCustomPhaseList.
func TestHandleCodexModelPickerNav_CustomRowEntersPhaseList(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, 3)
	if !handled {
		t.Error("HandleCodexModelPickerNav(enter, Custom) handled = false, want true")
	}
	if assignments != nil {
		t.Errorf("HandleCodexModelPickerNav(enter, Custom) should return nil assignments (entering sub-mode), got %v", assignments)
	}
	if state.CustomMode != screens.CodexCustomModePhaseList {
		t.Errorf("state.CustomMode = %v after entering Custom, want CodexCustomModePhaseList", state.CustomMode)
	}
}

// TestCodexCustomPhaseList_Has13Phases verifies that the phase list mode renders
// all 13 expected SDD phases.
func TestCodexCustomPhaseList_Has13Phases(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModePhaseList
	out := screens.RenderCodexModelPicker(state, 0)

	expectedPhases := []string{
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
	}
	for _, phase := range expectedPhases {
		if !strings.Contains(out, phase) {
			t.Errorf("Custom phase list missing phase %q; output:\n%s", phase, out)
		}
	}
}

// TestCodexCustomModelSearch_FiltersByQuery verifies that typing a search query
// in model-select sub-mode filters the displayed Codex models.
func TestCodexCustomModelSearch_FiltersByQuery(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeModelSelect
	state.CustomModelSearch = "codex"

	out := screens.RenderCodexModelPicker(state, 0)
	// BOTH codex-matching models must be present (AND assertion, not OR).
	if !strings.Contains(out, "gpt-5.2-codex") {
		t.Errorf("model search for 'codex' must show gpt-5.2-codex; output:\n%s", out)
	}
	if !strings.Contains(out, "gpt-5.3-codex") {
		t.Errorf("model search for 'codex' must show gpt-5.3-codex; output:\n%s", out)
	}
	// Non-matching models must NOT appear.
	if strings.Contains(out, "gpt-5.4-mini") {
		t.Errorf("model search for 'codex' must not show gpt-5.4-mini; output:\n%s", out)
	}
}

// TestCodexCustomModelSearch_EmptyQueryShowsAll verifies that an empty query
// shows all available models.
func TestCodexCustomModelSearch_EmptyQueryShowsAll(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeModelSelect
	state.CustomModelSearch = ""
	out := screens.RenderCodexModelPicker(state, 0)
	for _, m := range []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.2-codex", "gpt-5.3-codex"} {
		if !strings.Contains(out, m) {
			t.Errorf("model search empty query must show all models; missing %q; output:\n%s", m, out)
		}
	}
}

// TestCodexCustomEffortSelect_RendersOptions verifies that the effort sub-mode
// renders the 4 effort levels.
func TestCodexCustomEffortSelect_RendersOptions(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeEffortSelect
	out := screens.RenderCodexModelPicker(state, 0)
	for _, effort := range []string{"low", "medium", "high", "xhigh"} {
		if !strings.Contains(out, effort) {
			t.Errorf("effort select must show %q; output:\n%s", effort, out)
		}
	}
}

// TestCodexCustomConfirm_PopulatesAssignments verifies that confirming a Custom
// selection returns a non-nil assignments map (phase→effort from preset) and nil
// CodexPhaseModelAssignments when all phases use the default model (gpt-5.5).
// When at least one phase has a custom model, HandleCodexModelPickerNav returns
// the per-phase model map via a second return value.
func TestCodexCustom_PresetsStillWork(t *testing.T) {
	// Regression: after adding Custom row, preset rows must still work at
	// their original indices (0=LowCost, 1=Recommended, 2=Powerful).
	tests := []struct {
		cursor     int
		wantPreset screens.CodexModelPreset
	}{
		{0, screens.CodexPresetLowCost},
		{1, screens.CodexPresetRecommended},
		{2, screens.CodexPresetPowerful},
	}
	for _, tc := range tests {
		t.Run(string(tc.wantPreset), func(t *testing.T) {
			state := screens.NewCodexModelPickerState()
			handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, tc.cursor)
			if !handled {
				t.Errorf("cursor %d: handled = false, want true", tc.cursor)
			}
			if assignments == nil {
				t.Errorf("cursor %d: assignments = nil, want non-nil", tc.cursor)
			}
			if state.Preset != tc.wantPreset {
				t.Errorf("cursor %d: Preset = %q, want %q", tc.cursor, state.Preset, tc.wantPreset)
			}
		})
	}
}

// TestCodexCustom_SelectModelAndEffortUpdatesAssignment verifies the full
// sub-flow: enter Custom → navigate to phase → select model → select effort →
// assignments reflect the chosen phase model.
func TestCodexCustom_SelectModelAndEffortUpdatesAssignment(t *testing.T) {
	state := screens.NewCodexModelPickerState()

	// Enter Custom row (index 3).
	handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, 3)
	if !handled {
		t.Fatal("entering Custom row: handled = false")
	}
	if assignments != nil {
		t.Fatalf("entering Custom row: got assignments, want nil")
	}
	if state.CustomMode != screens.CodexCustomModePhaseList {
		t.Fatalf("after enter Custom: mode = %v, want PhaseList", state.CustomMode)
	}

	// Select first phase (sdd-explore) via HandleCodexCustomNav.
	screens.HandleCodexCustomNav("enter", &state, 0) // select phase 0 → model select

	if state.CustomMode != screens.CodexCustomModeModelSelect {
		t.Fatalf("after selecting phase: mode = %v, want ModelSelect", state.CustomMode)
	}

	// Select first model (gpt-5.5, index 0) → effort select.
	screens.HandleCodexCustomNav("enter", &state, 0)

	if state.CustomMode != screens.CodexCustomModeEffortSelect {
		t.Fatalf("after selecting model: mode = %v, want EffortSelect", state.CustomMode)
	}

	// Navigate to effort "high" (index 2: low=0, medium=1, high=2) via down,
	// then confirm. The outer cursor argument is irrelevant for EffortSelect —
	// CustomEffortCursor is the single source of truth.
	staleOuter := 0
	screens.HandleCodexCustomNav("down", &state, staleOuter) // cursor → 1 (medium)
	screens.HandleCodexCustomNav("down", &state, staleOuter) // cursor → 2 (high)
	screens.HandleCodexCustomNav("enter", &state, staleOuter)

	if state.CustomMode != screens.CodexCustomModePhaseList {
		t.Fatalf("after selecting effort: mode = %v, want PhaseList", state.CustomMode)
	}

	// Verify that the assignment was recorded.
	a, ok := state.CustomAssignments["sdd-explore"]
	if !ok {
		t.Fatalf("CustomAssignments missing sdd-explore; got: %v", state.CustomAssignments)
	}
	if a.ModelID != "gpt-5.5" {
		t.Errorf("CustomAssignments[sdd-explore].ModelID = %q, want gpt-5.5", a.ModelID)
	}
	if a.Effort != model.CodexEffortHigh {
		t.Errorf("CustomAssignments[sdd-explore].Effort = %q, want high", a.Effort)
	}
}

// TestCodexCustomModelSelect_MultipleDownReachesIndex2 verifies that pressing
// "down" multiple times in ModelSelect mode advances CustomModelCursor past
// index 1 and reaches index 2 (and beyond). This is a regression test for the
// bug where CustomModelCursor was reset to the stale outer cursor on every
// keypress, causing the inner cursor to oscillate 0↔1 and never advance past 1.
//
// The test simulates sequential keypresses with the same state, exactly as the
// TUI calls the handler, and asserts that after 3 down presses the cursor is at
// index 2 (the third model).
func TestCodexCustomModelSelect_MultipleDownReachesIndex2(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeModelSelect
	state.CustomModelSearch = "" // all models visible
	state.CustomModelCursor = 0

	// The stale outer cursor that model.go would pass (never advances because
	// HandleCodexCustomNav returns handled=true for up/down).
	staleOuterCursor := 0

	// Press down 3 times — each time passing the same stale outer cursor,
	// matching the real TUI call site (model.go passes m.Cursor which never
	// advances when handled=true is returned).
	for i := 0; i < 3; i++ {
		screens.HandleCodexCustomNav("down", &state, staleOuterCursor)
	}

	if state.CustomModelCursor < 2 {
		t.Errorf("after 3 down presses CustomModelCursor = %d, want >= 2 (cursor must advance past index 1)", state.CustomModelCursor)
	}
}

// TestCodexCustomModelSelect_EnterSelectsThirdModel verifies that after
// navigating to index 2 via sequential down keypresses, pressing enter selects
// the third model (not the second). This catches the symptom of the oscillation
// bug where the user always ends up selecting index 1 regardless of navigation.
func TestCodexCustomModelSelect_EnterSelectsThirdModel(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeModelSelect
	state.CustomModelSearch = ""
	state.CustomModelCursor = 0
	state.CustomPhaseIdx = 0

	staleOuterCursor := 0

	// Navigate to index 2 (third model).
	screens.HandleCodexCustomNav("down", &state, staleOuterCursor)
	screens.HandleCodexCustomNav("down", &state, staleOuterCursor)

	if state.CustomModelCursor != 2 {
		t.Fatalf("before enter: CustomModelCursor = %d, want 2", state.CustomModelCursor)
	}

	// Press enter to select.
	screens.HandleCodexCustomNav("enter", &state, staleOuterCursor)

	if state.CustomMode != screens.CodexCustomModeEffortSelect {
		t.Fatalf("after enter: mode = %v, want EffortSelect", state.CustomMode)
	}

	// The pending model must be the third model in the list, not the second.
	allModels := model.FilterCodexModels("")
	if len(allModels) < 3 {
		t.Skip("fewer than 3 models available")
	}
	expectedModel := allModels[2]
	if state.CustomPendingModel != expectedModel {
		t.Errorf("CustomPendingModel = %q, want %q (third model, index 2)", state.CustomPendingModel, expectedModel)
	}
}

// TestCodexCustomEffortSelect_MultipleDownReachesIndex2 verifies that pressing
// "down" multiple times in EffortSelect mode advances CustomEffortCursor past
// index 1, i.e., the same oscillation bug fix applies to the effort picker too.
func TestCodexCustomEffortSelect_MultipleDownReachesIndex2(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModeEffortSelect
	state.CustomEffortCursor = 0
	state.CustomPendingModel = "gpt-5.5"
	state.CustomPhaseIdx = 0

	staleOuterCursor := 0

	for i := 0; i < 3; i++ {
		screens.HandleCodexCustomNav("down", &state, staleOuterCursor)
	}

	if state.CustomEffortCursor < 2 {
		t.Errorf("after 3 down presses CustomEffortCursor = %d, want >= 2 (cursor must advance past index 1)", state.CustomEffortCursor)
	}
}

// TestCodexCustom_ConfirmReturnsPhaseModelAssignments verifies that when the
// user navigates to the Confirm row in phase list mode and presses enter,
// HandleCodexModelPickerNav returns a non-nil, non-empty assignments map from
// the stored custom per-phase assignments, and CustomConfirmed is set to true.
func TestCodexCustom_ConfirmReturnsPhaseModelAssignments(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	state.CustomMode = screens.CodexCustomModePhaseList
	// Pre-populate a custom assignment for one phase.
	state.CustomAssignments = map[string]screens.CodexCustomAssignment{
		"sdd-propose": {ModelID: "gpt-5.4", Effort: model.CodexEffortHigh},
	}

	// Confirm row is the LAST row in phase-list mode (after 13 phases).
	confirmIdx := 13 // 13 phases, confirm is at idx 13
	handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, confirmIdx)
	if !handled {
		t.Fatal("Confirm row: handled = false, want true")
	}
	if assignments == nil {
		t.Fatal("Confirm row: assignments = nil, want non-nil map")
	}
	if len(assignments) == 0 {
		t.Fatal("Confirm row: assignments is empty, want at least one entry")
	}
	// The effort for the assigned phase must be reflected.
	if assignments["sdd-propose"] != model.CodexEffortHigh {
		t.Errorf("assignments[sdd-propose] = %q, want high", assignments["sdd-propose"])
	}
	// CustomConfirmed must be set to true after Confirm.
	if !state.CustomConfirmed {
		t.Error("state.CustomConfirmed = false, want true after Confirm row")
	}
}
