package screens

// Note: this file is intentionally named sync_screen.go instead of sync.go
// because sync.go would conflict with the Go standard library "sync" package name.

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// RenderSync handles all states of the sync screen.
//
// State logic:
//  1. operationRunning → "Syncing configurations..." with spinner
//  2. hasSyncRun && (filesChanged > 0 || syncErr != nil) → show result
//  3. Otherwise → show confirmation screen
func RenderSync(filesChanged int, syncErr error, operationRunning bool, hasSyncRun bool, spinnerFrame int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Sync Configurations"))
	b.WriteString("\n\n")

	// State 1: sync is running
	if operationRunning {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Syncing configurations..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return b.String()
	}

	// State 2: sync has run — show result
	if hasSyncRun {
		b.WriteString(renderSyncResult(filesChanged, syncErr))
		return b.String()
	}

	// State 3: confirmation screen
	b.WriteString(renderSyncConfirm())
	return b.String()
}

func renderSyncConfirm() string {
	var b strings.Builder

	b.WriteString(styles.UnselectedStyle.Render("Sync will re-apply your dotfile configurations"))
	b.WriteString("\n")
	b.WriteString(styles.UnselectedStyle.Render("to all detected AI agents on this machine."))
	b.WriteString("\n\n")

	b.WriteString(styles.SubtextStyle.Render("This operation:"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Reads your current agent selections"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Re-writes agent config files from templates"))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render("  • Does not modify your global dotfiles"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Press enter to sync"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: confirm • esc: back • q: quit"))

	return b.String()
}

func renderSyncResult(filesChanged int, syncErr error) string {
	var b strings.Builder

	if syncErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Sync failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render(syncErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Check your configuration and try again."))
	} else if filesChanged == 0 {
		b.WriteString(styles.SuccessStyle.Render("✓ Sync complete"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("No agents detected or no files needed updating."))
	} else {
		b.WriteString(styles.SuccessStyle.Render("✓ Sync complete"))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("%s %s", styles.HeadingStyle.Render(fmt.Sprintf("%d file(s)", filesChanged)), styles.UnselectedStyle.Render("synchronized")))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))

	return b.String()
}
