package screens

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agentbuilder"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// sanitizeLabel removes ANSI escape codes, newlines, and non-printable control characters from text before rendering.
func sanitizeLabel(s string) string {
	s = ansiRegex.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// RenderCustomAgents renders the Custom Agents list and management screen.
// It displays existing custom agents and provides actions to create, delete, or return.
func RenderCustomAgents(agents []agentbuilder.RegistryEntry, cursor int, err error, hasEngines bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Manage Custom Agents"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Create or remove custom agents installed across your configured AI tools."))
	b.WriteString("\n\n")

	if err != nil {
		b.WriteString(styles.WarningStyle.Render("Error: " + err.Error()))
		b.WriteString("\n\n")
	}

	if len(agents) == 0 && err == nil {
		emptyMsg := "No custom agents created yet. Use 'Create new agent' to build one."
		if !hasEngines {
			emptyMsg = "No custom agents created yet. Install an agent-builder engine to create one."
		}
		b.WriteString(styles.SubtextStyle.Render(emptyMsg))
		b.WriteString("\n\n")
	}

	options := make([]string, 0, len(agents)+2)
	for _, a := range agents {
		name := sanitizeLabel(a.Name)
		title := sanitizeLabel(a.Title)
		label := fmt.Sprintf("• %s", name)
		if title != "" {
			label = fmt.Sprintf("• %s ─── %s", name, title)
		}
		options = append(options, label)
	}

	createLabel := "Create new agent"
	if !hasEngines {
		createLabel = "Create new agent (no engine available)"
	}
	options = append(options, createLabel)
	options = append(options, "Back")

	b.WriteString(renderOptions(options, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select/create • d: delete • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// CustomAgentsOptionCount returns the number of selectable options on the screen.
func CustomAgentsOptionCount(agents []agentbuilder.RegistryEntry) int {
	return len(agents) + 2
}

// RenderCustomAgentDelete renders the deletion screen with checkboxes for installed custom agents.
func RenderCustomAgentDelete(agents []agentbuilder.RegistryEntry, selected map[string]bool, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Delete Custom Agents"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Select custom agents to delete. Use space to toggle, enter on 'Delete Selected' to confirm."))
	b.WriteString("\n\n")

	if len(agents) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No custom agents installed."))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Back"}, cursor))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter/esc: back"))
		return styles.FrameStyle.Render(b.String())
	}

	for idx, a := range agents {
		name := sanitizeLabel(a.Name)
		title := sanitizeLabel(a.Title)
		label := name
		if title != "" {
			label = fmt.Sprintf("%s ─── %s", name, title)
		}
		checked := selected != nil && selected[a.Name]
		focused := idx == cursor
		b.WriteString(renderCheckbox(label, checked, focused))
	}

	b.WriteString("\n")
	actions := []string{"Delete Selected", "Cancel"}
	b.WriteString(renderOptions(actions, cursor-len(agents)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space: toggle • enter: confirm/cancel • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// CustomAgentDeleteOptionCount returns the number of options on the deletion screen.
func CustomAgentDeleteOptionCount(agents []agentbuilder.RegistryEntry) int {
	if len(agents) == 0 {
		return 1
	}
	return len(agents) + 2
}
