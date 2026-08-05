package cli

import (
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const reviewTransportCapabilityUnsupportedCode = "review_transport_capability_unsupported"

var reviewTransportCapabilityUnsupportedReason = reviewPreflightReason{
	Code:       reviewTransportCapabilityUnsupportedCode,
	Message:    "The active runtime does not advertise review transport capability.",
	NextAction: "stop",
}

// reviewTransportCapabilityForAgent is a seam: production always resolves
// through capabilitymanifest.ForAgent. Tests override it to synthesize the
// "advertised but fails admission" threat-matrix case without any real,
// compiled agent manifest ever needing to be permanently self-inconsistent.
var reviewTransportCapabilityForAgent = capabilitymanifest.ForAgent

// authorizeReviewTransportCapability is design.md decision 5 / task 5.4's
// admission gate: checked in `review start` before any authority, tier,
// lens, budget, or collection slot exists (reviewRuntimeWithImmutableTransport
// is the only other check that runs earlier, for the narrower immutable-
// receipt-transport property — this gate is the broader "can this runtime
// participate in review at all" precondition).
//
// An absent agent identity (the manual/non-agent compatibility path) is not
// gated at all: there is no adapter here to admit or deny, and direct
// `gentle-ai review start` remains compatibility-supported for explicit,
// manual, non-negotiated callers. A supplied identity that is unrecognised
// (capabilitymanifest.ForAgent returns ErrUnsupportedAgent), whose manifest
// does not advertise ContractReviewTransportV1 (Pi today — it declares only
// AutoInstall|SystemPrompt|MCP), or whose manifest fails its own canonical
// self-consistency Validate(), fails closed. The provider never probes a
// live runtime to decide this — it only ever re-checks the adapter's own
// self-declared shape.
func authorizeReviewTransportCapability(agent string) error {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return nil
	}
	manifest, err := reviewTransportCapabilityForAgent(model.AgentID(agent))
	if err != nil {
		return fmt.Errorf("unrecognised review runtime capability claim for %q: %w", agent, err)
	}
	if !manifest.Advertises(capabilitymanifest.ContractReviewTransportV1) {
		// refusal:by-design world-action: an adapter that never declared review transport cannot receive it by probing
		return fmt.Errorf("runtime %q does not advertise review transport capability", agent)
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("review transport capability manifest for %q is invalid: %w", agent, err)
	}
	return nil
}
