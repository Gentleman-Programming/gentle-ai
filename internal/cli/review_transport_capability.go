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
	// reviewImmutableTransportOpenCodeProviderInjected is a one-Task,
	// one-process relay. Go owns the provider contract, prompt materialization,
	// admission, capture, and completion binding; the OpenCode plugin only
	// relays opaque frames through its live child process.
	reviewImmutableTransportOpenCodeProviderInjected reviewImmutableTransport = "opencode_provider_injected"
	// reviewImmutableTransportCodexAdvisoryScratchProcess retains the canonical
	// Go-owned provider contract across a fresh Codex subprocess boundary.
	reviewImmutableTransportCodexAdvisoryScratchProcess reviewImmutableTransport = "codex_advisory_scratch_process"
	// reviewImmutableTransportPiHostRelay is host-mediated like OpenCode's
	// transport, but with the launcher owned by gentle-pi: the Pi host reads
	// the negotiated collection input, launches a brand-new print-mode pi
	// subprocess in an empty scratch directory with every discovery surface
	// disabled, forwards the Go-issued opaque prompt untouched, and returns
	// the raw final bytes through the exact capture operation. Go keeps
	// prompt materialization, admission, budgets, receipts, and gates.
	reviewImmutableTransportPiHostRelay reviewImmutableTransport = "pi_host_relay"
)

// reviewPiHostRelayContract is the exact relay contract this binary admits.
// The Pi launcher lives in gentle-pi and is versioned independently; it
// declares this identity on every invocation it relays, and any other value
// (or none) keeps Pi fail-closed at admission instead of freezing review
// authority no installed host can ever collect.
const reviewPiHostRelayContract = "gentle-pi.review-relay/v1"

const reviewPiHostRelayContractEnvironment = "GENTLE_PI_REVIEW_RELAY_CONTRACT"

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
	case model.AgentOpenCode:
		policy.Eligible = true
	case model.AgentPi:
		// The relay's declared contract is a required conjunct: it can only
		// narrow the compiled boundary, never expand it. Without the exact
		// handshake, `review start --agent pi` refuses before any repository,
		// target, or authority work, and Pi never appears as a suggested exit.
		if os.Getenv(reviewPiHostRelayContractEnvironment) != reviewPiHostRelayContract {
			return policy
		}
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
	case model.AgentPi:
		policy.Transport = reviewImmutableTransportPiHostRelay
	}
	return policy
}

func (capability reviewImmutableRuntimePolicy) supportsImmutableReceiptReview() bool {
	return capability.Transport == reviewImmutableTransportClaudePromptCarried ||
		capability.Transport == reviewImmutableTransportOpenCodeProviderInjected ||
		capability.Transport == reviewImmutableTransportCodexAdvisoryScratchProcess ||
		capability.Transport == reviewImmutableTransportPiHostRelay
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

// reviewPiRelayHandshakeIsSoleMissingCondition reports whether Pi's refusal is
// caused by nothing but the absent relay handshake: every other compiled
// conjunct already holds, so declaring the exact contract is the single
// remaining step. It decides which cause the operator reads and nothing else;
// reviewImmutableRuntimeCapability stays the only admission authority, and no
// runtime becomes eligible because of what this reports.
func reviewPiRelayHandshakeIsSoleMissingCondition(agent model.AgentID) bool {
	if agent != model.AgentPi {
		return false
	}
	if os.Getenv(reviewPiHostRelayContractEnvironment) == reviewPiHostRelayContract {
		return false
	}
	manifest, err := capabilitymanifest.ForAgent(agent)
	return err == nil && manifest.Advertises(capabilitymanifest.ContractImmutableReviewExecutorV1)
}

// reviewPiRelayHandshakeGuidance names the one missing condition instead of
// the kill switch. reviewTransportSupportedRuntimeIDs can never name `pi`
// here, because it is computed by calling reviewImmutableRuntimeCapability in
// this same process, under the exact condition that is failing. Offering
// `review mode disable` is worse than unhelpful: leaving receipt-driven
// review is a legitimate choice, but it is not the remedy for a runtime one
// declared contract away from eligible, and a reader who follows it concludes
// the runtime was dropped.
//
// The guidance names the variable but NOT its value, and that is deliberate
// rather than an oversight. This prose reaches the operator only as the
// negotiated envelope's `cause`, which every refusal crosses
// reviewScrubDefectReportField to reach: that gate rewrites any `KEY=VALUE`
// token to `<redacted>` in full, and any `/`-rooted run to `<redacted>` from
// the slash onward. The exact handshake collides with both rules, so
// `GENTLE_PI_REVIEW_RELAY_CONTRACT=gentle-pi.review-relay/v1` renders as
// `<redacted>` and `gentle-pi.review-relay/v1` renders as
// `gentle-pi.review-relay<redacted>`. Naming the variable alone survives the
// gate byte for byte (pinned by
// TestPiRelayHandshakeGuidanceSurvivesTheFailureCausePrivacyGate), so it is
// the most actionable cause that can actually reach the operator without
// relaxing a privacy boundary from a diagnostics change.
func reviewPiRelayHandshakeGuidance() string {
	return "; pi is eligible only while " + reviewPiHostRelayContractEnvironment +
		" declares the exact relay contract this binary admits, which the gentle-pi host exports on every invocation it relays; export it in this shell and re-run"
}

// reviewTransportRefusalGuidanceFor selects the guidance the refused runtime
// can actually act on. Every runtime other than a handshake-less Pi keeps the
// generic exit guidance unchanged, so the env-var remedy never leaks to a
// runtime it cannot help.
func reviewTransportRefusalGuidanceFor(agent model.AgentID) string {
	if reviewPiRelayHandshakeIsSoleMissingCondition(agent) {
		return reviewPiRelayHandshakeGuidance()
	}
	return reviewTransportRefusalExitGuidance()
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
		return "", fmt.Errorf("the active runtime is not eligible for immutable receipt review%s", reviewTransportRefusalGuidanceFor(identity))
	}
	if !capability.supportsImmutableReceiptReview() {
		// refusal:by-design world-action: unsupported transport cannot bind immutable evidence or capture an admissible result
		return "", fmt.Errorf("the active runtime lacks immutable receipt-review transport%s", reviewTransportRefusalGuidanceFor(identity))
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
