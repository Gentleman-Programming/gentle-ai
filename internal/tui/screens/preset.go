package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/tui/styles"
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
	model.PresetFullGentleman: "Ecosistema completo sin instalar temas visuales",
	model.PresetCustom:        "Elige cada componente: memoria, persona, herramientas y más",
}

var presetLabels = map[model.PresetID]string{
	model.PresetMinimal:       "Memory Only",
	model.PresetEcosystemOnly: "Dev Stack",
	model.PresetFullGentleman: "Ecosistema completo",
	model.PresetCustom:        "Custom",
}

func RenderPreset(selected model.PresetID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Seleccionar Preset del Ecosistema"))
	b.WriteString("\n\n")

	for idx, preset := range PresetOptions() {
		isSelected := preset == selected
		focused := idx == cursor
		b.WriteString(renderRadio(presetLabels[preset], isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+presetDescriptions[preset]) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Volver"}, cursor-len(PresetOptions())))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navegar • enter: seleccionar • esc: volver"))

	return b.String()
}
