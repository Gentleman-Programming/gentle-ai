package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/styles"
)

// ReviewStoreResetConfirmOptionCount reports how many options the confirmation
// screen exposes for a given survey.
//
// It is two in the ordinary case -- "Delete permanently" and "Cancel", the same
// shape RenderDeleteConfirm uses -- and one when the survey found reviews that
// have not reached a terminal state. In that case the only option is "Back":
// there is deliberately no keystroke in this TUI that destroys an open review.
// The override is a CLI flag the user has to type, and the screen prints the
// exact command.
func ReviewStoreResetConfirmOptionCount(report reviewtransaction.StoreResetReport, surveyErr error) int {
	return len(ReviewStoreResetConfirmOptions(report, surveyErr))
}

// ReviewStoreResetConfirmOptions returns the options for the given survey. Any
// state with nothing to destroy -- a failed survey, an empty store, or a store
// holding open reviews -- offers only "Back", so cursor 0 is never a
// destructive action except in the one case where destroying is what the user
// came to do.
func ReviewStoreResetConfirmOptions(report reviewtransaction.StoreResetReport, surveyErr error) []string {
	if surveyErr != nil || len(report.InFlight) > 0 || len(reviewStoreResetPresent(report.Removable)) == 0 {
		return []string{"Back"}
	}
	return []string{"Delete permanently", "Cancel"}
}

// ReviewStoreResetConfirmDefaultCursor reports where the cursor should rest
// when this screen is first shown.
//
// It is the "Cancel" row whenever a destructive row exists. The option order is
// kept the way every other confirmation in this TUI orders it, so the change is
// only in what is preselected: an accidental Enter cancels, and destroying the
// store takes a deliberate move onto a row labelled "Delete permanently". In
// every other state the sole option is "Back", which is already inert.
func ReviewStoreResetConfirmDefaultCursor(report reviewtransaction.StoreResetReport, surveyErr error) int {
	options := ReviewStoreResetConfirmOptions(report, surveyErr)
	if len(options) == 2 {
		return 1
	}
	return 0
}

// RenderReviewStoreResetConfirm renders the survey and asks for confirmation.
// Cursor 0 = "Delete permanently", Cursor 1 = "Cancel", unless the survey found
// open reviews, in which case the only option is "Back".
func RenderReviewStoreResetConfirm(report reviewtransaction.StoreResetReport, surveyErr error, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Reset Review Store"))
	b.WriteString("\n\n")

	if surveyErr != nil {
		b.WriteString(styles.ErrorStyle.Render("✗ Could not read the review store"))
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorStyle.Render("  " + surveyErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(renderOptions(ReviewStoreResetConfirmOptions(report, surveyErr), cursor))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter: back • esc: back"))
		return b.String()
	}

	b.WriteString(styles.HeadingStyle.Render("Store: "))
	b.WriteString(styles.SelectedStyle.Render(report.StoreRoot))
	b.WriteString("\n\n")

	removable := reviewStoreResetPresent(report.Removable)
	if len(removable) == 0 {
		b.WriteString(styles.SuccessStyle.Render("This clone holds no review lineage state. Nothing to remove."))
		b.WriteString("\n\n")
		b.WriteString(renderOptions(ReviewStoreResetConfirmOptions(report, surveyErr), cursor))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter: back • esc: back"))
		return b.String()
	}

	b.WriteString(styles.HeadingStyle.Render("Would remove:"))
	b.WriteString("\n")
	total := int64(0)
	for _, entry := range removable {
		total += entry.Bytes
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("  %-32s %6d files  %10s",
			entry.Name, entry.Files, ReviewStoreResetBytes(entry.Bytes))))
		b.WriteString("\n")
	}
	b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("  %-32s %19s", "total", ReviewStoreResetBytes(total))))
	b.WriteString("\n\n")

	if preserved := reviewStoreResetPresent(report.Preserved); len(preserved) > 0 {
		b.WriteString(styles.HeadingStyle.Render("Preserved:"))
		b.WriteString("\n")
		for _, entry := range preserved {
			b.WriteString(styles.SubtextStyle.Render(fmt.Sprintf("  %-32s %10s  %s",
				entry.Name, ReviewStoreResetBytes(entry.Bytes), entry.Reason)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(report.InFlight) > 0 {
		b.WriteString(styles.WarningStyle.Render(fmt.Sprintf(
			"%d review(s) have not reached a terminal state:", len(report.InFlight))))
		b.WriteString("\n")
		for _, lineage := range report.InFlight {
			label := string(lineage.State)
			if lineage.Unreadable != "" {
				label = "unreadable: " + lineage.Unreadable
			}
			b.WriteString(styles.WarningStyle.Render("  " + lineage.LineageID + " (" + label + ")"))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(styles.UnselectedStyle.Render("Finish or abandon them first. To remove them anyway, run:"))
		b.WriteString("\n")
		b.WriteString(styles.SelectedStyle.Render("  gentle-ai review store-reset --cwd " + report.Repository + " --confirm --include-in-flight"))
		b.WriteString("\n\n")
		b.WriteString(renderOptions(ReviewStoreResetConfirmOptions(report, surveyErr), cursor))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("enter: back • esc: back"))
		return b.String()
	}

	b.WriteString(styles.WarningStyle.Render(fmt.Sprintf(
		"Permanently delete %s of review state for this clone?", ReviewStoreResetBytes(total))))
	b.WriteString("\n")
	b.WriteString(styles.WarningStyle.Render("This action cannot be undone. Nothing is copied aside."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions(ReviewStoreResetConfirmOptions(report, surveyErr), cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// RenderReviewStoreResetResult reports what actually happened. A partial run is
// never shown as a success: the categories that failed are listed with their
// reasons, and the freed byte count covers only what really went away.
func RenderReviewStoreResetResult(report reviewtransaction.StoreResetReport, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Reset Review Store"))
	b.WriteString("\n\n")

	switch {
	// "Nothing was removed" is only true when nothing left its place. A run
	// that freed no bytes can still have moved a worktree registration it
	// could not put back, and that is a change to the store the user has to be
	// told about, so it routes to the partial branch instead.
	case err != nil && report.RemovedFiles == 0 && len(report.UnrestoredAdminDirs) == 0:
		b.WriteString(styles.ErrorStyle.Render("✗ Nothing was removed"))
		b.WriteString("\n\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
	case err != nil:
		b.WriteString(styles.WarningStyle.Render("⚠ Partially removed"))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("Freed %s across %d file(s).",
			ReviewStoreResetBytes(report.RemovedBytes), report.RemovedFiles)))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Not removed:"))
		b.WriteString("\n")
		for _, entry := range report.Removable {
			if entry.Skipped == "" {
				continue
			}
			b.WriteString(styles.ErrorStyle.Render("  " + entry.Name + ": " + entry.Skipped))
			b.WriteString("\n")
		}
		if report.Residue != "" {
			b.WriteString("\n")
			b.WriteString(styles.WarningStyle.Render("Staging directory left behind: " + report.Residue))
			b.WriteString("\n")
			b.WriteString(styles.SubtextStyle.Render("Those bytes are not reclaimed until it is deleted."))
			b.WriteString("\n")
		}
		if len(report.UnrestoredAdminDirs) > 0 {
			// "Not removed" above means untouched everywhere else on this
			// screen. For these views it does not, so the exception is
			// spelled out rather than left to be inferred from a byte count.
			b.WriteString("\n")
			b.WriteString(styles.ErrorStyle.Render("Worktree registrations moved aside and not put back:"))
			b.WriteString("\n")
			for _, dir := range report.UnrestoredAdminDirs {
				b.WriteString(styles.ErrorStyle.Render("  " + dir.Original))
				b.WriteString("\n")
				b.WriteString(styles.SubtextStyle.Render("    now at " + dir.Staged))
				b.WriteString("\n")
			}
			b.WriteString(styles.SubtextStyle.Render("Move each one back before using those candidate views again."))
		}
	default:
		b.WriteString(styles.SuccessStyle.Render("✓ Review store reset"))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render(fmt.Sprintf("Freed %s across %d file(s).",
			ReviewStoreResetBytes(report.RemovedBytes), report.RemovedFiles)))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render("The receipt-driven-development kill switch was not changed."))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: back to menu • esc: back"))

	return b.String()
}

func reviewStoreResetPresent(entries []reviewtransaction.StoreResetEntry) []reviewtransaction.StoreResetEntry {
	present := make([]reviewtransaction.StoreResetEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Present {
			present = append(present, entry)
		}
	}
	return present
}

// ReviewStoreResetBytes renders a size the way a person reading a terminal
// reads one.
func ReviewStoreResetBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, exponent := float64(bytes)/unit, 0
	for value >= unit && exponent < 3 {
		value /= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %sB", value, []string{"K", "M", "G", "T"}[exponent])
}
