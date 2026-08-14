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

type providerDefectFixRoute string

const (
	providerDefectRecommendPublishedFix providerDefectFixRoute = "recommend_published_fix"
	providerDefectAddOccurrenceComment  providerDefectFixRoute = "add_occurrence_comment"
	providerDefectPossibleRegression    providerDefectFixRoute = "possible_regression"
)

// providerDefectEvidenceChannel models the prompt contract only. It deliberately
// has no production caller: this PR defines evidence relevance, not installation
// or updater-channel behavior.
func providerDefectEvidenceChannel(installedBuild string) string {
	if strings.Contains(installedBuild, "-rc.") || strings.Contains(installedBuild, "-main.") {
		return "prerelease"
	}
	return "stable"
}

func providerDefectRouteForPublishedFix(installedBuild, fixChannel string, published, predatesFix, containsFixAndReproduces bool) providerDefectFixRoute {
	if !published || providerDefectEvidenceChannel(installedBuild) != fixChannel {
		return providerDefectAddOccurrenceComment
	}
	if predatesFix {
		return providerDefectRecommendPublishedFix
	}
	if containsFixAndReproduces {
		return providerDefectPossibleRegression
	}
	return providerDefectAddOccurrenceComment
}

// generic is the provider-neutral source that every agent-specific handoff must project.
const providerDefectHandoffCanonicalPath = "generic/sdd-orchestrator.md"

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
				"request for information, not a candidate answer",
				"answer it directly from the envelope already held",
				"re-present the complete choice envelope and keep waiting",
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
		{name: "explicit consent", text: "for explicit consent to report the apparent defect"},
		{name: "exact choice shape", text: "one single-select blocking envelope with exactly three semantic choices in this order"},
		{name: "exact answer tokens", text: "`report_and_continue`, `continue_without_reporting`, `stop_here`"},
		{name: "localized labels", text: "Localize their labels and descriptions without changing these semantics"},
		{name: "no internal labels", text: "do not expose machine or internal codes in user-facing labels"},
		{name: "report and continue choice", text: "**Report the Gentle AI defect and continue**: Only after explicit consent and that final privacy scan"},
		{name: "continue without reporting choice", text: "**Continue without reporting**: Perform no GitHub search, write, comment, or label, and no report-side privacy scan is required."},
		{name: "fixed repository", text: "`Gentleman-Programming/gentle-ai`"},
		{name: "definitive equivalent lookup", text: "complete a definitive lookup across open and closed issues for an equivalent defect or canonical tracker"},
		{name: "equivalent definition", text: "same observable defect and affected contract, backed by concrete evidence rather than title similarity alone"},
		{name: "canonical definition", text: "owns the causal class"},
		{name: "published fix ordering", text: "First establish that the equivalent has an identified fix verifiably contained by a published release. Then determine the installed build and derive its evidence channel only from its build string: the contract's recognized prerelease tags are `-rc.` and `-main.`; every other build is stable. That release is a relevant published fix only when it is in the installed build's evidence channel."},
		{name: "regression definition", text: "reproduction on a build proven to contain that fix"},
		{name: "definitive definition", text: "completed open+closed lookup with a classifiable result; incomplete, error, or unknown is not definitive"},
		{name: "published evidence exclusion", text: "A main-only commit, local/source build, unmerged PR, or unsupported assertion is not published-fix evidence, including for prerelease or main builds."},
		{name: "unfixed equivalent occurrence", text: "If the equivalent has no verifiable relevant published fix, add exactly one occurrence comment"},
		{name: "other-channel occurrence", text: "A fix published only to the other evidence channel is not a relevant published fix for this occurrence: add exactly one occurrence comment"},
		{name: "other-channel location note", text: "note where the fix is published"},
		{name: "channel choice ownership", text: "Do not recommend switching channels; channel choice is the user's."},
		{name: "outdated installed build", text: "If the installed build predates that release, recommend installing the published fix and reproducing; do not create or comment for that occurrence yet."},
		{name: "regression routing", text: "If the installed build demonstrably contains the fix and still reproduces, treat it as a possible regression: reproduction on a build proven to contain that fix; comment on a suitable canonical tracker, or create a linked regression issue when that tracker is unsuitable. Never reopen automatically."},
		{name: "create new automated report", text: "create a new automated provider-defect report"},
		{name: "confirmed creation identity", text: "Confirmed creation requires the GitHub create operation to confirm a newly-created issue identity/URL."},
		{name: "no output-only creation inference", text: "Never infer creation from output text alone."},
		{name: "definitive lookup gate", text: "Only a definitive lookup may branch to GitHub mutation."},
		{name: "uncertainty no further mutation", text: "perform no further GitHub mutation and no blind retry"},
		{name: "uncertainty continuation", text: "execute the exact captured provider-owned decline invocation exactly once, validate it, re-enter native negotiated STATUS, and resume the already-held consumer continuation."},
		{name: "comment exact canonical", text: "on that exact canonical/equivalent issue"},
		{name: "unconfirmed creation continuation", text: "If creation fails, is ambiguous, incomplete, times out, lacks permission, or has an unknown outcome, preserve all consumer state; do not search, comment, update, or retry creation until the exact created issue identity is resolved, then use the uncertainty continuation below."},
		{name: "report outcome continuation", text: "After a definitive successful report outcome, or any report-side uncertainty after stopping further GitHub mutation, execute the shared candidate-scoped continuation below."},
		{name: "captured provider decline", text: "exact captured provider-owned `choices[answer=\"declined\"].invocation` from the `gentle-ai.review-integration.consent/v3` envelope"},
		{name: "captured decline only", text: "Never synthesize the decline command, target, token, or consumer continuation from prose."},
		{name: "missing decline context fails closed", text: "If the captured exact v3 decline invocation, exact target identity, or consumer continuation context is unavailable or ambiguous, fail closed with all consumer state preserved and do not run a substitute command."},
		{name: "decline result validation", text: "validate `action: \"declined\"`, `consent: \"declined_this_candidate\"`, and the exact target identity match"},
		{name: "decline preserves ordinary delivery", text: "The result carries no lineage or receipt; ordinary delivery is unmanaged by the candidate choice, and the next candidate asks again."},
		{name: "native negotiated status re-entry", text: "re-enter through native negotiated STATUS, then resume the already-held consumer continuation"},
		{name: "continue paths reuse exact decline", text: "Both continue choices execute that exact captured decline invocation exactly once"},
		{name: "review mode disable prohibition", text: "Do not invoke `gentle-ai review mode disable` at clone or global scope within this handoff."},
		{name: "rdd mode preservation", text: "Do not turn RDD off or on within this handoff."},
		{name: "stop choice", text: "**Stop here**: Perform no GitHub operation and no decline invocation; preserve all consumer state and STOP."},
		{name: "observed evidence", text: "Report observed evidence, not an unconfirmed root cause"},
		{name: "sanitized diagnostics", text: "sanitized version/build, OS/architecture/client"},
		{name: "bounded evidence", text: "bounded attempts and outcomes, failure envelopes, mutation outcome"},
		{name: "reproduction evidence", text: "expected and actual behavior, a minimal reproduction"},
		{name: "opaque identifiers", text: "safe opaque reason/revision identifiers, and preserved-state evidence"},
		{name: "final privacy scan", text: "Immediately before the first GitHub operation, perform a final privacy scan"},
		{name: "privacy ordering", text: "This scan precedes the definitive lookup, report creation, and occurrence comment"},
		{name: "privacy exclusions", text: "raw argv, absolute paths, private project names, usernames, hostnames, credentials, diffs, source contents, and environment values"},
		{name: "published fix remains a normal resumption route", text: "an installed published fix"},
		{name: "installed prerelease qualifies", text: "A published prerelease or release candidate the user installed satisfies this."},
		{name: "maintainer-authorized native recovery", text: "an explicit maintainer-authorized, documented native recovery or reset that the runtime contract supports"},
		{name: "no unpublished code resume", text: "Never resume against unpublished code: a source checkout, a local build, or an unmerged pull request"},
	}
	allPaths := allSDDOrchestratorAssetPaths(t)
	canonicalFound := false
	for _, path := range allPaths {
		if path == providerDefectHandoffCanonicalPath {
			canonicalFound = true
			break
		}
	}
	if !canonicalFound {
		t.Fatalf("canonical provider-defect handoff source %q is not an orchestrator asset", providerDefectHandoffCanonicalPath)
	}
	canonical := providerDefectHandoffSection(t, providerDefectHandoffCanonicalPath)
	semanticChoicePattern := regexp.MustCompile(`(?m)^[ \t]+[0-9]+\. \*\*[^*]+\*\*:`)
	for _, path := range allPaths {
		t.Run(path, func(t *testing.T) {
			contract := providerDefectHandoffSection(t, path)
			if contract != canonical {
				t.Error("provider-defect handoff differs from the canonical cross-variant block")
			}
			choiceMatches := semanticChoicePattern.FindAllString(contract, -1)
			if got := len(choiceMatches); got != 3 {
				t.Errorf("provider-defect handoff has %d numbered semantic choices; want exactly 3", got)
			}
			for index, choice := range []string{
				"  1. **Report the Gentle AI defect and continue**:",
				"  2. **Continue without reporting**:",
				"  3. **Stop here**:",
			} {
				if index >= len(choiceMatches) {
					t.Errorf("provider-defect handoff is missing semantic choice %d: %q", index+1, choice)
					continue
				}
				if choiceMatches[index] != choice {
					t.Errorf("provider-defect handoff semantic choice %d = %q, want %q", index+1, choiceMatches[index], choice)
				}
			}
			lastTokenIndex := -1
			for _, token := range []string{"report_and_continue", "continue_without_reporting", "stop_here"} {
				tokenIndex := strings.Index(contract, "`"+token+"`")
				if tokenIndex < 0 || tokenIndex <= lastTokenIndex {
					t.Errorf("provider-defect handoff answer token %q is missing or out of order", token)
				}
				if count := strings.Count(contract, "`"+token+"`"); count != 1 {
					t.Errorf("provider-defect handoff answer token %q appears %d times; want exactly once", token, count)
				}
				lastTokenIndex = tokenIndex
			}
			for _, requirement := range requirements {
				t.Run(requirement.name, func(t *testing.T) {
					if !strings.Contains(contract, requirement.text) {
						t.Errorf("provider-defect handoff missing %q", requirement.text)
					}
				})
			}
			for _, prohibited := range []string{"Resume only after an installed published fix", "latest version", "gentle-" + "report"} {
				if strings.Contains(contract, prohibited) {
					t.Errorf("provider-defect handoff must not make published fixes the only resumption route: %q", prohibited)
				}
			}
			if count := strings.Count(contract, "gentle-ai review mode disable"); count != 1 {
				t.Errorf("provider-defect handoff mentions review mode disable %d times; want exactly its prohibition", count)
			}
			for _, invariant := range []struct {
				prefix string
				must   []string
			}{
				{prefix: "Confirmed creation requires the GitHub create operation", must: []string{"newly-created issue identity/URL", "Never infer creation from output text alone", "preserve all consumer state", "do not search, comment, update, or retry creation", "use the uncertainty continuation below"}},
				{prefix: "If search, comment, or creation fails, is ambiguous, incomplete, times out, lacks permission, or has an unknown outcome", must: []string{"perform no further GitHub mutation", "no blind retry", "preserve all consumer state", "exact captured provider-owned decline invocation exactly once", "validate it", "re-enter native negotiated STATUS", "already-held consumer continuation"}},
			} {
				line := providerDefectHandoffLine(t, contract, invariant.prefix)
				for _, required := range invariant.must {
					if !strings.Contains(line, required) {
						t.Errorf("provider-defect handoff invariant %q omits %q from %q", invariant.prefix, required, line)
					}
				}
			}
			for _, pair := range [][2]string{
				{"for explicit consent to report the apparent defect", "Immediately before the first GitHub operation, perform a final privacy scan"},
				{"Immediately before the first GitHub operation, perform a final privacy scan", "complete a definitive lookup across open and closed issues"},
				{"complete a definitive lookup across open and closed issues", "First establish that the equivalent has an identified fix verifiably contained by a published release"},
				{"First establish that the equivalent has an identified fix verifiably contained by a published release", "Then determine the installed build"},
				{"Then determine the installed build", "That release is a relevant published fix only when it is in the installed build's evidence channel"},
				{"Only a definitive lookup may branch to GitHub mutation", "create a new automated provider-defect report"},
				{"Only a definitive lookup may branch to GitHub mutation", "add exactly one occurrence comment"},
				{"create a new automated provider-defect report", "Confirmed creation requires the GitHub create operation"},
			} {
				if strings.Index(contract, pair[0]) >= strings.Index(contract, pair[1]) {
					t.Errorf("provider-defect handoff must place %q before %q", pair[0], pair[1])
				}
			}
		})
	}
}

func TestProviderDefectPublishedFixEvidenceChannelRouting(t *testing.T) {
	cases := []struct {
		name                     string
		installedBuild           string
		fixChannel               string
		published                bool
		predatesFix              bool
		containsFixAndReproduces bool
		want                     providerDefectFixRoute
	}{
		{name: "stable build receives newer stable fix", installedBuild: "2.4.0", fixChannel: "stable", published: true, predatesFix: true, want: providerDefectRecommendPublishedFix},
		{name: "stable build does not switch to RC-only fix", installedBuild: "2.4.0", fixChannel: "prerelease", published: true, predatesFix: true, want: providerDefectAddOccurrenceComment},
		{name: "prerelease build receives newer prerelease fix", installedBuild: "2.5.0-rc.1", fixChannel: "prerelease", published: true, predatesFix: true, want: providerDefectRecommendPublishedFix},
		{name: "main-only fix remains unpublished", installedBuild: "2.5.0-main.1", fixChannel: "prerelease", published: false, predatesFix: true, want: providerDefectAddOccurrenceComment},
		{name: "matching published fix that still reproduces is a possible regression", installedBuild: "2.5.0-rc.2", fixChannel: "prerelease", published: true, containsFixAndReproduces: true, want: providerDefectPossibleRegression},
	}

	contract := providerDefectHandoffSection(t, providerDefectHandoffCanonicalPath)
	for _, required := range []string{
		"First establish that the equivalent has an identified fix verifiably contained by a published release. Then determine the installed build and derive its evidence channel only from its build string: the contract's recognized prerelease tags are `-rc.` and `-main.`; every other build is stable. That release is a relevant published fix only when it is in the installed build's evidence channel.",
		"A fix published only to the other evidence channel is not a relevant published fix for this occurrence: add exactly one occurrence comment",
		"note where the fix is published",
		"Do not recommend switching channels; channel choice is the user's.",
		"A main-only commit, local/source build, unmerged PR, or unsupported assertion is not published-fix evidence, including for prerelease or main builds.",
		"If the installed build predates that release, recommend installing the published fix and reproducing",
	} {
		if !strings.Contains(contract, required) {
			t.Fatalf("canonical provider-defect handoff missing channel-routing rule %q", required)
		}
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := providerDefectRouteForPublishedFix(tt.installedBuild, tt.fixChannel, tt.published, tt.predatesFix, tt.containsFixAndReproduces); got != tt.want {
				t.Fatalf("route = %q, want %q", got, tt.want)
			}
		})
	}
}

func providerDefectHandoffLine(t *testing.T, contract, prefix string) string {
	t.Helper()
	start := strings.Index(contract, prefix)
	if start < 0 {
		t.Fatalf("provider-defect handoff missing invariant line starting %q", prefix)
	}
	end := strings.Index(contract[start:], "\n")
	if end < 0 {
		return contract[start:]
	}
	return contract[start : start+end]
}

// TestCoordinatorOrchestratorsCarrySDDEditAuthorityConsentRelay is #2570's
// (S6 of #2540) guard: every orchestrator variant teaches the lossless relay
// of the typed SDD edit-authority consent envelope that native status emits
// on blocked(edit_authority_missing) (#2563), byte-identical across variants
// like the provider-defect handoff above.
func TestCoordinatorOrchestratorsCarrySDDEditAuthorityConsentRelay(t *testing.T) {
	requirements := []string{
		"When native SDD status reports `blocked(edit_authority_missing)`",
		"typed `gentle-ai.sdd-integration.consent/v1` envelope",
		"optional `consent` block",
		"Treat that envelope as a Lossless Blocking Prompt under this contract",
		"same discipline as the review consent relay",
		"Present the complete envelope once in the active conversation language",
		"faithfully translate the headline, reason, `value`, the missing-root evidence, choice labels, every choice `effect`, and the off-path note",
		"preserving the original choices, order, selection mode, exact allowed-answer domain, and answer tokens",
		"Never translate or alter the machine answer tokens (`granted`, `declined`), commands, paths, or invocations",
		"Never summarize, reshape, reorder, merge, or omit any part",
		"never answer on the human's behalf and never run the grant unprompted",
		"Only after the human's explicit `granted` answer",
		"execute the envelope's exact grant invocation verbatim, exactly once",
		"then re-enter through native status",
		"granted roots project into `allowedEditRoots`",
		"per-change, audited, and dies with archive",
		"run the envelope's decline invocation",
		"nothing is persisted",
		"names both exits",
		"edit tasks.md so every work unit stays inside the authorized edit roots, or grant this change edit authority",
		"A blocked status without a `consent` block names the same two exits; relay them and stop.",
	}

	for _, path := range allSDDOrchestratorAssetPaths(t) {
		t.Run(path, func(t *testing.T) {
			contract := sddConsentRelaySection(t, path)
			if canonical := sddConsentRelaySection(t, providerDefectHandoffCanonicalPath); contract != canonical {
				t.Error("SDD edit-authority consent relay differs from the canonical cross-variant block")
			}
			for _, required := range requirements {
				if !strings.Contains(contract, required) {
					t.Errorf("SDD edit-authority consent relay missing %q", required)
				}
			}
		})
	}
}

func sddConsentRelaySection(t *testing.T, path string) string {
	t.Helper()
	const heading = "#### SDD Edit-Authority Consent Relay (MANDATORY)"
	content := MustRead(path)
	start := strings.Index(content, heading)
	if start == -1 {
		t.Fatalf("%s missing %q", path, heading)
	}
	contract := content[start:]
	const endMarker = "A blocked status without a `consent` block names the same two exits; relay them and stop."
	end := strings.Index(contract, endMarker)
	if end == -1 {
		t.Fatalf("%s SDD edit-authority consent relay missing terminal boundary", path)
	}
	return strings.TrimSpace(contract[:end+len(endMarker)])
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
	const endMarker = "Never resume against unpublished code: a source checkout, a local build, or an unmerged pull request."
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
