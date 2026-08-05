package cli

import (
	"errors"
	"strings"

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
	// reviewImmutableTransportOpenCodeProviderInjected is issue #2417's
	// restored transport: the OpenCode plugin (review-result-artifacts.ts)
	// asks `review lens-context` for the finished reviewer context through
	// its shell-less runNative channel and injects those exact bytes into
	// the reviewer task's prompt before the reviewer ever launches. The
	// provider materializes the evidence, applies the budget, and resolves
	// every refusal; the plugin assembles nothing. The generated lens holds no
	// bash and no read tool. OpenCode itself concatenates live project
	// instructions (AGENTS.md/CLAUDE.md/CONTEXT.md, local `instructions`
	// glob entries) and the skill catalog into every session's system
	// prompt regardless of tools, so the plugin also refuses to launch the
	// reviewer unless OPENCODE_DISABLE_PROJECT_CONFIG and
	// OPENCODE_DISABLE_EXTERNAL_SKILLS are both set. OpenCode also fetches
	// any remote (http/https) `instructions` entry unconditionally, from
	// any config layer, regardless of either variable, so the plugin
	// separately reads the effective configuration through its own OpenCode
	// client (client.config.get) and refuses to launch the reviewer if one
	// is present, naming the offending entry. Only then is the injected
	// block provably the reviewer's only byte source.
	reviewImmutableTransportOpenCodeProviderInjected reviewImmutableTransport = "opencode_provider_injected"
)

type reviewImmutableRuntimePolicy struct {
	Eligible  bool
	Transport reviewImmutableTransport
}

// reviewImmutableRuntimeCapability is the compiled receipt-review boundary.
// Generic adapter features and caller-supplied claims cannot expand it.
func reviewImmutableRuntimeCapability(agent model.AgentID) reviewImmutableRuntimePolicy {
	switch agent {
	case model.AgentClaudeCode:
		return reviewImmutableRuntimePolicy{Eligible: true, Transport: reviewImmutableTransportClaudePromptCarried}
	case model.AgentCodex:
		return reviewImmutableRuntimePolicy{Eligible: true, Transport: reviewImmutableTransportUnsupported}
	case model.AgentOpenCode:
		// #2417 restored genuine support through the provider-injected
		// shell-less channel; #2076 (per-session exact-value Bash-permission
		// binding) remains structurally impossible because OpenCode reads
		// its config only at process startup, before review.start mints any
		// dynamic value, and is no longer needed for support.
		return reviewImmutableRuntimePolicy{Eligible: true, Transport: reviewImmutableTransportOpenCodeProviderInjected}
	default:
		return reviewImmutableRuntimePolicy{Transport: reviewImmutableTransportUnsupported}
	}
}

func (capability reviewImmutableRuntimePolicy) supportsImmutableReceiptReview() bool {
	return capability.Transport == reviewImmutableTransportClaudePromptCarried ||
		capability.Transport == reviewImmutableTransportOpenCodeProviderInjected
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
		return "", errors.New("the active runtime is not eligible for immutable receipt review")
	}
	if !capability.supportsImmutableReceiptReview() {
		// refusal:by-design world-action: unsupported transport cannot bind immutable evidence or capture an admissible result
		return "", errors.New("the active runtime lacks immutable receipt-review transport")
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
