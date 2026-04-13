package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// ABPreviewActions returns the action options shown on the preview screen.
func ABPreviewActions() []string {
	return []string{"Install", "Regenerate", "Back"}
}

// RenderABPreview renders the generated-agent preview screen with a scrollable content pane.
func RenderABPreview(agent *agentbuilder.GeneratedAgent, targets []string, scroll int, height int, cursor int, installErr error, conflictWarning string) string {
	var b strings.Builder

	if agent == nil {
		b.WriteString(styles.ErrorStyle.Render("No agent generated yet."))
		return b.String()
	}

	if installErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Installation failed: " + installErr.Error()))
		b.WriteString("\n\n")
	}

	if conflictWarning != "" {
		b.WriteString(styles.WarningStyle.Render("⚠ " + conflictWarning))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.TitleStyle.Render("Preview: " + agent.Title))
	b.WriteString("\n\n")

	// Metadata block.
	b.WriteString(styles.HeadingStyle.Render("Name:        ") + styles.UnselectedStyle.Render(agent.Name))
	b.WriteString("\n")
	b.WriteString(styles.HeadingStyle.Render("Description: ") + styles.SubtextStyle.Render(agent.Description))
	b.WriteString("\n")
	b.WriteString(styles.HeadingStyle.Render("Trigger:     ") + styles.SubtextStyle.Render(agent.Trigger))
	b.WriteString("\n")

	if agent.SDDConfig != nil {
		sddInfo := string(agent.SDDConfig.Mode)
		if agent.SDDConfig.TargetPhase != "" {
			sddInfo += " → " + agent.SDDConfig.TargetPhase
		}
		b.WriteString(styles.HeadingStyle.Render("SDD:         ") + styles.SubtextStyle.Render(sddInfo))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Scrollable content pane.
	b.WriteString(styles.HeadingStyle.Render("SKILL.md content:"))
	b.WriteString("\n")

	lines := strings.Split(agent.Content, "\n")
	visibleLines := height - 18 // reserve rows for header, metadata, footer
	if visibleLines < 5 {
		visibleLines = 5
	}

	maxScroll := len(lines) - visibleLines
	if maxScroll < 0 {
		maxScroll = 0
	}

	actualScroll := scroll
	if actualScroll > maxScroll {
		actualScroll = maxScroll
	}
	if actualScroll < 0 {
		actualScroll = 0
	}

	end := actualScroll + visibleLines
	if end > len(lines) {
		end = len(lines)
	}

	for _, line := range lines[actualScroll:end] {
		b.WriteString(styles.SubtextStyle.Render("  " + line))
		b.WriteString("\n")
	}

	if len(lines) > visibleLines {
		b.WriteString(styles.HelpStyle.Render(fmt.Sprintf(
			"  [lines %d-%d of %d — ↑/↓ to scroll]",
			actualScroll+1, end, len(lines),
		)))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Installation targets.
	if len(targets) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Will be installed to:"))
		b.WriteString("\n")
		for _, t := range targets {
			b.WriteString(styles.SubtextStyle.Render("  " + t))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Action bar.
	b.WriteString(renderOptions(ABPreviewActions(), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("↑/↓: scroll content • j/k: navigate actions • enter: select • esc: back"))

	return b.String()
}
