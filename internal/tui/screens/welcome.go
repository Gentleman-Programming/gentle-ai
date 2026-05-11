package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// WelcomeOptions returns the welcome menu options.
// When showProfiles is true, an "OpenCode SDD Profiles" option is inserted
// after the plugins entry. profileCount is used to show a badge.
// When showVSCodeProfiles is true, a "VS Code SDD Profiles" option is inserted
// right after the OpenCode profiles entry. vscodeProfileCount is its badge.
// When hasEngines is false, "Create your own Agent" is shown as disabled.
func WelcomeOptions(updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, showVSCodeProfiles bool, vscodeProfileCount int) []string {
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

	if showProfiles {
		profilesLabel := "OpenCode SDD Profiles"
		if profileCount > 0 {
			profilesLabel = fmt.Sprintf("OpenCode SDD Profiles (%d)", profileCount)
		}
		opts = append(opts, profilesLabel)
	}

	if showVSCodeProfiles {
		vscLabel := "VS Code SDD Profiles"
		if vscodeProfileCount > 0 {
			vscLabel = fmt.Sprintf("VS Code SDD Profiles (%d)", vscodeProfileCount)
		}
		opts = append(opts, vscLabel)
	}

	opts = append(opts, "Manage backups")
	opts = append(opts, "Managed uninstall")
	opts = append(opts, "Quit")

	return opts
}

func RenderWelcome(cursor int, version string, updateBanner string, updateResults []update.UpdateResult, updateCheckDone bool, showProfiles bool, profileCount int, hasEngines bool, showVSCodeProfiles bool, vscodeProfileCount int) string {
	var b strings.Builder

	b.WriteString(styles.RenderLogo())
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render(styles.Tagline(version)))
	b.WriteString("\n")

	if updateBanner != "" {
		b.WriteString(styles.WarningStyle.Render(updateBanner))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HeadingStyle.Render("Menu"))
	b.WriteString("\n\n")
	b.WriteString(renderOptions(WelcomeOptions(updateResults, updateCheckDone, showProfiles, profileCount, hasEngines, showVSCodeProfiles, vscodeProfileCount), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • q: quit"))

	return styles.FrameStyle.Render(b.String())
}
