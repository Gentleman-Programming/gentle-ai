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

	if state.CustomAssignments["sdd-apply"] == model.ClaudeModelOpus {
		t.Error("CustomAssignments shares memory with the input map — expected a defensive copy")
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
