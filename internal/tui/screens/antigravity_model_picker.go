package screens

import (
	"fmt"
	"maps"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

type AntigravityModelPreset string

const (
	AntigravityPresetInherit     AntigravityModelPreset = "inherit"
	AntigravityPresetPerformance AntigravityModelPreset = "performance"
	AntigravityPresetEconomy     AntigravityModelPreset = "economy"
	AntigravityPresetCustom      AntigravityModelPreset = "custom"
)

var antigravityPresetDescriptions = map[AntigravityModelPreset]string{
	AntigravityPresetInherit:     "Subagents use the session model (agy default)",
	AntigravityPresetPerformance: "pro tier for Judgment Day judges and the fix agent",
	AntigravityPresetEconomy:     "flash judges and a flash_lite fix agent",
	AntigravityPresetCustom:      "Pick the agy tier for each Judgment Day role individually",
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

func NewAntigravityModelPickerState() AntigravityModelPickerState {
	return AntigravityModelPickerState{
		Preset:            AntigravityPresetInherit,
		CustomAssignments: model.AntigravityModelPresetInherit(),
	}
}

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

func nextAntigravityTier(current model.AntigravityModelTier) model.AntigravityModelTier {
	for i, tier := range antigravityTierOrder {
		if tier == current {
			return antigravityTierOrder[(i+1)%len(antigravityTierOrder)]
		}
	}
	return antigravityTierOrder[1]
}

func AntigravityModelPickerOptionCount(state AntigravityModelPickerState) int {
	if state.InCustomMode {
		return len(model.AntigravityModelRoles) + 2 // role rows + Confirm + Back
	}
	return len(antigravityPresetOrder) + 1 // presets + Back
}

func RenderAntigravityModelPicker(state AntigravityModelPickerState, cursor int) string {
	var b strings.Builder
	if !state.InCustomMode {
		b.WriteString(styles.TitleStyle.Render("Antigravity Model Assignments"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Choose the agy model tier for Antigravity subagents:"))
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
	b.WriteString("\n\n")
	for idx, role := range model.AntigravityModelRoles {
		label := fmt.Sprintf("%-20s [%s]", claudePhaseLabels[role], model.AntigravityModelID(state.CustomAssignments[role]))
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
