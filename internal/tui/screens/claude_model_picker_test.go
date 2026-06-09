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

// TestNextAliasCyclesThroughAllAliases verifies the fable → opus → sonnet →
// haiku cycle order and the sonnet fallback for unknown aliases.
func TestNextAliasCyclesThroughAllAliases(t *testing.T) {
	cases := []struct {
		name    string
		current model.ClaudeModelAlias
		want    model.ClaudeModelAlias
	}{
		{"fable → opus", model.ClaudeModelFable, model.ClaudeModelOpus},
		{"opus → sonnet", model.ClaudeModelOpus, model.ClaudeModelSonnet},
		{"sonnet → haiku", model.ClaudeModelSonnet, model.ClaudeModelHaiku},
		{"haiku → fable", model.ClaudeModelHaiku, model.ClaudeModelFable},
		{"unknown falls back to sonnet", model.ClaudeModelAlias("bogus"), model.ClaudeModelSonnet},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextAlias(tc.current); got != tc.want {
				t.Errorf("nextAlias(%q) = %q, want %q", tc.current, got, tc.want)
			}
		})
	}
}

// TestHandleCustomPhaseNav_CycleReachesEveryAlias drives the picker's nav
// handler through a full cycle on one phase and verifies every alias is
// reachable and the cycle returns to its starting alias.
func TestHandleCustomPhaseNav_CycleReachesEveryAlias(t *testing.T) {
	state := NewClaudeModelPickerState()
	state.InCustomMode = true
	phase := claudePhases[0]
	start := state.CustomAssignments[phase]

	seen := map[model.ClaudeModelAlias]bool{start: true}
	for i := 0; i < len(claudeAliasOrder); i++ {
		handled, assignments := HandleClaudeModelPickerNav("enter", &state, 0)
		if !handled {
			t.Fatal("enter on a phase row should be handled")
		}
		if assignments != nil {
			t.Fatal("cycling a phase should not confirm the screen")
		}
		seen[state.CustomAssignments[phase]] = true
	}

	for _, alias := range claudeAliasOrder {
		if !seen[alias] {
			t.Errorf("cycling never reached alias %q", alias)
		}
	}
	if state.CustomAssignments[phase] != start {
		t.Errorf("after a full cycle, alias = %q, want starting alias %q", state.CustomAssignments[phase], start)
	}
}

// TestRenderClaudeModelPicker_CustomModeRendersFable verifies that custom
// mode renders the [fable] badge for a fable-assigned phase and that the help
// text advertises the four-alias cycle.
func TestRenderClaudeModelPicker_CustomModeRendersFable(t *testing.T) {
	state := NewClaudeModelPickerStateFromAssignments(map[string]model.ClaudeModelAlias{
		"sdd-propose": model.ClaudeModelFable,
	})
	state.InCustomMode = true

	out := RenderClaudeModelPicker(state, 0)
	if !strings.Contains(out, "[fable]") {
		t.Errorf("expected [fable] tag in custom mode render, got:\n%s", out)
	}
	if !strings.Contains(out, "fable → opus → sonnet → haiku") {
		t.Errorf("expected cycle help text to include fable, got:\n%s", out)
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
