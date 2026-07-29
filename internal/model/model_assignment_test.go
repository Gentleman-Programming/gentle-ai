package model

import "testing"

// TestModelAssignment_EffortZeroValue verifies that a ModelAssignment constructed
// without setting Effort has an empty string as its zero value.
func TestModelAssignment_EffortZeroValue(t *testing.T) {
	a := ModelAssignment{ProviderID: "anthropic", ModelID: "claude-sonnet-4"}
	if a.Effort != "" {
		t.Errorf("ModelAssignment.Effort zero value = %q, want %q", a.Effort, "")
	}
}

// TestModelAssignment_FullIDUnaffectedByEffort verifies that FullID() is not
// changed by the presence of an Effort value.
func TestModelAssignment_FullIDUnaffectedByEffort(t *testing.T) {
	a := ModelAssignment{ProviderID: "anthropic", ModelID: "claude-opus-4", Effort: "high"}
	want := "anthropic/claude-opus-4"
	if a.FullID() != want {
		t.Errorf("FullID() = %q, want %q", a.FullID(), want)
	}
}

func TestSplitModelSpec(t *testing.T) {
	tests := []struct {
		name         string
		spec         string
		wantProvider string
		wantModel    string
		wantOK       bool
	}{
		{name: "slash separator", spec: "anthropic/claude-sonnet-4", wantProvider: "anthropic", wantModel: "claude-sonnet-4", wantOK: true},
		{name: "colon separator", spec: "anthropic:claude-sonnet-4", wantProvider: "anthropic", wantModel: "claude-sonnet-4", wantOK: true},
		{name: "first separator wins", spec: "openrouter/qwen/qwen3.6-plus:free", wantProvider: "openrouter", wantModel: "qwen/qwen3.6-plus:free", wantOK: true},
		{name: "missing separator", spec: "claude-sonnet-4"},
		{name: "empty provider", spec: "/claude-sonnet-4"},
		{name: "empty model", spec: "anthropic/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, modelID, ok := SplitModelSpec(tt.spec)
			if provider != tt.wantProvider || modelID != tt.wantModel || ok != tt.wantOK {
				t.Fatalf("SplitModelSpec(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.spec, provider, modelID, ok, tt.wantProvider, tt.wantModel, tt.wantOK)
			}
		})
	}
}
