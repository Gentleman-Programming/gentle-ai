package model_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

func TestClaudeModelAliasValid(t *testing.T) {
	tests := []struct {
		name  string
		input model.ClaudeModelAlias
		want  bool
	}{
		{"opus", model.ClaudeModelOpus, true},
		{"sonnet", model.ClaudeModelSonnet, true},
		{"haiku", model.ClaudeModelHaiku, true},
		{"fable", model.ClaudeModelFable, true},
		{"empty", model.ClaudeModelAlias(""), false},
		{"garbage", model.ClaudeModelAlias("garbage"), false},
		{"uppercase", model.ClaudeModelAlias("FABLE"), false},
		{"partial", model.ClaudeModelAlias("fab"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.Valid(); got != tc.want {
				t.Errorf("ClaudeModelAlias(%q).Valid() = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestClaudeModelAliasString(t *testing.T) {
	tests := []struct {
		alias model.ClaudeModelAlias
		want  string
	}{
		{model.ClaudeModelOpus, "opus"},
		{model.ClaudeModelSonnet, "sonnet"},
		{model.ClaudeModelHaiku, "haiku"},
		{model.ClaudeModelFable, "fable"},
	}
	for _, tc := range tests {
		if got := tc.alias.String(); got != tc.want {
			t.Errorf("ClaudeModelAlias(%q).String() = %q, want %q", tc.alias, got, tc.want)
		}
	}
}

func TestClaudeModelPresetsCoverAllPhasesAndAllValuesAreValid(t *testing.T) {
	presets := []struct {
		name string
		fn   func() map[string]model.ClaudeModelAlias
	}{
		{"Balanced", model.ClaudeModelPresetBalanced},
		{"Performance", model.ClaudeModelPresetPerformance},
		{"Economy", model.ClaudeModelPresetEconomy},
		{"Diversity", model.ClaudeModelPresetDiversity},
	}

	requiredKeys := []string{
		"orchestrator",
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
		"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
	}

	for _, tc := range presets {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.fn()
			for _, k := range requiredKeys {
				v, ok := m[k]
				if !ok {
					t.Errorf("%s preset missing key %q", tc.name, k)
					continue
				}
				if !v.Valid() {
					t.Errorf("%s preset[%q] = %q is not a valid ClaudeModelAlias", tc.name, k, v)
				}
			}
		})
	}
}

func TestClaudeModelPresetsDoNotUseFable(t *testing.T) {
	// Presets should remain unchanged: fable is selectable but not a new default.
	presets := []struct {
		name string
		fn   func() map[string]model.ClaudeModelAlias
	}{
		{"Balanced", model.ClaudeModelPresetBalanced},
		{"Performance", model.ClaudeModelPresetPerformance},
		{"Economy", model.ClaudeModelPresetEconomy},
		{"Diversity", model.ClaudeModelPresetDiversity},
	}
	for _, tc := range presets {
		t.Run(tc.name, func(t *testing.T) {
			for phase, alias := range tc.fn() {
				if alias == model.ClaudeModelFable {
					t.Errorf("%s preset[%q] = %q: presets should not default to fable", tc.name, phase, alias)
				}
			}
		})
	}
}
