package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func SDDModeOptions() []model.SDDModeID {
	return []model.SDDModeID{model.SDDModeSingle, model.SDDModeMulti}
}

var sddModeDescriptions = map[model.SDDModeID]string{
	model.SDDModeSingle: "Single orchestrator — one agent handles all SDD phases",
	model.SDDModeMulti:  "Multi-agent — dedicated sub-agent per SDD phase (9 hidden agents)",
}

func RenderSDDMode(selected model.SDDModeID, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select SDD Mode"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("How should the SDD orchestrator be configured for OpenCode?"))
	b.WriteString("\n\n")

	for idx, mode := range SDDModeOptions() {
		isSelected := mode == selected
		focused := idx == cursor
		b.WriteString(renderRadio(string(mode), isSelected, focused))
		b.WriteString(styles.SubtextStyle.Render("    "+sddModeDescriptions[mode]) + "\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Back"}, cursor-len(SDDModeOptions())))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
