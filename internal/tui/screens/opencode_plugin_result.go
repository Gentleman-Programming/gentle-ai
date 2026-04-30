package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

func RenderOpenCodePluginResult(results []opencodeplugin.Result, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("OpenCode Community Plugins"))
	b.WriteString("\n\n")

	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("Plugin registration failed"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(err.Error()))
		b.WriteString("\n\n")
	} else if len(results) == 0 {
		b.WriteString(styles.WarningStyle.Render("No plugins selected."))
		b.WriteString("\n\n")
	} else {
		changed := 0
		files := map[string]bool{}
		for _, result := range results {
			if result.Changed {
				changed++
			}
			for _, file := range result.Files {
				files[file] = true
			}
		}

		b.WriteString(styles.SuccessStyle.Render("✓ OpenCode community plugins registered"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("%d selected, %d updated. Restart/reload OpenCode so it refreshes plugins.", len(results), changed)))
		b.WriteString("\n")
		for file := range files {
			b.WriteString(styles.SubtextStyle.Render("Updated: " + file))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.SelectedStyle.Render("> Return to menu"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return to menu • q: quit"))

	return styles.FrameStyle.Render(b.String())
}
