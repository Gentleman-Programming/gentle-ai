package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

func RenderOpenCodePlugins(selected []model.OpenCodeCommunityPluginID, cursor int) string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Optional OpenCode Community Plugins"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Install community TUI plugins now, or open their repos first to review them."))
	b.WriteString("\n\n")

	defs := opencodeplugin.Definitions()
	selectedSet := map[model.OpenCodeCommunityPluginID]bool{}
	for _, id := range selected {
		selectedSet[id] = true
	}

	row := 0
	for _, def := range defs {
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

func OpenCodePluginsOptionCount() int {
	return len(opencodeplugin.Definitions())*2 + 2
}
