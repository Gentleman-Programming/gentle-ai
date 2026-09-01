package sdd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

const (
	// sharedOrchestratorSectionsAsset holds the canonical body of every
	// orchestrator subsection that each runtime states identically. #3817: the
	// contract lives in twelve hand-maintained near-duplicates, and a single
	// edit to one of them is how ten runtimes were left carrying a contradicted
	// dispatcher guard. Sections that genuinely differ per runtime stay in the
	// runtime asset; only the ones measured as non-drifted moved here.
	sharedOrchestratorSectionsAsset = "skills/_shared/sdd-orchestrator-sections.md"
	sharedOrchestratorSectionOpen   = "{{GENTLE_AI_SDD_SECTION:"

	openCodeBackgroundPolicyAsset  = "opencode/background-subagents.md"
	openCodeBackgroundPolicyMarker = "<!-- gentle-ai:opencode-background-subagents -->"
	openCodeBackgroundPolicyEnd    = "<!-- /gentle-ai:opencode-background-subagents -->"
)

// OrchestratorRenderOptions carries already-resolved prompt policy selections.
// The renderer does not resolve intent or runtime capability. Callers MUST set
// IncludeOpenCodeBackgroundPolicy only after a later resolution step has done
// that work; the zero value preserves the historical prompt bytes.
type OrchestratorRenderOptions struct {
	IncludeOpenCodeBackgroundPolicy bool
}

// sharedOrchestratorSection returns the canonical body for one shared section,
// or the empty string when the shared asset does not define it.
func sharedOrchestratorSection(name string) string {
	source := assets.MustRead(sharedOrchestratorSectionsAsset)
	start := strings.Index(source, "<!-- sdd-orchestrator-section:"+name+":start -->")
	end := strings.Index(source, "<!-- sdd-orchestrator-section:"+name+":end -->")
	if start < 0 || end < start {
		return ""
	}
	start += len("<!-- sdd-orchestrator-section:" + name + ":start -->")
	return strings.TrimSpace(source[start:end])
}

// sharedOrchestratorSectionPlaceholder matches one {{GENTLE_AI_SDD_SECTION:<name>}}.
var sharedOrchestratorSectionPlaceholder = regexp.MustCompile(`\{\{GENTLE_AI_SDD_SECTION:([^}]*)\}\}`)

// substituteSharedOrchestratorSections resolves every shared-section
// placeholder in one pass. An unresolvable placeholder panics rather than
// shipping a prompt with a literal template token in it, which is the failure
// mode the rendered goldens would otherwise hide.
//
// One pass, deliberately: a canonical body is literal text, so a placeholder
// appearing inside one is not a nested reference to expand. The earlier
// implementation rescanned from the start after each substitution, which would
// not terminate if a body ever contained a placeholder.
func substituteSharedOrchestratorSections(content string) string {
	return sharedOrchestratorSectionPlaceholder.ReplaceAllStringFunc(content, func(match string) string {
		name := sharedOrchestratorSectionPlaceholder.FindStringSubmatch(match)[1]
		body := sharedOrchestratorSection(name)
		if body == "" {
			panic(fmt.Sprintf("sdd: shared orchestrator section %q has no canonical body", name))
		}
		return body
	})
}

// composeOrchestratorPrompt is the renderer-owned source seam for every SDD
// orchestrator. It composes the selected historical asset before the existing
// bounded-review and runtime-identity substitutions.
func composeOrchestratorPrompt(agent model.AgentID, options ...OrchestratorRenderOptions) string {
	path := sddOrchestratorAsset(agent)
	content := substituteSharedOrchestratorSections(assets.MustRead(path))
	var renderOptions OrchestratorRenderOptions
	if len(options) > 0 {
		renderOptions = options[0]
	}
	if policy := renderOpenCodeBackgroundPolicy(agent, renderOptions); policy != "" {
		content = appendOpenCodeBackgroundPolicy(content, policy)
	}
	content = replacePiClosedSingleSelectRoute(content, agent)
	content = renderBoundedReviewAssetBodyFromContent(agent, path, content)
	return bindRuntimeAgentIdentity(content, agent)
}

const genericFallbackOnlyNativeRoute = "- Native route: This variant has no classified native question UI for this contract; always use the plain chat or terminal fallback below. When the closed domain of a single-select envelope is unrepresentable here, fall through to the Fallback clause below."

const piClosedSingleSelectNativeRoute = "- Native route: For every strictly closed single-select envelope, use ask_user_choice only when the interactive Pi TUI can represent its complete one-question 2-4 ordered-option domain. Pass each user-facing label and description with the envelope-owned canonical option token as value. The selector returns exactly one value; map it to the exact envelope-owned choice once, then select any envelope-owned continuation or invocation once where present. It has no custom/free-text or multi-select path. If the native TUI is unavailable or the envelope is not exactly representable, use the complete chat fallback. ask_user_question is the external open/free-text questionnaire and must not be used for a closed domain; open/free-text questionnaires may use ask_user_question when exactly representable. For gentle-ai.review-integration.consent/v3, the chosen continuation is still the exact captured provider-owned choice invocation, used once without synthesis."

func replacePiClosedSingleSelectRoute(content string, agent model.AgentID) string {
	if agent != model.AgentPi {
		return content
	}
	if count := strings.Count(content, genericFallbackOnlyNativeRoute); count != 1 {
		panic(fmt.Sprintf("sdd: Pi native route source clause count = %d, want 1", count))
	}
	return strings.Replace(content, genericFallbackOnlyNativeRoute, piClosedSingleSelectNativeRoute, 1)
}

func renderOpenCodeBackgroundPolicy(agent model.AgentID, options ...OrchestratorRenderOptions) string {
	var renderOptions OrchestratorRenderOptions
	if len(options) > 0 {
		renderOptions = options[0]
	}
	if agent != model.AgentOpenCode || !renderOptions.IncludeOpenCodeBackgroundPolicy {
		return ""
	}
	return mustReadOpenCodeBackgroundPolicy()
}

func mustReadOpenCodeBackgroundPolicy() string {
	content := assets.MustRead(openCodeBackgroundPolicyAsset)
	if err := validateOpenCodeBackgroundPolicy(content, true); err != nil {
		panic(err.Error())
	}
	return content
}

func appendOpenCodeBackgroundPolicy(content, policy string) string {
	markerCount := strings.Count(content, openCodeBackgroundPolicyMarker)
	endCount := strings.Count(content, openCodeBackgroundPolicyEnd)
	if markerCount != 0 || endCount != 0 {
		if err := validateOpenCodeBackgroundPolicy(content, false); err != nil {
			panic(err.Error())
		}
		return content
	}

	if err := validateOpenCodeBackgroundPolicy(policy, true); err != nil {
		panic(err.Error())
	}
	return strings.TrimRight(content, "\n") + "\n\n" + policy + "\n"
}

func validateOpenCodeBackgroundPolicy(content string, standalone bool) error {
	trimmed := strings.TrimSpace(content)
	start := strings.Index(trimmed, openCodeBackgroundPolicyMarker)
	end := strings.Index(trimmed, openCodeBackgroundPolicyEnd)
	if strings.Count(trimmed, openCodeBackgroundPolicyMarker) != 1 ||
		strings.Count(trimmed, openCodeBackgroundPolicyEnd) != 1 || start < 0 || end <= start ||
		(start+len(openCodeBackgroundPolicyMarker) < len(trimmed) && trimmed[start+len(openCodeBackgroundPolicyMarker)] != '\n') ||
		(end > 0 && trimmed[end-1] != '\n') ||
		(start > 0 && trimmed[start-1] != '\n') ||
		(end+len(openCodeBackgroundPolicyEnd) < len(trimmed) && trimmed[end+len(openCodeBackgroundPolicyEnd)] != '\n') {
		return fmt.Errorf("sdd: inconsistently marked OpenCode background policy")
	}
	if standalone && (start != 0 || end+len(openCodeBackgroundPolicyEnd) != len(trimmed)) {
		return fmt.Errorf("assets: OpenCode background policy must contain only its marked section")
	}
	if strings.TrimSpace(trimmed[start+len(openCodeBackgroundPolicyMarker):end]) == "" {
		return fmt.Errorf("sdd: empty OpenCode background policy")
	}
	return nil
}

// sddOrchestratorAsset returns the embedded asset path for the SDD orchestrator
// content based on the agent. Agent-specific assets take priority; generic is fallback.
func sddOrchestratorAsset(agent model.AgentID) string {
	switch agent {
	case model.AgentClaudeCode:
		return "claude/sdd-orchestrator.md"
	case model.AgentGeminiCLI:
		return "gemini/sdd-orchestrator.md"
	case model.AgentCodex:
		return "codex/sdd-orchestrator.md"
	case model.AgentAntigravity:
		return "antigravity/sdd-orchestrator.md"
	case model.AgentWindsurf:
		return "windsurf/sdd-orchestrator.md"
	case model.AgentCursor:
		return "cursor/sdd-orchestrator.md"
	case model.AgentKimi:
		return "kimi/sdd-orchestrator.md"
	case model.AgentQwenCode:
		return "qwen/sdd-orchestrator.md"
	case model.AgentKiroIDE:
		return "kiro/sdd-orchestrator.md"
	case model.AgentHermes:
		return "hermes/sdd-orchestrator.md"
	case model.AgentOpenCode, model.AgentKilocode:
		return "opencode/sdd-orchestrator.md"
	default:
		return "generic/sdd-orchestrator.md"
	}
}
