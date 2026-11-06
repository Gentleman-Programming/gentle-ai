package screens

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ShellDetectedInstall is the read-only discovery summary. It grants no
// installation or replacement authority.
type ShellDetectedInstall struct {
	Target, Mode, Channel         string
	Persona, Background           string
	TDD, RDD, CodeGraph, Context7 string
}

// ShellDetectedEntry is one installation row in the multi-install selector.
type ShellDetectedEntry struct {
	Target, Mode, Channel string
}

var (
	detectedCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.ColorLavender).
				Padding(1, 2)

	detectedLabelStyle = lipgloss.NewStyle().
				Foreground(styles.ColorSubtext).
				Width(14)

	detectedValueStyle = lipgloss.NewStyle().
				Foreground(styles.ColorText)

	detectedAccentStyle = lipgloss.NewStyle().
				Foreground(styles.ColorLavender).
				Bold(true)

	detectedSubtleStyle = lipgloss.NewStyle().
				Foreground(styles.ColorSubtext)
)

func shellDetectedTarget(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func shellDetectedValue(value string) string {
	switch value {
	case "":
		return "Inherit"
	case "on":
		return "On"
	case "off":
		return "Off"
	case "gentleman":
		return "Gentleman"
	case "neutral":
		return "Neutral"
	case "unmanaged":
		return "Unmanaged"
	default:
		return value
	}
}

func shellDetectedToggle(label, value string) string {
	raw := value
	if raw == "" {
		raw = "inherit"
	}
	color := styles.ColorOverlay
	switch raw {
	case "on":
		color = styles.ColorGreen
	case "off":
		color = styles.ColorRed
	}
	return label + " " + lipgloss.NewStyle().Foreground(color).Render(raw)
}

func shellDetectedRow(label, value string) string {
	return detectedLabelStyle.Render(label) + detectedValueStyle.Render(value)
}

func shellDetectedCard(content string, width, height int) string {
	// Keep the same left-aligned, full-width reading order as the other TUI
	// screens instead of floating the card in the middle of the terminal.
	cardWidth := max(30, width-6)
	return detectedCardStyle.Width(cardWidth).Render(content)
}

// RenderShellDetected identifies an existing managed installation before the
// operator enters the same customization screen used for a fresh install.
func RenderShellDetected(install ShellDetectedInstall, cursor, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	target := shellDetectedTarget(install.Target)
	if runes := []rune(target); len(runes) > 58 {
		target = string(runes[:28]) + "…" + string(runes[len(runes)-28:])
	}
	var lines []string
	lines = append(lines,
		detectedAccentStyle.Render("◆  Gentle-Shell detected"),
		detectedSubtleStyle.Render("Found a managed Gentle-Shell installation"),
		"",
		styles.SuccessStyle.Render("● Managed")+"  "+detectedSubtleStyle.Render(target),
		"",
		shellDetectedRow("MODE", install.Mode),
		shellDetectedRow("CHANNEL", install.Channel),
		shellDetectedRow("PERSONA", shellDetectedValue(install.Persona)),
		shellDetectedRow("BACKGROUND", shellDetectedValue(install.Background)),
		shellDetectedRow("WORKFLOWS", shellDetectedToggle("TDD", install.TDD)+"  ·  "+shellDetectedToggle("RDD", install.RDD)+"  ·  "+shellDetectedToggle("CodeGraph", install.CodeGraph)),
		shellDetectedRow("INTEGRATIONS", shellDetectedToggle("Context7", install.Context7)),
		"",
		detectedSubtleStyle.Render(strings.Repeat("─", 64)),
	)
	actions := []string{"Manage this installation", "Back"}
	for index, action := range actions {
		marker := "   "
		label := detectedValueStyle.Render(action)
		if index == cursor {
			marker = " ▸ "
			label = detectedAccentStyle.Render(action)
		}
		lines = append(lines, marker+label)
	}
	lines = append(lines, "", detectedSubtleStyle.Render("j/k: navigate  •  enter: select  •  esc: back"))
	return shellDetectedCard(strings.Join(lines, "\n"), width, height)
}

// RenderShellDetectedList never guesses between installations: the operator
// selects the target to manage.
func RenderShellDetectedList(entries []ShellDetectedEntry, cursor, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := []string{
		detectedAccentStyle.Render("◆  Gentle-Shell installations detected"),
		detectedSubtleStyle.Render("Choose the installation to manage"),
		"",
	}
	for index, entry := range entries {
		marker := "   "
		mode := detectedValueStyle.Render(entry.Mode)
		if index == cursor {
			marker = " ▸ "
			mode = detectedAccentStyle.Render(entry.Mode)
		}
		target := shellDetectedTarget(entry.Target)
		if runes := []rune(target); len(runes) > 56 {
			target = string(runes[:26]) + "…" + string(runes[len(runes)-26:])
		}
		lines = append(lines,
			marker+mode+detectedSubtleStyle.Render("  ·  "+entry.Channel),
			"      "+detectedSubtleStyle.Render(target),
		)
	}
	lines = append(lines, "", detectedSubtleStyle.Render("j/k: navigate  •  enter: manage selected  •  esc: back"))
	return shellDetectedCard(strings.Join(lines, "\n"), width, height)
}
