package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// RenderUpgradeSync handles all states of the combined upgrade+sync screen.
//
// State logic:
//  1. operationRunning && upgradeReport == nil && upgradeErr == nil → "Upgrading tools..." with spinner
//  2. operationRunning && (upgradeReport != nil || upgradeErr != nil) → "Syncing configurations..." with spinner
//  3. !operationRunning && (upgradeReport != nil || upgradeErr != nil) → show combined results
//  4. Otherwise → show confirmation screen
func RenderUpgradeSync(results []update.UpdateResult, upgradeReport *upgrade.UpgradeReport, syncFilesChanged int, upgradeErr error, syncErr error, operationRunning bool, updateCheckDone bool, cursor int, spinnerFrame int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Upgrade + Sync"))
	b.WriteString("\n\n")

	// State 1: upgrade is running (report not yet available)
	if operationRunning && upgradeReport == nil && upgradeErr == nil {
		if !updateCheckDone {
			b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Checking for updates..."))
		} else {
			b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Upgrading tools..."))
		}
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return b.String()
	}

	// State 2: upgrade done, sync now running
	if operationRunning && (upgradeReport != nil || upgradeErr != nil) {
		if upgradeErr != nil {
			b.WriteString(styles.ErrorStyle.Render("✗ Upgrade failed"))
		} else {
			b.WriteString(styles.SuccessStyle.Render("✓ Upgrade complete"))
		}
		b.WriteString("\n\n")
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Syncing configurations..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return b.String()
	}

	// State 3: both operations done — show combined results
	// Triggered when not running and either upgrade report or upgrade error is present.
	if !operationRunning && (upgradeReport != nil || upgradeErr != nil) {
		b.WriteString(renderUpgradeSyncResult(upgradeReport, syncFilesChanged, upgradeErr, syncErr))
		return b.String()
	}

	// State 4: confirmation screen
	b.WriteString(renderUpgradeSyncConfirm(results, updateCheckDone, spinnerFrame))
	return b.String()
}

func renderUpgradeSyncConfirm(results []update.UpdateResult, updateCheckDone bool, spinnerFrame int) string {
	var b strings.Builder

	if !updateCheckDone {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Checking for updates..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Waiting for version check to complete..."))
		return b.String()
	}

	b.WriteString(styles.UnselectedStyle.Render("This will perform two operations in sequence:"))
	b.WriteString("\n\n")

	b.WriteString("  " + styles.WarningStyle.Render("1.") + " " + styles.HeadingStyle.Render("Upgrade tools"))
	b.WriteString("\n")
	b.WriteString("     " + styles.SubtextStyle.Render("Updates gentle-ai, engram, and gga to latest versions"))
	b.WriteString("\n\n")

	b.WriteString("  " + styles.WarningStyle.Render("2.") + " " + styles.HeadingStyle.Render("Sync configurations"))
	b.WriteString("\n")
	b.WriteString("     " + styles.SubtextStyle.Render("Re-applies dotfile configs to all detected agents"))
	b.WriteString("\n\n")

	// Show tool update summary if available
	if len(results) > 0 {
		hasUpdates := false
		for _, r := range results {
			if r.Status == update.UpdateAvailable {
				hasUpdates = true
				break
			}
		}
		if hasUpdates {
			b.WriteString(styles.WarningStyle.Render("Updates available — tools will be upgraded"))
		} else {
			b.WriteString(styles.SubtextStyle.Render("All tools are already up to date (sync will still run)"))
		}
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HeadingStyle.Render("Press enter to begin"))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: confirm • esc: back • q: quit"))

	return b.String()
}

func renderUpgradeSyncResult(report *upgrade.UpgradeReport, syncFilesChanged int, upgradeErr error, syncErr error) string {
	var b strings.Builder

	// --- Upgrade section ---
	b.WriteString(styles.HeadingStyle.Render("Upgrade Results"))
	b.WriteString("\n\n")

	if upgradeErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Upgrade failed: " + upgradeErr.Error()))
		b.WriteString("\n")
	} else if report != nil {
		if len(report.Results) == 0 {
			b.WriteString("  " + styles.SuccessStyle.Render("✓ All tools are up to date"))
			b.WriteString("\n")
		}

		upgradeSucceeded, upgradeFailed, upgradeSkipped := 0, 0, 0

		for _, r := range report.Results {
			switch r.Status {
			case upgrade.UpgradeSucceeded:
				upgradeSucceeded++
				line := fmt.Sprintf("%s  %s → %s",
					r.ToolName,
					styles.SubtextStyle.Render(r.OldVersion),
					styles.SuccessStyle.Render(r.NewVersion),
				)
				b.WriteString("  " + styles.SuccessStyle.Render("✓") + "  " + line)
			case upgrade.UpgradeFailed:
				upgradeFailed++
				b.WriteString("  " + styles.ErrorStyle.Render("✗") + "  " + styles.ErrorStyle.Render(r.ToolName))
				if r.Err != nil {
					b.WriteString("\n     " + styles.SubtextStyle.Render(r.Err.Error()))
				}
			case upgrade.UpgradeSkipped:
				upgradeSkipped++
				hint := ""
				if r.ManualHint != "" {
					hint = "  " + styles.SubtextStyle.Render(r.ManualHint)
				}
				b.WriteString("  " + styles.SubtextStyle.Render("-") + "  " + styles.SubtextStyle.Render(r.ToolName+" (skipped)") + hint)
			}
			b.WriteString("\n")
		}

		// Upgrade summary
		parts := []string{}
		if upgradeSucceeded > 0 {
			parts = append(parts, styles.SuccessStyle.Render(fmt.Sprintf("%d upgraded", upgradeSucceeded)))
		}
		if upgradeFailed > 0 {
			parts = append(parts, styles.ErrorStyle.Render(fmt.Sprintf("%d failed", upgradeFailed)))
		}
		if upgradeSkipped > 0 {
			parts = append(parts, styles.SubtextStyle.Render(fmt.Sprintf("%d skipped", upgradeSkipped)))
		}
		if len(parts) > 0 {
			b.WriteString("  " + strings.Join(parts, "  "))
			b.WriteString("\n")
		}

		if report.BackupWarning != "" {
			b.WriteString("  " + styles.WarningStyle.Render("⚠ "+report.BackupWarning))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// --- Sync section ---
	b.WriteString(styles.HeadingStyle.Render("Sync Results"))
	b.WriteString("\n\n")

	if syncErr != nil {
		b.WriteString("  " + styles.ErrorStyle.Render("✗ Sync failed: "+syncErr.Error()))
	} else if syncFilesChanged == 0 {
		b.WriteString("  " + styles.SubtextStyle.Render("No files needed updating"))
	} else {
		b.WriteString("  " + styles.SuccessStyle.Render("✓") + "  " + fmt.Sprintf("%s synchronized", styles.HeadingStyle.Render(fmt.Sprintf("%d file(s)", syncFilesChanged))))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))

	return b.String()
}
