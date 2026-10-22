package screens

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/gentleman-programming/gentle-ai/v4/internal/tui/styles"
)

// ShellInstallHeroTitle retains the original campaign copy.
const ShellInstallHeroTitle = "Install Gentle-Shell, our own agent"

// RenderShellInstallHero restores the original install campaign, not installation state claims.
func RenderShellInstallHero(selected bool, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	plain := gentleShellHeroViewport(gentleShellHeroLines(selected, width), width, height)
	if width <= gentleShellHeroFrame.GetHorizontalFrameSize() {
		return plain
	}
	innerWidth := width - gentleShellHeroFrame.GetHorizontalFrameSize()
	framed := gentleShellHeroFrame.Width(innerWidth).Render(strings.Join(gentleShellHeroLines(selected, innerWidth), "\n"))
	if gentleShellHeroFits(framed, width, height) {
		return framed
	}
	return plain
}

var gentleShellHeroFrame = styles.PanelStyle.
	Border(lipgloss.RoundedBorder()).
	BorderForeground(styles.ColorMauve)

func gentleShellHeroLines(selected bool, width int) []string {
	content := gentleShellHeroContent()
	lines := make([]string, 0, 8)
	title := styles.TitleStyle
	if selected && content.title != "" {
		lines = append(lines, gentleShellHeroStyledLines(styles.SelectedStyle, styles.Cursor+content.title, width)...)
	} else {
		lines = append(lines, gentleShellHeroStyledLines(title, content.title, width)...)
	}
	lines = append(lines, gentleShellHeroStyledLines(styles.SubtextStyle, content.subtitle, width)...)
	if len(content.capabilities) > 0 {
		lines = append(lines, "")
		lines = append(lines, gentleShellHeroCapabilityLines(content.capabilities, width)...)
		lines = append(lines, "")
	}
	for _, detail := range content.details {
		lines = append(lines, gentleShellHeroStyledLines(styles.UnselectedStyle, detail, width)...)
	}
	return lines
}

type gentleShellHeroCapability struct {
	label string
	style lipgloss.Style
}

type gentleShellHeroContentModel struct {
	title        string
	subtitle     string
	capabilities []gentleShellHeroCapability
	details      []string
}

func gentleShellHeroCapabilityLines(capabilities []gentleShellHeroCapability, width int) []string {
	var line strings.Builder
	for index, capability := range capabilities {
		if index > 0 {
			line.WriteString(styles.SubtextStyle.Render(" • "))
		}
		line.WriteString(capability.style.Render(capability.label))
	}
	return gentleShellHeroWrap(line.String(), width)
}

func gentleShellHeroContent() gentleShellHeroContentModel {
	return gentleShellHeroContentModel{
		title:    ShellInstallHeroTitle,
		subtitle: "The complete Pi experience.",
		capabilities: []gentleShellHeroCapability{
			{label: "MEMORY", style: styles.TitleStyle},
			{label: "ODD", style: styles.HeadingStyle},
			{label: "SKILLS", style: lipgloss.NewStyle().Foreground(styles.ColorBlue).Bold(true)},
			{label: "SAFE AUTOMATION", style: lipgloss.NewStyle().Foreground(styles.ColorTeal).Bold(true)},
		},
		details: []string{"A managed foundation for your Pi workflow."},
	}
}

func gentleShellHeroStyledLines(style lipgloss.Style, text string, width int) []string {
	lines := make([]string, 0)
	for _, line := range gentleShellHeroWrap(text, width) {
		lines = append(lines, style.Render(gentleShellHeroClip(line, width)))
	}
	return lines
}

func gentleShellHeroWrap(text string, width int) []string {
	if width <= 0 {
		return strings.Split(text, "\n")
	}
	wrapped := ansi.Hardwrap(text, width, true)
	parts := strings.Split(wrapped, "\n")
	for i := range parts {
		parts[i] = gentleShellHeroClip(parts[i], width)
	}
	return parts
}

func gentleShellHeroClip(text string, width int) string {
	if width <= 0 || ansi.StringWidth(text) <= width {
		return text
	}
	return ansi.Cut(text, 0, width)
}

func gentleShellHeroViewport(lines []string, width, height int) string {
	for i := range lines {
		lines[i] = gentleShellHeroClip(lines[i], width)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func gentleShellHeroFits(view string, width, height int) bool {
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > width {
			return false
		}
	}
	return lipgloss.Height(view) <= height
}
