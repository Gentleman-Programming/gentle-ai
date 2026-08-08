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
	// reviewImmutableTransportOpenCodeProviderInjected is the shared advisory
	// transport (rdd-advisory-transport SKILL.md): the OpenCode plugin
	// (review-result-artifacts.ts) asks `review lens-context` for the
	// finished reviewer context through its shell-less runNative channel and
	// injects those exact bytes into the reviewer task's prompt before the
	// reviewer ever launches. The provider materializes the evidence, applies
	// the budget, and resolves every refusal; the plugin assembles nothing,
	// interprets no binding field, and captures no result -- it hands the
	// model's raw final text back for native admission. The generated lens
	// holds no bash and no read tool. An ordinary already-running OpenCode
	// session is sufficient: no restart, no child process, no special
	// user-visible session, and no OPENCODE_DISABLE_* variable, because the
	// runtime's output is advisory and cannot mint authority until Go admits
	// it.
	reviewImmutableTransportOpenCodeProviderInjected reviewImmutableTransport = "opencode_provider_injected"
	// reviewImmutableTransportCodexAdvisoryScratchProcess is the shared
	// advisory transport's Codex boundary (rdd-advisory-transport SKILL.md):
	// internal/advisoryreview's CodexAdapter launches a brand-new `codex
	// exec` process in an empty scratch directory it creates and deletes
	// itself, handing it only the canonical provider-rendered prompt
	// (advisoryreview.PromptFor). Codex's own shell tool stays permitted even
	// under --sandbox read-only (that flag bounds writes and network, not
	// reads), so the enforced boundary is the empty directory, not a
	// no-tool agent config this CLI does not have for Codex. Proven
	// organically by TestRealCodexReviewerOrdinarySessionAdmitsRawOutput and
	// its fail-closed companions in e2e/organicruntime: the reviewer's raw
	// output reached native admission and a terminal receipt while a
	// poisoned live worktree never did.
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
