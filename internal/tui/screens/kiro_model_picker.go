package screens

import (
	"fmt"
	"maps"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

type KiroModelPreset string

const (
	KiroPresetBalanced    KiroModelPreset = "balanced"
	KiroPresetPerformance KiroModelPreset = "performance"
	KiroPresetEconomy     KiroModelPreset = "economy"
	KiroPresetOpenWeight  KiroModelPreset = "open-weight"
	KiroPresetCustom      KiroModelPreset = "custom"
)

var kiroPresetDescriptions = map[KiroModelPreset]string{
	KiroPresetBalanced:    "Kiro Auto for most phases, Opus for design, Haiku for lightweight archive/onboard work",
	KiroPresetPerformance: "Frontier Claude-family models for architecture, verification, and review-heavy phases",
	KiroPresetEconomy:     "Low-credit Kiro options: Qwen, DeepSeek, and MiniMax for budget-conscious runs",
	KiroPresetOpenWeight:  "Kiro open-weight families: MiniMax, GLM, DeepSeek, and Qwen",
	KiroPresetCustom:      "Pick the Kiro model option for each SDD phase, JD agent, and general delegation entry individually",
}

var kiroPresetOrder = []KiroModelPreset{
	KiroPresetBalanced,
	KiroPresetPerformance,
	KiroPresetEconomy,
	KiroPresetOpenWeight,
	KiroPresetCustom,
}

var kiroPresetConstructors = map[KiroModelPreset]func() map[string]model.KiroModelAlias{
	KiroPresetBalanced:    model.KiroModelPresetBalanced,
	KiroPresetPerformance: model.KiroModelPresetPerformance,
	KiroPresetEconomy:     model.KiroModelPresetEconomy,
	KiroPresetOpenWeight:  model.KiroModelPresetOpenWeight,
}

var kiroAliasOrder = []model.KiroModelAlias{
	model.KiroModelAuto,
	model.KiroModelOpus,
	model.KiroModelSonnet,
	model.KiroModelHaiku,
	model.KiroModelMiniMax,
	model.KiroModelGLM,
	model.KiroModelDeepSeek,
	model.KiroModelQwen,
}

// KiroModelPickerState holds navigation state for the Kiro model picker screen.
type KiroModelPickerState struct {
	Preset            KiroModelPreset
	CustomAssignments map[string]model.KiroModelAlias
	InCustomMode      bool
}

func NewKiroModelPickerState() KiroModelPickerState {
	return KiroModelPickerState{
		Preset:            KiroPresetBalanced,
		CustomAssignments: model.KiroModelPresetBalanced(),
		InCustomMode:      false,
	}
}

func NewKiroModelPickerStateFromAssignments(assignments map[string]model.KiroModelAlias) KiroModelPickerState {
	if len(assignments) == 0 {
		return NewKiroModelPickerState()
	}
	for preset, constructor := range kiroPresetConstructors {
		if kiroAssignmentsEqual(constructor(), assignments) {
			return KiroModelPickerState{
				Preset:            preset,
				CustomAssignments: maps.Clone(assignments),
				InCustomMode:      false,
			}
		}
	}
	return KiroModelPickerState{
		Preset:            KiroPresetCustom,
		CustomAssignments: maps.Clone(assignments),
		InCustomMode:      false,
	}
}

func kiroAssignmentsEqual(a, b map[string]model.KiroModelAlias) bool {
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

func HandleKiroModelPickerNav(
	key string,
	state *KiroModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.KiroModelAlias) {
	if !state.InCustomMode {
		return handleKiroPresetNav(key, state, cursor)
	}
	return handleKiroCustomPhaseNav(key, state, cursor)
}

func handleKiroPresetNav(
	key string,
	state *KiroModelPickerState,
	cursor int,
) (bool, map[string]model.KiroModelAlias) {
	if key != "enter" {
		return false, nil
	}
	if cursor >= len(kiroPresetOrder) {
		return false, nil
	}

	selected := kiroPresetOrder[cursor]
	state.Preset = selected
	if selected == KiroPresetCustom {
		state.InCustomMode = true
		if state.CustomAssignments == nil {
			state.CustomAssignments = model.KiroModelPresetBalanced()
		}
		return true, nil
	}

	assignments := kiroPresetConstructors[selected]()
	state.CustomAssignments = assignments
	return true, assignments
}

func handleKiroCustomPhaseNav(
	key string,
	state *KiroModelPickerState,
	cursor int,
) (bool, map[string]model.KiroModelAlias) {
	switch key {
	case "esc":
		state.InCustomMode = false
		return true, nil
	case "enter":
		if cursor < len(claudePhases) {
			phase := claudePhases[cursor]
			state.CustomAssignments[phase] = nextKiroAlias(state.CustomAssignments[phase])
			return true, nil
		}
		if cursor == len(claudePhases) {
			return true, state.CustomAssignments
		}
		state.InCustomMode = false
		return true, nil
	}
	return false, nil
}

func nextKiroAlias(current model.KiroModelAlias) model.KiroModelAlias {
	for i, alias := range kiroAliasOrder {
		if alias == current {
			return kiroAliasOrder[(i+1)%len(kiroAliasOrder)]
		}
	}
	return model.KiroModelAuto
}

func KiroModelPickerOptionCount(state KiroModelPickerState) int {
	if state.InCustomMode {
		return len(claudePhases) + 2 // phase rows + Confirm + Back
	}
	return len(kiroPresetOrder) + 1 // presets + Back
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
	b.WriteString(styles.SubtextStyle.Render("Choose how Kiro models are assigned to each SDD execution phase (explore → apply → archive):"))
	b.WriteString("\n\n")

	for idx, preset := range kiroPresetOrder {
		isSelected := preset == state.Preset
		focused := idx == cursor
		b.WriteString(renderRadio(string(preset), isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+kiroPresetDescriptions[preset]) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(kiroPresetOrder)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func renderKiroCustomPhaseList(state KiroModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom Kiro Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Press enter on a phase to cycle: auto → opus → sonnet → haiku → minimax → glm → deepseek → qwen"))
	b.WriteString("\n\n")

	for idx, phase := range claudePhases {
		focused := idx == cursor
		alias := state.CustomAssignments[phase]
		if alias == "" {
			alias = model.KiroModelAuto
		}

		label := fmt.Sprintf("%-20s %s", claudePhaseLabels[phase], kiroAliasTag(alias))

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

func kiroAliasTag(alias model.KiroModelAlias) string {
	switch alias {
	case model.KiroModelAuto:
		return styles.SuccessStyle.Render("[auto]")
	case model.KiroModelOpus:
		return styles.WarningStyle.Render("[opus]")
	case model.KiroModelHaiku:
		return styles.SubtextStyle.Render("[haiku]")
	case model.KiroModelMiniMax:
		return styles.SuccessStyle.Render("[minimax]")
	case model.KiroModelGLM:
		return styles.SuccessStyle.Render("[glm]")
	case model.KiroModelDeepSeek:
		return styles.SuccessStyle.Render("[deepseek]")
	case model.KiroModelQwen:
		return styles.SubtextStyle.Render("[qwen]")
	default:
		return styles.SuccessStyle.Render("[sonnet]")
	}
}
