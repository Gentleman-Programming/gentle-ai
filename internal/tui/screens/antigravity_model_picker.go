package screens

import (
	"fmt"
	"maps"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// AntigravityModelPreset represents a named tier configuration preset for Antigravity subagents.
type AntigravityModelPreset string

const (
	AntigravityPresetInherit     AntigravityModelPreset = "inherit"
	AntigravityPresetPerformance AntigravityModelPreset = "performance"
	AntigravityPresetEconomy     AntigravityModelPreset = "economy"
	AntigravityPresetCustom      AntigravityModelPreset = "custom"
)

var antigravityPresetDescriptions = map[AntigravityModelPreset]string{
	AntigravityPresetInherit:     "Subagents use the session model (agy default)",
	AntigravityPresetPerformance: "pro tier for workers, verifiers, judges, and fix agents; flash for explorer",
	AntigravityPresetEconomy:     "flash workers, verifiers, and judges; flash_lite for explorer and fixes",
	AntigravityPresetCustom:      "Pick the agy tier for each ODD and Judgment Day role individually",
}

var antigravityRoleLabels = map[string]string{
	"gentle-ai-explore": "ODD Explorer",
	"gentle-ai-worker":  "ODD Worker",
	"gentle-ai-verify":  "ODD Verify",
	"jd-judge-a":        "JD Judge A",
	"jd-judge-b":        "JD Judge B",
	"jd-fix-agent":      "JD Fix Agent",
}

var antigravityPresetOrder = []AntigravityModelPreset{
	AntigravityPresetInherit,
	AntigravityPresetPerformance,
	AntigravityPresetEconomy,
	AntigravityPresetCustom,
}

var antigravityPresetConstructors = map[AntigravityModelPreset]func() map[string]model.AntigravityModelTier{
	AntigravityPresetInherit:     model.AntigravityModelPresetInherit,
	AntigravityPresetPerformance: model.AntigravityModelPresetPerformance,
	AntigravityPresetEconomy:     model.AntigravityModelPresetEconomy,
}

var antigravityTierOrder = []model.AntigravityModelTier{
	model.AntigravityModelInherit,
	model.AntigravityModelFlashLite,
	model.AntigravityModelFlash,
	model.AntigravityModelPro,
}

// AntigravityModelPickerState holds navigation state for the Antigravity model picker screen.
type AntigravityModelPickerState struct {
	Preset            AntigravityModelPreset
	CustomAssignments map[string]model.AntigravityModelTier
	InCustomMode      bool
}

// NewAntigravityModelPickerState returns an initial picker state with the inherit preset selected.
func NewAntigravityModelPickerState() AntigravityModelPickerState {
	return AntigravityModelPickerState{
		Preset:            AntigravityPresetInherit,
		CustomAssignments: model.AntigravityModelPresetInherit(),
	}
}

// NewAntigravityModelPickerStateFromAssignments restores picker state from previously saved custom assignments.
func NewAntigravityModelPickerStateFromAssignments(assignments map[string]model.AntigravityModelTier) AntigravityModelPickerState {
	if len(assignments) == 0 {
		return NewAntigravityModelPickerState()
	}
	state := AntigravityModelPickerState{Preset: AntigravityPresetCustom, CustomAssignments: maps.Clone(assignments)}
	for preset, constructor := range antigravityPresetConstructors {
		if maps.Equal(constructor(), assignments) {
			state.Preset = preset
		}
	}
	return state
}

// HandleAntigravityModelPickerNav handles navigation keys (up/down/enter/esc) for the Antigravity model picker.
func HandleAntigravityModelPickerNav(
	key string,
	state *AntigravityModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.AntigravityModelTier) {
	if !state.InCustomMode {
		if key != "enter" || cursor >= len(antigravityPresetOrder) {
			return false, nil
		}
		state.Preset = antigravityPresetOrder[cursor]
		if state.Preset == AntigravityPresetCustom {
			state.InCustomMode = true
			if state.CustomAssignments == nil {
				state.CustomAssignments = model.AntigravityModelPresetInherit()
			}
			return true, nil
		}
		state.CustomAssignments = antigravityPresetConstructors[state.Preset]()
		return true, maps.Clone(state.CustomAssignments)
	}
	roles := model.AntigravityModelRoles
	switch {
	case key == "esc":
		state.InCustomMode = false
		return true, nil
	case key != "enter":
		return false, nil
	case cursor < len(roles):
		state.CustomAssignments[roles[cursor]] = nextAntigravityTier(state.CustomAssignments[roles[cursor]])
		return true, nil
	case cursor == len(roles):
		return true, maps.Clone(state.CustomAssignments)
	}
	state.InCustomMode = false
	return true, nil
}

// nextAntigravityTier returns the next tier in round-robin order when cycling in custom mode.
func nextAntigravityTier(current model.AntigravityModelTier) model.AntigravityModelTier {
	for i, tier := range antigravityTierOrder {
		if tier == current {
			return antigravityTierOrder[(i+1)%len(antigravityTierOrder)]
		}
	}
	return antigravityTierOrder[1]
}

// AntigravityModelPickerOptionCount returns the number of selectable options for the current picker state.
func AntigravityModelPickerOptionCount(state AntigravityModelPickerState) int {
	if state.InCustomMode {
		return len(model.AntigravityModelRoles) + 2 // role rows + Confirm + Back
	}
	return len(antigravityPresetOrder) + 1 // presets + Back
}

// RenderAntigravityModelPicker renders the Antigravity model picker screen for the given state and cursor index.
func RenderAntigravityModelPicker(state AntigravityModelPickerState, cursor int) string {
	var b strings.Builder
	if !state.InCustomMode {
		b.WriteString(styles.TitleStyle.Render("Antigravity Model Assignments"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Choose the agy model tier for Antigravity subagents:"))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("Note: Antigravity native subagents support tiers (inherit/flash_lite/flash/pro); arbitrary model/provider/effort is unsupported by the host."))
		b.WriteString("\n\n")
		for idx, preset := range antigravityPresetOrder {
			b.WriteString(renderRadio(string(preset), preset == state.Preset, idx == cursor))
			b.WriteString(styles.SubtextStyle.Render("    "+antigravityPresetDescriptions[preset]) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(renderOptions([]string{"← Back"}, cursor-len(antigravityPresetOrder)))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))
		return b.String()
	}

	b.WriteString(styles.TitleStyle.Render("Custom Antigravity Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Press enter on a role to cycle: inherit → flash_lite → flash → pro"))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("Note: Antigravity native subagents support tiers (inherit/flash_lite/flash/pro); arbitrary model/provider/effort is unsupported by the host."))
	b.WriteString("\n\n")
	for idx, role := range model.AntigravityModelRoles {
		roleLabel := antigravityRoleLabels[role]
		if roleLabel == "" {
			roleLabel = claudePhaseLabels[role]
		}
		if roleLabel == "" {
			roleLabel = role
		}
		label := fmt.Sprintf("%-20s [%s]", roleLabel, model.AntigravityModelID(state.CustomAssignments[role]))
		if idx == cursor {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Confirm", "← Back"}, cursor-len(model.AntigravityModelRoles)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: cycle/select • esc: back"))
	return b.String()
}
