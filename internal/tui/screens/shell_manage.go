package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ShellManageChange is one detected preference or component difference.
type ShellManageChange struct {
	Label, From, To string
	Component       bool
}

// RenderShellManage summarizes the selected deltas before applying anything.
func RenderShellManage(target, mode string, preferences, components []ShellManageChange, applied bool, applyErr string, actions []string, cursor, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	kind, launcher := shellManageInstallKind(mode)
	lines := []string{
		styles.TitleStyle.Render("Manage installation"),
		detectedSubtleStyle.Render(shellDetectedTarget(target)),
		detectedLabelStyle.Render("TYPE") + detectedValueStyle.Render(kind) + detectedSubtleStyle.Render("  ·  opens with "+launcher),
		"",
	}
	if len(preferences) == 0 && len(components) == 0 {
		lines = append(lines, "No changes selected. Back to change preferences.")
	}
	changeLines := func(changes []ShellManageChange) []string {
		var out []string
		for _, change := range changes {
			out = append(out, fmt.Sprintf("  %-14s %s → %s", change.Label, stringOr(change.From, "inherit"), stringOr(change.To, "inherit")))
		}
		return out
	}
	if len(preferences) > 0 {
		lines = append(lines, styles.HeadingStyle.Render("PREFERENCE CHANGES"))
		lines = append(lines, changeLines(preferences)...)
		lines = append(lines, "")
	}
	if len(components) > 0 {
		lines = append(lines, styles.WarningStyle.Render("COMPONENT CHANGES (not applied here)"))
		lines = append(lines, changeLines(components)...)
		lines = append(lines, detectedSubtleStyle.Render("These need a component update; changing preferences does not reinstall the stack."), "")
	}
	if applyErr != "" {
		lines = append(lines, styles.ErrorStyle.Render(applyErr), "")
	}
	if applied {
		lines = append(lines, styles.SuccessStyle.Render("Preferences updated."))
		if len(components) > 0 {
			lines = append(lines, styles.WarningStyle.Render("Component changes remain pending."))
		}
		lines = append(lines, "", detectedSubtleStyle.Render("Enter: back to menu"))
		return strings.Join(lines, "\n")
	}
	for index, action := range actions {
		if action == "Uninstall this installation" {
			lines = append(lines, "")
		}
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

// RenderShellUninstall asks for explicit confirmation and preserves the tree.
func RenderShellUninstall(target, mode string, uninstalled, working bool, backup, applyErr string, cursor, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	kind, launcher := shellManageInstallKind(mode)
	lines := []string{
		styles.TitleStyle.Render("Uninstall installation"),
		detectedSubtleStyle.Render(shellDetectedTarget(target)),
		detectedLabelStyle.Render("TYPE") + detectedValueStyle.Render(kind) + detectedSubtleStyle.Render("  ·  opens with "+launcher),
		"",
	}
	if working {
		lines = append(lines, styles.WarningStyle.Render("Working: uninstalling and pruning the backup..."), detectedSubtleStyle.Render("Do not close this terminal."))
		return strings.Join(lines, "\n")
	}
	if uninstalled {
		lines = append(lines, styles.SuccessStyle.Render("Installation uninstalled."), detectedSubtleStyle.Render("Backup: "+backup), "", detectedSubtleStyle.Render("Enter: back to menu"))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "This moves the target to a timestamped backup and removes it from detection.")
	if mode == "agent" {
		lines = append(lines, "Managed Pi settings/MCP are restored only if they are unchanged; otherwise uninstall aborts.", "Your projects are untouched.")
	} else {
		lines = append(lines, "Your personal Pi and projects are untouched.")
	}
	lines = append(lines, "")
	if applyErr != "" {
		lines = append(lines, styles.ErrorStyle.Render(applyErr), "")
	}
	for index, action := range []string{"Uninstall (keep backup)", "Back"} {
		marker, style := "   ", detectedValueStyle
		if index == cursor {
			marker, style = " ▸ ", detectedAccentStyle
		}
		lines = append(lines, marker+style.Render(action))
	}
	lines = append(lines, "", detectedSubtleStyle.Render("j/k: navigate  •  enter: select  •  esc: back"))
	return strings.Join(lines, "\n")
}

func shellManageInstallKind(mode string) (string, string) {
	switch mode {
	case "agent":
		return "On my existing Pi", "pi"
	case "shared":
		return "Shared Pi prefix", "pi"
	default:
		return "Separate install", "gentle-shell"
	}
}

func stringOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
