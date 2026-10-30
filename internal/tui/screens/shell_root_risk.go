package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// RenderShellRootRisk is the typed root acceptance screen shown before any
// plan-driven install when the operator runs as root. Nothing is installed
// until the exact phrase is entered; Escape returns to the previous screen.
func RenderShellRootRisk(input string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{
		styles.TitleStyle.Render("Root installation warning"),
		"",
		"You are installing as root. The private sandbox (capability drop,",
		"NoNewPrivileges, delegated unit limits) is disabled for root, and every",
		"command the agent runs will run with root powers.",
		"",
		"Recommended alternative: create a regular user and install from it",
		"(adduser <name> && su - <name>).",
		"",
		fmt.Sprintf("Type %q (without quotes) to accept the risk.", shellinstaller.RootRiskPhrase),
		"> " + input,
		"",
		styles.HelpStyle.Render("Enter: accept • Escape: cancel"),
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
