package model

import "testing"

func TestKiroModelID(t *testing.T) {
	tests := []struct {
		alias KiroModelAlias
		want  string
	}{
		{KiroModelAuto, "auto"},
		{KiroModelOpus, "claude-opus-5.5"},
		{KiroModelSonnet, "claude-sonnet-5.5"},
		{KiroModelHaiku, "claude-haiku-4.5"},
		{KiroModelLuna, "gpt-5.6-luna"},
		{KiroModelTerra, "gpt-5.6-terra"},
		{KiroModelSol, "gpt-5.6-sol"},
		{KiroModelMiniMax, "minimax-m2.5"},
		{KiroModelGLM, "glm-5"},
		{KiroModelDeepSeek, "deepseek-3.2"},
		{KiroModelQwen, "qwen3-coder-next"},
		{"unknown", "claude-sonnet-5.5"},
		{"", "claude-sonnet-5.5"},
	}
	for _, tt := range tests {
		if got := KiroModelID(tt.alias); got != tt.want {
			t.Errorf("KiroModelID(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}

func TestKiroModelPresetAssignments(t *testing.T) {
	roles := []string{
		"orchestrator", "odd-explorer", "odd-worker", "odd-verify",
		"jd-judge-a", "jd-judge-b", "jd-fix-agent",
		"risk", "readability", "reliability", "resilience", "refuter", "validator",
		"default",
	}
	const (
		auto     = KiroModelAuto
		opus     = KiroModelOpus
		sonnet   = KiroModelSonnet
		luna     = KiroModelLuna
		minimax  = KiroModelMiniMax
		glm      = KiroModelGLM
		deepseek = KiroModelDeepSeek
		qwen     = KiroModelQwen
	)
	presets := []struct {
		name string
		got  map[string]KiroModelAlias
		want []KiroModelAlias // same order as roles
	}{
		{"balanced", KiroModelPresetBalanced(), []KiroModelAlias{
			auto, luna, luna, sonnet,
			sonnet, luna, sonnet,
			sonnet, luna, sonnet, sonnet, sonnet, sonnet,
			auto,
		}},
		{"performance", KiroModelPresetPerformance(), []KiroModelAlias{
			opus, sonnet, sonnet, opus,
			opus, opus, sonnet,
			opus, sonnet, opus, opus, opus, opus,
			sonnet,
		}},
		{"economy", KiroModelPresetEconomy(), []KiroModelAlias{
			auto, luna, luna, luna,
			luna, luna, luna,
			luna, luna, minimax, minimax, luna, luna,
			luna,
		}},
		{"open-weight", KiroModelPresetOpenWeight(), []KiroModelAlias{
			glm, qwen, qwen, minimax,
			glm, qwen, qwen,
			glm, qwen, minimax, minimax, deepseek, deepseek,
			qwen,
		}},
	}
	for _, preset := range presets {
		t.Run(preset.name, func(t *testing.T) {
			if len(preset.got) != len(roles) {
				t.Errorf("preset has %d roles, want %d: %v", len(preset.got), len(roles), preset.got)
			}
			for i, role := range roles {
				if got := preset.got[role]; got != preset.want[i] {
					t.Errorf("%s = %q, want %q", role, got, preset.want[i])
				}
			}
		})
	}
}
