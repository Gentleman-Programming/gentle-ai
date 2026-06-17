package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// ABWorkflowOptions returns the workflow names with "Back" appended.
func ABWorkflowOptions(workflows []string) []string {
	opts := make([]string, 0, len(workflows)+1)
	for _, w := range workflows {
		opts = append(opts, w)
	}
	opts = append(opts, "Back")
	return opts
}

// RenderABWorkflow renders the workflow selection screen.
// workflows[0] is always "sdd" (the default).
func RenderABWorkflow(workflows []string, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select Workflow"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Which workflow should your agent follow?"))
	b.WriteString("\n\n")

	if len(workflows) == 0 {
		b.WriteString(styles.WarningStyle.Render("No workflows available."))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Back"}, cursor))
	} else {
		b.WriteString(renderOptions(ABWorkflowOptions(workflows), cursor))
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
