package model_test

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// TestClaudeModelAliasValid verifies that Valid accepts exactly the four
// known aliases and rejects empty, unknown, uppercase, and full-model-ID inputs.
func TestClaudeModelAliasValid(t *testing.T) {
	tests := []struct {
		name  string
		input model.ClaudeModelAlias
		want  bool
	}{
		{"fable", model.ClaudeModelFable, true},
		{"opus", model.ClaudeModelOpus, true},
		{"sonnet", model.ClaudeModelSonnet, true},
		{"haiku", model.ClaudeModelHaiku, true},
		{"empty", model.ClaudeModelAlias(""), false},
		{"junk", model.ClaudeModelAlias("junk"), false},
		{"uppercase", model.ClaudeModelAlias("FABLE"), false},
		{"full model id", model.ClaudeModelAlias("claude-fable-5"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.Valid(); got != tc.want {
				t.Errorf("ClaudeModelAlias(%q).Valid() = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestClaudeModelAliasString verifies that the fable alias renders as its
// literal string value.
func TestClaudeModelAliasString(t *testing.T) {
	if got := model.ClaudeModelFable.String(); got != "fable" {
		t.Errorf("ClaudeModelFable.String() = %q, want %q", got, "fable")
	}
}

// TestClaudeModelPresetsContainOnlyValidAliases verifies that every preset
// assigns a valid alias to all 14 phase keys, guarding against preset drift
// when new aliases are introduced.
func TestClaudeModelPresetsContainOnlyValidAliases(t *testing.T) {
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
			m := tc.fn()
			if len(m) != 14 {
				t.Errorf("%s preset has %d keys, want 14", tc.name, len(m))
			}
			for k, v := range m {
				if !v.Valid() {
					t.Errorf("%s preset[%q] = %q is not a valid ClaudeModelAlias", tc.name, k, v)
				}
			}
		})
	}
}
