package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderProfileDelete renders the profile delete confirmation screen.
// When isVSCode is false, shows OpenCode wording (11 agent keys, "Delete & Sync").
// When isVSCode is true, shows VS Code wording (10 agent files, "Delete").
func RenderProfileDelete(profileName string, cursor int, isVSCode bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Delete Profile"))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("Are you sure you want to delete profile %q?", profileName)))
	b.WriteString("\n\n")

	if isVSCode {
		b.WriteString(styles.SubtextStyle.Render("The following 10 agent files will be removed from ~/.copilot/agents/:"))
		b.WriteString("\n\n")

		vscodePhases := []string{
			"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
			"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
		}
		for _, phase := range vscodePhases {
			b.WriteString(styles.UnselectedStyle.Render("  • " + phase + "-" + profileName + ".agent.md"))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render("This action cannot be undone."))
		b.WriteString("\n\n")

		b.WriteString(renderOptions([]string{"Delete", "Cancel"}, cursor))
	} else {
		b.WriteString(styles.SubtextStyle.Render("The following 11 agent keys will be removed from opencode.json:"))
		b.WriteString("\n\n")

		// Show orchestrator key.
		b.WriteString(styles.UnselectedStyle.Render("  • sdd-orchestrator-" + profileName))
		b.WriteString("\n")

		// Show phase keys using the canonical phase list from the sdd package.
		for _, phase := range sdd.ProfilePhaseOrder() {
			b.WriteString(styles.UnselectedStyle.Render("  • " + phase + "-" + profileName))
			b.WriteString("\n")
		}

		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render("This action cannot be undone."))
		b.WriteString("\n\n")

		b.WriteString(renderOptions([]string{"Delete & Sync", "Cancel"}, cursor))
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: confirm • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// ProfileDeleteOptionCount returns the number of options on the delete
// confirmation screen: "Delete & Sync" + "Cancel" = 2.
func ProfileDeleteOptionCount() int {
	return 2
}
