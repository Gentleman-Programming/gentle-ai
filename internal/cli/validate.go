package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

type InstallInput struct {
	Selection model.Selection
	Scope     InstallScope
	Channel   InstallChannel
	DryRun    bool
}

func NormalizeInstallFlags(flags InstallFlags, detection system.DetectionResult) (InstallInput, error) {
	selection := model.Selection{}

	agents := defaultAgentsFromDetection(detection)
	if len(flags.Agents) > 0 {
		parsed, err := asAgentIDs(flags.Agents)
		if err != nil {
			return InstallInput{}, err
		}
		agents = parsed
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

	components, err := normalizeComponents(flags.Components, selection.Preset, selection.Persona)
	if err != nil {
		return InstallInput{}, err
	}
	if len(flags.Components) == 0 && strings.TrimSpace(flags.Preset) == "" && isPiOnlyAgents(selection.Agents) {
		components = piOnlyComponents()
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

	scope, err := ResolveInstallScope(flags.Scope)
	if err != nil {
		return InstallInput{}, err
	}

	channel, err := ResolveInstallChannel(flags.Channel)
	if err != nil {
		return InstallInput{}, err
	}

	return InstallInput{Selection: selection, Scope: scope, Channel: channel, DryRun: flags.DryRun}, nil
}

func normalizePersona(value string) (model.PersonaID, error) {
	if strings.TrimSpace(value) == "" {
		return model.PersonaGentleman, nil
	}

	switch model.PersonaID(value) {
	case model.PersonaGentleman, model.PersonaGentlemanNeutralArtifacts, model.PersonaNeutral, model.PersonaCustom:
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

func normalizeComponents(values []string, preset model.PresetID, persona model.PersonaID) ([]model.ComponentID, error) {
	if len(values) == 0 {
		return componentsForPreset(preset, persona), nil
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

func componentsForPreset(preset model.PresetID, persona model.PersonaID) []model.ComponentID {
	return model.ComponentsForPreset(preset, persona)
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
		case string(model.AgentKimi):
			agents = append(agents, model.AgentKimi)
		case string(model.AgentQwenCode):
			agents = append(agents, model.AgentQwenCode)
		case string(model.AgentKiroIDE):
			agents = append(agents, model.AgentKiroIDE)
		case string(model.AgentOpenClaw):
			agents = append(agents, model.AgentOpenClaw)
		case string(model.AgentPi):
			agents = append(agents, model.AgentPi)
		case string(model.AgentTrae):
			agents = append(agents, model.AgentTrae)
		case string(model.AgentHermes):
			agents = append(agents, model.AgentHermes)
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

// asAgentIDs converts raw --agent/--agents flag values into model.AgentID,
// rejecting any value that is not a real, supported agent. The valid set is
// derived from catalog.AllAgents() -- the same canonical agent registry used
// by internal/app/app.go's default agent list -- so it can never drift from
// a hand-written list (install/sync surface audit finding 3: an unknown
// value like `cluade` previously converted silently and was later dropped
// without any error, so `gentle-ai sync --agent cluade` reported success
// having synced nothing).
func asAgentIDs(values []string) ([]model.AgentID, error) {
	supported := catalog.AllAgents()
	allowed := make(map[model.AgentID]struct{}, len(supported))
	names := make([]string, 0, len(supported))
	for _, agent := range supported {
		allowed[agent.ID] = struct{}{}
		names = append(names, string(agent.ID))
	}
	sort.Strings(names)

	agents := make([]model.AgentID, 0, len(values))
	for _, value := range values {
		id := model.AgentID(value)
		if _, ok := allowed[id]; !ok {
			return nil, fmt.Errorf("unsupported agent %q (valid: %s)", value, strings.Join(names, ", "))
		}
		agents = append(agents, id)
	}

	return agents, nil
}

func isPiOnlyAgents(agents []model.AgentID) bool {
	return len(agents) == 1 && agents[0] == model.AgentPi
}

func piOnlyComponents() []model.ComponentID {
	return []model.ComponentID{model.ComponentEngram}
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
