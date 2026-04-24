package model

import "testing"

func TestSplitProviderModel(t *testing.T) {
	tests := []struct {
		name       string
		spec       string
		wantProv   string
		wantModel  string
		wantOK     bool
	}{
		{
			name:      "colon separator",
			spec:      "anthropic:claude-sonnet-4-20250514",
			wantProv:  "anthropic",
			wantModel: "claude-sonnet-4-20250514",
			wantOK:    true,
		},
		{
			name:      "slash separator",
			spec:      "zai-coding-plan/glm-5-turbo",
			wantProv:  "zai-coding-plan",
			wantModel: "glm-5-turbo",
			wantOK:    true,
		},
		// Regression #260: OpenRouter free models contain BOTH '/' and ':'.
		// The provider is the first segment; the rest (slashes and colons
		// included) is the model. Splitting on ':' first would mis-attribute
		// "openrouter/qwen/qwen3.6-plus" to the provider.
		{
			name:      "openrouter free suffix — slash before colon",
			spec:      "openrouter/qwen/qwen3.6-plus:free",
			wantProv:  "openrouter",
			wantModel: "qwen/qwen3.6-plus:free",
			wantOK:    true,
		},
		{
			name:      "openrouter paid — multiple slashes, no colon",
			spec:      "openrouter/anthropic/claude-sonnet-4",
			wantProv:  "openrouter",
			wantModel: "anthropic/claude-sonnet-4",
			wantOK:    true,
		},
		{
			name:      "colon appears before slash in model id",
			spec:      "provider:model/variant",
			wantProv:  "provider",
			wantModel: "model/variant",
			wantOK:    true,
		},
		{
			name:   "empty spec",
			spec:   "",
			wantOK: false,
		},
		{
			name:   "no separator",
			spec:   "just-a-string",
			wantOK: false,
		},
		{
			name:   "leading slash",
			spec:   "/model-id",
			wantOK: false,
		},
		{
			name:   "leading colon",
			spec:   ":model-id",
			wantOK: false,
		},
		{
			name:   "trailing slash with empty model",
			spec:   "provider/",
			wantOK: false,
		},
		{
			name:   "trailing colon with empty model",
			spec:   "provider:",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProv, gotModel, gotOK := SplitProviderModel(tt.spec)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if gotProv != tt.wantProv {
				t.Errorf("providerID = %q, want %q", gotProv, tt.wantProv)
			}
			if gotModel != tt.wantModel {
				t.Errorf("modelID = %q, want %q", gotModel, tt.wantModel)
			}
		})
	}
}
