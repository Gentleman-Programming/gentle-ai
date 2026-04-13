package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// ABSDDOptions returns the display labels for SDD integration modes.
func ABSDDOptions() []string {
	return []string{
		"Standalone — no SDD integration",
		"New SDD Phase — add a new phase to the SDD graph",
		"Phase Support — enhance an existing SDD phase",
		"Back",
	}
}

// ABSDDPhases returns the ordered list of SDD phase names.
func ABSDDPhases() []string {
	return []string{
		"explore",
		"propose",
		"spec",
		"design",
		"tasks",
		"apply",
		"verify",
		"archive",
	}
}

// RenderABSDD renders the SDD integration mode selection screen.
func RenderABSDD(mode string, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("SDD Integration"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("How should your agent integrate with the SDD workflow?"))
	b.WriteString("\n\n")

	opts := ABSDDOptions()
	for idx, opt := range opts {
		focused := idx == cursor
		// Determine if this option is the currently selected mode.
		var isSelected bool
		switch idx {
		case 0:
			isSelected = mode == "standalone"
		case 1:
			isSelected = mode == "new-phase"
		case 2:
			isSelected = mode == "phase-support"
		}
		if idx < len(opts)-1 {
			b.WriteString(renderRadio(opt, isSelected, focused))
		} else {
			// "Back" rendered as a plain option.
			b.WriteString(renderOptions([]string{opt}, cursor-len(opts)+1))
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// RenderABSDDPhase renders the SDD phase selection screen for new-phase or phase-support modes.
func RenderABSDDPhase(phases []string, cursor int, isNewPhase bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Select SDD Phase"))
	b.WriteString("\n\n")

	if isNewPhase {
		b.WriteString(styles.SubtextStyle.Render("Insert after which phase? (your new phase will follow it)"))
	} else {
		b.WriteString(styles.SubtextStyle.Render("Support which phase? (your agent will enhance this phase)"))
	}
	b.WriteString("\n\n")

	allOpts := make([]string, 0, len(phases)+1)
	for _, phase := range phases {
		var label string
		if isNewPhase {
			label = "Insert after: " + phase
		} else {
			label = "Support phase: " + phase
		}
		allOpts = append(allOpts, label)
	}
	allOpts = append(allOpts, "Back")

	b.WriteString(renderOptions(allOpts, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}
