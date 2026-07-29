package agents

import (
	"errors"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

var (
	ErrCapabilityNotSupported = errors.New("capability not supported")
	ErrAgentNotSupported      = errors.New("agent not supported")
	ErrDuplicateAdapter       = errors.New("adapter already registered")
)

type CapabilityNotSupportedError struct {
	Agent      model.AgentID
	Capability Capability
}

func (e CapabilityNotSupportedError) Error() string {
	return fmt.Sprintf("agent %q does not support capability %q", e.Agent, e.Capability)
}

func (e CapabilityNotSupportedError) Is(target error) bool {
	return target == ErrCapabilityNotSupported
}

type AgentNotSupportedError struct {
	Agent model.AgentID
}

func (e AgentNotSupportedError) Error() string {
	return fmt.Sprintf("agent %q is not supported in MVP", e.Agent)
}

func (e AgentNotSupportedError) Is(target error) bool {
	return target == ErrAgentNotSupported
}
