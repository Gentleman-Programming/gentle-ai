package screens

import (
	"fmt"
	"maps"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

type KiloModelPreset string

const (
	KiloPresetBalanced KiloModelPreset = "balanced"
	KiloPresetQuality  KiloModelPreset = "quality"
	KiloPresetCustom   KiloModelPreset = "custom"
)

var kiloPresetDescriptions = map[KiloModelPreset]string{
	KiloPresetBalanced: "Auto for most phases, Haiku for lightweight archive/onboard work",
	KiloPresetQuality:  "Sonnet for reasoning phases, Opus for design, Haiku for lightweight",
	KiloPresetCustom:   "Pick the Kilo model option for each SDD phase individually",
}

var kiloPresetOrder = []KiloModelPreset{
	KiloPresetBalanced,
	KiloPresetQuality,
	KiloPresetCustom,
}

var kiloPresetConstructors = map[KiloModelPreset]func() map[string]model.KiloModelAlias{
	KiloPresetBalanced: model.KiloModelPresetFree,
	KiloPresetQuality:  model.KiloModelPresetQuality,
}

var kiloAliasOrder = model.KnownKiloAliases

// KiloModelPickerState holds navigation state for the Kilo model picker screen.
type KiloModelPickerState struct {
	Preset            KiloModelPreset
	CustomAssignments map[string]model.KiloModelAlias
	InCustomMode      bool
}

func NewKiloModelPickerState() KiloModelPickerState {
	return KiloModelPickerState{
		Preset:            KiloPresetBalanced,
		CustomAssignments: model.KiloModelPresetBalanced(),
		InCustomMode:      false,
	}
}

func NewKiloModelPickerStateFromAssignments(assignments map[string]model.KiloModelAlias) KiloModelPickerState {
	if len(assignments) == 0 {
		return NewKiloModelPickerState()
	}
	for preset, constructor := range kiloPresetConstructors {
		if kiloAssignmentsEqual(constructor(), assignments) {
			return KiloModelPickerState{
				Preset:            preset,
				CustomAssignments: maps.Clone(assignments),
				InCustomMode:      false,
			}
		}
	}
	return KiloModelPickerState{
		Preset:            KiloPresetCustom,
		CustomAssignments: maps.Clone(assignments),
		InCustomMode:      false,
	}
}

func kiloAssignmentsEqual(a, b map[string]model.KiloModelAlias) bool {
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

func HandleKiloModelPickerNav(
	key string,
	state *KiloModelPickerState,
	cursor int,
) (handled bool, assignments map[string]model.KiloModelAlias) {
	if !state.InCustomMode {
		return handleKiloPresetNav(key, state, cursor)
	}
	return handleKiloCustomPhaseNav(key, state, cursor)
}

func handleKiloPresetNav(
	key string,
	state *KiloModelPickerState,
	cursor int,
) (bool, map[string]model.KiloModelAlias) {
	if key != "enter" {
		return false, nil
	}
	if cursor >= len(kiloPresetOrder) {
		return false, nil
	}

	selected := kiloPresetOrder[cursor]
	state.Preset = selected
	if selected == KiloPresetCustom {
		state.InCustomMode = true
		if state.CustomAssignments == nil {
			state.CustomAssignments = model.KiloModelPresetBalanced()
		}
		return true, nil
	}

	assignments := kiloPresetConstructors[selected]()
	state.CustomAssignments = assignments
	return true, assignments
}

func handleKiloCustomPhaseNav(
	key string,
	state *KiloModelPickerState,
	cursor int,
) (bool, map[string]model.KiloModelAlias) {
	switch key {
	case "esc":
		state.InCustomMode = false
		return true, nil
	case "enter":
		if cursor < len(sddPhases) {
			phase := sddPhases[cursor]
			state.CustomAssignments[phase] = nextKiloAlias(state.CustomAssignments[phase])
			return true, nil
		}
		if cursor == len(sddPhases) {
			return true, state.CustomAssignments
		}
		state.InCustomMode = false
		return true, nil
	}
	return false, nil
}

func nextKiloAlias(current model.KiloModelAlias) model.KiloModelAlias {
	for i, alias := range kiloAliasOrder {
		if alias == current {
			return kiloAliasOrder[(i+1)%len(kiloAliasOrder)]
		}
	}
	return model.KiloModelAuto
}

func KiloModelPickerOptionCount(state KiloModelPickerState) int {
	if state.InCustomMode {
		return len(sddPhases) + 2 // phase rows + Confirm + Back
	}
	return len(kiloPresetOrder) + 1 // presets + Back
}

func RenderKiloModelPicker(state KiloModelPickerState, cursor int) string {
	if state.InCustomMode {
		return renderKiloCustomPhaseList(state, cursor)
	}
	return renderKiloPresetList(state, cursor)
}

func renderKiloPresetList(state KiloModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Kilo Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how Kilo models are assigned to each SDD execution phase (explore → apply → archive):"))
	b.WriteString("\n\n")

	for idx, preset := range kiloPresetOrder {
		isSelected := preset == state.Preset
		focused := idx == cursor
		b.WriteString(renderRadio(string(preset), isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+kiloPresetDescriptions[preset]) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"← Back"}, cursor-len(kiloPresetOrder)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

func renderKiloCustomPhaseList(state KiloModelPickerState, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Custom Kilo Model Assignments"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Press enter on a phase to cycle through known models (auto, sonnet, opus, gpt-4o, gemini, deepseek, ...)"))
	b.WriteString("\n\n")

	for idx, phase := range sddPhases {
		focused := idx == cursor
		alias := state.CustomAssignments[phase]
		if alias == "" {
			alias = model.KiloModelAuto
		}

		label := fmt.Sprintf("%-20s %s", sddPhaseLabels[phase], kiloAliasTag(alias))

		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
	}

	b.WriteString("\n")
	actionCursor := cursor - len(sddPhases)
	b.WriteString(renderOptions([]string{"Confirm", "← Back"}, actionCursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: cycle/select • esc: back"))

	return b.String()
}

func kiloAliasTag(alias model.KiloModelAlias) string {
	switch alias {
	case model.KiloModelAuto:
		return styles.SuccessStyle.Render("[auto]")
	case model.KiloModelGateway:
		return styles.SuccessStyle.Render("[gateway]")
	// Anthropic
	case model.KiloModelSonnet, model.KiloModelSonnet4:
		return styles.SuccessStyle.Render("[sonnet]")
	case model.KiloModelOpus, model.KiloModelOpus4:
		return styles.WarningStyle.Render("[opus]")
	case model.KiloModelHaiku:
		return styles.SubtextStyle.Render("[haiku]")
	// OpenAI
	case model.KiloModelGPT4o, model.KiloModelGPT4oMini:
		return styles.SuccessStyle.Render("[gpt-4o]")
	case model.KiloModelO1, model.KiloModelO3, model.KiloModelO3Mini, model.KiloModelO4Mini:
		return styles.WarningStyle.Render("[openai-reasoning]")
	// Google
	case model.KiloModelGemini25Pro, model.KiloModelGemini25Flash, model.KiloModelGemini20Flash:
		return styles.SuccessStyle.Render("[gemini]")
	// Open-weight
	case model.KiloModelDeepSeek, model.KiloModelDeepSeekR1:
		return styles.WarningStyle.Render("[deepseek]")
	case model.KiloModelQwen, model.KiloModelQwen3:
		return styles.SuccessStyle.Render("[qwen]")
	case model.KiloModelLlama:
		return styles.SuccessStyle.Render("[llama]")
	case model.KiloModelMistral:
		return styles.SuccessStyle.Render("[mistral]")
	default:
		// Custom pass-through model: show the alias string itself.
		return styles.SubtextStyle.Render("[" + string(alias) + "]")
	}
}
