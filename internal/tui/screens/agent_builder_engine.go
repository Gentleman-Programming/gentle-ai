package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ABEngineOptions returns display names for the available generation engines.
func ABEngineOptions(engines []model.AgentID) []string {
	opts := make([]string, 0, len(engines)+1)
	for _, e := range engines {
		opts = append(opts, string(e))
	}
	opts = append(opts, "Back")
	return opts
}

// RenderABEngine renders the engine selection screen for the agent builder flow.
// When availableEngines is empty and detecting is true, it displays that an engine scan is in progress.
func RenderABEngine(availableEngines []model.AgentID, cursor int, detecting ...bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Choose Your AI Engine"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Which installed agent should help you build your sub-agent?"))
	b.WriteString("\n\n")

	if len(availableEngines) == 0 {
		isDetecting := len(detecting) > 0 && detecting[0]
		if isDetecting {
			b.WriteString(styles.SubtextStyle.Render("Scanning for supported AI agent binaries..."))
		} else {
			b.WriteString(styles.WarningStyle.Render("No supported AI agent binaries found on PATH."))
			b.WriteString("\n" + styles.SubtextStyle.Render("Install claude, opencode, gemini, or codex and try again."))
		}
		b.WriteString("\n\n" + renderOptions([]string{"Back"}, cursor))
	} else {
		b.WriteString(renderOptions(ABEngineOptions(availableEngines), cursor))
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
