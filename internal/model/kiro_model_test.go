package model

import "testing"

func TestKiroModelID(t *testing.T) {
	tests := []struct {
		alias KiroModelAlias
		want  string
	}{
		{KiroModelAuto, "auto"},
		{KiroModelOpus, "claude-opus-4.8"},
		{KiroModelSonnet, "claude-sonnet-4.6"},
		{KiroModelHaiku, "claude-haiku-4.5"},
		{KiroModelMiniMax, "minimax-m2.5"},
		{KiroModelGLM, "glm-5"},
		{KiroModelDeepSeek, "deepseek-3.2"},
		{KiroModelQwen, "qwen3-coder-next"},
		{"unknown", "claude-sonnet-4.6"},
		{"", "claude-sonnet-4.6"},
	}
	for _, tt := range tests {
		if got := KiroModelID(tt.alias); got != tt.want {
			t.Errorf("KiroModelID(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}

func TestKiroModelPresetsUseActiveRoles(t *testing.T) {
	presets := []struct {
		name   string
		values map[string]KiroModelAlias
	}{
		{"balanced", KiroModelPresetBalanced()},
		{"performance", KiroModelPresetPerformance()},
		{"economy", KiroModelPresetEconomy()},
		{"open-weight", KiroModelPresetOpenWeight()},
	}
	for _, preset := range presets {
		t.Run(preset.name, func(t *testing.T) {
			for _, role := range []string{"odd-explorer", "odd-worker", "odd-verify", "risk", "readability", "reliability", "resilience", "refuter", "validator", "jd-judge-a", "default"} {
				if !preset.values[role].Valid() {
					t.Errorf("missing or invalid model for %s", role)
				}
			}
			for role := range preset.values {
				if len(role) >= 4 && role[:4] == "sdd-" {
					t.Errorf("retired role %s in preset", role)
				}
			}
		})
	}
	if KiroModelPresetBalanced()["odd-explorer"] != KiroModelAuto || KiroModelPresetPerformance()["odd-explorer"] != KiroModelSonnet || KiroModelPresetEconomy()["odd-explorer"] != KiroModelQwen {
		t.Fatal("Kiro presets lost their distinct model strategies")
	}
}
