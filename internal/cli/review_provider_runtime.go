package cli

import (
	"fmt"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

// reviewProviderAdapter resolves a compiled runtime only after the role's
// canonical contract has declared the shared transport capability. Execution
// can therefore never bypass the role registry that owns the schema, budget,
// prompt, and immutable storage binding.
func reviewProviderAdapter(role string, agent model.AgentID) (reviewerprovider.Adapter, error) {
	contract, err := reviewProviderRoleContractFor(role)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(contract.RequiredCapabilities, reviewProviderTransportCapability) {
		return nil, fmt.Errorf("reviewer provider role %q does not permit the compiled transport", contract.Role) // refusal:by-design world-action: a role must explicitly opt in to the compiled provider transport
	}
	return reviewProviderAdapterFor(contract, agent)
}

var reviewProviderAdapterFor = func(contract reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
	if !slices.Contains(contract.RequiredCapabilities, reviewerprovider.TransportCapability) {
		return nil, fmt.Errorf("reviewer provider role %q does not permit the compiled transport", contract.Role) // refusal:by-design world-action: a role must explicitly opt in to the compiled provider transport
	}
	switch agent {
	case model.AgentClaudeCode:
		return reviewerprovider.NewClaudeAdapter(), nil
	case model.AgentCodex:
		return reviewerprovider.NewCodexAdapter(), nil
	case model.AgentOpenCode:
		return nil, fmt.Errorf("reviewer provider runtime %q is host-mediated; launch the provider-issued OpenCode reviewer task", agent) // refusal:by-design world-action: OpenCode must relay through its ordinary managed host
	default:
		return nil, fmt.Errorf("reviewer provider runtime %q has no registered adapter", agent) // refusal:by-design world-action: immutable reviewer execution requires a compiled adapter binding
	}
}

func reviewProviderCaptureRuntime(agent model.AgentID) bool {
	return agent == model.AgentClaudeCode || agent == model.AgentCodex
}
