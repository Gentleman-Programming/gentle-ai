package screens

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// RenderShellUpdate reports channel-scoped component availability only.
func RenderShellUpdate(target, mode, channel string, checking, available bool, components []string, detail, applyErr string, cursor, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	kind, launcher := shellManageInstallKind(mode)
	lines := []string{
		styles.TitleStyle.Render("Gentle-Shell updates"),
		detectedSubtleStyle.Render(shellDetectedTarget(target)),
		detectedLabelStyle.Render("TYPE") + detectedValueStyle.Render(kind) + detectedSubtleStyle.Render("  ·  opens with "+launcher),
		detectedLabelStyle.Render("CHANNEL") + detectedValueStyle.Render(channel),
		"",
	}
	if checking {
		lines = append(lines, "Checking this channel for component updates…", "", detectedSubtleStyle.Render("Esc: back"))
		return strings.Join(lines, "\n")
	}
	if applyErr != "" {
		lines = append(lines, styles.ErrorStyle.Render(applyErr), "")
	}
	if available {
		lines = append(lines, styles.WarningStyle.Render("Update available"))
		for _, component := range components {
			lines = append(lines, "  • "+component)
		}
		if detail != "" {
			lines = append(lines, detectedSubtleStyle.Render(detail))
		}
	} else {
		lines = append(lines, styles.SuccessStyle.Render("Up to date for this channel."))
	}
	lines = append(lines, "")
	actions := []string{"Back"}
	if available {
		actions = []string{"Update now", "Back"}
	}
	for index, action := range actions {
		marker, style := "   ", detectedValueStyle
		if index == cursor {
			marker, style = " ▸ ", detectedAccentStyle
		}
		lines = append(lines, marker+style.Render(action))
	}
	lines = append(lines, "", detectedSubtleStyle.Render("j/k: navigate  •  enter: select  •  esc: back"))
	if len(lines) > height {
		lines = lines[:height]
	}
	for index, line := range lines {
		if len(line) > width {
			lines[index] = line[:width]
		}
	}
	return strings.Join(lines, "\n")
}
