package cli

import (
	"errors"
	"fmt"
	"os"
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
	}
	return policy
}

func (capability reviewImmutableRuntimePolicy) supportsImmutableReceiptReview() bool {
	return capability.Transport == reviewImmutableTransportClaudePromptCarried ||
		capability.Transport == reviewImmutableTransportOpenCodeProviderInjected
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

func reviewOpenCodeIsolationPreflight() error {
	missing := make([]string, 0, 2)
	for _, name := range []string{"OPENCODE_DISABLE_PROJECT_CONFIG", "OPENCODE_DISABLE_EXTERNAL_SKILLS"} {
		if os.Getenv(name) != "1" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	// refusal:by-design world-action: OpenCode cannot launch a fresh reviewer safely until its host disables every local instruction channel
	return fmt.Errorf("OpenCode immutable review requires its host process to set %s to 1 before review start%s", strings.Join(missing, " and "), reviewTransportRefusalExitGuidance())
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
	if identity == model.AgentOpenCode {
		if err := reviewOpenCodeIsolationPreflight(); err != nil {
			return "", err
		}
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
