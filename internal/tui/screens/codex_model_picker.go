package screens

import (
	"fmt"
	"maps"
	"strings"
	"unicode"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// CodexModelPreset represents a named effort-tier preset for Codex per-phase
// reasoning_effort assignments. Each preset corresponds to a ChatGPT plan tier.
type CodexModelPreset string

const (
	// CodexPresetLowCost targets ChatGPT Plus ($20/mo) — minimal effort to
	// stay within the plan's tight usage limits.
	CodexPresetLowCost CodexModelPreset = "low-cost"

	// CodexPresetRecommended targets ChatGPT Pro ($100/mo) — balanced effort
	// for most SDD work. This is the default preset.
	CodexPresetRecommended CodexModelPreset = "recommended"

	// CodexPresetPowerful targets ChatGPT Pro ($200/mo) — xhigh effort for
	// architecture-heavy and review-heavy phases.
	CodexPresetPowerful CodexModelPreset = "powerful"
)

var codexPresetOrder = []CodexModelPreset{
	CodexPresetLowCost,
	CodexPresetRecommended,
	CodexPresetPowerful,
}

var codexPresetDescriptions = map[CodexModelPreset]string{
	CodexPresetLowCost:     "Minimal effort — preserves tight ChatGPT Plus ($20/mo) usage limits",
	CodexPresetRecommended: "Balanced effort — high on key phases, low on lightweight work (Pro $100/mo)",
	CodexPresetPowerful:    "Maximum effort — xhigh on architecture, design, and verification (Pro $200/mo)",
}

var codexPresetConstructors = map[CodexModelPreset]func() map[string]model.CodexEffort{
	CodexPresetLowCost:     model.CodexModelPresetLowCost,
	CodexPresetRecommended: model.CodexModelPresetRecommended,
	CodexPresetPowerful:    model.CodexModelPresetPowerful,
}

// codexCustomPhases is the ordered list of the 13 SDD phases for the Custom
// per-phase model picker. Order matches codexTierGroups phase groupings.
var codexCustomPhases = []string{
	"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks",
	"sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	"jd-judge-a", "jd-judge-b", "jd-fix-agent", "default",
}

// CodexCustomMode represents the active sub-mode of the Custom picker flow.
type CodexCustomMode int

const (
	// CodexCustomModeNone means the main picker is showing (no Custom sub-mode active).
	CodexCustomModeNone CodexCustomMode = iota
	// CodexCustomModePhaseList shows the 13 phases with their current assignments.
	CodexCustomModePhaseList
	// CodexCustomModeModelSelect shows the searchable model list for a selected phase.
	CodexCustomModeModelSelect
	// CodexCustomModeEffortSelect shows the effort level list for the selected model.
	CodexCustomModeEffortSelect
)

// CodexCustomAssignment holds the model id + reasoning effort for one phase in
// the Custom per-phase picker.
type CodexCustomAssignment struct {
	ModelID string
	Effort  model.CodexEffort
}

// CodexModelPickerState holds navigation state for the Codex model picker screen.
// Includes 3 presets + Custom mode + Back.
type CodexModelPickerState struct {
	Preset CodexModelPreset

	// Custom picker sub-state.
	CustomMode         CodexCustomMode
	CustomPhaseIdx     int                              // phase row under cursor in phase-list
	CustomModelSearch  string                           // active search query in model-select
	CustomModelCursor  int                              // cursor position in filtered model list
	CustomEffortCursor int                              // cursor position in effort list
	CustomPendingModel string                           // model ID selected in model-select (pending effort)
	CustomAssignments  map[string]CodexCustomAssignment // phase → assignment
	CustomConfirmed    bool                             // true after user presses Confirm
}

// NewCodexModelPickerState returns the initial picker state: Recommended preset.
func NewCodexModelPickerState() CodexModelPickerState {
	return CodexModelPickerState{
		Preset:            CodexPresetRecommended,
		CustomAssignments: make(map[string]CodexCustomAssignment),
	}
}

// NewCodexModelPickerStateFromAssignments returns the picker state initialized
// from previously persisted Codex model assignments. If the assignments match
// a known preset (LowCost/Recommended/Powerful), that preset is preselected.
// Otherwise, falls back to Recommended (custom assignments do not restore the
// sub-flow state — only preset selection is recoverable from effort-only maps).
func NewCodexModelPickerStateFromAssignments(assignments map[string]model.CodexEffort) CodexModelPickerState {
	if len(assignments) == 0 {
		return NewCodexModelPickerState()
	}
	for preset, constructor := range codexPresetConstructors {
		if codexAssignmentsEqual(constructor(), assignments) {
			return CodexModelPickerState{
				Preset:            preset,
				CustomAssignments: make(map[string]CodexCustomAssignment),
			}
		}
	}
	// Unknown assignments → fall back to Recommended.
	return CodexModelPickerState{
		Preset:            CodexPresetRecommended,
		CustomAssignments: make(map[string]CodexCustomAssignment),
	}
}

func codexAssignmentsEqual(a, b map[string]model.CodexEffort) bool {
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

// CodexModelPickerOptionCount returns the total number of selectable rows based
// on the active sub-mode:
//   - Main picker: 3 presets + Custom + Back = 5
//   - Phase list: 13 phases + Confirm = 14
//   - Model select / Effort select: navigated by HandleCodexModelPickerNav
//     directly (cursor is managed by the sub-flow, not the outer optionCount).
func CodexModelPickerOptionCount(state CodexModelPickerState) int {
	switch state.CustomMode {
	case CodexCustomModePhaseList:
		return len(codexCustomPhases) + 1 // phases + Confirm
	case CodexCustomModeModelSelect:
		models := model.FilterCodexModels(state.CustomModelSearch)
		if len(models) == 0 {
			return 0
		}
		return len(models)
	case CodexCustomModeEffortSelect:
		return len(codexEffortOptions)
	}
	return len(codexPresetOrder) + 1 + 1 // presets + Custom + Back
}

// HandleCodexModelPickerNav processes a key event for the Codex model picker.
//
// Dispatch:
//   - When CustomMode != None, delegates to the custom sub-flow.
//   - Otherwise, handles the main preset/Custom/Back rows on "enter".
//
// Returns:
//   - (true, assignments): a preset was confirmed — assignments are the
//     per-phase effort map; or Custom Confirm was pressed — assignments are
//     the per-phase effort map derived from CustomAssignments.
//   - (true, nil): Back was selected, or entering the Custom sub-mode.
//   - (false, nil): key was not handled (navigation should fall through).
func HandleCodexModelPickerNav(
	key string,
	state *CodexModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.CodexEffort) {
	// Delegate to custom sub-flow when active.
	if state.CustomMode != CodexCustomModeNone {
		return handleCodexCustomNav(key, state, cursor)
	}

	if key != "enter" {
		return false, nil
	}

	// Back row: last row after presets + Custom.
	backIdx := len(codexPresetOrder) + 1
	if cursor >= backIdx {
		return true, nil
	}

	// Custom row: index len(codexPresetOrder) = 3.
	if cursor == len(codexPresetOrder) {
		state.CustomMode = CodexCustomModePhaseList
		state.CustomPhaseIdx = 0
		state.CustomModelSearch = ""
		state.CustomModelCursor = 0
		state.CustomEffortCursor = 0
		if state.CustomAssignments == nil {
			state.CustomAssignments = make(map[string]CodexCustomAssignment)
		}
		return true, nil
	}

	// Preset rows.
	if cursor < len(codexPresetOrder) {
		selected := codexPresetOrder[cursor]
		state.Preset = selected
		// Clear Custom state so a later re-entry to Custom starts fresh.
		state.CustomConfirmed = false
		a := maps.Clone(codexPresetConstructors[selected]())
		return true, a
	}

	return false, nil
}

// HandleCodexCustomNav is an exported helper for tests: processes a key event
// while the Custom sub-flow is active. Callers in production code should use
// HandleCodexModelPickerNav, which dispatches here automatically.
func HandleCodexCustomNav(key string, state *CodexModelPickerState, cursor int) (handled bool, assignments map[string]model.CodexEffort) {
	return handleCodexCustomNav(key, state, cursor)
}

func handleCodexCustomNav(key string, state *CodexModelPickerState, cursor int) (handled bool, assignments map[string]model.CodexEffort) {
	switch state.CustomMode {
	case CodexCustomModePhaseList:
		return handleCustomPhaseListNav(key, state, cursor)
	case CodexCustomModeModelSelect:
		// CustomModelCursor is the single source of truth for this sub-mode.
		// Do NOT sync from the outer cursor: the outer cursor (m.Cursor in
		// model.go) never advances for ModelSelect because the handler returns
		// handled=true for up/down, preventing model.go's generic cursor switch
		// from running. Syncing here resets CustomModelCursor to the stale outer
		// value on every keypress, causing the cursor to oscillate 0↔1.
		return handleCustomModelSelectNav(key, state)
	case CodexCustomModeEffortSelect:
		// Same rationale as ModelSelect: CustomEffortCursor is the single source
		// of truth. Do NOT sync from the outer cursor.
		return handleCustomEffortSelectNav(key, state)
	}
	return false, nil
}

func handleCustomPhaseListNav(key string, state *CodexModelPickerState, cursor int) (bool, map[string]model.CodexEffort) {
	phaseCount := len(codexCustomPhases)
	// Sync state cursor with the outer cursor so j/k navigation propagates.
	state.CustomPhaseIdx = cursor

	switch key {
	case "esc":
		// Back to main preset list.
		state.CustomMode = CodexCustomModeNone
		return true, nil
	case "enter":
		// Confirm row is at index phaseCount.
		if cursor == phaseCount {
			// Build effort assignments from CustomAssignments.
			// Phases without a custom assignment use Recommended preset defaults.
			base := model.CodexModelPresetRecommended()
			for phase, a := range state.CustomAssignments {
				if a.Effort != "" {
					base[phase] = a.Effort
				}
			}
			state.CustomConfirmed = true
			state.CustomMode = CodexCustomModeNone
			return true, base
		}
		// Select a phase → enter model-select.
		if cursor < phaseCount {
			state.CustomPhaseIdx = cursor
			state.CustomMode = CodexCustomModeModelSelect
			state.CustomModelSearch = ""
			state.CustomModelCursor = 0
			return true, nil
		}
	}
	return false, nil
}

func handleCustomModelSelectNav(key string, state *CodexModelPickerState) (bool, map[string]model.CodexEffort) {
	models := model.FilterCodexModels(state.CustomModelSearch)

	switch key {
	case "up", "k":
		if state.CustomModelCursor > 0 {
			state.CustomModelCursor--
		}
		return true, nil
	case "down", "j":
		if state.CustomModelCursor < len(models)-1 {
			state.CustomModelCursor++
		}
		return true, nil
	case "enter":
		if len(models) == 0 {
			return true, nil
		}
		selected := models[state.CustomModelCursor]
		state.CustomPendingModel = selected
		state.CustomMode = CodexCustomModeEffortSelect
		state.CustomEffortCursor = 0
		return true, nil
	case "backspace":
		if state.CustomModelSearch != "" {
			runes := []rune(state.CustomModelSearch)
			state.CustomModelSearch = string(runes[:len(runes)-1])
			state.CustomModelCursor = 0
		}
		return true, nil
	case "ctrl+u":
		state.CustomModelSearch = ""
		state.CustomModelCursor = 0
		return true, nil
	case "esc":
		state.CustomMode = CodexCustomModePhaseList
		state.CustomModelSearch = ""
		state.CustomModelCursor = 0
		return true, nil
	default:
		if isCodexSearchInput(key) {
			state.CustomModelSearch += key
			state.CustomModelCursor = 0
			return true, nil
		}
	}
	return false, nil
}

var codexEffortOptions = []model.CodexEffort{
	model.CodexEffortLow,
	model.CodexEffortMedium,
	model.CodexEffortHigh,
	model.CodexEffortXHigh,
}

func handleCustomEffortSelectNav(key string, state *CodexModelPickerState) (bool, map[string]model.CodexEffort) {
	switch key {
	case "up", "k":
		if state.CustomEffortCursor > 0 {
			state.CustomEffortCursor--
		}
		return true, nil
	case "down", "j":
		if state.CustomEffortCursor < len(codexEffortOptions)-1 {
			state.CustomEffortCursor++
		}
		return true, nil
	case "enter":
		effort := codexEffortOptions[state.CustomEffortCursor]
		phase := codexCustomPhases[state.CustomPhaseIdx]
		if state.CustomAssignments == nil {
			state.CustomAssignments = make(map[string]CodexCustomAssignment)
		}
		state.CustomAssignments[phase] = CodexCustomAssignment{
			ModelID: state.CustomPendingModel,
			Effort:  effort,
		}
		state.CustomPendingModel = ""
		state.CustomMode = CodexCustomModePhaseList
		state.CustomEffortCursor = 0
		return true, nil
	case "esc":
		state.CustomMode = CodexCustomModeModelSelect
		state.CustomEffortCursor = 0
		state.CustomPendingModel = ""
		return true, nil
	}
	return false, nil
}

func isCodexSearchInput(key string) bool {
	runes := []rune(key)
	if len(runes) != 1 {
		return false
	}
	return unicode.IsPrint(runes[0]) && runes[0] != 'j' && runes[0] != 'k'
}

// RenderCodexModelPicker renders the Codex preset selection screen.
// In default mode: title + 3 presets + Custom + Back.
// In custom sub-modes: delegates to the appropriate sub-renderer.
func RenderCodexModelPicker(state CodexModelPickerState, cursor int) string {
	switch state.CustomMode {
	case CodexCustomModePhaseList:
		return renderCodexCustomPhaseList(state, cursor)
	case CodexCustomModeModelSelect:
		return renderCodexCustomModelSelect(state)
	case CodexCustomModeEffortSelect:
		return renderCodexCustomEffortSelect(state)
	}
	return renderCodexMainPicker(state, cursor)
}

func renderCodexMainPicker(state CodexModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Codex Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose the reasoning_effort tier for Codex SDD phases (tied to your ChatGPT plan):"))
	b.WriteString("\n\n")

	for idx, preset := range codexPresetOrder {
		isSelected := preset == state.Preset
		focused := idx == cursor
		b.WriteString(renderRadio(CodexPresetLabel(preset), isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+codexPresetDescriptions[preset]) + "\n")
	}

	// Custom row at index 3.
	customIdx := len(codexPresetOrder)
	customFocused := cursor == customIdx
	customLabel := "Custom — per-phase model + effort"
	if customFocused {
		b.WriteString(styles.SelectedStyle.Render(styles.Cursor+customLabel) + "\n")
	} else {
		b.WriteString(styles.UnselectedStyle.Render("  "+customLabel) + "\n")
	}
	b.WriteString(styles.SubtextStyle.Render("    Assign a specific model and effort to each of the 13 SDD phases") + "\n")

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(codexPresetOrder)-1))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func renderCodexCustomPhaseList(state CodexModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom — Per-Phase Model & Effort"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Select a phase to assign its model and effort. Unassigned phases use Recommended defaults."))
	b.WriteString("\n\n")

	for idx, phase := range codexCustomPhases {
		focused := idx == cursor
		a, hasAssignment := state.CustomAssignments[phase]

		var label string
		if hasAssignment {
			label = fmt.Sprintf("%-14s %s / %s", phase, a.ModelID, string(a.Effort))
		} else {
			label = fmt.Sprintf("%-14s (default)", phase)
		}

		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	b.WriteString("\n")
	confirmIdx := len(codexCustomPhases)
	confirmFocused := cursor == confirmIdx
	if confirmFocused {
		b.WriteString(styles.SelectedStyle.Render(styles.Cursor+"Confirm assignments") + "\n")
	} else {
		b.WriteString(styles.UnselectedStyle.Render("  Confirm assignments") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: assign / confirm • esc: back to presets"))

	return b.String()
}

func renderCodexCustomModelSelect(state CodexModelPickerState) string {
	var b strings.Builder

	phase := ""
	if state.CustomPhaseIdx < len(codexCustomPhases) {
		phase = codexCustomPhases[state.CustomPhaseIdx]
	}

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Select model for %s:", phase)))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Search: " + codexModelSearchDisplay(state.CustomModelSearch)))
	b.WriteString("\n\n")

	models := model.FilterCodexModels(state.CustomModelSearch)
	cursor := state.CustomModelCursor
	if cursor >= len(models) && len(models) > 0 {
		cursor = len(models) - 1
	}

	if len(models) == 0 {
		b.WriteString(styles.WarningStyle.Render("  No models match your search."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("type: search • backspace: delete • ctrl+u: clear • esc: back"))
		return b.String()
	}

	for i, m := range models {
		focused := i == cursor
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+m) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+m) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • type: search • backspace: delete • ctrl+u: clear • enter: select • esc: back"))

	return b.String()
}

func renderCodexCustomEffortSelect(state CodexModelPickerState) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Select effort for %s:", state.CustomPendingModel)))
	b.WriteString("\n\n")

	for i, effort := range codexEffortOptions {
		focused := i == state.CustomEffortCursor
		label := string(effort)
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func codexModelSearchDisplay(query string) string {
	if query == "" {
		return "_"
	}
	return query + "_"
}

// CodexPresetLabel returns the human-readable plan label for a preset.
// Labels are self-describing: they include the model id and effort tier per
// carril so the user can see what will be written to profile files.
//
// Format: "<Plan> — Razonamiento gpt-5.5/<effort> · Código gpt-5.5/<effort> · Liviano gpt-5.4-mini/low"
func CodexPresetLabel(preset CodexModelPreset) string {
	switch preset {
	case CodexPresetLowCost:
		return "Plus $20 — Razonamiento gpt-5.5/medium · Código gpt-5.5/medium · Liviano gpt-5.4-mini/low"
	case CodexPresetRecommended:
		return "Pro $100 — Razonamiento gpt-5.5/high · Código gpt-5.5/medium · Liviano gpt-5.4-mini/low"
	case CodexPresetPowerful:
		return "Pro $200 — Razonamiento gpt-5.5/xhigh · Código gpt-5.5/high · Liviano gpt-5.4-mini/low"
	default:
		return string(preset)
	}
}

// CodexPresetDescription returns a one-line description for a preset.
func CodexPresetDescription(preset CodexModelPreset) string {
	return codexPresetDescriptions[preset]
}
