package screens

import "github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"

// ShellBackupsView is the complete render state of the Manage → Replaced
// backups screen. Exactly one of Loading, Confirm, Working or Done is active;
// otherwise the list (or its empty state) is rendered. Err carries the
// preservation error shown on the result state.
type ShellBackupsView struct {
	Target  string
	Mode    string
	Backups []shellinstaller.UserReplacedBackup
	Loading bool
	Confirm bool
	Working bool
	Done    bool
	Err     string
}

// RenderShellBackups renders the Manage → Replaced backups screen: loading,
// the sorted candidate list (or the empty state), the older-cleanup
// confirmation, and the working/result states.
//
// PR C tests-only stub: it always returns the empty string so the render
// tests below are RED on Linux until the screen is implemented.
func RenderShellBackups(view ShellBackupsView, cursor, width, height int) string {
	return ""
}
