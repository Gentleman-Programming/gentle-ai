package model

import "testing"

func TestModelCapability(t *testing.T) {
	tests := []struct {
		modelID       string
		wantCapability string
	}{
		{"gemini-3-flash", "small"},
		{"gpt-4o-mini", "small"},
		{"claude-haiku", "small"},
		{"claude-sonnet-4", "capable"},
		{"gpt-4o", "capable"},
		{"", "capable"},
		{"GEMINI-3-FLASH", "small"},   // case-insensitive
		{"GPT-4O-MINI", "small"},      // case-insensitive
		{"Claude-Haiku", "small"},     // case-insensitive
		{"anthropic/claude-haiku-3-5", "small"},
		{"openai/gpt-4o-mini-2024-07-18", "small"},
		{"google/gemini-2.0-flash", "small"},
		{"claude-opus-4", "capable"},
		{"gpt-4-turbo", "capable"},
		{"claude-sonnet-4-20250514", "capable"},
		{"qwen3-30b-a3b", "small"},
		{"qwen3-70b", "capable"},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			got := ModelCapability(tt.modelID)
			if got != tt.wantCapability {
				t.Errorf("ModelCapability(%q) = %q, want %q", tt.modelID, got, tt.wantCapability)
			}
		})
	}
}