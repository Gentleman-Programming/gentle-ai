package update

import (
	"fmt"
	"strings"
)

// RenderCLI produces a plain-text table summarizing update check results.
// Designed for the CLI path (no lipgloss — plain text only).
func RenderCLI(results []UpdateResult) string {
	var b strings.Builder

	b.WriteString("Update Check\n")
	b.WriteString("============\n\n")

	updatesAvailable := 0
	checksFailed := 0
	registeredPending := 0

	for _, r := range results {
		status := statusIcon(r.Status)
		installed := r.InstalledVersion
		if installed == "" {
			installed = "-"
		}
		latest := r.LatestVersion
		if latest == "" {
			latest = "-"
		}

		fmt.Fprintf(&b, "  %s %-12s  installed: %-10s  latest: %-10s", status, r.Tool.Name, installed, latest)

		if r.Status == UpdateAvailable && r.UpdateHint != "" {
			fmt.Fprintf(&b, "  %s", r.UpdateHint)
			updatesAvailable++
		} else if r.Status == UpdateAvailable {
			updatesAvailable++
		} else if r.Status == CheckFailed {
			b.WriteString("  check failed")
			checksFailed++
		} else if r.Status == RegisteredNotMaterialized {
			registeredPending++
			b.WriteString("  registered, pending OpenCode materialization")
			if r.UpdateHint != "" {
				fmt.Fprintf(&b, "  %s", r.UpdateHint)
			}
		}

		b.WriteString("\n")
	}

	b.WriteString("\n")

	if updatesAvailable > 0 && checksFailed > 0 {
		fmt.Fprintf(&b, "%d update(s) available. %d check(s) failed.\n", updatesAvailable, checksFailed)
	} else if updatesAvailable > 0 {
		fmt.Fprintf(&b, "%d update(s) available.\n", updatesAvailable)
	} else if checksFailed > 0 {
		fmt.Fprintf(&b, "Update check incomplete: %d tool(s) failed to check.\n", checksFailed)
	} else if registeredPending > 0 {
		fmt.Fprintf(&b, "%d OpenCode plugin(s) registered, pending materialization.\n", registeredPending)
	} else {
		b.WriteString("All tools are up to date!\n")
	}

	return b.String()
}

// statusIcon returns a single-character status indicator for CLI output.
func statusIcon(status UpdateStatus) string {
	switch status {
	case UpToDate:
		return "[ok]"
	case UpdateAvailable:
		return "[UP]"
	case NotInstalled:
		return "[--]"
	case RegisteredNotMaterialized:
		return "[reg]"
	case VersionUnknown:
		return "[??]"
	case CheckFailed:
		return "[!!]"
	case DevBuild:
		return "[dev]"
	default:
		return "[  ]"
	}
}

// UpdateSummaryLine returns a short one-liner for TUI banners, e.g.
// "engram 1.7.0 -> 1.8.1, gga 1.0.0 -> 2.0.0".
// Returns empty string if no updates are available.
func UpdateSummaryLine(results []UpdateResult) string {
	var parts []string
	for _, r := range results {
		if r.Status == UpdateAvailable {
			parts = append(parts, fmt.Sprintf("%s %s -> %s", r.Tool.Name, r.InstalledVersion, r.LatestVersion))
		}
	}
	return strings.Join(parts, ", ")
}

// HasUpdates returns true if any result has UpdateAvailable status.
func HasUpdates(results []UpdateResult) bool {
	for _, r := range results {
		if r.Status == UpdateAvailable {
			return true
		}
	}
	return false
}

// CheckFailures returns the names of tools whose remote update check failed.
func CheckFailures(results []UpdateResult) []string {
	failed := make([]string, 0)
	for _, r := range results {
		if r.Status == CheckFailed {
			failed = append(failed, r.Tool.Name)
		}
	}
	return failed
}

// HasCheckFailures returns true when any tool update check failed.
func HasCheckFailures(results []UpdateResult) bool {
	return len(CheckFailures(results)) > 0
}
