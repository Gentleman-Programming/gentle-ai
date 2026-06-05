package screens

import (
	"maps"
	"strings"

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

// CodexModelPickerState holds navigation state for the Codex model picker screen.
// There is no custom mode — only 3 presets + Back.
type CodexModelPickerState struct {
	Preset CodexModelPreset
}

// NewCodexModelPickerState returns the initial picker state: Recommended preset.
func NewCodexModelPickerState() CodexModelPickerState {
	return CodexModelPickerState{
		Preset: CodexPresetRecommended,
	}
}

// NewCodexModelPickerStateFromAssignments returns the picker state initialized
// from previously persisted Codex model assignments. If the assignments match
// a known preset (LowCost/Recommended/Powerful), that preset is preselected.
// Otherwise, falls back to Recommended (no custom fallback — presets only).
func NewCodexModelPickerStateFromAssignments(assignments map[string]model.CodexEffort) CodexModelPickerState {
	if len(assignments) == 0 {
		return NewCodexModelPickerState()
	}
	for preset, constructor := range codexPresetConstructors {
		if codexAssignmentsEqual(constructor(), assignments) {
			return CodexModelPickerState{Preset: preset}
		}
	}
	// Unknown assignments → fall back to Recommended (no custom mode).
	return CodexModelPickerState{Preset: CodexPresetRecommended}
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

// CodexModelPickerOptionCount returns the total number of selectable rows:
// 3 presets + 1 Back row. Does NOT take a state argument (no custom mode).
func CodexModelPickerOptionCount() int {
	return len(codexPresetOrder) + 1
}

// HandleCodexModelPickerNav processes a key event for the Codex model picker.
// Returns (true, assignments) when a preset is confirmed, (true, nil) when Back
// is selected, and (false, nil) for all other keys.
func HandleCodexModelPickerNav(
	key string,
	state *CodexModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.CodexEffort) {
	if key != "enter" {
		return false, nil
	}

	// Back row
	if cursor >= len(codexPresetOrder) {
		return true, nil
	}

	selected := codexPresetOrder[cursor]
	state.Preset = selected
	a := maps.Clone(codexPresetConstructors[selected]())
	return true, a
}

// RenderCodexModelPicker renders the Codex preset selection screen.
// Title: "Codex Model Assignments"; 3 preset rows + Back; no custom mode.
func RenderCodexModelPicker(state CodexModelPickerState, cursor int) string {
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

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(codexPresetOrder)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// CodexPresetLabel returns the human-readable plan label for a preset.
func CodexPresetLabel(preset CodexModelPreset) string {
	switch preset {
	case CodexPresetLowCost:
		return "Low-cost (ChatGPT Plus $20/mo)"
	case CodexPresetRecommended:
		return "Recommended (ChatGPT Pro $100/mo)"
	case CodexPresetPowerful:
		return "Powerful (ChatGPT Pro $200/mo)"
	default:
		return string(preset)
	}
}

// CodexPresetDescription returns a one-line description for a preset.
func CodexPresetDescription(preset CodexModelPreset) string {
	return codexPresetDescriptions[preset]
}
