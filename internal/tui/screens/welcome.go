package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/v2/internal/update"
	"github.com/rivo/uniseg"
)

const (
	welcomeAdvisoryMaxRegionHeight = 7
	welcomePrimaryContentHeight    = 44
	welcomeHelpText                = "j/k: navigate • enter: select • q: quit"
)

type WelcomeAdvisory struct {
	Message string
	URL     string
	Scroll  int
}

// WelcomeOptions returns the welcome menu options.
// When showProfiles is true, an "OpenCode SDD Profiles" option is inserted
// between "Configure models" and "Manage backups".
// profileCount is used to show a badge with the current profile count.
// When hasEngines is false, "Create your own Agent" is shown as disabled
// (labelled "(no agents)") to signal that no supported AI engine is installed.
// When hasGit is false, "Reset review store" is shown as disabled
// (labelled "Reset review store (requires a Git repository)") to signal that
// repository-scoped store operations require a Git repository.
func WelcomeOptions(updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, hasGit bool) []string {
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
	resetLabel := "Reset review store"
	if !hasGit {
		resetLabel = "Reset review store (requires a Git repository)"
	}
	opts = append(opts, resetLabel)
	opts = append(opts, "Receipt-Driven Development")
	opts = append(opts, "Managed uninstall")
	opts = append(opts, "Community Tools/Plugins")
	opts = append(opts, "Quit")

	return opts
}

func RenderWelcome(cursor int, version string, updateBanner string, updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, hasGit bool) string {
	return RenderWelcomeWithWidth(cursor, version, updateBanner, updateResults, updateCheckDone, showProfiles, profileCount, hasEngines, hasGit, 0)
}

func RenderWelcomeWithWidth(cursor int, version string, updateBanner string, updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, hasGit bool, width int) string {
	return RenderWelcomeWithAdvisory(cursor, version, updateBanner, updateResults, updateCheckDone, showProfiles, profileCount, hasEngines, hasGit, width, 0, WelcomeAdvisory{})
}

func RenderWelcomeWithAdvisory(cursor int, version string, updateBanner string, updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, hasGit bool, width int, height int, advisory WelcomeAdvisory) string {
	render := func(includeLogo, includeOptional, compact bool) string {
		var b strings.Builder

		if includeLogo {
			b.WriteString(styles.RenderLogo())
			b.WriteString("\n\n")
		}
		if !compact {
			b.WriteString(styles.SubtextStyle.Render(styles.Tagline(version)))
			b.WriteString("\n")

			if includeOptional {
				if updateBanner != "" {
					b.WriteString(styles.WarningStyle.Render(wrapWelcomeBanner(updateBanner, welcomeContentWidth(width))))
					b.WriteString("\n")
				}
				if rendered := renderWelcomeAdvisory(advisory, width, height); rendered != "" {
					b.WriteString(rendered)
					b.WriteString("\n")
				}
			}
		}

		if !compact && includeLogo {
			b.WriteString("\n")
		}
		if compact {
			b.WriteString(renderWelcomeText(styles.HeadingStyle, "Menu", width))
		} else {
			b.WriteString(styles.HeadingStyle.Render("Menu"))
		}
		if compact || !includeLogo {
			b.WriteString("\n")
		} else {
			b.WriteString("\n\n")
		}
		options := WelcomeOptions(updateResults, updateCheckDone, showProfiles, profileCount, hasEngines, hasGit)
		if compact {
			b.WriteString(renderWelcomeOptions(options, cursor, width))
		} else {
			b.WriteString(renderOptions(options, cursor))
		}
		if !compact && includeLogo {
			b.WriteString("\n")
		}
		if compact {
			b.WriteString(renderWelcomeText(styles.HelpStyle, welcomeHelpText, width))
		} else {
			b.WriteString(styles.HelpStyle.Render(welcomeHelpText))
		}

		if compact {
			return b.String()
		}
		return welcomeFrameStyle(width).Render(b.String())
	}
	if width > 0 && width <= styles.FrameStyle.GetHorizontalBorderSize() {
		return renderWelcomeMinimum(width, height, cursor)
	}
	fitsViewport := func(view string) bool {
		return (width <= 0 || lipgloss.Width(view) <= width) &&
			(height <= 0 || lipgloss.Height(view) <= height)
	}

	view := render(true, true, false)
	if !fitsViewport(view) {
		// The logo is optional chrome; keep the menu and controls visible when
		// the measured terminal viewport cannot hold the full welcome screen.
		view = render(false, true, false)
	}
	if !fitsViewport(view) {
		// Drop optional chrome and the frame only after measuring the logo-free view.
		// The complete menu, primary action, and help line remain visible.
		view = render(false, false, true)
	}
	if !fitsViewport(view) {
		// A viewport shorter than the compact menu still needs a safe, actionable
		// state instead of allowing the terminal to clip the rendered content.
		view = renderWelcomeMinimum(width, height, cursor)
	}
	return view
}

func renderWelcomeText(style lipgloss.Style, text string, width int) string {
	if width <= 0 {
		return style.Render(text)
	}
	return style.Render(strings.Join(wrapPlainLine(text, width), "\n"))
}

func renderWelcomeOptions(options []string, cursor int, width int) string {
	if width <= 0 {
		return renderOptions(options, cursor)
	}

	var b strings.Builder
	for idx, option := range options {
		prefix := "  "
		style := styles.UnselectedStyle
		if idx == cursor {
			prefix = styles.Cursor
			style = styles.SelectedStyle
		}
		for _, line := range wrapPlainLine(prefix+option, width) {
			b.WriteString(style.Render(line))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func renderWelcomeMinimum(width int, height int, cursor int) string {
	const primaryAction = "Start installation"
	const compactPrimaryAction = "Go"
	const narrowPrimaryAction = ">"
	const compactHelp = "j/k, enter, q"
	const narrowHelp = "q"

	primaryLabel := primaryAction
	if width > 0 && lipgloss.Width(primaryLabel) > width {
		primaryLabel = compactPrimaryAction
		if lipgloss.Width(primaryLabel) > width {
			primaryLabel = narrowPrimaryAction
		}
	}
	primaryLines := wrapWelcomeLine(primaryLabel, width)
	helpLines := wrapWelcomeLine(welcomeHelpText, width)
	if height > 0 && len(primaryLines)+len(helpLines) > height {
		helpLabel := compactHelp
		if width > 0 && lipgloss.Width(helpLabel) > width {
			helpLabel = narrowHelp
		}
		helpLines = wrapWelcomeLine(helpLabel, width)
	}

	lines := append(primaryLines, helpLines...)
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	if len(lines) == 0 {
		return ""
	}

	primaryLineCount := len(primaryLines)
	if primaryLineCount > len(lines) {
		primaryLineCount = len(lines)
	}
	primaryStyle := styles.UnselectedStyle
	if cursor == 0 {
		primaryStyle = styles.SelectedStyle
	}
	rendered := primaryStyle.Render(strings.Join(lines[:primaryLineCount], "\n"))
	if primaryLineCount == len(lines) {
		return rendered
	}
	return rendered + "\n" + styles.HelpStyle.Render(strings.Join(lines[primaryLineCount:], "\n"))
}

func wrapWelcomeLine(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	return wrapPlainLine(text, width)
}

func WelcomeAdvisoryScrollBounds(message string, releaseURL string, width int, height int) (int, int) {
	lines := welcomeAdvisoryLines(message, releaseURL, welcomeContentWidth(width))
	regionHeight := welcomeAdvisoryRegionHeight(height)
	if len(lines) == 0 || regionHeight == 0 {
		return 0, 0
	}
	if len(lines) <= regionHeight {
		return len(lines), 0
	}
	if regionHeight < 2 {
		return 0, 0
	}
	pageSize := regionHeight - 1
	return pageSize, len(lines) - pageSize
}

func renderWelcomeAdvisory(advisory WelcomeAdvisory, width int, height int) string {
	lines := welcomeAdvisoryLines(advisory.Message, advisory.URL, welcomeContentWidth(width))
	pageSize, maxScroll := WelcomeAdvisoryScrollBounds(advisory.Message, advisory.URL, width, height)
	if pageSize == 0 {
		return ""
	}
	scroll := min(max(0, advisory.Scroll), maxScroll)
	end := min(scroll+pageSize, len(lines))
	rendered := styles.WarningStyle.Render(strings.Join(lines[scroll:end], "\n"))
	if maxScroll > 0 {
		hint := fmt.Sprintf("PgUp/PgDn: scroll  •  lines %d-%d/%d", scroll+1, end, len(lines))
		rendered += "\n" + styles.HelpStyle.Render(hint)
	}
	return rendered
}

func welcomeAdvisoryLines(message string, releaseURL string, width int) []string {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil
	}
	if !strings.HasPrefix(message, "Advisory:") {
		message = "Advisory: " + message
	}
	messageLines := strings.Split(wrapWelcomeBanner(message, width), "\n")
	if releaseURL == "" {
		return messageLines
	}
	link := "Latest release: " + releaseURL
	if width <= 0 {
		return append([]string{link}, messageLines...)
	}
	return append(wrapPlainLine(link, width), messageLines...)
}

func welcomeAdvisoryRegionHeight(height int) int {
	if height <= 0 {
		return welcomeAdvisoryMaxRegionHeight
	}
	return min(welcomeAdvisoryMaxRegionHeight, max(0, height-welcomePrimaryContentHeight))
}

func welcomeContentWidth(width int) int {
	if width <= 0 {
		return 0
	}
	return max(0, width-styles.FrameStyle.GetHorizontalFrameSize())
}

func welcomeFrameStyle(width int) lipgloss.Style {
	if width <= 0 {
		return styles.FrameStyle
	}
	borderWidth := styles.FrameStyle.GetHorizontalBorderSize()
	if width <= borderWidth {
		return styles.FrameStyle.MaxWidth(width)
	}
	return styles.FrameStyle.Width(width - borderWidth)
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
