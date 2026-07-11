package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/opencodeplugin"
	"github.com/gentleman-programming/gentle-ai/internal/model"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderOpenCodePluginUninstallSelect renders the list of installed OpenCode
// community plugins available for uninstall. Cursor is the highlighted row.
// Returns "" if there are no plugins to show.
func RenderOpenCodePluginUninstallSelect(installed []model.OpenCodeCommunityPluginID, cursor int) string {
	if len(installed) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render("Uninstall OpenCode Community Plugins"))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Select a plugin to uninstall. The 4-layer cleanup will be journaled for rollback."))
	b.WriteString("\n\n")

	row := 0
	for _, id := range installed {
		def, hasDef := opencodeplugin.DefinitionFor(id)
		name := string(id)
		description := ""
		if hasDef {
			name = def.Name
			description = def.Description
		}
		label := fmt.Sprintf("[ ] %s — %s", name, description)
		if cursor == row {
			b.WriteString(styles.SelectedStyle.Render("▸ "+label) + "\n")
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  "+label) + "\n")
		}
		row++
	}

	// Back row — single-select flow: cursor on the Back row returns to Welcome.
	if cursor == row {
		b.WriteString(styles.SelectedStyle.Render("▸ Back") + "\n")
	} else {
		b.WriteString(styles.UnselectedStyle.Render("  Back") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("↑/↓: navigate • enter: select or back • esc: back"))
	return styles.FrameStyle.Render(b.String())
}

// OpenCodePluginUninstallOptionCount returns the number of interactive rows
// in the select screen (one per installed plugin + Back).
func OpenCodePluginUninstallOptionCount(installed []model.OpenCodeCommunityPluginID) int {
	return len(installed) + 1
}

// RenderOpenCodePluginUninstallConfirm renders the confirmation prompt
// before running the uninstall. Running is true when the async uninstall is
// in flight (show spinner); spinner frames are picked from a small fixed set.
func RenderOpenCodePluginUninstallConfirm(selected model.OpenCodeCommunityPluginID, running bool, spinnerFrame int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Confirm OpenCode Plugin Uninstall"))
	b.WriteString("\n\n")

	name := pluginDisplayName(selected)

	if running {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Uninstalling " + name + "..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return styles.FrameStyle.Render(b.String())
	}

	b.WriteString(styles.SubtextStyle.Render("About to uninstall " + name + "."))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Layered cleanup:"))
	b.WriteString("\n")
	if selected == model.OpenCodePluginGentleLogo {
		b.WriteString(styles.UnselectedStyle.Render("  • tui.json: entry removed"))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  • tui-plugins/gentle-logo.tsx: removed"))
	} else {
		b.WriteString(styles.UnselectedStyle.Render("  • Layer 1 — tui.json: entry removed"))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  • Layer 2 — package.json: dependency removed"))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  • Layer 3 — node_modules/<pkg>: directory removed"))
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("  • Layer 4 — cache: ~/.cache/opencode/packages/<pkg>@latest removed"))
	}
	b.WriteString("\n\n")
	b.WriteString(styles.WarningStyle.Render("A journal snapshot is captured before any change; on failure the changes are rolled back."))
	b.WriteString("\n\n")
	b.WriteString(styles.SubtextStyle.Render("Press enter to confirm, esc to cancel."))

	return styles.FrameStyle.Render(b.String())
}

// RenderOpenCodePluginUninstallResult renders the success/failure summary
// of the uninstall operation. Err is nil for success.
func RenderOpenCodePluginUninstallResult(result opencodeplugin.UninstallResult, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("OpenCode Plugin Uninstall"))
	b.WriteString("\n\n")

	if err != nil {
		b.WriteString(styles.ErrorStyle.Render("Uninstall failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Error:"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
	} else if result.PluginID == "" {
		// Empty result reached when the launcher found no installed plugins to
		// offer for uninstall. Surface a clear empty-state rather than a
		// blank frame.
		b.WriteString(styles.WarningStyle.Render("No OpenCode community plugins are installed."))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("Install one first to be able to uninstall it."))
		b.WriteString("\n\n")
	} else if result.PluginID != "" {
		name := pluginDisplayName(result.PluginID)
		b.WriteString(styles.SuccessStyle.Render("✓ " + name + " uninstalled"))
		b.WriteString("\n\n")

		if result.PluginID == model.OpenCodePluginGentleLogo {
			if result.TSXPath != "" {
				b.WriteString(styles.SubtextStyle.Render("  • TSX: " + result.TSXPath))
				b.WriteString("\n")
			}
			if result.ChangedTUI {
				b.WriteString(styles.SubtextStyle.Render("  • Layer 1 — tui.json: updated"))
				b.WriteString("\n")
			}
		} else {
			if result.ChangedTUI {
				b.WriteString(styles.SubtextStyle.Render("  • Layer 1 — tui.json: updated"))
				b.WriteString("\n")
			}
			if result.ChangedPackageJSON {
				b.WriteString(styles.SubtextStyle.Render("  • Layer 2 — package.json: updated"))
				b.WriteString("\n")
			}
			if result.ChangedNodeModules {
				b.WriteString(styles.SubtextStyle.Render("  • Layer 3 — node_modules: removed " + result.NodeModulesPath))
				b.WriteString("\n")
			}
			if result.CacheEntryRemoved != "" {
				b.WriteString(styles.SubtextStyle.Render("  • Layer 4 — cache: removed " + result.CacheEntryRemoved))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.SelectedStyle.Render("▸ Return to menu"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return to menu • q: quit"))

	return styles.FrameStyle.Render(b.String())
}

func pluginDisplayName(id model.OpenCodeCommunityPluginID) string {
	if def, ok := opencodeplugin.DefinitionFor(id); ok {
		return def.Name
	}
	switch id {
	case model.OpenCodePluginGentleLogo:
		return "Gentle Logo"
	}
	return string(id)
}