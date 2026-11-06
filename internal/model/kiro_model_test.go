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
		{KiroModelHaiku, "claude-haiku-5.5"},
		{KiroModelMiniMax, "minimax-m2.5"},
		{KiroModelGLM, "glm-5"},
		{KiroModelDeepSeek, "deepseek-3.2"},
		{KiroModelQwen, "qwen3-coder-next"},
		// Unknown aliases, including GPT-5.6 names the picker no longer offers,
		// resolve to auto instead of pinning an unavailable model.
		{"luna", "auto"},
		{"terra", "auto"},
		{"sol", "auto"},
		{"unknown", "auto"},
		{"", "auto"},
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
		haiku    = KiroModelHaiku
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
			auto, haiku, haiku, sonnet,
			sonnet, haiku, sonnet,
			sonnet, haiku, sonnet, sonnet, sonnet, sonnet,
			auto,
		}},
		{"performance", KiroModelPresetPerformance(), []KiroModelAlias{
			opus, sonnet, sonnet, opus,
			opus, opus, sonnet,
			opus, sonnet, opus, opus, opus, opus,
			sonnet,
		}},
		{"economy", KiroModelPresetEconomy(), []KiroModelAlias{
			auto, haiku, haiku, haiku,
			haiku, haiku, haiku,
			haiku, haiku, haiku, haiku, haiku, haiku,
			haiku,
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

func TestKiroModelAliasValidRejectsRetiredGPTAliases(t *testing.T) {
	for _, alias := range []KiroModelAlias{"luna", "terra", "sol"} {
		if alias.Valid() {
			t.Errorf("KiroModelAlias(%q).Valid() = true, want false", alias)
		}
	}
	for _, alias := range []KiroModelAlias{KiroModelAuto, KiroModelOpus, KiroModelSonnet, KiroModelHaiku, KiroModelMiniMax, KiroModelGLM, KiroModelDeepSeek, KiroModelQwen} {
		if !alias.Valid() {
			t.Errorf("KiroModelAlias(%q).Valid() = false, want true", alias)
		}
	}
}

// TestKiroModelPresetPerformancePinsJudgmentDayModels guards against the
// performance preset degrading to Auto: every Judgment Day role must write a
// concrete Kiro model ID.
func TestKiroModelPresetPerformancePinsJudgmentDayModels(t *testing.T) {
	preset := KiroModelPresetPerformance()
	for _, role := range []string{"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default"} {
		if id := KiroModelID(preset[role]); id == "auto" {
			t.Errorf("performance %s resolves to %q, want a pinned model ID", role, id)
		}
	}
}
