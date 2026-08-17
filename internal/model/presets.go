package model

// VisualPolishComponents returns the complete managed visual-polish inventory.
// Cleanup flows use the full inventory; preset installs filter theme by agent.
func VisualPolishComponents() []ComponentID {
	return []ComponentID{ComponentTheme, ComponentClaudeTheme, ComponentOpenCodeGentleLogo}
}

// installSafePresetVisualComponents returns visual components that do not depend
// on the selected agent.
func installSafePresetVisualComponents() []ComponentID {
	return []ComponentID{ComponentClaudeTheme, ComponentOpenCodeGentleLogo}
}

// ComponentsForPreset returns the managed components implied by a preset/persona
// pair. PersonaCustom opts out of managed persona only; preset choice still
// controls visual polish.
func ComponentsForPreset(preset PresetID, persona PersonaID, agents ...AgentID) []ComponentID {
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
		for _, agent := range agents {
			if agent == AgentOpenCode {
				components = append(components, ComponentTheme)
				break
			}
		}
		components = append(components, installSafePresetVisualComponents()...)
	}
	if persona != PersonaCustom {
		components = append(components, ComponentPersona)
	}
	return components
}
