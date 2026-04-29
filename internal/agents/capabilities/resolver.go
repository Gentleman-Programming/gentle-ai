package capabilities

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

const PiMultiModelRequiresPiSubagentsMessage = "PI multi-model requires installing the `pi-subagents` extension."

const RequirementPiSubagentsInstalled RuntimeRequirementID = "pi-subagents-installed"

type RuntimeRequirementID string

type RuntimeRequirement struct {
	ID      RuntimeRequirementID
	Message string
	Reason  string
}

type ResolvedCapabilities struct {
	SupportsSDDMultiMode   bool
	SupportsModelPicker    bool
	SupportsGeneratedMulti bool
	Requires               []RuntimeRequirement
}

type PiSubagentsDetector interface {
	DetectPiSubagents(homeDir string, workspaceDir string) (bool, error)
}

type Resolver struct {
	piDetector PiSubagentsDetector
}

func NewResolver(piDetector PiSubagentsDetector) *Resolver {
	if piDetector == nil {
		piDetector = pi.NewAdapter()
	}

	return &Resolver{piDetector: piDetector}
}

func (r *Resolver) Resolve(homeDir string, workspaceDir string, agentID model.AgentID) (ResolvedCapabilities, error) {
	switch agentID {
	case model.AgentOpenCode:
		return enabledMultiModelCapabilities(), nil
	case model.AgentPiCodingAgent:
		detected, err := r.piDetector.DetectPiSubagents(homeDir, workspaceDir)
		if err != nil {
			return disabledPiCapabilities(fmt.Sprintf("pi-subagents detection failed: %v", err)), nil
		}
		if !detected {
			return disabledPiCapabilities("pi-subagents extension not detected"), nil
		}
		return enabledMultiModelCapabilities(), nil
	default:
		return ResolvedCapabilities{}, nil
	}
}

func enabledMultiModelCapabilities() ResolvedCapabilities {
	return ResolvedCapabilities{
		SupportsSDDMultiMode:   true,
		SupportsModelPicker:    true,
		SupportsGeneratedMulti: true,
	}
}

func disabledPiCapabilities(reason string) ResolvedCapabilities {
	return ResolvedCapabilities{
		SupportsSDDMultiMode:   false,
		SupportsModelPicker:    false,
		SupportsGeneratedMulti: false,
		Requires:               []RuntimeRequirement{piSubagentsRequirement(reason)},
	}
}

func piSubagentsRequirement(reason string) RuntimeRequirement {
	return RuntimeRequirement{
		ID:      RequirementPiSubagentsInstalled,
		Message: PiMultiModelRequiresPiSubagentsMessage,
		Reason:  reason,
	}
}
