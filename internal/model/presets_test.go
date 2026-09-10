package model

import (
	"slices"
	"testing"
)

func TestComponentsForPresetFullGentlemanThemeFollowsOpenCodeSelection(t *testing.T) {
	tests := []struct {
		name      string
		persona   PersonaID
		agents    []AgentID
		wantTheme bool
	}{
		{name: "OpenCode gentleman persona", persona: PersonaGentleman, agents: []AgentID{AgentOpenCode}, wantTheme: true},
		{name: "OpenCode custom persona", persona: PersonaCustom, agents: []AgentID{AgentOpenCode}, wantTheme: true},
		{name: "non-OpenCode", persona: PersonaGentleman, agents: []AgentID{AgentClaudeCode}, wantTheme: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComponentsForPreset(PresetFullGentleman, tt.persona, tt.agents...)

			if themeIncluded := slices.Contains(got, ComponentTheme); themeIncluded != tt.wantTheme {
				t.Fatalf("ComponentsForPreset() theme included = %v, want %v: %v", themeIncluded, tt.wantTheme, got)
			}
			for _, want := range []ComponentID{ComponentClaudeTheme, ComponentOpenCodeGentleLogo} {
				if !slices.Contains(got, want) {
					t.Errorf("ComponentsForPreset() missing visual component %q: %v", want, got)
				}
			}
		})
	}
}

func TestVisualPolishComponentsReturnsCompleteManagedCleanupInventory(t *testing.T) {
	want := []ComponentID{ComponentTheme, ComponentClaudeTheme, ComponentOpenCodeGentleLogo}
	if got := VisualPolishComponents(); !slices.Equal(got, want) {
		t.Fatalf("VisualPolishComponents() = %v, want complete cleanup inventory %v", got, want)
	}
}
