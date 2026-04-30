package model

import "testing"

func TestKiroModelID(t *testing.T) {
	tests := []struct {
		alias ClaudeModelAlias
		want  string
	}{
		{ClaudeModelOpus, "claude-opus-4.6"},
		{ClaudeModelSonnet, "claude-sonnet-4.6"},
		{ClaudeModelHaiku, "claude-haiku-4.5"},
		{"unknown", "claude-sonnet-4.6"},
		{"", "claude-sonnet-4.6"},
	}
	for _, tt := range tests {
		if got := KiroModelID(tt.alias); got != tt.want {
			t.Errorf("KiroModelID(%q) = %q, want %q", tt.alias, got, tt.want)
		}
	}
}
