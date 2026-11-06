package agentguidance

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// OrchestratorSectionID is the managed marker section that owns the
// orchestrator instructions (coordinator role, lossless blocking prompts,
// delegation and verification gates, and the native review lifecycle).
//
// The ODD protocol itself is owned by the routing section: every orchestrator
// asset points at `## Implementation Routing` instead of restating it, so the
// two sections together carry the protocol exactly once.
const OrchestratorSectionID = "orchestrator"

// legacyOrchestratorSectionID is the marker v3.7.0 installed the same
// orchestrator under, when the retired SDD component owned it. Upgrades convert
// that block in place so a prompt never carries two orchestrators.
const legacyOrchestratorSectionID = "sdd-orchestrator"

const (
	sharedOrchestratorSectionsAsset = "skills/_shared/odd-orchestrator-sections.md"
	runtimeAgentIDPlaceholder       = "{{GENTLE_AI_RUNTIME_AGENT_ID}}"
	reviewExecutionHeading          = "#### Review Execution Contract"
	reviewExecutionNextHeading      = "Cost and Context Balance"
	providerDefectHandoffHeading    = "#### Gentle AI Provider Defect Handoff (MANDATORY)"
	nativeCheckingHeading           = "#### Native Checking Contract"
	nativeCheckingSection           = "Native Checking Contract"
	// oddOnlySectionSuffix names the shared-section variant a runtime without
	// receipt-driven development receives in place of the canonical body.
	oddOnlySectionSuffix = " (ODD only)"
)

// nonRDDReplacements rewrites the inline review wording the runtime assets
// share into its ODD-only form. Whole sections are handled separately.
var nonRDDReplacements = strings.NewReplacer(
	"| Tests, builds, installs, or native review actions |", "| Tests, builds, installs, or verification actions |",
	"| High-risk change, or long suites, builds, installs, or native review actions of a large task |", "| High-risk change, or long suites, builds, installs, or verification actions of a large task |",
	"tests, builds, installs, and native review actors may use fresh workers", "tests, builds, installs, and verification actors may use fresh workers",
	"run applicable tests, builds, and native review actions as bounded steps", "run applicable tests, builds, and verification actions as bounded steps",
	"- Let the native review and delivery providers select checking and delivery actions; repeated gates reuse exact authority and never reopen review for unchanged content.", "- Let the user and ordinary repository policy decide delivery; do not infer authorization from checking output.",
	"- Let native review select its bounded checking plan; delivery remains human-owned under ordinary repository policy.", "- Let the user and ordinary repository policy decide delivery; do not infer authorization from checking output.",
)

// nonRDDLeakMarkers fail a render closed: a runtime without receipt-driven
// development must never be handed a piece of the review lifecycle because an
// asset gained wording the gate above does not know about yet.
var nonRDDLeakMarkers = []string{
	"RDD",
	"receipt",
	"Receipt",
	"refuter",
	"native review",
	"Native review",
	"gentle-ai review",
	"review-integration",
	"Native Compact Review Orchestration",
	"Provider Defect Handoff",
}

// ErrMissingReviewContract fails closed when a runtime that advertises the
// native review transport would receive an orchestrator without its review
// execution contract: installing a review lifecycle with no procedure in it is
// worse than refusing the install.
var ErrMissingReviewContract = errors.New("review execution contract source is not configured")

// ReviewContractSource renders the runtime-bound native review execution
// contract. It is reviewassets.ReviewExecutionContractFor in production; the
// seam exists only because reviewassets already imports this package. Callers
// pass it through RoutingOptions.ReviewContract or RenderOrchestratorWithSource.
type ReviewContractSource func(model.AgentID) (string, error)

var (
	reviewContractMu     sync.RWMutex
	reviewContractSource ReviewContractSource
)

// reviewExecutionContract returns the contract for a runtime that receives
// receipt-driven development, or ok=false for one that does not. A non-nil
// source wins over the package-level fallback.
func reviewExecutionContract(agent model.AgentID, source ReviewContractSource) (contract string, ok bool, err error) {
	if _, err := capabilitymanifest.ForAgent(agent); err != nil {
		return "", false, err
	}
	if !model.SupportsReceiptDrivenDevelopment(agent) {
		return "", false, nil
	}
	if source == nil {
		reviewContractMu.RLock()
		source = reviewContractSource
		reviewContractMu.RUnlock()
	}
	if source == nil {
		return "", false, fmt.Errorf("review contract source was not wired for %q (set RoutingOptions.ReviewContract): %w", agent, ErrMissingReviewContract)
	}
	contract, err = source(agent)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(contract) == "" {
		return "", false, ErrMissingReviewContract
	}
	return contract, true, nil
}

// sharedOrchestratorSectionPlaceholder matches one {{GENTLE_AI_ODD_SECTION:<name>}}.
var sharedOrchestratorSectionPlaceholder = regexp.MustCompile(`\{\{GENTLE_AI_ODD_SECTION:([^}]*)\}\}`)

// orchestratorAsset selects the embedded orchestrator for one runtime. It
// mirrors the v3.7.0 selection exactly: runtime-specific assets win, Kilo shares
// the OpenCode asset, and every other runtime receives the generic asset.
func orchestratorAsset(agent model.AgentID) string {
	switch agent {
	case model.AgentClaudeCode:
		return "claude/orchestrator.md"
	case model.AgentGeminiCLI:
		return "gemini/orchestrator.md"
	case model.AgentCodex:
		return "codex/orchestrator.md"
	case model.AgentAntigravity:
		return "antigravity/orchestrator.md"
	case model.AgentWindsurf:
		return "windsurf/orchestrator.md"
	case model.AgentCursor:
		return "cursor/orchestrator.md"
	case model.AgentKimi:
		return "kimi/orchestrator.md"
	case model.AgentQwenCode:
		return "qwen/orchestrator.md"
	case model.AgentKiroIDE:
		return "kiro/orchestrator.md"
	case model.AgentHermes:
		return "hermes/orchestrator.md"
	case model.AgentOpenCode, model.AgentKilocode:
		return "opencode/orchestrator.md"
	default:
		return "generic/orchestrator.md"
	}
}

// selectGenericOrchestrator selects instruction text, never an execution model.
// Validate both shipped sections before replacing either so malformed assets
// fail closed rather than silently losing common surrounding content.
func selectGenericOrchestrator(content, capability string) (string, error) {
	selected := "capable"
	if capability == "small" {
		selected = "small"
	}
	type section struct {
		start, end int
		body, name string
	}
	var sections []section
	for _, name := range []string{"capable", "small"} {
		open := "<!-- section:model-" + name + " -->"
		close := "<!-- /section:model-" + name + " -->"
		start, end := strings.Index(content, open), strings.Index(content, close)
		if strings.Count(content, open) != 1 || strings.Count(content, close) != 1 || end < start+len(open) {
			return "", fmt.Errorf("invalid generic orchestrator model-%s section", name)
		}
		sections = append(sections, section{start, end + len(close), content[start+len(open) : end], name})
	}
	if sections[0].end > sections[1].start {
		return "", fmt.Errorf("generic orchestrator model sections overlap or are out of order")
	}
	for i := len(sections) - 1; i >= 0; i-- {
		s := sections[i]
		body := ""
		if s.name == selected {
			body = s.body
		}
		content = content[:s.start] + body + content[s.end:]
	}
	return content, nil
}

// RenderOrchestratorWithSource is RenderOrchestrator with an explicit review
// contract source, which takes precedence over the package-level fallback.
// capability selects the generic instruction variant: "small" selects the
// small-model text and any other value, including empty, selects capable.
func RenderOrchestratorWithSource(agent model.AgentID, source ReviewContractSource, capability string) (string, error) {
	return renderOrchestrator(agent, source, capability, false)
}

// renderOrchestrator is the single render path. annotate keeps the shared
// module fragment markers for the internal core/module construction; the
// public render always strips those exact marker lines.
func renderOrchestrator(agent model.AgentID, source ReviewContractSource, capability string, annotate bool) (string, error) {
	if agent == model.AgentPi {
		return "", fmt.Errorf("render orchestrator for %q: the Pi prompt is owned by Gentle Shell", agent)
	}

	path := orchestratorAsset(agent)
	content, err := assets.Read(path)
	if err != nil {
		return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
	}

	if path == "generic/orchestrator.md" {
		content, err = selectGenericOrchestrator(content, capability)
		if err != nil {
			return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
		}
	}

	rdd := model.SupportsReceiptDrivenDevelopment(agent)
	content, err = expandSharedOrchestratorSections(content, rdd, annotate)
	if err != nil {
		return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
	}

	content, err = replaceOpenCodeConsentV3QuestionRoute(content, agent)
	if err != nil {
		return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
	}

	content, err = replaceKiloNativeDelegationRoute(content, agent)
	if err != nil {
		return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
	}

	// v3.7.0 removed the section for runtimes that do not advertise the
	// review transport instead of handing them a lifecycle they cannot run.
	contract, reviews, err := reviewExecutionContract(agent, source)
	if err != nil {
		return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
	}
	if reviews {
		content = replaceSectionBody(content, reviewExecutionHeading, reviewExecutionNextHeading, contract)
	} else {
		content = removeSection(content, reviewExecutionHeading, reviewExecutionNextHeading)
	}

	if !rdd {
		content, err = stripReceiptDrivenDevelopment(content)
		if err != nil {
			return "", fmt.Errorf("render orchestrator for %q: %w", agent, err)
		}
	}

	content = strings.ReplaceAll(content, runtimeAgentIDPlaceholder, string(agent))
	if strings.Contains(content, "{{GENTLE_AI_") {
		return "", fmt.Errorf("render orchestrator for %q: unresolved template placeholder in %s", agent, path)
	}
	if !annotate && strings.Contains(content, orchestratorFragmentMarkerToken) {
		return "", fmt.Errorf("render orchestrator for %q: malformed module fragment marker survived the render", agent)
	}
	return strings.TrimSpace(content) + "\n", nil
}

// stripReceiptDrivenDevelopment turns a rendered orchestrator into its ODD-only
// form for a runtime without receipt-driven development.
func stripReceiptDrivenDevelopment(content string) (string, error) {
	content = removeHeadingBlock(content, providerDefectHandoffHeading)

	shared, err := assets.Read(sharedOrchestratorSectionsAsset)
	if err != nil {
		return "", err
	}
	checking := sharedOrchestratorSection(shared, nativeCheckingSection+oddOnlySectionSuffix)
	if checking == "" {
		return "", fmt.Errorf("shared orchestrator section %q has no canonical body", nativeCheckingSection+oddOnlySectionSuffix)
	}
	content = replaceHeadingBlockBody(content, nativeCheckingHeading, checking)
	content = nonRDDReplacements.Replace(content)

	for _, marker := range nonRDDLeakMarkers {
		if strings.Contains(content, marker) {
			return "", fmt.Errorf("orchestrator without receipt-driven development still carries review content %q", marker)
		}
	}
	return content, nil
}

// expandSharedOrchestratorSections resolves every shared-section placeholder in
// one pass. A canonical body is literal text, so a placeholder inside one is not
// a nested reference. An unknown section fails rather than shipping a prompt
// with a literal template token in it. Without receipt-driven development, a
// section's "(ODD only)" variant replaces its canonical body when one exists.
// annotate keeps the module fragment markers inside the canonical bodies.
func expandSharedOrchestratorSections(content string, rdd, annotate bool) (string, error) {
	shared, err := assets.Read(sharedOrchestratorSectionsAsset)
	if err != nil {
		return "", err
	}
	var missing []string
	expanded := sharedOrchestratorSectionPlaceholder.ReplaceAllStringFunc(content, func(match string) string {
		name := sharedOrchestratorSectionPlaceholder.FindStringSubmatch(match)[1]
		section := sharedOrchestratorSection
		if annotate {
			section = annotatedSharedOrchestratorSection
		}
		body := ""
		if !rdd {
			body = section(shared, name+oddOnlySectionSuffix)
		}
		if body == "" {
			body = section(shared, name)
		}
		if body == "" {
			missing = append(missing, name)
			return match
		}
		return body
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("shared orchestrator sections %q have no canonical body", missing)
	}
	return expanded, nil
}

// sharedOrchestratorSection returns a canonical body without its module
// fragment marker lines, byte for byte the body before the markers existed.
func sharedOrchestratorSection(shared, name string) string {
	return strings.TrimSpace(stripOrchestratorFragmentMarkers(rawSharedOrchestratorSection(shared, name)))
}

func annotatedSharedOrchestratorSection(shared, name string) string {
	return strings.TrimSpace(rawSharedOrchestratorSection(shared, name))
}

func rawSharedOrchestratorSection(shared, name string) string {
	open := "<!-- odd-orchestrator-section:" + name + ":start -->"
	closing := "<!-- odd-orchestrator-section:" + name + ":end -->"
	start := strings.Index(shared, open)
	end := strings.Index(shared, closing)
	if start < 0 || end < start {
		return ""
	}
	return shared[start+len(open) : end]
}

// sectionBounds returns the byte range of the section that opens at heading and
// ends before the next heading named nextHeading, at any level.
func sectionBounds(content, heading, nextHeading string) (int, int, bool) {
	start := strings.Index(content, heading)
	if start < 0 {
		return 0, 0, false
	}
	end := len(content)
	remainder := content[start+len(heading):]
	for _, candidate := range []string{"\n#### " + nextHeading, "\n### " + nextHeading, "\n## " + nextHeading} {
		if relative := strings.Index(remainder, candidate); relative >= 0 {
			end = start + len(heading) + relative + 1
			break
		}
	}
	return start, end, true
}

// headingBlockBounds returns the byte range of the block that opens at the
// heading line and ends before the next heading of the same or a higher level.
func headingBlockBounds(content, heading string) (int, int, bool) {
	start := lineStartIndex(content, heading+"\n")
	if start < 0 {
		return 0, 0, false
	}
	level := len(heading) - len(strings.TrimLeft(heading, "#"))
	offset := start + len(heading) + 1
	for offset < len(content) {
		lineEnd := strings.IndexByte(content[offset:], '\n')
		line := content[offset:]
		if lineEnd >= 0 {
			line = content[offset : offset+lineEnd]
		}
		if hashes := len(line) - len(strings.TrimLeft(line, "#")); hashes > 0 && hashes <= level && strings.HasPrefix(line[hashes:], " ") {
			return start, offset, true
		}
		if lineEnd < 0 {
			break
		}
		offset += lineEnd + 1
	}
	return start, len(content), true
}

// removeHeadingBlock drops the heading and its body.
func removeHeadingBlock(content, heading string) string {
	start, end, ok := headingBlockBounds(content, heading)
	if !ok {
		return content
	}
	return strings.TrimRight(content[:start], "\n") + "\n\n" + strings.TrimLeft(content[end:], "\n")
}

// replaceHeadingBlockBody keeps the heading and replaces its body.
func replaceHeadingBlockBody(content, heading, body string) string {
	start, end, ok := headingBlockBounds(content, heading)
	if !ok {
		return content
	}
	return content[:start] + heading + "\n\n" + strings.TrimSpace(body) + "\n\n" + strings.TrimLeft(content[end:], "\n")
}

func replaceSectionBody(content, heading, nextHeading, body string) string {
	start, end, ok := sectionBounds(content, heading, nextHeading)
	if !ok {
		return content
	}
	replacement := heading + "\n\n" + strings.TrimSpace(body) + "\n\n"
	return strings.TrimRight(content[:start], "\n") + "\n\n" + replacement + strings.TrimLeft(content[end:], "\n")
}

func removeSection(content, heading, nextHeading string) string {
	start, end, ok := sectionBounds(content, heading, nextHeading)
	if !ok {
		return content
	}
	return strings.TrimRight(content[:start], "\n") + "\n\n" + strings.TrimLeft(content[end:], "\n")
}

const openCodeNativeQuestionSourceRoute = "- Native route: The classified native question UI is `question`. Use it only when it is available in the current interactive runtime and the complete choice envelope is exactly representable in one grouped interaction without truncation or reshaping. When the closed domain of a single-select envelope is representable as the classified native question UI, use it; otherwise fall through to the Fallback clause below."

const openCodeConsentV3QuestionRoute = "- Native route: For `gentle-ai.review-integration.consent/v3`: Display labels and provider-owned answer tokens may differ; that difference alone never makes an otherwise complete closed single-select domain unrepresentable. Before invocation, inspect the active classified `question` schema. Set `multiple: false` when exposed. Set `custom: false` only when exposed; otherwise omit `custom`. Missing `custom` alone is not a compatibility block: a text-capable UI does not expand the allowed-answer domain. Never invent unsupported parameters. Preserve the complete envelope, including headline, reason, value, risk evidence, option order, labels, descriptions, effects, and off-path note in one native question. Invoke `question` once per presentation, then STOP and wait. Before any provider invocation, require exactly one question answer containing exactly one value. Trim whitespace and compare case-insensitively against the offered labels; accept only the unambiguous ordinal aliases explicitly permitted by Answer validation below. A typed value is valid only if it resolves to exactly one offered option; it is not a new choice. Empty, multiple, unknown, arbitrary prose, or ambiguous answers authorize no provider invocation: re-present the complete native question and STOP to wait again. Never use chat text as consent, auto-select, or synthesize a continuation. Map the uniquely validated option to its provider-owned answer token once; retain the exact captured target binding and invoke only its exact provider-owned choice invocation once. If `question` is unavailable or the complete envelope cannot be represented, report that compatibility limitation and STOP without invoking any provider continuation; no chat-token fallback. For envelopes other than `gentle-ai.review-integration.consent/v3`, the classified native question UI is `question`. Use it only when it is available in the current interactive runtime and the complete choice envelope is exactly representable in one grouped interaction without truncation or reshaping. When the closed domain of a single-select envelope is representable as the classified native question UI, use it; otherwise fall through to the Fallback clause below."

const openCodeFallbackSourceClause = "- Fallback: If a native UI is unavailable, denied, the runtime is noninteractive, or the complete envelope is oversized or otherwise unrepresentable because of question-count, option-count, or text-length limits, emit the COMPLETE choice envelope as a plain chat or terminal response. Include the required answer syntax and why the input blocks progress. Then STOP. Do not choose, default, infer, launch dependent work, or continue. Native-tool-only wording elsewhere never disables this fallback."

const openCodeConsentV3FallbackClause = "- Fallback: For envelopes other than `gentle-ai.review-integration.consent/v3`, if a native UI is unavailable, denied, the runtime is noninteractive, or the complete envelope is oversized or otherwise unrepresentable because of question-count, option-count, or text-length limits, emit the COMPLETE choice envelope as a plain chat or terminal response. Include the required answer syntax and why the input blocks progress. Then STOP. Do not choose, default, infer, launch dependent work, or continue. Native-tool-only wording elsewhere never disables this fallback."

// replaceOpenCodeConsentV3QuestionRoute adds the OpenCode-only consent-v3
// native question route (as v3.7.0 rendered it, minus the retired SDD consent
// envelope) without changing the shared asset Kilo also consumes.
func replaceOpenCodeConsentV3QuestionRoute(content string, agent model.AgentID) (string, error) {
	if agent != model.AgentOpenCode {
		return content, nil
	}
	if count := strings.Count(content, openCodeNativeQuestionSourceRoute); count != 1 {
		return "", fmt.Errorf("OpenCode native route source clause count = %d, want 1", count)
	}
	content = strings.Replace(content, openCodeNativeQuestionSourceRoute, openCodeConsentV3QuestionRoute, 1)
	return strings.ReplaceAll(content, openCodeFallbackSourceClause, openCodeConsentV3FallbackClause), nil
}

const openCodeInstalledAgentRoute = "Route read-only mapping to the installed `gentle-ai-explore` agent, implementation or command execution to the installed `gentle-ai-worker` agent, and read-only technical verification to the installed `gentle-ai-verify` agent."

const kiloNativeDelegationRoute = "Use Kilo's native subagent delegation for delegated-direct work. Summarize any needed handoff explicitly."

// replaceKiloNativeDelegationRoute routes Kilo's delegated-direct work to its
// native subagents: Kilo shares the OpenCode asset but never installs the
// gentle-ai-* agents that asset names (#5254).
func replaceKiloNativeDelegationRoute(content string, agent model.AgentID) (string, error) {
	if agent != model.AgentKilocode {
		return content, nil
	}
	if count := strings.Count(content, openCodeInstalledAgentRoute); count != 1 {
		return "", fmt.Errorf("OpenCode installed-agent route clause count = %d, want 1", count)
	}
	return strings.Replace(content, openCodeInstalledAgentRoute, kiloNativeDelegationRoute, 1), nil
}

// injectOrchestratorSection merges the orchestrator block into existing
// guidance, keeping the v3.7.0 layout: the orchestrator sits ahead of the
// routing block it points to, and a legacy SDD-owned block is converted where
// it stands. Content outside managed markers is never touched.
func injectOrchestratorSection(existing, orchestrator string) string {
	existing = migrateLegacyOrchestratorSection(existing)

	open := managedOpenMarker(OrchestratorSectionID)
	if strings.Contains(existing, open) {
		return filemerge.InjectMarkdownSection(existing, OrchestratorSectionID, orchestrator)
	}

	routing := lineStartIndex(existing, managedOpenMarker(RoutingSectionID))
	if routing < 0 {
		return filemerge.InjectMarkdownSection(existing, OrchestratorSectionID, orchestrator)
	}
	block := open + "\n" + strings.TrimRight(orchestrator, "\n") + "\n" + managedCloseMarker(OrchestratorSectionID) + "\n\n"
	return existing[:routing] + block + existing[routing:]
}

// migrateLegacyOrchestratorSection renames the first well-formed v3.7.0
// sdd-orchestrator block to the current marker so the fresh orchestrator is
// injected in its place, then drops every remaining legacy copy.
func migrateLegacyOrchestratorSection(existing string) string {
	legacyOpen := managedOpenMarker(legacyOrchestratorSectionID)
	legacyClose := managedCloseMarker(legacyOrchestratorSectionID)
	if !strings.Contains(existing, legacyOpen) && !strings.Contains(existing, legacyClose) {
		return existing
	}

	if !strings.Contains(existing, managedOpenMarker(OrchestratorSectionID)) {
		openIdx := strings.Index(existing, legacyOpen)
		closeIdx := strings.Index(existing, legacyClose)
		if openIdx >= 0 && closeIdx > openIdx {
			existing = existing[:openIdx] + managedOpenMarker(OrchestratorSectionID) +
				existing[openIdx+len(legacyOpen):closeIdx] + managedCloseMarker(OrchestratorSectionID) +
				existing[closeIdx+len(legacyClose):]
		}
	}
	return filemerge.InjectMarkdownSection(existing, legacyOrchestratorSectionID, "")
}

func managedOpenMarker(sectionID string) string  { return "<!-- gentle-ai:" + sectionID + " -->" }
func managedCloseMarker(sectionID string) string { return "<!-- /gentle-ai:" + sectionID + " -->" }

// lineStartIndex returns the first index of needle that starts a line.
func lineStartIndex(content, needle string) int {
	offset := 0
	for {
		idx := strings.Index(content[offset:], needle)
		if idx < 0 {
			return -1
		}
		abs := offset + idx
		if abs == 0 || content[abs-1] == '\n' {
			return abs
		}
		offset = abs + 1
	}
}
