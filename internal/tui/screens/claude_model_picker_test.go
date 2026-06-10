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

func TestNextAliasIncludesFable(t *testing.T) {
	// Verify fable is reachable and cycles correctly.
	cases := []struct {
		current model.ClaudeModelAlias
		want    model.ClaudeModelAlias
	}{
		{model.ClaudeModelOpus, model.ClaudeModelSonnet},
		{model.ClaudeModelSonnet, model.ClaudeModelHaiku},
		{model.ClaudeModelHaiku, model.ClaudeModelFable},
		{model.ClaudeModelFable, model.ClaudeModelOpus},
		// Unknown aliases fall back to sonnet (nextAlias default).
		{model.ClaudeModelAlias("unknown"), model.ClaudeModelSonnet},
	}
	for _, tc := range cases {
		t.Run(string(tc.current), func(t *testing.T) {
			got := nextAlias(tc.current)
			if got != tc.want {
				t.Errorf("nextAlias(%q) = %q, want %q", tc.current, got, tc.want)
			}
		})
	}
}

func TestAliasTagFable(t *testing.T) {
	// fable should produce a non-empty tag string, not fall through to sonnet default.
	tag := aliasTag(model.ClaudeModelFable)
	if tag == "" {
		t.Error("aliasTag(ClaudeModelFable) returned empty string")
	}
	if strings.Contains(tag, "sonnet") {
		t.Errorf("aliasTag(ClaudeModelFable) = %q, should not fall back to sonnet tag", tag)
	}
}
