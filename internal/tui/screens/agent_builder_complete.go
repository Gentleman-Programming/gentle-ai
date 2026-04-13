package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderABComplete renders the agent-builder completion screen.
func RenderABComplete(agent *agentbuilder.GeneratedAgent, results []agentbuilder.InstallResult) string {
	var b strings.Builder

	b.WriteString(styles.SuccessStyle.Render("✓ Agent Created!"))
	b.WriteString("\n\n")

	if agent != nil {
		b.WriteString(styles.HeadingStyle.Render("Agent: ") + styles.UnselectedStyle.Render(agent.Title))
		b.WriteString("\n\n")
	}

	// Per-agent install status lines.
	if len(results) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Installed to:"))
		b.WriteString("\n")
		for _, r := range results {
			if r.Success {
				b.WriteString("  " + styles.SuccessStyle.Render("✓") + "  " + styles.UnselectedStyle.Render(string(r.AgentID)))
				b.WriteString("\n")
				b.WriteString("     " + styles.SubtextStyle.Render(r.Path))
				b.WriteString("\n")
			} else {
				b.WriteString("  " + styles.ErrorStyle.Render("✗") + "  " + styles.ErrorStyle.Render(string(r.AgentID)))
				b.WriteString("\n")
				if r.Err != nil {
					b.WriteString("     " + styles.SubtextStyle.Render(r.Err.Error()))
					b.WriteString("\n")
				}
			}
		}
		b.WriteString("\n")
	}

	// Usage hint.
	if agent != nil && agent.Trigger != "" {
		b.WriteString(styles.HeadingStyle.Render("How to use:"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("  Your agent is active. Trigger it with:"))
		b.WriteString("\n")
		b.WriteString(styles.SelectedStyle.Render("  " + agent.Trigger))
		b.WriteString("\n\n")
	}

	b.WriteString(renderOptions([]string{"Done"}, 0))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("enter: return to menu • q: quit"))

	return b.String()
}
