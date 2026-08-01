package assets

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

type blockingPromptRoute struct {
	nativeTool string
}

var blockingPromptRoutes = map[string]blockingPromptRoute{
	"antigravity/sdd-orchestrator.md": {},
	"claude/sdd-orchestrator.md":      {nativeTool: "`AskUserQuestion`"},
	"codex/sdd-orchestrator.md":       {},
	"cursor/sdd-orchestrator.md":      {},
	"gemini/sdd-orchestrator.md":      {},
	"generic/sdd-orchestrator.md":     {},
	"hermes/sdd-orchestrator.md":      {},
	"kimi/sdd-orchestrator.md":        {},
	"kiro/sdd-orchestrator.md":        {},
	"opencode/sdd-orchestrator.md":    {nativeTool: "`question`"},
	"qwen/sdd-orchestrator.md":        {},
	"windsurf/sdd-orchestrator.md":    {},
}

func TestCoordinatorOrchestratorsCarryLosslessBlockingPromptRule(t *testing.T) {
	allPaths := allSDDOrchestratorAssetPaths(t)
	if len(allPaths) != len(blockingPromptRoutes) {
		t.Fatalf("discovered %d orchestrator variants, but %d have an explicit blocking-prompt route; classify every variant",
			len(allPaths), len(blockingPromptRoutes))
	}

	for _, path := range allPaths {
		route, classified := blockingPromptRoutes[path]
		if !classified {
			t.Fatalf("unclassified orchestrator variant %q; native/fallback routing must fail closed", path)
		}

		t.Run(path, func(t *testing.T) {
			contract := blockingPromptContractSection(t, path)
			for _, required := range []string{
				"complete user-facing choice envelope",
				"why input is required",
				"every group and question in original order",
				"including every group header",
				"every option label and description",
				"exact allowed-answer domain",
				"Never summarize, abbreviate, reorder, relabel, merge, or omit choices",
				"Never silently split an atomic business choice",
				"COMPLETE choice envelope as a plain chat or terminal response",
				"unavailable, denied, the runtime is noninteractive",
				"question-count, option-count, or text-length limits",
				"Then STOP",
				"Do not choose, default, infer, launch dependent work, or continue",
				"Accept an answer only when each response belongs to the exact allowed-answer domain",
				"free text or multi-select only when the original prompt allowed it",
				"invalid or ambiguous",
				"same blocked actor exactly once",
			} {
				if !strings.Contains(contract, required) {
					t.Errorf("lossless blocking-prompt contract missing %q", required)
				}
			}

			if route.nativeTool == "" {
				const fallbackOnly = "This variant has no classified native question UI for this contract; always use the plain chat or terminal fallback below."
				if !strings.Contains(contract, fallbackOnly) {
					t.Errorf("fallback-only variant missing explicit route %q", fallbackOnly)
				}
				return
			}

			for _, required := range []string{
				fmt.Sprintf("The classified native question UI is %s.", route.nativeTool),
				"Use it only when it is available in the current interactive runtime",
				"exactly representable in one grouped interaction",
			} {
				if !strings.Contains(contract, required) {
					t.Errorf("native route missing %q", required)
				}
			}
		})
	}
}

func TestBlockingPromptFallbackCoversWindsurfToolResults(t *testing.T) {
	content := MustRead("windsurf/sdd-orchestrator.md")
	if !strings.Contains(content, "There are no sub-agents") {
		t.Fatal("Windsurf must still identify its solo-agent execution model")
	}
	contract := blockingPromptContractSection(t, "windsurf/sdd-orchestrator.md")
	for _, required := range []string{
		"sub-agent or tool",
		"always use the plain chat or terminal fallback",
		"Then STOP",
	} {
		if !strings.Contains(contract, required) {
			t.Errorf("Windsurf tool-result fallback missing %q", required)
		}
	}
}

func TestNativeBlockingPromptRulesRetainInteractiveUIWithFailClosedFallback(t *testing.T) {
	tests := []struct {
		name string
		path string
		tool string
	}{
		{name: "Claude", path: "claude/sdd-orchestrator.md", tool: "`AskUserQuestion`"},
		{name: "OpenCode", path: "opencode/sdd-orchestrator.md", tool: "`question`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := blockingPromptContractSection(t, tt.path)
			if !strings.Contains(contract, "The classified native question UI is "+tt.tool+".") {
				t.Fatalf("%s does not retain its native interactive question UI", tt.path)
			}
			for _, scenario := range []string{
				"unavailable",
				"denied",
				"the runtime is noninteractive",
				"question-count, option-count, or text-length limits",
			} {
				if !strings.Contains(contract, scenario) {
					t.Errorf("%s has no complete fallback for %s", tt.path, scenario)
				}
			}
		})
	}
}

func TestCoordinatorOrchestratorsCarryGentleAIProviderDefectHandoff(t *testing.T) {
	requirements := []struct {
		name string
		text string
	}{
		{name: "admissibility before lossless relay", text: "Before losslessly relaying any blocking choice envelope, classify its semantic admissibility"},
		{name: "direct repair prohibition", text: "never offer to switch to, inspect, modify, or directly repair the Gentle AI repository"},
		{name: "invalid upstream envelope", text: "reject it as semantically inadmissible and issue this separate orchestrator-owned handoff envelope"},
		{name: "localized consent", text: "Ask the user first, in the active orchestrator conversation language"},
		{name: "exact choice shape", text: "one single-select blocking envelope with exactly two semantic choices"},
		{name: "localized labels", text: "Localize their labels and descriptions without changing these semantics"},
		{name: "no internal labels", text: "do not expose machine or internal codes in user-facing labels"},
		{name: "report choice", text: "**Report the Gentle AI defect**: Only after explicit consent"},
		{name: "fixed repository", text: "`Gentleman-Programming/gentle-ai`"},
		{name: "open and closed duplicate search", text: "search open and closed issues"},
		{name: "comment duplicate", text: "comment on an equivalent issue with the new occurrence and evidence"},
		{name: "create only without duplicate", text: "create a new bug report only if no duplicate exists"},
		{name: "reported path preserves state", text: "Then STOP with all consumer state preserved"},
		{name: "stop choice", text: "**Stop here**: Create no GitHub issue or comment, preserve all consumer state, and STOP"},
		{name: "observed evidence", text: "Report observed evidence, not an unconfirmed root cause"},
		{name: "sanitized diagnostics", text: "sanitized version/build, OS/architecture/client"},
		{name: "bounded evidence", text: "bounded attempts and outcomes, failure envelopes, mutation outcome"},
		{name: "reproduction evidence", text: "expected and actual behavior, a minimal reproduction"},
		{name: "opaque identifiers", text: "safe opaque reason/revision identifiers, and preserved-state evidence"},
		{name: "final privacy scan", text: "perform a final privacy scan"},
		{name: "privacy exclusions", text: "raw argv, absolute paths, private project names, usernames, hostnames, credentials, diffs, source contents, and environment values"},
		{name: "released fix only", text: "Resume only after an installed released fix"},
		{name: "native status re-entry", text: "re-enter through native status"},
		{name: "no source checkout resume", text: "Never resume against a source checkout or unmerged pull request"},
	}

	allPaths := allSDDOrchestratorAssetPaths(t)
	canonical := providerDefectHandoffSection(t, allPaths[0])
	semanticChoicePattern := regexp.MustCompile(`(?m)^[ \t]+[0-9]+\. \*\*[^*]+\*\*:`)
	for _, path := range allPaths {
		t.Run(path, func(t *testing.T) {
			contract := providerDefectHandoffSection(t, path)
			if contract != canonical {
				t.Error("provider-defect handoff differs from the canonical cross-variant block")
			}
			if got := len(semanticChoicePattern.FindAllString(contract, -1)); got != 2 {
				t.Errorf("provider-defect handoff has %d numbered semantic choices; want exactly 2", got)
			}
			for _, requirement := range requirements {
				t.Run(requirement.name, func(t *testing.T) {
					if !strings.Contains(contract, requirement.text) {
						t.Errorf("provider-defect handoff missing %q", requirement.text)
					}
				})
			}
		})
	}
}

func providerDefectHandoffSection(t *testing.T, path string) string {
	t.Helper()
	const heading = "#### Gentle AI Provider Defect Handoff (MANDATORY)"
	content := MustRead(path)
	start := strings.Index(content, heading)
	if start == -1 {
		t.Fatalf("%s missing %q", path, heading)
	}
	contract := content[start:]
	const endMarker = "Never resume against a source checkout or unmerged pull request."
	end := strings.Index(contract, endMarker)
	if end == -1 {
		t.Fatalf("%s provider-defect handoff missing terminal release boundary", path)
	}
	return strings.TrimSpace(contract[:end+len(endMarker)])
}

func blockingPromptContractSection(t *testing.T, path string) string {
	t.Helper()
	const heading = "### Lossless Blocking Prompts (MANDATORY)"
	content := MustRead(path)
	start := strings.Index(content, heading)
	if start == -1 {
		t.Fatalf("%s missing %q", path, heading)
	}
	contract := content[start:]
	if end := strings.Index(contract[len(heading):], "\n##"); end >= 0 {
		contract = contract[:len(heading)+end]
	}
	return contract
}
