package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ClaudeModelPreset represents a named preset for Claude model assignments.
type ClaudeModelPreset string

const (
	ClaudePresetBalanced    ClaudeModelPreset = "balanced"
	ClaudePresetPerformance ClaudeModelPreset = "performance"
	ClaudePresetEconomy     ClaudeModelPreset = "economy"
	ClaudePresetDiversity   ClaudeModelPreset = "diversity"
	ClaudePresetCustom      ClaudeModelPreset = "custom"
)

// claudePresetDescriptions describes each preset.
var claudePresetDescriptions = map[ClaudeModelPreset]string{
	ClaudePresetBalanced:    "Smart defaults for ODD delegation and review roles",
	ClaudePresetPerformance: "Maximum quality for delegation and review roles",
	ClaudePresetEconomy:     "Cost-optimised delegation and review roles",
	ClaudePresetDiversity:   "Diversity: Opus for Judge A, Haiku for Judge B, Sonnet for fixes",
	ClaudePresetCustom:      "Pick model and supported effort for ODD, JD and RDD roles individually",
}

// claudePresetOrder is the display order for presets.
var claudePresetOrder = []ClaudeModelPreset{
	ClaudePresetBalanced,
	ClaudePresetPerformance,
	ClaudePresetEconomy,
	ClaudePresetDiversity,
	ClaudePresetCustom,
}

// claudePhases is the ordered list of model-assignment keys shown in custom mode.
var claudePhases = []string{
	"odd-explorer",
	"odd-worker",
	"odd-verify",
	"jd-judge-a",
	"jd-judge-b",
	"jd-fix-agent",
	"risk",
	"readability",
	"reliability",
	"resilience",
	"refuter",
	"validator",
	"default",
}

// claudePhaseLabels are the human-readable labels for each configurable
// agent role (ODD, JD, RDD, and the general delegation row).
var claudePhaseLabels = map[string]string{
	"odd-explorer": "ODD Explorer",
	"odd-worker":   "ODD Worker",
	"odd-verify":   "ODD Verify",
	"jd-judge-a":   "JD Judge A",
	"jd-judge-b":   "JD Judge B",
	"jd-fix-agent": "JD Fix Agent",
	"risk":         "RDD Risk",
	"readability":  "RDD Readability",
	"reliability":  "RDD Reliability",
	"resilience":   "RDD Resilience",
	"refuter":      "RDD Refuter",
	"validator":    "RDD Validator",
	"default":      "General delegation",
}

// isClaudeInheritRole reports whether a row has no preset value: without a pick
// its agent runs on the session model, and its model list offers inherit to
// clear a pick.
func isClaudeInheritRole(phase string) bool {
	return model.IsClaudeODDRole(phase)
}

// claudeModelOptionCount is the number of model choices for a row, including
// inherit on inherit rows and excluding Back.
func claudeModelOptionCount(phase string) int {
	if isClaudeInheritRole(phase) {
		return len(claudeAliasOrder) + 1
	}
	return len(claudeAliasOrder)
}

// claudeAliasOrder defines the display order in the model selection screen.
var claudeAliasOrder = []model.ClaudeModelAlias{
	model.ClaudeModelFable,
	model.ClaudeModelOpus,
	model.ClaudeModelSonnet,
	model.ClaudeModelHaiku,
}

// ClaudePickerMode identifies the current Claude picker sub-screen.
type ClaudePickerMode int

const (
	ClaudeModePresetList ClaudePickerMode = iota
	ClaudeModePhaseList
	ClaudeModeModelSelect
	ClaudeModeEffortSelect
)

// ClaudeModelPickerState holds navigation state for the Claude model picker screen.
type ClaudeModelPickerState struct {
	// Preset holds the currently selected preset (or custom).
	Preset ClaudeModelPreset

	// CustomAssignments holds per-phase model+effort assignments in custom mode.
	// When a preset is selected, this mirrors the preset map with default effort.
	CustomAssignments map[string]model.ClaudePhaseAssignment

	// InCustomMode is true when the user has selected ClaudePresetCustom.
	// Kept as a coarse compatibility flag for the parent TUI navigation.
	InCustomMode bool

	// Mode identifies which custom picker sub-screen is active.
	Mode ClaudePickerMode

	// SelectedPhase is the phase currently being edited in model/effort sub-screens.
	SelectedPhase string
}

// NewClaudeModelPickerState returns the initial picker state: balanced preset selected.
func NewClaudeModelPickerState() ClaudeModelPickerState {
	return ClaudeModelPickerState{
		Preset:            ClaudePresetBalanced,
		CustomAssignments: claudePickerPresetAssignments(model.ClaudeModelPresetBalanced()),
		InCustomMode:      false,
		Mode:              ClaudeModePresetList,
	}
}

// NewClaudeModelPickerStateFromAssignments returns the picker state initialized
// from previously persisted Claude model assignments. If the assignments match
// a known preset (Balanced/Performance/Economy), that preset is preselected.
// Otherwise the picker opens in Custom mode preserving the user's exact assignments.
// When assignments is empty or nil, it falls back to the balanced default.
func NewClaudeModelPickerStateFromAssignments(assignments map[string]model.ClaudeModelAlias) ClaudeModelPickerState {
	return NewClaudeModelPickerStateFromPhaseAssignments(model.ClaudePhaseAssignmentsFromLegacy(assignments))
}

// NewClaudeModelPickerStateFromPhaseAssignments returns the picker state initialized
// from model+effort assignments. If assignments only contain default efforts and
// match a known preset, that preset is preselected; otherwise custom mode is used.
func NewClaudeModelPickerStateFromPhaseAssignments(assignments map[string]model.ClaudePhaseAssignment) ClaudeModelPickerState {
	if len(assignments) == 0 {
		return NewClaudeModelPickerState()
	}
	assignments = model.WithoutLegacyClaudeODDSeeds(assignments)
	for preset, constructor := range presetConstructors {
		presetAssignments := claudePickerPresetAssignments(constructor())
		if phaseAssignmentsEqual(presetAssignments, visibleClaudeAssignments(assignments)) ||
			phaseAssignmentsEqual(model.ClaudePhaseAssignmentsFromModelPreset(constructor()), assignments) {
			return ClaudeModelPickerState{
				Preset:            preset,
				CustomAssignments: copyPhaseAssignments(assignments),
				InCustomMode:      false,
				Mode:              ClaudeModePresetList,
			}
		}
	}
	return ClaudeModelPickerState{
		Preset:            ClaudePresetCustom,
		CustomAssignments: copyPhaseAssignments(assignments),
		InCustomMode:      false,
		Mode:              ClaudeModePresetList,
	}
}

func visibleClaudeAssignments(assignments map[string]model.ClaudePhaseAssignment) map[string]model.ClaudePhaseAssignment {
	visible := make(map[string]model.ClaudePhaseAssignment)
	for _, role := range claudePhases {
		if value, ok := assignments[role]; ok {
			visible[role] = value
		}
	}
	// The orchestrator is a persisted assignment, but not an editable role row.
	if value, ok := assignments["orchestrator"]; ok {
		visible["orchestrator"] = value
	}
	return visible
}

func claudePickerPresetAssignments(preset map[string]model.ClaudeModelAlias) map[string]model.ClaudePhaseAssignment {
	assignments := model.ClaudePhaseAssignmentsFromModelPreset(preset)
	for key := range assignments {
		if strings.HasPrefix(key, "sdd-") {
			delete(assignments, key)
		}
	}
	return assignments
}

func phaseAssignmentsEqual(a, b map[string]model.ClaudePhaseAssignment) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func copyPhaseAssignments(m map[string]model.ClaudePhaseAssignment) map[string]model.ClaudePhaseAssignment {
	out := make(map[string]model.ClaudePhaseAssignment, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// presetConstructors maps preset IDs to their constructor functions.
var presetConstructors = map[ClaudeModelPreset]func() map[string]model.ClaudeModelAlias{
	ClaudePresetBalanced:    model.ClaudeModelPresetBalanced,
	ClaudePresetPerformance: model.ClaudeModelPresetPerformance,
	ClaudePresetEconomy:     model.ClaudeModelPresetEconomy,
	ClaudePresetDiversity:   model.ClaudeModelPresetDiversity,
}

// HandleClaudeModelPickerNav processes a key press on the Claude model picker screen.
//
// In preset mode (InCustomMode == false):
//   - Enter on a preset option → sets CustomAssignments and returns (true, assignments).
//   - Enter on "custom" → enters custom mode, returns (true, nil) — screen stays open.
//
// In custom mode (InCustomMode == true):
//   - Enter on a phase row → opens model selection, then supported effort selection.
//
// Returns (true, assignments) when the user confirms a preset and the screen should advance.
// Returns (true, nil) when handled but the screen should stay open.
// Returns (false, nil) when the key was not handled by this function.
func HandleClaudeModelPickerNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.ClaudePhaseAssignment) {
	if !state.InCustomMode {
		state.Mode = ClaudeModePresetList
		return handlePresetNav(key, state, cursor)
	}
	if state.Mode == ClaudeModePresetList {
		state.Mode = ClaudeModePhaseList
	}
	switch state.Mode {
	case ClaudeModeModelSelect:
		return handleClaudeCustomModelSelectNav(key, state, cursor)
	case ClaudeModeEffortSelect:
		return handleClaudeCustomEffortSelectNav(key, state, cursor)
	default:
		return handleCustomPhaseNav(key, state, cursor)
	}
}

func handlePresetNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (bool, map[string]model.ClaudePhaseAssignment) {
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
		state.Mode = ClaudeModePhaseList
		if state.CustomAssignments == nil {
			state.CustomAssignments = claudePickerPresetAssignments(model.ClaudeModelPresetBalanced())
		}
		return true, nil
	}

	// Named preset — build assignments and signal that the screen is done.
	constructor := presetConstructors[selected]
	assignments := claudePickerPresetAssignments(constructor())
	for key, value := range state.CustomAssignments {
		if strings.HasPrefix(key, "sdd-") {
			assignments[key] = value // Preserve legacy persisted values without exposing retired rows.
		}
	}
	state.CustomAssignments = copyPhaseAssignments(assignments)
	return true, copyPhaseAssignments(assignments)
}

func handleCustomPhaseNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (bool, map[string]model.ClaudePhaseAssignment) {
	switch key {
	case "esc":
		// Exit custom mode back to preset list.
		state.InCustomMode = false
		state.Mode = ClaudeModePresetList
		state.SelectedPhase = ""
		return true, nil

	case "enter":
		if cursor < len(claudePhases) {
			state.SelectedPhase = claudePhases[cursor]
			state.Mode = ClaudeModeModelSelect
			return true, nil
		}

		// "Confirm" row (cursor == len(claudePhases)) — done.
		if cursor == len(claudePhases) {
			return true, copyPhaseAssignments(state.CustomAssignments)
		}

		// "Back" row — exit custom mode.
		state.InCustomMode = false
		state.Mode = ClaudeModePresetList
		state.SelectedPhase = ""
		return true, nil
	}

	return false, nil
}

func handleClaudeCustomModelSelectNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (bool, map[string]model.ClaudePhaseAssignment) {
	switch key {
	case "esc":
		state.Mode = ClaudeModePhaseList
		return true, nil
	case "enter":
		phase := state.SelectedPhase
		if cursor == len(claudeAliasOrder) && isClaudeInheritRole(phase) {
			// Inherit: no pick, so the agent runs on the session model.
			delete(state.CustomAssignments, phase)
			state.Mode = ClaudeModePhaseList
			return true, nil
		}
		if cursor >= len(claudeAliasOrder) {
			state.Mode = ClaudeModePhaseList
			return true, nil
		}
		assignment := state.CustomAssignments[phase]
		assignment.Model = claudeAliasOrder[cursor]
		if !model.ClaudeEffortAllowedForModel(assignment.Model, assignment.Effort) {
			assignment.Effort = model.ClaudeEffortDefault
		}
		state.CustomAssignments[phase] = assignment
		if len(model.ClaudeEffortsForModel(assignment.Model)) > 1 {
			state.Mode = ClaudeModeEffortSelect
		} else {
			state.Mode = ClaudeModePhaseList
		}
		return true, nil
	}
	return false, nil
}

func handleClaudeCustomEffortSelectNav(
	key string,
	state *ClaudeModelPickerState,
	cursor int,
) (bool, map[string]model.ClaudePhaseAssignment) {
	phase := state.SelectedPhase
	assignment := state.CustomAssignments[phase]
	levels := model.ClaudeEffortsForModel(assignment.Model)
	switch key {
	case "esc":
		state.Mode = ClaudeModeModelSelect
		return true, nil
	case "enter":
		if cursor >= len(levels) {
			state.Mode = ClaudeModeModelSelect
			return true, nil
		}
		assignment.Effort = levels[cursor]
		state.CustomAssignments[phase] = assignment
		state.Mode = ClaudeModePhaseList
		return true, nil
	}
	return false, nil
}

// RenderClaudeModelPicker renders the Claude model picker screen.
func RenderClaudeModelPicker(state ClaudeModelPickerState, cursor int, height ...int) string {
	if state.InCustomMode {
		switch state.Mode {
		case ClaudeModeModelSelect:
			return renderCustomModelSelect(state, cursor)
		case ClaudeModeEffortSelect:
			return renderCustomEffortSelect(state, cursor)
		default:
			availableHeight := 0
			if len(height) > 0 {
				availableHeight = height[0]
			}
			return renderCustomPhaseList(state, cursor, availableHeight)
		}
	}
	return renderPresetList(state, cursor)
}

func renderPresetList(state ClaudeModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Claude Model Assignments"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("Current: " + string(state.Preset)))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how Claude models are assigned to ODD and review roles:"))
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

func renderCustomPhaseList(state ClaudeModelPickerState, cursor, height int) string {
	// Keep the title, instruction, and help visible while following the cursor
	// through the phase and action rows on short terminals.
	start, end := 0, len(claudePhases)+2
	if height > 0 && height < end+6 {
		visible := height - 6
		if visible < 1 {
			visible = 1
		}
		start = cursor - visible + 1
		if start < 0 {
			start = 0
		}
		if start > end-visible {
			start = end - visible
		}
		if start < 0 {
			start = 0
		}
		end = start + visible
		if end > len(claudePhases)+2 {
			end = len(claudePhases) + 2
		}
	}
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom Claude Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Select a role to choose its model, then choose a supported effort level."))
	b.WriteString("\n\n")

	for idx, phase := range claudePhases {
		if idx < start || idx >= end {
			continue
		}
		focused := idx == cursor
		assignment := state.CustomAssignments[phase]
		var label string
		if !assignment.Model.Valid() && isClaudeInheritRole(phase) {
			label = fmt.Sprintf("%-20s %s", claudePhaseLabels[phase], styles.SubtextStyle.Render("[inherit]"))
		} else {
			if !assignment.Model.Valid() {
				assignment.Model = model.ClaudeModelSonnet
			}
			if !model.ClaudeEffortAllowedForModel(assignment.Model, assignment.Effort) {
				assignment.Effort = model.ClaudeEffortDefault
			}
			label = fmt.Sprintf("%-20s %s %s", claudePhaseLabels[phase], aliasTag(assignment.Model), effortTag(assignment.Effort))
		}

		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	for idx, label := range []string{"Confirm", "← Back"} {
		row := len(claudePhases) + idx
		if row >= start && row < end {
			b.WriteString(renderOptions([]string{label}, cursor-row))
		}
	}
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: edit role / confirm • esc: back to presets"))

	return b.String()
}

func renderCustomModelSelect(state ClaudeModelPickerState, cursor int) string {
	var b strings.Builder
	phase := state.SelectedPhase
	current := string(state.CustomAssignments[phase].Model)
	if current == "" && isClaudeInheritRole(phase) {
		current = model.ClaudeModelInherit
	}
	b.WriteString(styles.TitleStyle.Render("Select Claude model"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(claudePhaseLabels[phase] + " — current " + current))
	b.WriteString("\n\n")
	labels := make([]string, 0, claudeModelOptionCount(phase))
	for _, alias := range claudeAliasOrder {
		labels = append(labels, fmt.Sprintf("%-8s %s", alias, effortSummary(alias)))
	}
	if isClaudeInheritRole(phase) {
		labels = append(labels, fmt.Sprintf("%-8s %s", model.ClaudeModelInherit, "no pick: the agent runs on the session model"))
	}
	for idx, label := range labels {
		if idx == cursor {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(labels)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select model • esc: back"))
	return b.String()
}

func renderCustomEffortSelect(state ClaudeModelPickerState, cursor int) string {
	var b strings.Builder
	phase := state.SelectedPhase
	assignment := state.CustomAssignments[phase]
	levels := model.ClaudeEffortsForModel(assignment.Model)
	b.WriteString(styles.TitleStyle.Render("Select Claude effort"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%s — model %s", claudePhaseLabels[phase], assignment.Model)))
	b.WriteString("\n\n")
	for idx, effort := range levels {
		focused := idx == cursor
		label := effortLabel(effort)
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(levels)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select effort • esc: back to model"))
	return b.String()
}

func effortSummary(alias model.ClaudeModelAlias) string {
	levels := model.ClaudeEffortsForModel(alias)
	labels := make([]string, 0, len(levels))
	for _, level := range levels {
		labels = append(labels, effortLabel(level))
	}
	return strings.Join(labels, ", ")
}

func effortLabel(effort model.ClaudeEffort) string {
	if effort == model.ClaudeEffortDefault {
		return "default"
	}
	return string(effort)
}

// aliasTag returns a styled badge for the alias value.
func effortTag(effort model.ClaudeEffort) string {
	if effort == model.ClaudeEffortDefault {
		return styles.SubtextStyle.Render("[default]")
	}
	return styles.WarningStyle.Render("[" + string(effort) + "]")
}

func aliasTag(alias model.ClaudeModelAlias) string {
	switch alias {
	case model.ClaudeModelFable:
		return styles.TitleStyle.Render("[fable]")
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
		switch state.Mode {
		case ClaudeModeModelSelect:
			return claudeModelOptionCount(state.SelectedPhase) + 1 // aliases (+ inherit) + Back
		case ClaudeModeEffortSelect:
			assignment := state.CustomAssignments[state.SelectedPhase]
			return len(model.ClaudeEffortsForModel(assignment.Model)) + 1 // efforts + Back
		default:
			return len(claudePhases) + 2 // phases + Confirm + Back
		}
	}
	return len(claudePresetOrder) + 1 // presets + Back
}
