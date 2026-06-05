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
	// Must return 4: 3 presets + 1 Back row
	count := screens.CodexModelPickerOptionCount()
	if count != 4 {
		t.Errorf("CodexModelPickerOptionCount() = %d, want 4", count)
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
	// Back row is at index 3 (len(presets) = 3)
	handled, assignments := screens.HandleCodexModelPickerNav("enter", &state, 3)
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

func TestRenderCodexModelPicker_NoCustom(t *testing.T) {
	state := screens.NewCodexModelPickerState()
	out := screens.RenderCodexModelPicker(state, 0)
	if strings.Contains(out, "Custom") || strings.Contains(out, "Confirm") {
		t.Errorf("RenderCodexModelPicker must not contain 'Custom' or 'Confirm': %s", out)
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
