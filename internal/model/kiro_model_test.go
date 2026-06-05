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
