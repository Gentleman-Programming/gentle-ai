package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// RenderShellPlan lists the installation steps the host worker will complete in
// order after the terminal is restored. It grants no installation authority.
func RenderShellPlan(steps []string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{styles.TitleStyle.Render("Installation plan")}
	for index, step := range steps {
		lines = append(lines, fmt.Sprintf("  %d. %s", index+1, step))
	}
	lines = append(lines, "", styles.HelpStyle.Render("Enter: install • Esc: back"))
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
