package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// KiroModelPickerState reuses the same phase-assignment mechanics as Claude
// aliases (opus|sonnet|haiku), but remains a separate UI flow and persisted map.
type KiroModelPickerState struct {
	Preset            ClaudeModelPreset
	CustomAssignments map[string]model.ClaudeModelAlias
	InCustomMode      bool
}

func NewKiroModelPickerState() KiroModelPickerState {
	return KiroModelPickerState{
		Preset:            ClaudePresetBalanced,
		CustomAssignments: model.ClaudeModelPresetBalanced(),
		InCustomMode:      false,
	}
}

func HandleKiroModelPickerNav(
	key string,
	state *KiroModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.ClaudeModelAlias) {
	// Reuse the same navigation engine by bridging through Claude state.
	bridge := ClaudeModelPickerState{
		Preset:            state.Preset,
		CustomAssignments: state.CustomAssignments,
		InCustomMode:      state.InCustomMode,
	}
	handled, assignments = HandleClaudeModelPickerNav(key, &bridge, cursor)
	state.Preset = bridge.Preset
	state.CustomAssignments = bridge.CustomAssignments
	state.InCustomMode = bridge.InCustomMode
	return handled, assignments
}

func KiroModelPickerOptionCount(state KiroModelPickerState) int {
	if state.InCustomMode {
		return len(claudePhases) + 2 // phase rows + Confirm + Back
	}
	return len(claudePresetOrder) + 1 // presets + Back
}

func RenderKiroModelPicker(state KiroModelPickerState, cursor int) string {
	if state.InCustomMode {
		return renderKiroCustomPhaseList(state, cursor)
	}
	return renderKiroPresetList(state, cursor)
}

func renderKiroPresetList(state KiroModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Kiro Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how Kiro models are assigned to each SDD execution phase (explore → apply → archive + orchestrator):"))
	b.WriteString("\n\n")

	for idx, preset := range claudePresetOrder {
		isSelected := preset == state.Preset
		focused := idx == cursor
		b.WriteString(renderRadio(string(preset), isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+claudePresetDescriptions[preset]) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(claudePresetOrder)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func renderKiroCustomPhaseList(state KiroModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom Kiro Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Press enter on a phase to cycle: opus → sonnet → haiku"))
	b.WriteString("\n\n")

	for idx, phase := range claudePhases {
		focused := idx == cursor
		alias := state.CustomAssignments[phase]
		if alias == "" {
			alias = model.ClaudeModelSonnet
		}

		label := fmt.Sprintf("%-20s %s", claudePhaseLabels[phase], aliasTag(alias))

		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	b.WriteString("\n")
	actionCursor := cursor - len(claudePhases)
	b.WriteString(renderOptions([]string{"Confirm", "← Back"}, actionCursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: cycle/select • esc: back"))

	return b.String()
}
