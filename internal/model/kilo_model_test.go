package model

import "testing"

func TestKiloModelAliasValid(t *testing.T) {
	tests := []struct {
		alias KiloModelAlias
		want  bool
	}{
		{KiloModelAuto, true},
		{KiloModelSonnet, true},
		{KiloModelOpus, true},
		{KiloModelHaiku, true},
		{KiloModelGateway, true},
		{"unknown", false},
		{"", false},
		{"Sonnet", false},
	}
	for _, tt := range tests {
		if got := tt.alias.Valid(); got != tt.want {
			t.Errorf("KiloModelAlias(%q).Valid() = %v, want %v", tt.alias, got, tt.want)
		}
	}
}

func TestKiloModelID(t *testing.T) {
	tests := []struct {
		alias KiloModelAlias
		want  string
	}{
		{KiloModelAuto, "auto"},
		{KiloModelSonnet, "anthropic/claude-sonnet-4-20250514"},
		{KiloModelOpus, "anthropic/claude-opus-4-20250514"},
		{KiloModelHaiku, "anthropic/claude-haiku-4-20250514"},
		{KiloModelGateway, "gateway/auto"},
		{"unknown", "anthropic/claude-sonnet-4-20250514"},
		{"", "anthropic/claude-sonnet-4-20250514"},
	}
	for _, tt := range tests {
		if got := KiloModelID(tt.alias); got != tt.want {
			t.Errorf("KiloModelID(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}

func TestKiloModelPresetBalancedCompleteness(t *testing.T) {
	preset := KiloModelPresetBalanced()

	// All required SDD phases must have non-empty aliases.
	requiredPhases := []string{
		"orchestrator", "sdd-explore", "sdd-propose", "sdd-spec",
		"sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify",
		"sdd-archive", "sdd-onboard", "default",
	}
	for _, phase := range requiredPhases {
		alias, ok := preset[phase]
		if !ok {
			t.Errorf("KiloModelPresetBalanced() missing phase %q", phase)
			continue
		}
		if !alias.Valid() {
			t.Errorf("KiloModelPresetBalanced()[%q] = %q (invalid alias)", phase, alias)
		}
		if KiloModelID(alias) == "" {
			t.Errorf("KiloModelPresetBalanced()[%q] resolves to empty model ID", phase)
		}
	}
}
