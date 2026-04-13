package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderABInstalling renders the installation-in-progress (or error) screen.
func RenderABInstalling(engineName string, spinnerFrame int, installErr error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Installing Your Agent..."))
	b.WriteString("\n\n")

	if installErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Installation failed"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  Engine: " + engineName))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  Error: " + installErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Retry", "Back"}, 0))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter: select • esc: back"))
		return b.String()
	}

	b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Writing skill files..."))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Installing SKILL.md to all detected agents. This should be quick."))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("please wait..."))

	return b.String()
}
