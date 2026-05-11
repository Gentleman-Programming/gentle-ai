package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// DuplicatedSDDPhases lists the SDD phases that appear in both the Copilot
// native (.agent.md) and the Claude (.md) agent registries. VS Code Copilot
// scans both formats, so when a user installs SDD multi-mode for both
// adapters these phases visually duplicate in the Agent customizations panel.
//
// sdd-init and sdd-onboard are intentionally omitted: the Claude adapter
// does not ship them as sub-agents (they are orchestrator-driven flows), so
// they never duplicate. Keep this list aligned with the Claude adapter's
// embedded agents directory.
func DuplicatedSDDPhases() []string {
	return []string{
		"sdd-apply",
		"sdd-archive",
		"sdd-design",
		"sdd-explore",
		"sdd-propose",
		"sdd-spec",
		"sdd-tasks",
		"sdd-verify",
	}
}

// SDDDuplicateAgentsWarningOptions returns the selectable options for the
// warning screen, in cursor order.
func SDDDuplicateAgentsWarningOptions() []string {
	return []string{"Continue anyway", "← Back to adapter selection"}
}

// RenderSDDDuplicateAgentsWarning renders an informational warning that
// fires when SDD multi-mode is paired with both VS Code Copilot and a
// Claude-format adapter. The user can continue (accept the duplication)
// or go back to the adapter selection.
func RenderSDDDuplicateAgentsWarning(cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Heads up: VS Code will show duplicated SDD agents"))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("You're installing SDD multi-mode for both VS Code Copilot and Claude Code."))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("VS Code Copilot's agent panel reads two formats in parallel:"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  • Copilot native: ~/.copilot/agents/*.agent.md"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("  • Claude format:  ~/.claude/agents/*.md"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("These 8 sub-agents will appear twice in VS Code:"))
	b.WriteString("\n")
	phases := DuplicatedSDDPhases()
	for _, phase := range phases {
		b.WriteString(styles.UnselectedStyle.Render("  • " + phase))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(styles.SubtextStyle.Render("Each file is correct and works in its own host — no behavior difference."))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("This is purely a UI quirk of VS Code's multi-format agent scanner."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions(SDDDuplicateAgentsWarningOptions(), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return styles.FrameStyle.Render(b.String())
}
