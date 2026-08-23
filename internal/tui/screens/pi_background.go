package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// PiBackgroundOptions returns the choices shown when the Pi background
// preference is otherwise unresolved in the interactive installer.
func PiBackgroundOptions() []string {
	return []string{"Enable managed background subagents", "Keep foreground"}
}

// RenderPiBackground renders the final Pi background execution choice before
// the install transaction starts.
func RenderPiBackground(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Pi Background Subagents"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how Pi should run SDD subagents."))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("The resolved policy is projected for the installed pi-subagents extension."))
	b.WriteString("\n\n")
	b.WriteString(renderOptions(PiBackgroundOptions(), cursor))
	b.WriteString(renderOptions([]string{"Back"}, cursor-len(PiBackgroundOptions())))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
