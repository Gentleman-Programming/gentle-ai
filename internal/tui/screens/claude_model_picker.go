package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// ClaudeModelPreset represents a named preset for Claude model assignments.
type ClaudeModelPreset string

const (
	ClaudePresetBalanced    ClaudeModelPreset = "balanced"
	ClaudePresetPerformance ClaudeModelPreset = "performance"
	ClaudePresetEconomy     ClaudeModelPreset = "economy"
	ClaudePresetCustom      ClaudeModelPreset = "custom"
)

// claudePresetDescriptions describes each preset.
var claudePresetDescriptions = map[ClaudeModelPreset]string{
	ClaudePresetBalanced:    "Smart defaults: opus for architecture, sonnet for most phases, haiku for archiving",
	ClaudePresetPerformance: "Maximum quality: opus for architecture, planning & verification phases",
	ClaudePresetEconomy:     "Cost-optimised: sonnet for all phases, haiku for archiving",
	ClaudePresetCustom:      "Pick the model alias for each SDD phase individually",
}

// claudePresetOrder is the display order for presets.
var claudePresetOrder = []ClaudeModelPreset{
	ClaudePresetBalanced,
	ClaudePresetPerformance,
	ClaudePresetEconomy,
	ClaudePresetCustom,
}

// claudePhases is the ordered list of model-assignment keys shown in custom mode.
var claudePhases = []string{
	"orchestrator",
	"sdd-explore",
	"sdd-propose",
	"sdd-spec",
	"sdd-design",
	"sdd-tasks",
	"sdd-apply",
	"sdd-verify",
	"sdd-archive",
	"default",
}

// claudePhaseLabels are the human-readable labels for each SDD phase.
var claudePhaseLabels = map[string]string{
	"orchestrator": "Orchestrator",
	"sdd-explore":  "Explore",
	"sdd-propose":  "Propose",
	"sdd-spec":     "Spec",
	"sdd-design":   "Design",
	"sdd-tasks":    "Tasks",
	"sdd-apply":    "Apply",
	"sdd-verify":   "Verify",
	"sdd-archive":  "Archive",
	"default":      "General delegation",
}

// claudeAliasOrder defines the cycling order when pressing Enter on a phase row.
var claudeAliasOrder = []model.ClaudeModelAlias{
	model.ClaudeModelOpus,
	model.ClaudeModelSonnet,
	model.ClaudeModelHaiku,
}

// ClaudeModelPickerState holds navigation state for the Claude model picker screen.
type ClaudeModelPickerState struct {
	// Preset holds the currently selected preset (or custom).
	Preset ClaudeModelPreset

	// CustomAssignments holds per-phase aliases in custom mode.
	// When a preset is selected, this mirrors the preset map.
	CustomAssignments map[string]model.ClaudeModelAlias

	// InCustomMode is true when the user has selected ClaudePresetCustom
	// and is navigating the per-phase list.
	InCustomMode bool
}

// NewClaudeModelPickerState returns the initial picker state: balanced preset selected.
func NewClaudeModelPickerState() ClaudeModelPickerState {
	return ClaudeModelPickerState{
		Preset:            ClaudePresetBalanced,
		CustomAssignments: model.ClaudeModelPresetBalanced(),
		InCustomMode:      false,
	}
}

// presetConstructors maps preset IDs to their constructor functions.
var presetConstructors = map[ClaudeModelPreset]func() map[string]model.ClaudeModelAlias{
	ClaudePresetBalanced:    model.ClaudeModelPresetBalanced,
	ClaudePresetPerformance: model.ClaudeModelPresetPerformance,
	ClaudePresetEconomy:     model.ClaudeModelPresetEconomy,
}

// HandleClaudeModelPickerNav processes a key press on the Claude model picker screen.
//
// In preset mode (InCustomMode == false):
//   - Enter on a preset option → sets CustomAssignments and returns (true, assignments).
//   - Enter on "custom" → enters custom mode, returns (true, nil) — screen stays open.
//
// In custom mode (InCustomMode == true):
//   - Enter on a phase row → cycles the alias for that phase, returns (true, nil).
//
// Returns (true, assignments) when the user confirms a preset and the screen should advance.
// Returns (true, nil) when handled but the screen should stay open.
// Returns (false, nil) when the key was not handled by this function.
func HandleClaudeModelPickerNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.ClaudeModelAlias) {
	if !state.InCustomMode {
		return handlePresetNav(key, state, cursor)
	}
	return handleCustomPhaseNav(key, state, cursor)
}

func handlePresetNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (bool, map[string]model.ClaudeModelAlias) {
	if key != "enter" {
		return false, nil
	}

	if cursor >= len(claudePresetOrder) {
		// Back option — caller handles screen transition.
		return false, nil
	}

	selected := claudePresetOrder[cursor]
	state.Preset = selected

	if selected == ClaudePresetCustom {
		// Enter custom mode — keep existing CustomAssignments (or defaults).
		state.InCustomMode = true
		if state.CustomAssignments == nil {
			state.CustomAssignments = model.ClaudeModelPresetBalanced()
		}
		return true, nil
	}

	// Named preset — build assignments and signal that the screen is done.
	constructor := presetConstructors[selected]
	assignments := constructor()
	state.CustomAssignments = assignments
	return true, assignments
}

func handleCustomPhaseNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (bool, map[string]model.ClaudeModelAlias) {
	switch key {
	case "esc":
		// Exit custom mode back to preset list.
		state.InCustomMode = false
		return true, nil

	case "enter":
		if cursor < len(claudePhases) {
			// Cycle the alias for this phase.
			phase := claudePhases[cursor]
			current := state.CustomAssignments[phase]
			state.CustomAssignments[phase] = nextAlias(current)
			return true, nil
		}

		// "Confirm" row (cursor == len(claudePhases)) — done.
		if cursor == len(claudePhases) {
			return true, state.CustomAssignments
		}

		// "Back" row — exit custom mode.
		state.InCustomMode = false
		return true, nil
	}

	return false, nil
}

// nextAlias cycles through opus → sonnet → haiku → opus.
func nextAlias(current model.ClaudeModelAlias) model.ClaudeModelAlias {
	for i, a := range claudeAliasOrder {
		if a == current {
			return claudeAliasOrder[(i+1)%len(claudeAliasOrder)]
		}
	}
	return model.ClaudeModelSonnet
}

// RenderClaudeModelPicker renders the Claude model picker screen.
func RenderClaudeModelPicker(state ClaudeModelPickerState, cursor int) string {
	if state.InCustomMode {
		return renderCustomPhaseList(state, cursor)
	}
	return renderPresetList(state, cursor)
}

func renderPresetList(state ClaudeModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Claude Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how Claude models are assigned to each SDD phase:"))
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

func renderCustomPhaseList(state ClaudeModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom Model Assignments"))
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
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: cycle model / confirm • esc: back to presets"))

	return b.String()
}

// aliasTag returns a styled badge for the alias value.
func aliasTag(alias model.ClaudeModelAlias) string {
	switch alias {
	case model.ClaudeModelOpus:
		return styles.WarningStyle.Render("[opus]")
	case model.ClaudeModelHaiku:
		return styles.SubtextStyle.Render("[haiku]")
	default:
		return styles.SuccessStyle.Render("[sonnet]")
	}
}

// ClaudeModelPickerOptionCount returns the number of navigable options for the screen.
// Used by model.go's optionCount() method.
func ClaudeModelPickerOptionCount(state ClaudeModelPickerState) int {
	if state.InCustomMode {
		return len(claudePhases) + 2 // phases + Confirm + Back
	}
	return len(claudePresetOrder) + 1 // presets + Back
}
