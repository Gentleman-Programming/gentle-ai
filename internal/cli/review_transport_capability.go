package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const reviewImmutableTransportUnsupportedCode = "immutable_review_transport_unsupported"

var reviewImmutableTransportUnsupportedReason = reviewPreflightReason{
	Code:       reviewImmutableTransportUnsupportedCode,
	Message:    "The active runtime cannot provide immutable receipt-review transport.",
	NextAction: "stop",
}

type reviewImmutableTransport string

const (
	reviewImmutableTransportUnsupported         reviewImmutableTransport = "unsupported"
	reviewImmutableTransportClaudePromptCarried reviewImmutableTransport = "claude_prompt_carried"
	// reviewImmutableTransportOpenCodeProviderInjected is a one-Task,
	// one-process relay. Go owns the provider contract, prompt materialization,
	// admission, capture, and completion binding; the OpenCode plugin only
	// relays opaque frames through its live child process.
	reviewImmutableTransportOpenCodeProviderInjected reviewImmutableTransport = "opencode_provider_injected"
	// reviewImmutableTransportCodexAdvisoryScratchProcess retains the canonical
	// Go-owned provider contract across a fresh Codex subprocess boundary.
	reviewImmutableTransportCodexAdvisoryScratchProcess reviewImmutableTransport = "codex_advisory_scratch_process"
)

type reviewImmutableRuntimePolicy struct {
	Eligible  bool
	Transport reviewImmutableTransport
}

// reviewImmutableRuntimeCapability is the compiled receipt-review boundary.
// Generic adapter features and caller-supplied claims cannot expand it.
func reviewImmutableRuntimeCapability(agent model.AgentID) reviewImmutableRuntimePolicy {
	policy := reviewImmutableRuntimePolicy{Transport: reviewImmutableTransportUnsupported}
	switch agent {
	case model.AgentClaudeCode:
		policy.Eligible = true
	case model.AgentCodex:
		policy.Eligible = true
	case model.AgentKilocode:
		policy.Eligible = true
	case model.AgentOpenCode:
		policy.Eligible = true
	default:
		return policy
	}
	manifest, err := capabilitymanifest.ForAgent(agent)
	if err != nil || !manifest.Advertises(capabilitymanifest.ContractImmutableReviewExecutorV1) {
		return policy
	}
	switch agent {
	case model.AgentClaudeCode:
		policy.Transport = reviewImmutableTransportClaudePromptCarried
	case model.AgentOpenCode:
		policy.Transport = reviewImmutableTransportOpenCodeProviderInjected
	case model.AgentCodex:
		policy.Transport = reviewImmutableTransportCodexAdvisoryScratchProcess
	}
	return policy
}

func (capability reviewImmutableRuntimePolicy) supportsImmutableReceiptReview() bool {
	return capability.Transport == reviewImmutableTransportClaudePromptCarried ||
		capability.Transport == reviewImmutableTransportOpenCodeProviderInjected ||
		capability.Transport == reviewImmutableTransportCodexAdvisoryScratchProcess
}

// reviewTransportSupportedRuntimeIDs derives the actionable runtime list from
// the compiled boundary. A refused runtime cannot appear as a substitute.
func reviewTransportSupportedRuntimeIDs() []string {
	supported := make([]string, 0)
	for _, agent := range catalog.AllAgents() {
		if reviewImmutableRuntimeCapability(agent.ID).supportsImmutableReceiptReview() {
			supported = append(supported, string(agent.ID))
		}
	}
	return supported
}

func reviewTransportRefusalExitGuidance() string {
	return "; exit receipt-driven review with `gentle-ai review mode disable --scope clone --cwd <repo>`; supported immutable review runtimes: " +
		strings.Join(reviewTransportSupportedRuntimeIDs(), ", ")
}

// reviewRuntimeWithImmutableTransport accepts only the exact compiled runtime
// identities. It never selects a substitute transport for an unsupported one.
func reviewRuntimeWithImmutableTransport(agent string) (model.AgentID, error) {
	if strings.TrimSpace(agent) == "" || strings.TrimSpace(agent) != agent {
		// refusal:by-design world-action: an unknown runtime cannot safely receive immutable review authority
		return "", errors.New("the active review runtime is unknown")
	}
	identity := model.AgentID(agent)
	capability := reviewImmutableRuntimeCapability(identity)
	if !capability.Eligible {
		// refusal:by-design world-action: runtimes outside the fixed RDD policy cannot receive immutable review authority
		return "", fmt.Errorf("the active runtime is not eligible for immutable receipt review%s", reviewTransportRefusalExitGuidance())
	}
	if !capability.supportsImmutableReceiptReview() {
		// refusal:by-design world-action: unsupported transport cannot bind immutable evidence or capture an admissible result
		return "", fmt.Errorf("the active runtime lacks immutable receipt-review transport%s", reviewTransportRefusalExitGuidance())
	}
	return identity, nil
}

func reviewRuntimeAgentCount(args []string) int {
	count := 0
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			break
		}
		nameValue := strings.TrimPrefix(strings.TrimPrefix(argument, "-"), "-")
		if nameValue == argument || nameValue == "" {
			continue
		}
		name, hasValue := nameValue, false
		if separator := strings.IndexByte(nameValue, '='); separator >= 0 {
			name, hasValue = nameValue[:separator], true
		}
		if name != "agent" {
			continue
		}
		count++
		if !hasValue {
			index++
		}
	}
	return count
}
