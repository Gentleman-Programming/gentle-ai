package upgrade

import (
	"errors"

	"github.com/gentleman-programming/gentle-ai/internal/update"
)

// ToolUpgradeStatus describes the outcome of a single tool upgrade attempt.
type ToolUpgradeStatus string

const (
	UpgradeSucceeded ToolUpgradeStatus = "succeeded"
	UpgradeFailed    ToolUpgradeStatus = "failed"
	UpgradeSkipped   ToolUpgradeStatus = "skipped" // dry-run, dev build, or unsupported platform
)

// ManualFallbackError signals that a tool requires manual intervention rather
// than automated upgrade. Callers (e.g. executeOne) must treat this as
// UpgradeSkipped with ManualHint populated — NOT as UpgradeFailed.
type ManualFallbackError struct {
	Hint string
}

func (e *ManualFallbackError) Error() string {
	return e.Hint
}

// AsManualFallback unwraps err to check if it is a ManualFallbackError.
// Returns (hint, true) when it is, ("", false) otherwise.
func AsManualFallback(err error) (string, bool) {
	var mfe *ManualFallbackError
	if errors.As(err, &mfe) {
		return mfe.Hint, true
	}
	return "", false
}

// ToolUpgradeResult holds the outcome of upgrading a single tool.
type ToolUpgradeResult struct {
	ToolName   string
	OldVersion string
	NewVersion string
	Method     update.InstallMethod
	Status     ToolUpgradeStatus
	Err        error
	// ManualHint is set when the tool requires manual intervention instead of
	// automated upgrade (e.g. Windows self-replace, unsupported binary path).
	ManualHint string
}

// UpgradeReport is the top-level result returned by Execute.
type UpgradeReport struct {
	// BackupID is the snapshot ID created before upgrade execution.
	// Empty when no upgrades were executed (nothing to back up).
	BackupID string

	// BackupWarning is set when backup creation was attempted but failed.
	// A non-empty value means the upgrade ran without a pre-execution backup.
	// This surfaces the G6 gap: backup failures are no longer silently skipped.
	BackupWarning string

	Results []ToolUpgradeResult
	DryRun  bool
}
