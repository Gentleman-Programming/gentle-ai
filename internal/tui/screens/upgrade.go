package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
	"github.com/gentleman-programming/gentle-ai/internal/update"
	"github.com/gentleman-programming/gentle-ai/internal/update/upgrade"
)

// spinnerFrames are the unicode spinner animation frames used across screens.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerChar returns the spinner animation character for the given frame index.
// Exported so model.go can use it without duplicating spinnerFrames.
func SpinnerChar(frame int) string {
	return spinnerFrames[frame%len(spinnerFrames)]
}

// RenderUpgrade handles all states of the upgrade screen.
//
// State logic:
//  1. operationRunning && upgradeReport == nil → "Upgrading tools..." with spinner
//  2. !updateCheckDone → "Checking for updates..." with spinner
//  3. upgradeReport != nil → show upgrade results (success/failure per tool)
//  4. upgradeErr != nil (and report == nil) → show error with return prompt
//  5. Otherwise → show list of tools with status, option to upgrade
func RenderUpgrade(results []update.UpdateResult, report *upgrade.UpgradeReport, upgradeErr error, operationRunning bool, updateCheckDone bool, cursor int, spinnerFrame int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Upgrade Tools"))
	b.WriteString("\n\n")

	// State 1: upgrade is running
	if operationRunning && report == nil {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Upgrading tools..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Please wait..."))
		return b.String()
	}

	// State 2: update check still running
	if !updateCheckDone {
		b.WriteString(styles.WarningStyle.Render(SpinnerChar(spinnerFrame) + "  Checking for updates..."))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("Fetching latest version info..."))
		return b.String()
	}

	// State 3: upgrade report is available — show results
	if report != nil {
		return renderUpgradeResult(&b, report)
	}

	// State 4: upgrade error — show error and allow returning
	if upgradeErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Upgrade failed: " + upgradeErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))
		return b.String()
	}

	// State 5: ready state — show tool list and upgrade prompt
	return renderUpgradeReady(&b, results)
}

func renderUpgradeReady(b *strings.Builder, results []update.UpdateResult) string {
	hasUpdates := false

	for _, r := range results {
		switch r.Status {
		case update.UpdateAvailable:
			hasUpdates = true
			line := fmt.Sprintf("%s  %s → %s",
				r.Tool.Name,
				styles.SubtextStyle.Render(r.InstalledVersion),
				styles.SuccessStyle.Render(r.LatestVersion),
			)
			b.WriteString("  " + styles.WarningStyle.Render("↑") + "  " + line)
		case update.UpToDate:
			line := styles.SelectedStyle.Render(r.Tool.Name) + "  " + styles.SuccessStyle.Render("✓ up to date")
			if r.InstalledVersion != "" {
				line += "  " + styles.SubtextStyle.Render(r.InstalledVersion)
			}
			b.WriteString("  " + line)
		case update.NotInstalled:
			line := r.Tool.Name + "  " + styles.SubtextStyle.Render("(not installed)")
			b.WriteString("  " + styles.SubtextStyle.Render(line))
		case update.DevBuild:
			line := r.Tool.Name + "  " + styles.SubtextStyle.Render("(dev build — skip)")
			b.WriteString("  " + styles.SubtextStyle.Render(line))
		default:
			line := r.Tool.Name + "  " + styles.SubtextStyle.Render("(unknown)")
			b.WriteString("  " + styles.SubtextStyle.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if hasUpdates {
		b.WriteString(styles.HeadingStyle.Render("Press enter to upgrade all"))
	} else {
		b.WriteString(styles.SuccessStyle.Render("✓ All tools are up to date"))
	}

	b.WriteString("\n\n")
	if hasUpdates {
		b.WriteString(styles.HelpStyle.Render("enter: upgrade • esc: back • q: quit"))
	} else {
		b.WriteString(styles.HelpStyle.Render("esc: back • q: quit"))
	}

	return b.String()
}

func renderUpgradeResult(b *strings.Builder, report *upgrade.UpgradeReport) string {
	if len(report.Results) == 0 {
		b.WriteString("  " + styles.SuccessStyle.Render("✓ All tools are up to date"))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))
		return b.String()
	}

	succeeded, failed, skipped := 0, 0, 0

	for _, r := range report.Results {
		switch r.Status {
		case upgrade.UpgradeSucceeded:
			succeeded++
			line := fmt.Sprintf("%s  %s → %s",
				r.ToolName,
				styles.SubtextStyle.Render(r.OldVersion),
				styles.SuccessStyle.Render(r.NewVersion),
			)
			b.WriteString("  " + styles.SuccessStyle.Render("✓") + "  " + line)
		case upgrade.UpgradeFailed:
			failed++
			line := r.ToolName
			b.WriteString("  " + styles.ErrorStyle.Render("✗") + "  " + styles.ErrorStyle.Render(line))
			if r.Err != nil {
				b.WriteString("\n     " + styles.SubtextStyle.Render(r.Err.Error()))
			}
		case upgrade.UpgradeSkipped:
			skipped++
			line := r.ToolName + "  " + styles.SubtextStyle.Render("(skipped)")
			b.WriteString("  " + styles.SubtextStyle.Render("-") + "  " + styles.SubtextStyle.Render(line))
			if r.ManualHint != "" {
				b.WriteString("\n     " + styles.SubtextStyle.Render(r.ManualHint))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Summary line
	parts := []string{}
	if succeeded > 0 {
		parts = append(parts, styles.SuccessStyle.Render(fmt.Sprintf("%d upgraded", succeeded)))
	}
	if failed > 0 {
		parts = append(parts, styles.ErrorStyle.Render(fmt.Sprintf("%d failed", failed)))
	}
	if skipped > 0 {
		parts = append(parts, styles.SubtextStyle.Render(fmt.Sprintf("%d skipped", skipped)))
	}

	if len(parts) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Summary: ") + strings.Join(parts, "  "))
	}

	if report.BackupWarning != "" {
		b.WriteString("\n")
		b.WriteString(styles.WarningStyle.Render("⚠ Backup warning: " + report.BackupWarning))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: return • esc: back • q: quit"))

	return b.String()
}
