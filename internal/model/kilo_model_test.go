package model

import "testing"

func TestKiloModelAliasValid(t *testing.T) {
	// Valid() always returns true — unknown aliases pass through as model IDs.
	tests := []struct {
		alias KiloModelAlias
	}{
		{KiloModelAuto},
		{KiloModelSonnet},
		{KiloModelOpus},
		{KiloModelHaiku},
		{KiloModelGateway},
		{"unknown"},
		{""},
		{"Sonnet"},
		{"openai/gpt-4o"},
		{"meta-llama/llama-4-maverick"},
	}
	for _, tt := range tests {
		if got := tt.alias.Valid(); !got {
			t.Errorf("KiloModelAlias(%q).Valid() = %v, want true", tt.alias, got)
		}
	}
}

func TestKiloModelID(t *testing.T) {
	tests := []struct {
		alias KiloModelAlias
		want  string
	}{
		// Kilo Gateway
		{KiloModelAuto, "kilo/kilo-auto/free"},
		{KiloModelGateway, "gateway/auto"},
		// Anthropic
		{KiloModelOpus, "anthropic/claude-opus-4-20250514"},
		{KiloModelSonnet, "anthropic/claude-sonnet-4-20250514"},
		{KiloModelHaiku, "anthropic/claude-haiku-4-20250514"},
		{KiloModelSonnet4, "anthropic/claude-sonnet-4-20250514"},
		{KiloModelOpus4, "anthropic/claude-opus-4-20250514"},
		// OpenAI
		{KiloModelGPT4o, "openai/gpt-4o"},
		{KiloModelGPT4oMini, "openai/gpt-4o-mini"},
		{KiloModelO1, "openai/o1"},
		{KiloModelO3, "openai/o3"},
		{KiloModelO3Mini, "openai/o3-mini"},
		{KiloModelO4Mini, "openai/o4-mini"},
		// Google
		{KiloModelGemini25Pro, "google/gemini-2.5-pro"},
		{KiloModelGemini25Flash, "google/gemini-2.5-flash"},
		{KiloModelGemini20Flash, "google/gemini-2.0-flash"},
		// Open-weight
		{KiloModelDeepSeek, "deepseek/deepseek-chat"},
		{KiloModelDeepSeekR1, "deepseek/deepseek-reasoner"},
		{KiloModelQwen, "qwen/qwen-2.5-72b-instruct"},
		{KiloModelQwen3, "qwen/qwen3-235b-a22b"},
		{KiloModelLlama, "meta-llama/llama-4-maverick"},
		{KiloModelMistral, "mistral/mistral-large-latest"},
	}
	for _, tt := range tests {
		if got := KiloModelID(tt.alias); got != tt.want {
			t.Errorf("KiloModelID(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}

func TestKiloModelIDPassThrough(t *testing.T) {
	// Unknown aliases pass through as-is — this is the custom model ID feature.
	tests := []struct {
		alias KiloModelAlias
		want  string
	}{
		{"openai/gpt-4o", "openai/gpt-4o"},
		{"anthropic/claude-sonnet-4-20250514", "anthropic/claude-sonnet-4-20250514"},
		{"meta-llama/llama-4-maverick", "meta-llama/llama-4-maverick"},
		{"deepseek/deepseek-v3", "deepseek/deepseek-v3"},
		{"custom/my-model", "custom/my-model"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := KiloModelID(tt.alias); got != tt.want {
			t.Errorf("KiloModelID(%q) = %q, want %q (pass-through)", tt.alias, got, tt.want)
		}
	}
}

func TestIsKnownKiloAlias(t *testing.T) {
	if !IsKnownKiloAlias(KiloModelAuto) {
		t.Error("IsKnownKiloAlias(auto) = false, want true")
	}
	if !IsKnownKiloAlias(KiloModelGPT4o) {
		t.Error("IsKnownKiloAlias(gpt-4o) = false, want true")
	}
	if !IsKnownKiloAlias(KiloModelGemini25Pro) {
		t.Error("IsKnownKiloAlias(gemini-2.5-pro) = false, want true")
	}
	if IsKnownKiloAlias("openai/gpt-4o") {
		t.Error("IsKnownKiloAlias(openai/gpt-4o) = true, want false (custom pass-through)")
	}
	if IsKnownKiloAlias("") {
		t.Error("IsKnownKiloAlias(\"\") = true, want false")
	}
}

func TestKnownKiloAliasesLength(t *testing.T) {
	// Ensure the aliases slice and labels map are in sync.
	if len(KnownKiloAliases) != len(KnownKiloModelLabels) {
		t.Errorf("KnownKiloAliases length %d != KnownKiloModelLabels length %d",
			len(KnownKiloAliases), len(KnownKiloModelLabels))
	}
	for _, alias := range KnownKiloAliases {
		if _, ok := KnownKiloModelLabels[alias]; !ok {
			t.Errorf("KnownKiloAliases contains %q but KnownKiloModelLabels does not", alias)
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
		if KiloModelID(alias) == "" {
			t.Errorf("KiloModelPresetBalanced()[%q] resolves to empty model ID", phase)
		}
	}
}
