package cli

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// NewShellInstallModelWithExperience hands configuration to the existing selector.
// Inspection still refuses effects that the selected backend does not support.
func NewShellInstallModelWithExperience(cancel context.CancelFunc, profile shellinstaller.UserExperience) tea.Model {
	model := NewShellInstallModel(cancel).(shellInstallModel)
	model.req.Experience = &profile
	return NewShellInstallModelWithRequest(cancel, model.req)
}

// NewShellInstallModelWithRequest resumes editing without reusing old consent.
func NewShellInstallModelWithRequest(cancel context.CancelFunc, req shellinstaller.UserInstallRequest) tea.Model {
	model := NewShellInstallModel(cancel).(shellInstallModel)
	req.Confirmation = ""
	model.req = shellInstallRequestSnapshot(req)
	if req.Experience != nil {
		model.err = model.req.Experience.Validate()
	}
	return model
}

// ShellInstallPresentation is a snapshot for the common destination/Ready views.
// It grants no consent or execution authority and does not start a worker.
type ShellInstallPresentation struct {
	Request shellinstaller.UserInstallRequest
	Field   int
	Review  bool
	Preview string
	Error   error
}

func ShellInstallPresentationOf(model tea.Model) (ShellInstallPresentation, bool) {
	selection, ok := model.(shellInstallModel)
	if !ok {
		return ShellInstallPresentation{}, false
	}
	return ShellInstallPresentation{Request: shellInstallRequestSnapshot(selection.req), Field: selection.field, Review: selection.review, Preview: selection.preview, Error: selection.err}, true
}

func shellInstallRequestSnapshot(req shellinstaller.UserInstallRequest) shellinstaller.UserInstallRequest {
	if req.Experience != nil {
		profile := *req.Experience
		req.Experience = &profile
	}
	return req
}
