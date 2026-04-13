package screens

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderABPrompt renders the prompt input screen for the agent builder flow.
func RenderABPrompt(ta textarea.Model) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Describe Your Agent"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("What should your custom agent do? Some ideas:"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Review CSS for a11y issues"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Generate API docs from code"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Validate DB migrations"))
	b.WriteString("\n\n")

	b.WriteString(ta.View())
	b.WriteString("\n\n")

	if ta.Value() == "" {
		b.WriteString(styles.SubtextStyle.Render("(type a description to continue)"))
	} else {
		b.WriteString(styles.SuccessStyle.Render("tab: continue (enter adds a new line)"))
	}
	b.WriteString("\n\n")

	b.WriteString(renderOptions([]string{"Back"}, -1))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("type your description • ctrl+enter or tab to continue • esc: back"))

	return b.String()
}
