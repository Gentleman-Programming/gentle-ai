package cli

import (
	"context"
	"errors"
	"io"

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
	req := selection.Request
	profile := req.Experience
	req.Experience = nil // Encoded separately, never discarded from the request.
	args := append([]string{"install"}, shellEntryValues(req)...)
	if profile != nil {
		encoded, err := profile.Encode()
		if err != nil || len(encoded) > 256 {
			return nil
		}
		args = append(args, "--experience", encoded)
	}
	return args
}

// RunShellInstallSelection runs only after the owning TUI restores its terminal.
// Confirmed is not authority: the physical selection and token are checked again.
func RunShellInstallSelection(selection ShellInstallSelection, stdout io.Writer) (resultErr error) {
	if !selection.Confirmed {
		return nil
	}
	defer func() { resultErr = preserveShellFailure(resultErr) }()
	ctx, cancel, self, err := shellExecutionContext()
	if err != nil {
		return err
	}
	defer cancel()
	return runShellInstallRequest(ctx, self, selection.Request, false, stdout)
}
