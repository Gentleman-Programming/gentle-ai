package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// OpenCodeBackgroundOptions returns the choices shown when the background
// preference is otherwise unresolved in the interactive installer.
func OpenCodeBackgroundOptions() []string {
	return []string{"Enable managed background subagents", "Keep foreground"}
}

// RenderOpenCodeBackground renders the final OpenCode background execution
// choice before the install transaction starts.
func RenderOpenCodeBackground(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("OpenCode Background Subagents"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Choose how OpenCode should run SDD subagents."))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("Managed activation starts OpenCode with its background-subagent environment."))
	b.WriteString("\n\n")
	b.WriteString(renderOptions(OpenCodeBackgroundOptions(), cursor))
	b.WriteString(renderOptions([]string{"Back"}, cursor-len(OpenCodeBackgroundOptions())))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
