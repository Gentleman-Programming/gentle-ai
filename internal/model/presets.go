package model

// VisualPolishComponents returns the complete managed visual-polish inventory.
// Cleanup flows use this inventory, which intentionally includes the generic theme
// even though presets do not install it.
func VisualPolishComponents() []ComponentID {
	return []ComponentID{ComponentTheme, ComponentClaudeTheme, ComponentOpenCodeGentleLogo}
}

// installSafePresetVisualComponents returns only the agent-specific visual
// components that presets can install without overwriting a generic theme.
func installSafePresetVisualComponents() []ComponentID {
	return []ComponentID{ComponentClaudeTheme, ComponentOpenCodeGentleLogo}
}

// ComponentsForPreset returns the managed components implied by a preset/persona
// pair. PersonaCustom opts out of managed persona only; preset choice still
// controls visual polish.
func ComponentsForPreset(preset PresetID, persona PersonaID) []ComponentID {
	var components []ComponentID
	switch preset {
	case PresetMinimal:
		components = []ComponentID{ComponentEngram}
	case PresetEcosystemOnly:
		components = []ComponentID{ComponentEngram, ComponentSDD, ComponentSkills, ComponentContext7, ComponentGGA}
	case PresetCustom:
		return nil
	default: // full-gentleman
		components = []ComponentID{
			ComponentEngram,
			ComponentSDD,
			ComponentSkills,
			ComponentContext7,
			ComponentPermission,
			ComponentGGA,
		}
		components = append(components, installSafePresetVisualComponents()...)
	}
	if persona != PersonaCustom {
		components = append(components, ComponentPersona)
	}
	return components
}
