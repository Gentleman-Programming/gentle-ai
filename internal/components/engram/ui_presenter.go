package engram

import (
	"errors"
	"fmt"
)

// FormatBytes renders a byte count as a human-readable string (e.g. "1.2 MB").
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// PreviewFileNames returns a list of formatted file descriptions for UI display.
// Each entry is "filename (size)" e.g. "engram.db (12.5 KB)".
func PreviewFileNames(files []FileInfo) []string {
	names := make([]string, 0, len(files))
	for _, fi := range files {
		names = append(names, fmt.Sprintf("%s (%s)", fi.Name, FormatBytes(fi.Size)))
	}
	return names
}

// ConfirmTitle returns the confirmation screen title for the given action.
func ConfirmTitle(action Action) string {
	switch action {
	case ActionClean:
		return "CONFIRM CLEAN DATA"
	case ActionMigrate:
		return "CONFIRM MIGRATION"
	case ActionStartFresh:
		return "CONFIRM DELETE & START FRESH"
	}
	return "CONFIRM ACTION"
}

// ConfirmMessage returns the confirmation screen message for the given action.
func ConfirmMessage(action Action, srcDir, dstDir string) string {
	switch action {
	case ActionClean:
		return fmt.Sprintf("This will permanently delete all Engram data at:\n%s", srcDir)
	case ActionMigrate:
		return fmt.Sprintf("This will move all Engram data from:\n  %s\nto:\n  %s", srcDir, dstDir)
	case ActionStartFresh:
		return fmt.Sprintf("This will delete all existing Engram data at:\n  %s\nand create a new empty database at:\n  %s", srcDir, dstDir)
	}
	return ""
}

// ConfirmWarning returns the warning line for the confirmation screen.
func ConfirmWarning(action Action) string {
	switch action {
	case ActionClean:
		return "This cannot be undone. All memory will be lost."
	case ActionMigrate:
		return "The original files will be deleted after successful migration."
	case ActionStartFresh:
		return "All existing memory will be permanently lost."
	}
	return ""
}

// FeedbackTitle returns the feedback screen title for the given action.
func FeedbackTitle(action Action) string {
	switch action {
	case ActionClean:
		return "DATA CLEANED"
	case ActionMigrate:
		return "MIGRATION COMPLETE"
	case ActionStartFresh:
		return "FRESH DATABASE CREATED"
	}
	return "COMPLETE"
}

// FeedbackDetails returns the detail line for the feedback screen.
func FeedbackDetails(action Action, filesMoved int, bytesMoved uint64) string {
	if action == ActionMigrate && filesMoved > 0 {
		return fmt.Sprintf("%d files moved, %s transferred", filesMoved, FormatBytes(bytesMoved))
	}
	return ""
}

// PreviewMessage returns the contextual subtext shown below the file list
// on the choose screen when a custom path is selected.
func PreviewMessage(action Action) string {
	switch action {
	case ActionMigrate:
		return "The files above will be copied to the new location, verified, then removed from the original location."
	case ActionStartFresh:
		return "All existing data above will be permanently deleted, then a new empty database will be created at the new location."
	}
	return ""
}

// WarningMessage returns the contextual warning for destructive options
// on the choose screen.
func WarningMessage(action Action) string {
	switch action {
	case ActionStartFresh:
		return "Warning: existing data will be deleted. A new empty database will be created at the path you specify."
	case ActionClean:
		return "Warning: all existing Engram data will be permanently deleted. This cannot be undone."
	}
	return ""
}

// ErrorMessage wraps a backend/service error into a user-friendly string.
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrLocked):
		return "Engram data appears to be in use. Close any running engram processes and try again."
	case errors.Is(err, ErrInsufficientSpace):
		return err.Error()
	case errors.Is(err, ErrInvalidPath):
		return "The path you entered is invalid. Please check the path and try again."
	case errors.Is(err, ErrPathNotWritable):
		return "The selected directory is not writable. Please choose a different location or fix permissions."
	case errors.Is(err, ErrTargetHasData):
		return "The selected directory already contains Engram data. Choose an empty directory, keep that location explicitly, or clean it first."
	default:
		return err.Error()
	}
}
