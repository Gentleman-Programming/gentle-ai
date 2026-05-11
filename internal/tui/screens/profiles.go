package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderProfiles renders the SDD Profiles list screen for the given adapter.
// adapterLabel is "OpenCode" or "VS Code" — it drives the title and subtitle text.
// deleteErr is displayed when non-nil (e.g. RemoveProfileAgents returned an error).
func RenderProfiles(profiles []model.Profile, cursor int, deleteErr error, adapterLabel string) string {
	var b strings.Builder

	title := adapterLabel + " SDD Profiles"
	b.WriteString(styles.TitleStyle.Render(title))
	b.WriteString("\n\n")

	var subtitle string
	switch adapterLabel {
	case "VS Code":
		subtitle = "Your SDD model profiles for VS Code Copilot. Each profile creates 10 .agent.md files in ~/.copilot/agents/."
	default:
		subtitle = "Your SDD model profiles for OpenCode. Each profile creates its own orchestrator (visible with Tab)."
	}
	b.WriteString(styles.SubtextStyle.Render(subtitle))
	b.WriteString("\n\n")

	if deleteErr != nil {
		b.WriteString(styles.WarningStyle.Render("Error deleting profile: " + deleteErr.Error()))
		b.WriteString("\n\n")
	}

	if len(profiles) == 0 {
		b.WriteString(styles.SubtextStyle.Render("No named profiles yet. Create one to assign different models per profile."))
		b.WriteString("\n\n")
	}

	// Build the full options list: profiles + "Create new profile" + "Back".
	options := make([]string, 0, len(profiles)+2)
	for _, p := range profiles {
		var label string
		if p.OrchestratorModel.ProviderID != "" {
			label = fmt.Sprintf("• %s ─── %s/%s", p.Name, p.OrchestratorModel.ProviderID, p.OrchestratorModel.ModelID)
		} else {
			label = fmt.Sprintf("• %s ─── (no model assigned)", p.Name)
		}
		options = append(options, label)
	}
	options = append(options, "Create new profile")
	options = append(options, "Back")

	b.WriteString(renderOptions(options, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: edit • n: new • d: delete • esc: back"))

	return styles.FrameStyle.Render(b.String())
}

// ProfileListOptionCount returns the number of selectable options on the profiles
// screen: one per profile + "Create new profile" + "Back".
func ProfileListOptionCount(profiles []model.Profile) int {
	return len(profiles) + 2
}
