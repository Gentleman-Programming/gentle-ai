package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func RenderCommunityTools(selected []model.CommunityToolID, cursor int, statuses []communitytool.Status, loading bool, statusErr error) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Community Tools/Plugins"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Optional cross-agent tools Gentle AI can install and wire for you."))
	b.WriteString("\n\n")

	if loading {
		b.WriteString(styles.SelectedStyle.Render("⠋ Detecting installed tool and agent wiring…"))
		b.WriteString("\n\n")
	} else if statusErr != nil {
		b.WriteString(styles.WarningStyle.Render("Could not detect current community tool status: " + statusErr.Error()))
		b.WriteString("\n\n")
	} else if len(statuses) > 0 {
		for _, status := range statuses {
			renderCommunityToolStatus(&b, status)
		}
		b.WriteString("\n")
	}

	selectedSet := map[model.CommunityToolID]bool{}
	for _, id := range selected {
		selectedSet[id] = true
	}

	row := 0
	for _, def := range communitytool.Definitions() {
		checkbox := "[ ]"
		if selectedSet[def.ID] {
			checkbox = "[x]"
		}
		line := fmt.Sprintf("%s %s — %s", checkbox, def.Name, def.Description)
		if cursor == row {
			b.WriteString(styles.SelectedStyle.Render("> "+line) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+line) + "\n")
		}
		row++
		repoLine := fmt.Sprintf("View repo: %s", def.RepoURL)
		if cursor == row {
			b.WriteString(styles.SelectedStyle.Render("> "+repoLine) + "\n")
		} else {
			b.WriteString(styles.SubtextStyle.Render("  "+repoLine) + "\n")
		}
		row++
	}

	for _, action := range []string{"Continue", "Back"} {
		if cursor == row {
			b.WriteString(styles.SelectedStyle.Render("> "+action) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+action) + "\n")
		}
		row++
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("space/enter: toggle • repo row: open browser • esc: back"))
	return styles.FrameStyle.Render(b.String())
}

func CommunityToolsOptionCount() int {
	return len(communitytool.Definitions())*2 + 2
}

func RenderCommunityToolInstalling(selected []model.CommunityToolID, spinner string, statuses []communitytool.Status) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Community Tools/Plugins"))
	b.WriteString("\n\n")
	b.WriteString(styles.SelectedStyle.Render(fmt.Sprintf("%s Installing community tools…", spinner)))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%d selected.", len(selected))))
	b.WriteString("\n")

	if len(selected) > 0 {
		selectedNames := selectedCommunityToolNames(selected)
		b.WriteString(styles.SubtextStyle.Render("Installing: " + strings.Join(selectedNames, ", ")))
		b.WriteString("\n")
	}
	if len(statuses) > 0 {
		for _, status := range statuses {
			_, configured, missing := status.DetectedConfiguredMissingCounts()
			b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("Current %s state: %d configured, %d missing detected agent wiring", statusName(status.Tool), configured, missing)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("Please wait — setup is still running."))
	return styles.FrameStyle.Render(b.String())
}

func renderCommunityToolStatus(b *strings.Builder, status communitytool.Status) {
	cli := "missing"
	if status.CLI == communitytool.AvailabilityAvailable {
		cli = "available"
		if status.CLIPath != "" {
			cli += " at " + status.CLIPath
		}
	}
	b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%s CLI: %s", statusName(status.Tool), cli)))
	b.WriteString("\n")
	detected, configured, missing := status.DetectedConfiguredMissingCounts()
	b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("Agent wiring: %d detected • %d configured • %d missing", detected, configured, missing)))
	b.WriteString("\n")
	for _, agent := range status.Agents {
		if !agent.Detected {
			continue
		}
		marker := "missing"
		if agent.Configured {
			marker = "configured"
		}
		line := fmt.Sprintf("  - %s: %s", agent.Name, marker)
		if agent.Path != "" {
			line += " (" + agent.Path + ")"
		}
		b.WriteString(styles.SubtextStyle.Render(line))
		b.WriteString("\n")
	}
}

func statusName(id model.CommunityToolID) string {
	if def, ok := communitytool.DefinitionFor(id); ok {
		return def.Name
	}
	return string(id)
}

func selectedCommunityToolNames(selected []model.CommunityToolID) []string {
	namesByID := make(map[model.CommunityToolID]string, len(communitytool.Definitions()))
	for _, def := range communitytool.Definitions() {
		namesByID[def.ID] = def.Name
	}

	names := make([]string, 0, len(selected))
	for _, id := range selected {
		if name, ok := namesByID[id]; ok {
			names = append(names, name)
			continue
		}
		names = append(names, string(id))
	}
	return names
}
