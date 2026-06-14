package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// UpdatePromptOptions returns the display labels for the three update-prompt options.
// The order matches the cursor index used in model.go:
//
//	0 = Update now
//	1 = View changes
//	2 = Keep current version
func UpdatePromptOptions() []string {
	return []string{
		"Update now",
		"View changes",
		"Keep current version",
	}
}

// RenderUpdatePrompt renders the pre-Welcome update prompt screen.
//
// While the update check is still in-flight (updateCheckDone=false) a spinner
// is shown. Once done and updates are present the full prompt is rendered.
//
// Cursor starts on "Keep current version" (safe default).
// Enter confirms the highlighted option; shortcuts bypass the cursor.
//
// Keys (displayed as hints):
//
//	↑/↓    = move cursor between options
//	enter  = confirm highlighted option (default: Keep current version)
//	u      = shortcut: Update now → run upgrade then close
//	v      = shortcut: View changes → open release notes in browser (or print URL)
//	c      = shortcut: Keep current version → proceed to Welcome
func RenderUpdatePrompt(results []update.UpdateResult, cursor int, spinnerFrame int, updateCheckDone bool) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Update Available"))
	b.WriteString("\n\n")

	if !updateCheckDone {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Checking for updates..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return b.String()
	}

	// Collect update entries for display.
	var updateLines []string
	var releaseURL string
	for _, r := range results {
		if r.Status == update.UpdateAvailable {
			line := fmt.Sprintf("%s  %s → %s",
				r.Tool.Name,
				styles.SubtextStyle.Render(r.InstalledVersion),
				styles.SuccessStyle.Render(r.LatestVersion),
			)
			updateLines = append(updateLines, line)
			if releaseURL == "" && r.ReleaseURL != "" {
				releaseURL = r.ReleaseURL
			}
		}
	}

	if len(updateLines) == 0 {
		// No updates to display — this screen should not normally appear in this state.
		b.WriteString(styles.SuccessStyle.Render("✓ All tools are up to date"))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("c / enter: continue • q: quit"))
		return b.String()
	}

	b.WriteString(styles.HeadingStyle.Render("New versions available:"))
	b.WriteString("\n\n")
	for _, line := range updateLines {
		b.WriteString("  " + styles.WarningStyle.Render("↑") + "  " + line + "\n")
	}
	b.WriteString("\n")

	if releaseURL != "" {
		b.WriteString(styles.SubtextStyle.Render("Release notes: " + releaseURL))
		b.WriteString("\n\n")
	}

	opts := UpdatePromptOptions()
	b.WriteString(renderOptions(opts, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("↑/↓: move • enter: confirm (default: keep) • u: update • v: view changes • c: keep • q: quit"))

	return styles.FrameStyle.Render(b.String())
}
