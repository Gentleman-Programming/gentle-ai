package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// PluginInstallPayload holds everything the plugin install result screen needs.
type PluginInstallPayload struct {
	// FilesChanged is the number of files written during installation.
	FilesChanged int
	// AlreadyInstalled is true when the plugin was already present and up to date.
	AlreadyInstalled bool
	// PackageWarning is non-empty when package.json is missing required deps.
	PackageWarning string
	// Err is non-nil when the installation failed.
	Err error
}

// RenderPluginInstall renders the result of the "Install OpenCode Plugin" operation.
func RenderPluginInstall(payload PluginInstallPayload) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Install OpenCode Plugin"))
	b.WriteString("\n\n")

	if payload.Err != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Installation failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render(payload.Err.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Check your configuration and try again."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))
		return b.String()
	}

	if payload.AlreadyInstalled {
		b.WriteString(styles.SuccessStyle.Render("✓ Plugin already installed"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("plugin-sdd-opencode is up to date."))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("tui.json already contains the plugin entry."))
	} else {
		b.WriteString(styles.SuccessStyle.Render("✓ Plugin installed"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("plugin-sdd-opencode copied to ~/.config/opencode/plugins/"))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("tui.json updated with plugin entry."))
		if payload.FilesChanged > 0 {
			b.WriteString("\n")
			b.WriteString(styles.UnselectedStyle.Render(
				strings.Repeat(" ", 2) + fmt.Sprintf("%d file(s) written.", payload.FilesChanged),
			))
		}
	}

	if payload.PackageWarning != "" {
		b.WriteString("\n\n")
		b.WriteString(styles.WarningStyle.Render("⚠ " + payload.PackageWarning))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("Restart OpenCode to activate the plugin."))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))

	return b.String()
}
