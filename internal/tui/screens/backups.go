package screens

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/backup"
	"github.com/gentleman-programming/gentle-ai/internal/tui/styles"
)

// BackupMaxVisible is the maximum number of backup items shown at once.
// Exported so model.go can compute scroll adjustments.
const BackupMaxVisible = 10

// RenderBackups renders the backup selection screen with scroll support.
// It uses manifest.DisplayLabel() to show source + timestamp for each backup.
func RenderBackups(backups []backup.Manifest, cursor int, scrollOffset int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Backup Management"))
	b.WriteString("\n\n")

	if len(backups) == 0 {
		b.WriteString(styles.WarningStyle.Render("No backups found yet."))
		b.WriteString("\n\n")
		b.WriteString(renderOptions([]string{"Back"}, 0))
		return b.String()
	}

	end := scrollOffset + BackupMaxVisible
	if end > len(backups) {
		end = len(backups)
	}

	if scrollOffset > 0 {
		b.WriteString(styles.SubtextStyle.Render("  ↑ more"))
		b.WriteString("\n")
	}

	for i := scrollOffset; i < end; i++ {
		snapshot := backups[i]
		// Use DisplayLabel for richer labels: "install — 2026-03-22 15:04 (5 files)"
		// Falls back to "unknown source — 2026-03-22 15:04" for old manifests.
		displayLabel := snapshot.DisplayLabel()
		if snapshot.CreatedByVersion != "" {
			displayLabel = fmt.Sprintf("%s  [v%s]", displayLabel, snapshot.CreatedByVersion)
		}
		if snapshot.Description != "" {
			displayLabel = fmt.Sprintf("%s  — %s", displayLabel, snapshot.Description)
		}
		label := fmt.Sprintf("%s  (%s)", snapshot.ID, displayLabel)
		focused := i == cursor
		if focused {
			b.WriteString(styles.SelectedStyle.Render(styles.Cursor + label))
		} else {
			b.WriteString(styles.UnselectedStyle.Render("  " + label))
		}
		b.WriteString("\n")
	}

	if end < len(backups) {
		b.WriteString(styles.SubtextStyle.Render("  ↓ more"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(renderOptions([]string{"Back"}, cursor-len(backups)))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: restore • r: rename • d: delete • esc: back"))

	return b.String()
}

// RenderRestoreConfirm renders the restore confirmation screen.
// It shows the backup identity and asks the user to confirm or cancel.
// Cursor 0 = "Restore", Cursor 1 = "Cancel".
func RenderRestoreConfirm(manifest backup.Manifest, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Restore Backup"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Backup: "))
	b.WriteString(styles.SelectedStyle.Render(manifest.ID))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render("This will overwrite your current configuration."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions([]string{"Restore", "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// RenderRestoreResult renders the restore result screen.
// Shows a success message when err is nil, or an error message with details.
func RenderRestoreResult(manifest backup.Manifest, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Restore Result"))
	b.WriteString("\n\n")

	if err == nil {
		b.WriteString(styles.SuccessStyle.Render("✓ Restore complete"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Restored: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("Your configuration has been restored from this backup."))
	} else {
		b.WriteString(styles.ErrorStyle.Render("✗ Restore failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Backup: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Error:"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Your files were not modified."))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: back to backups • esc: back"))

	return b.String()
}

// RenderDeleteConfirm renders the delete confirmation screen.
// Shows backup info and asks the user to confirm or cancel the deletion.
// Cursor 0 = "Delete", Cursor 1 = "Cancel".
func RenderDeleteConfirm(manifest backup.Manifest, cursor int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Delete Backup"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Backup: "))
	b.WriteString(styles.SelectedStyle.Render(manifest.ID))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
	b.WriteString("\n\n")

	b.WriteString(styles.WarningStyle.Render("Are you sure you want to permanently delete this backup?"))
	b.WriteString("\n")
	b.WriteString(styles.WarningStyle.Render("This action cannot be undone."))
	b.WriteString("\n\n")

	b.WriteString(renderOptions([]string{"Delete", "Cancel"}, cursor))
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("j/k: navigate • enter: select • esc: back"))

	return b.String()
}

// RenderDeleteResult renders the delete result screen.
// Shows a success message when err is nil, or an error message with details.
func RenderDeleteResult(manifest backup.Manifest, err error) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Delete Result"))
	b.WriteString("\n\n")

	if err == nil {
		b.WriteString(styles.SuccessStyle.Render("✓ Backup deleted"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Deleted: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n")
		b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
		b.WriteString("\n\n")
		b.WriteString(styles.UnselectedStyle.Render("The backup has been permanently removed."))
	} else {
		b.WriteString(styles.ErrorStyle.Render("✗ Delete failed"))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("Backup: "))
		b.WriteString(styles.SelectedStyle.Render(manifest.ID))
		b.WriteString("\n\n")
		b.WriteString(styles.HeadingStyle.Render("Error:"))
		b.WriteString("\n")
		b.WriteString(styles.ErrorStyle.Render("  " + err.Error()))
		b.WriteString("\n\n")
		b.WriteString(styles.SubtextStyle.Render("The backup directory may still exist."))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("enter: back to backups • esc: back"))

	return b.String()
}

// RenderRenameBackup renders the rename backup screen with a text input field.
// Shows current description and a text field for the new description.
func RenderRenameBackup(manifest backup.Manifest, inputText string, cursorPos int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Rename Backup"))
	b.WriteString("\n\n")

	b.WriteString(styles.HeadingStyle.Render("Backup: "))
	b.WriteString(styles.SelectedStyle.Render(manifest.ID))
	b.WriteString("\n")
	b.WriteString(styles.SubtextStyle.Render(manifest.DisplayLabel()))
	b.WriteString("\n\n")

	if manifest.Description != "" {
		b.WriteString(styles.SubtextStyle.Render("Current description: "))
		b.WriteString(styles.UnselectedStyle.Render(manifest.Description))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HeadingStyle.Render("New description:"))
	b.WriteString("\n")

	// Render text input with cursor indicator.
	runes := []rune(inputText)
	var inputDisplay strings.Builder
	for i, r := range runes {
		if i == cursorPos {
			inputDisplay.WriteString(styles.SelectedStyle.Render("|"))
		}
		inputDisplay.WriteRune(r)
	}
	if cursorPos == len(runes) {
		inputDisplay.WriteString(styles.SelectedStyle.Render("|"))
	}

	b.WriteString(styles.UnselectedStyle.Render("  > "))
	b.WriteString(inputDisplay.String())
	b.WriteString("\n\n")

	b.WriteString(styles.HelpStyle.Render("enter: save • esc: cancel • ←/→: move cursor • backspace: delete"))

	return b.String()
}
