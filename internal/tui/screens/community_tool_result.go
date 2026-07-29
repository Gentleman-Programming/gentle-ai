package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/communitytool"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

func RenderCommunityToolResult(results []communitytool.Result, err error) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Community Tools/Plugins"))
	b.WriteString("\n\n")

	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("Community tool setup failed"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(err.Error()))
		b.WriteString("\n\n")
		renderCommunityToolResultDetails(&b, results)
	} else if len(results) == 0 {
		b.WriteString(styles.WarningStyle.Render("No community tools selected."))
		b.WriteString("\n\n")
	} else {
		b.WriteString(styles.SuccessStyle.Render("✓ Community tools configured"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%d selected.", len(results))))
		b.WriteString("\n")
		renderCommunityToolResultDetails(&b, results)
		b.WriteString("\n")
	}

	b.WriteString(styles.SelectedStyle.Render("> Return to menu"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return to menu • q: quit"))
	return styles.FrameStyle.Render(b.String())
}

func renderCommunityToolResultDetails(b *strings.Builder, results []communitytool.Result) {
	for _, result := range results {
		status := result.StatusAfter
		if status == nil {
			status = result.StatusBefore
		}
		if status == nil {
			continue
		}
		detected, configured, missing := status.DetectedConfiguredMissingCounts()
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%s: CLI %s • %d detected agents • %d configured • %d missing", toolName(result.Tool), status.CLI, detected, configured, missing)))
		b.WriteString("\n")
		for _, agent := range status.Agents {
			if !agent.Detected {
				continue
			}
			state := "missing"
			if agent.Configured {
				state = "configured"
			}
			b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("  - %s: %s", agent.Name, state)))
			b.WriteString("\n")
		}
	}
	for _, result := range results {
		for _, action := range result.ManualActions {
			b.WriteString(styles.SubtextStyle.Render("Next: " + action))
			b.WriteString("\n")
		}
	}
}

func toolName(id model.CommunityToolID) string {
	if def, ok := communitytool.DefinitionFor(id); ok {
		return def.Name
	}
	return string(id)
}
