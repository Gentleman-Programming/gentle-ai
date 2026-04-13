package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderABGenerating renders the generation-in-progress (or error) screen.
func RenderABGenerating(engineName string, spinnerFrame int, genErr error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Generating Your Agent..."))
	b.WriteString("\n\n")

	if genErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Generation failed"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  Engine: " + engineName))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  Error: " + genErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Retry", "Back"}, 0))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter: select • esc: back"))
		return b.String()
	}

	b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Running " + engineName + "..."))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Composing prompt and calling generation engine. This may take a moment."))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("esc: cancel"))

	return b.String()
}
