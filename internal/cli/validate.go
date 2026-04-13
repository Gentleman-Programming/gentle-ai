package cli

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/system"
)

type InstallInput struct {
	Selection model.Selection
	DryRun    bool
}

func NormalizeInstallFlags(flags InstallFlags, detection system.DetectionResult) (InstallInput, error) {
	selection := model.Selection{}

	agents := defaultAgentsFromDetection(detection)
	if len(flags.Agents) > 0 {
		agents = asAgentIDs(flags.Agents)
	}
	selection.Agents = unique(agents)

	persona, err := normalizePersona(flags.Persona)
	if err != nil {
		return InstallInput{}, err
	}
	selection.Persona = persona

	preset, err := normalizePreset(flags.Preset)
	if err != nil {
		return InstallInput{}, err
	}
	selection.Preset = preset

	components, err := normalizeComponents(flags.Components, selection.Preset)
	if err != nil {
		return InstallInput{}, err
	}
	selection.Components = components

	skills, err := normalizeSkills(flags.Skills)
	if err != nil {
		return InstallInput{}, err
	}
	selection.Skills = skills

	sddMode, err := normalizeSDDMode(flags.SDDMode)
	if err != nil {
		return InstallInput{}, err
	}
	selection.SDDMode = sddMode

	return InstallInput{Selection: selection, DryRun: flags.DryRun}, nil
}

func normalizePersona(value string) (model.PersonaID, error) {
	if strings.TrimSpace(value) == "" {
		return model.PersonaGentleman, nil
	}

	switch model.PersonaID(value) {
	case model.PersonaGentleman, model.PersonaNeutral, model.PersonaCustom:
		return model.PersonaID(value), nil
	default:
		return "", fmt.Errorf("unsupported persona %q", value)
	}
}

func normalizePreset(value string) (model.PresetID, error) {
	if strings.TrimSpace(value) == "" {
		return model.PresetFullGentleman, nil
	}

	switch model.PresetID(value) {
	case model.PresetFullGentleman, model.PresetEcosystemOnly, model.PresetMinimal, model.PresetCustom:
		return model.PresetID(value), nil
	default:
		return "", fmt.Errorf("unsupported preset %q", value)
	}
}

func normalizeComponents(values []string, preset model.PresetID) ([]model.ComponentID, error) {
	if len(values) == 0 {
		return componentsForPreset(preset), nil
	}

	allowed := map[model.ComponentID]struct{}{}
	for _, component := range catalog.MVPComponents() {
		allowed[component.ID] = struct{}{}
	}

	components := []model.ComponentID{}
	for _, raw := range values {
		component := model.ComponentID(raw)
		if _, ok := allowed[component]; !ok {
			return nil, fmt.Errorf("unsupported component %q", raw)
		}
		components = append(components, component)
	}

	return unique(components), nil
}

func normalizeSkills(values []string) ([]model.SkillID, error) {
	if len(values) == 0 {
		return nil, nil
	}

	allowed := map[model.SkillID]struct{}{}
	for _, skill := range catalog.MVPSkills() {
		allowed[skill.ID] = struct{}{}
	}

	skills := []model.SkillID{}
	for _, raw := range values {
		skill := model.SkillID(raw)
		if _, ok := allowed[skill]; !ok {
			return nil, fmt.Errorf("unsupported skill %q", raw)
		}
		skills = append(skills, skill)
	}

	return unique(skills), nil
}

func normalizeSDDMode(value string) (model.SDDModeID, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}

	switch model.SDDModeID(value) {
	case model.SDDModeSingle, model.SDDModeMulti:
		return model.SDDModeID(value), nil
	default:
		return "", fmt.Errorf("unsupported sdd-mode %q (valid: single, multi)", value)
	}
}

func componentsForPreset(preset model.PresetID) []model.ComponentID {
	switch preset {
	case model.PresetMinimal:
		return []model.ComponentID{model.ComponentEngram}
	case model.PresetEcosystemOnly:
		return []model.ComponentID{model.ComponentEngram, model.ComponentSDD, model.ComponentSkills, model.ComponentContext7, model.ComponentGGA}
	case model.PresetCustom:
		return nil
	default:
		return []model.ComponentID{
			model.ComponentEngram,
			model.ComponentSDD,
			model.ComponentSkills,
			model.ComponentContext7,
			model.ComponentPersona,
			model.ComponentPermission,
			model.ComponentGGA,
		}
	}
}

func defaultAgentsFromDetection(detection system.DetectionResult) []model.AgentID {
	agents := []model.AgentID{}
	for _, state := range detection.Configs {
		if !state.Exists {
			continue
		}

		switch strings.TrimSpace(state.Agent) {
		case string(model.AgentClaudeCode):
			agents = append(agents, model.AgentClaudeCode)
		case string(model.AgentOpenCode):
			agents = append(agents, model.AgentOpenCode)
		case string(model.AgentKilocode):
			agents = append(agents, model.AgentKilocode)
		case string(model.AgentGeminiCLI):
			agents = append(agents, model.AgentGeminiCLI)
		case string(model.AgentCursor):
			agents = append(agents, model.AgentCursor)
		case string(model.AgentVSCodeCopilot):
			agents = append(agents, model.AgentVSCodeCopilot)
		case string(model.AgentCodex):
			agents = append(agents, model.AgentCodex)
		case string(model.AgentAntigravity):
			agents = append(agents, model.AgentAntigravity)
		case string(model.AgentWindsurf):
			agents = append(agents, model.AgentWindsurf)
		case string(model.AgentQwenCode):
			agents = append(agents, model.AgentQwenCode)
		case string(model.AgentKiroIDE):
			agents = append(agents, model.AgentKiroIDE)
		}
	}

	if len(agents) > 0 {
		return agents
	}

	catalogAgents := catalog.AllAgents()
	agents = make([]model.AgentID, 0, len(catalogAgents))
	for _, agent := range catalogAgents {
		agents = append(agents, agent.ID)
	}

	return agents
}

func asAgentIDs(values []string) []model.AgentID {
	agents := make([]model.AgentID, 0, len(values))
	for _, value := range values {
		agents = append(agents, model.AgentID(value))
	}

	return agents
}

func unique[T comparable](items []T) []T {
	seen := make(map[T]struct{}, len(items))
	result := make([]T, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}

	return result
}
