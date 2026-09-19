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
			if slices.Contains(got, ComponentOpenCodeGentleLogo) {
				t.Fatalf("ComponentsForPreset() must not include ComponentOpenCodeGentleLogo: %v", got)
			}
			if !slices.Contains(got, ComponentClaudeTheme) {
				t.Fatalf("ComponentsForPreset() missing ComponentClaudeTheme: %v", got)
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

func TestExistingPresetsDoNotImplyCommunityTools(t *testing.T) {
	for _, preset := range []PresetID{PresetFullGentleman, PresetEcosystemOnly, PresetMinimal, PresetCustom} {
		t.Run(string(preset), func(t *testing.T) {
			selection := Selection{
				Preset:     preset,
				Components: ComponentsForPreset(preset, PersonaGentleman),
			}
			if len(selection.CommunityTools) != 0 {
				t.Fatalf("preset %q community tools = %v, want none", preset, selection.CommunityTools)
			}
		})
	}
}
