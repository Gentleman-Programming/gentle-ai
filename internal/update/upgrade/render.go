package upgrade

import (
	"fmt"
	"strings"
)

// RenderUpgradeReport produces a plain-text report of upgrade results.
// Designed for the CLI path — no lipgloss, no color.
func RenderUpgradeReport(report UpgradeReport) string {
	var b strings.Builder

	if report.DryRun {
		b.WriteString("Upgrade (dry-run)\n")
	} else {
		b.WriteString("Upgrade\n")
	}
	b.WriteString("=======\n\n")

	// Preamble: explain what upgrade does (binary-only, no install/sync).
	b.WriteString("  Upgrades managed tool binaries only.\n")
	b.WriteString("  Agent configs are preserved — no install or sync is performed.\n\n")

	if len(report.Results) == 0 {
		b.WriteString("  No upgrades available. All managed tools are up to date.\n")
		return b.String()
	}

	succeeded := 0
	failed := 0
	skipped := 0

	for _, r := range report.Results {
		icon := upgradeIcon(r.Status)
		fmt.Fprintf(&b, "  %s %-12s", icon, r.ToolName)

		switch r.Status {
		case UpgradeSucceeded:
			fmt.Fprintf(&b, "  %s → %s\n", r.OldVersion, r.NewVersion)
			succeeded++
		case UpgradeFailed:
			errMsg := ""
			if r.Err != nil {
				errMsg = r.Err.Error()
			}
			fmt.Fprintf(&b, "  FAILED: %s\n", errMsg)
			failed++
		case UpgradeSkipped:
			if r.ManualHint != "" {
				fmt.Fprintf(&b, "  manual update required: %s\n", r.ManualHint)
			} else if report.DryRun {
				fmt.Fprintf(&b, "  %s → %s  (dry-run)\n", r.OldVersion, r.NewVersion)
			} else {
				fmt.Fprintf(&b, "  skipped\n")
			}
			skipped++
		}
	}

	b.WriteString("\n")

	if report.BackupID != "" {
		fmt.Fprintf(&b, "  Config backup: %s\n", report.BackupID)
	}
	if report.BackupWarning != "" {
		fmt.Fprintf(&b, "  WARNING: %s\n", report.BackupWarning)
	}

	if report.DryRun {
		// Count only actionable upgrades (no ManualHint) as pending.
		// Manual-hint items (DevBuild, VersionUnknown) will not run even
		// without --dry-run, so counting them as "pending" is misleading.
		actionable := 0
		for _, r := range report.Results {
			if r.Status == UpgradeSkipped && r.ManualHint == "" {
				actionable++
			}
		}
		if actionable > 0 {
			fmt.Fprintf(&b, "  %d upgrade(s) pending. Run without --dry-run to apply.\n", actionable)
		}
		if skipped-actionable > 0 {
			fmt.Fprintf(&b, "  %d tool(s) require manual attention (see hints above).\n", skipped-actionable)
		}
		if actionable == 0 && skipped == 0 {
			b.WriteString("  No actionable upgrades found.\n")
		}
	} else {
		fmt.Fprintf(&b, "  %d succeeded, %d failed, %d skipped.\n", succeeded, failed, skipped)
	}

	return b.String()
}

// upgradeIcon returns a status indicator for upgrade CLI output.
func upgradeIcon(status ToolUpgradeStatus) string {
	switch status {
	case UpgradeSucceeded:
		return "[ok]"
	case UpgradeFailed:
		return "[!!]"
	case UpgradeSkipped:
		return "[--]"
	default:
		return "[  ]"
	}
}
