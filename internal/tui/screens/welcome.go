package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/rivo/uniseg"
)

// WelcomeOptions returns the welcome menu options.
// When showProfiles is true, an "OpenCode SDD Profiles" option is inserted
// between "Configure models" and "Manage backups".
// profileCount is used to show a badge with the current profile count.
// When hasEngines is false, "Create your own Agent" is shown as disabled
// (labelled "(no agents)") to signal that no supported AI engine is installed.
func WelcomeOptions(updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool) []string {
	upgradeLabel := "Upgrade tools"
	if updateCheckDone && update.HasUpdates(updateResults) {
		upgradeLabel = "Upgrade tools ★"
	} else if updateCheckDone && !update.HasUpdates(updateResults) {
		upgradeLabel = "Upgrade tools (up to date)"
	}

	agentLabel := "Create your own Agent"
	if !hasEngines {
		agentLabel = "Create your own Agent (no agents)"
	}

	opts := []string{
		"Start installation",
		upgradeLabel,
		"Sync configs",
		"Upgrade + Sync",
		"Configure models",
		agentLabel,
		"OpenCode Community Plugins",
	}

	// Slice 3b — standalone launcher for the 4-layer managed uninstall of
	// OpenCode community plugins. Sits next to the install shortcut above
	// so the menu pairs install + uninstall as mirror operations.
	opts = append(opts, "Uninstall OpenCode Plugin")

	if showProfiles {
		profilesLabel := "OpenCode SDD Profiles"
		if profileCount > 0 {
			profilesLabel = fmt.Sprintf("OpenCode SDD Profiles (%d)", profileCount)
		}
		opts = append(opts, profilesLabel)
	}

	opts = append(opts, "Manage backups")
	opts = append(opts, "Managed uninstall")
	opts = append(opts, "Community Tools/Plugins")
	opts = append(opts, "Quit")

	return opts
}

func RenderWelcome(cursor int, version string, updateBanner string, updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool) string {
	return RenderWelcomeWithWidth(cursor, version, updateBanner, updateResults, updateCheckDone, showProfiles, profileCount, hasEngines, 0)
}

func RenderWelcomeWithWidth(cursor int, version string, updateBanner string, updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, width int) string {
	var b strings.Builder

	b.WriteString(styles.RenderLogo())
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render(styles.Tagline(version)))
	b.WriteString("\n")

	if updateBanner != "" {
		b.WriteString(styles.WarningStyle.Render(wrapWelcomeBanner(updateBanner, welcomeContentWidth(width))))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HeadingStyle.Render("Menu"))
	b.WriteString("\n\n")
	b.WriteString(renderOptions(WelcomeOptions(updateResults, updateCheckDone, showProfiles, profileCount, hasEngines), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • q: quit"))

	if width > 0 {
		return styles.FrameStyle.Width(width - 4).Render(b.String())
	}
	return styles.FrameStyle.Render(b.String())
}

func welcomeContentWidth(width int) int {
	const frameHorizontalSize = 10 // double borders plus left/right padding from FrameStyle.
	if width <= frameHorizontalSize {
		return 0
	}
	return width - frameHorizontalSize
}

func wrapWelcomeBanner(text string, width int) string {
	text = formatWelcomeAdvisoryList(text)
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapPlainLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func formatWelcomeAdvisoryList(text string) string {
	const advisoryPrefix = "Advisory: "
	const plusMarker = ". Plus: "
	if !strings.HasPrefix(text, advisoryPrefix) || !strings.Contains(text, plusMarker) {
		return text
	}

	body := strings.TrimPrefix(text, advisoryPrefix)
	head, rest, ok := strings.Cut(body, plusMarker)
	if !ok {
		return text
	}

	header, firstFeature, ok := strings.Cut(head, ": ")
	if !ok || strings.TrimSpace(firstFeature) == "" {
		return text
	}

	featuresPart, suffix := splitAdvisorySuffix(rest)
	features := []string{strings.TrimSpace(firstFeature) + "."}
	features = append(features, splitAdvisoryFeatures(featuresPart)...)

	var b strings.Builder
	b.WriteString(advisoryPrefix)
	b.WriteString(strings.TrimSpace(header))
	b.WriteString(":")
	for _, feature := range features {
		feature = strings.TrimSpace(strings.TrimSuffix(feature, "."))
		feature = strings.TrimPrefix(feature, "and ")
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		b.WriteString("\n• ")
		b.WriteString(feature)
		b.WriteString(".")
	}
	if suffix != "" {
		b.WriteString("\n")
		b.WriteString(suffix)
	}
	return b.String()
}

func splitAdvisorySuffix(text string) (string, string) {
	markers := []string{" Thanks ", " See "}
	cutAt := -1
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 && (cutAt == -1 || idx < cutAt) {
			cutAt = idx
		}
	}
	if cutAt == -1 {
		return text, ""
	}
	return strings.TrimSpace(text[:cutAt]), strings.TrimSpace(text[cutAt:])
}

func splitAdvisoryFeatures(text string) []string {
	text = strings.ReplaceAll(text, ", and ", ", ")
	parts := strings.Split(text, ",")
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "and ")
		if part != "" {
			features = append(features, part)
		}
	}
	return features
}

func wrapPlainLine(line string, width int) []string {
	if lipgloss.Width(line) <= width {
		return []string{line}
	}

	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	var current strings.Builder
	for _, word := range words {
		wordWidth := lipgloss.Width(word)
		currentWidth := lipgloss.Width(current.String())
		if wordWidth > width {
			if currentWidth > 0 {
				lines = append(lines, current.String())
				current.Reset()
			}
			lines = append(lines, hardWrapWord(word, width)...)
			continue
		}
		if currentWidth == 0 {
			current.WriteString(word)
			continue
		}
		if currentWidth+1+wordWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(word)
			continue
		}
		current.WriteString(" ")
		current.WriteString(word)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func hardWrapWord(word string, width int) []string {
	lines := make([]string, 0)
	var current strings.Builder
	currentWidth := 0

	graphemes := uniseg.NewGraphemes(word)
	for graphemes.Next() {
		cluster := graphemes.Str()
		clusterWidth := lipgloss.Width(cluster)
		if currentWidth > 0 && currentWidth+clusterWidth > width {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteString(cluster)
		currentWidth += clusterWidth
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}
