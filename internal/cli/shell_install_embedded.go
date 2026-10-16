package cli

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// ShellInstallSelection is the reviewed request, not execution authority.
// The worker still validates the physical selection and its confirmation.
type ShellInstallSelection struct {
	Request   shellinstaller.UserInstallRequest
	Confirmed bool
}

// NewShellInstallModel creates a selector for embedding in an existing program.
// It never starts an installer worker; confirmation quits the selector so its
// owner can restore the terminal before executing ShellInstallArguments.
func NewShellInstallModel(cancel context.CancelFunc) tea.Model {
	if cancel == nil {
		cancel = func() {}
	}
	return newShellInstallSelectionModel(cancel)
}

// ShellInstallOutcome reads the final selector without performing installation.
func ShellInstallOutcome(model tea.Model) (ShellInstallSelection, error) {
	selection, ok := model.(shellInstallModel)
	if !ok {
		return ShellInstallSelection{}, errors.New("installer selection is unavailable; run gentle-ai shell install to review a new selection")
	}
	return ShellInstallSelection{Request: selection.req, Confirmed: selection.confirmed}, selection.err
}

// ShellInstallArguments preserves the backend's existing entry protocol.
// Unconfirmed and cancelled selectors have no executable argument vector.
func ShellInstallArguments(selection ShellInstallSelection) []string {
	if !selection.Confirmed {
		return nil
	}
	return append([]string{"install"}, shellEntryValues(selection.Request)...)
}
