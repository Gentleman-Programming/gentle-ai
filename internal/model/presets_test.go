package model

import (
	"slices"
	"testing"
)

func TestComponentsForPresetFullGentlemanUsesInstallSafeVisualInventory(t *testing.T) {
	tests := []struct {
		name    string
		persona PersonaID
	}{
		{name: "gentleman persona", persona: PersonaGentleman},
		{name: "custom persona", persona: PersonaCustom},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComponentsForPreset(PresetFullGentleman, tt.persona)

			if slices.Contains(got, ComponentTheme) {
				t.Fatalf("ComponentsForPreset() includes generic ComponentTheme: %v", got)
			}
			for _, want := range []ComponentID{ComponentClaudeTheme, ComponentOpenCodeGentleLogo} {
				if !slices.Contains(got, want) {
					t.Errorf("ComponentsForPreset() missing safe visual component %q: %v", want, got)
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
