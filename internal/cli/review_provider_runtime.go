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
	case model.AgentPi:
		return nil, fmt.Errorf("reviewer provider runtime %q is host-mediated; launch the provider-issued Pi reviewer task", agent) // refusal:by-design world-action: Pi's launcher lives in gentle-pi and relays the Go-issued opaque task
	default:
		return nil, fmt.Errorf("reviewer provider runtime %q has no registered adapter", agent) // refusal:by-design world-action: immutable reviewer execution requires a compiled adapter binding
	}
}

// reviewProviderCaptureRuntime reads the one canonical answer rather than
// restating it. The installed orchestrator contract renders from the same
// predicate, so the transport this dispatcher will execute and the transport
// the contract tells a parent to use can never describe different runtimes
// (issue #3825).
func reviewProviderCaptureRuntime(agent model.AgentID) bool {
	return reviewerprovider.CapturesInProcess(agent)
}

// reviewProviderHostRelayMaterializeRuntime reports whether the runtime's
// compiled immutable transport is the Pi host relay: the one transport whose
// host first prints the exact Go-materialized opaque provider task
// (`review capture-result ... --agent=pi --materialize=true`), runs its own
// fresh locked-down reviewer subprocess on those bytes, and then submits the
// raw result through the existing --input path with the same binding. The
// answer stays false without the exact relay handshake, so materialization is
// never offered to a Pi installation whose launcher cannot collect it.
func reviewProviderHostRelayMaterializeRuntime(agent model.AgentID) bool {
	return reviewImmutableRuntimeCapability(agent).Transport == reviewImmutableTransportPiHostRelay
}
