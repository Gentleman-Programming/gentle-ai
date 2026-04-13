package kiro

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// AgentNotInstallableError is returned when Kiro IDE cannot be installed automatically.
// Kiro IDE is a desktop application (VS Code fork) that must be installed manually
// or via package manager from https://kiro.dev/downloads.
type AgentNotInstallableError struct {
	Agent model.AgentID
}

func (e AgentNotInstallableError) Error() string {
	return fmt.Sprintf("agent %q cannot be auto-installed; download from https://kiro.dev/downloads", e.Agent)
}
