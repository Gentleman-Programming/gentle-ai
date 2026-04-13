package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderProfileDelete renders the profile delete confirmation screen.
// It shows the profile name, the 11 agent keys that will be removed, and
// "Delete & Sync" / "Cancel" options.
func RenderProfileDelete(profileName string, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Delete Profile"))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("Are you sure you want to delete profile %q?", profileName)))
	b.WriteString("\n\n")

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
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: confirm • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// ProfileDeleteOptionCount returns the number of options on the delete
// confirmation screen: "Delete & Sync" + "Cancel" = 2.
func ProfileDeleteOptionCount() int {
	return 2
}
