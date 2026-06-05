package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

func PresetOptions() []model.PresetID {
	return []model.PresetID{
		model.PresetFullGentleman,
		model.PresetEcosystemOnly,
		model.PresetMinimal,
		model.PresetCustom,
	}
}

var presetDescriptions = map[model.PresetID]string{
	model.PresetMinimal:       "Just Engram persistent memory across sessions",
	model.PresetEcosystemOnly: "Memory + SDD + skills + docs + GGA",
	model.PresetFullGentleman: "Dev Stack + security gates, theme, and logo",
	model.PresetCustom:        "Choose components and skills manually; keep existing persona/settings unmanaged",
}

var presetLabels = map[model.PresetID]string{
	model.PresetMinimal:       "Memory Only",
	model.PresetEcosystemOnly: "Dev Stack",
	model.PresetFullGentleman: "Dev Stack + Polish",
	model.PresetCustom:        "Custom",
}

func RenderPreset(selected model.PresetID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select Ecosystem Preset"))
	b.WriteString("\n\n")

	for idx, preset := range PresetOptions() {
		isSelected := preset == selected
		focused := idx == cursor
		b.WriteString(renderRadio(presetLabels[preset], isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+presetDescriptions[preset]) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Back"}, cursor-len(PresetOptions())))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
