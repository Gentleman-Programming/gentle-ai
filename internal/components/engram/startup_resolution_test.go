package engram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
)

// Startup project-resolution contract (#1903). These are structural checks on
// rendered and injected protocol text: they prove what every surface tells the
// model before its initial project-scoped memory read, not that a model
// executes the calls in that order. The branches mirror the observed Engram
// 2.2.1 `mem_current_project` shapes: unique (`git_remote`/`dir_basename`),
// ambiguous (`project_source: ambiguous`, empty `project`, non-empty
// `available_projects` holding workspace alternatives such as "a" while the
// repository itself resolves to "repo-a"), and a schema without a `cwd` argument.
var fullStartupResolutionContract = []deliveryGuaranteeInvariant{
	{"resolve-before-initial-reads", "before any initial `mem_context`, `mem_search`, or `mem_review` call, call `mem_current_project` and wait for its result (never in parallel)"},
	{"cwd-only-when-schema-supports", "as `cwd` (or `directory`) only when the tool schema exposes that parameter; otherwise call it with no arguments"},
	{"first-match-stops", "apply the first matching branch below, in this order, and stop there"},
	{"workspace-check-terminal", "if the runtime workspace directory is unknown, the returned `cwd` is missing or does not match it, or the tool is unavailable or fails, stop: skip initial project-scoped reads, trust neither `project` nor `available_projects` from that result"},
	{"ambiguity-before-placeholder", "if `available_projects` is non-empty or `project_source` is `ambiguous`, whether `project` is empty or non-empty, never guess and do not read memory yet"},
	{"ambiguous-exact-choice-not-key", "ask the user to choose exactly one value from `available_projects`; that value is a workspace alternative, not a project key, so never pass it as `project`"},
	{"ambiguous-reresolve-or-unresolved", "accept only a unique result whose `cwd` matches it. otherwise the project stays unresolved: skip initial project-scoped reads"},
	{"unique-only-after-verified", "only after the workspace check passed and no ambiguity was found, a non-empty `project`"},
	{"unique-exact-key", "pass that exact returned value as `project`; never rewrite or guess it"},
	{"no-project-skips", "if `project` is empty and there are no `available_projects`, skip initial project-scoped reads"},
	{"no-invented-or-broad-fallback", "never invent a project name and never fall back to an omitted `project` or `all_projects` search"},
	{"explicit-cross-project-recall-only", "**cross-project recall exception (independent of the step 1 branches and their outcome)**: search outside the resolved project, or when none resolved, only when the user explicitly asks to recall memory across projects or from a named other project; that requested scope never becomes the current project, and asking to work on another project is not such a request"},
	{"searches-use-resolved-project", "every search below uses the project resolved at session start"},
}

var slimStartupResolutionContract = []deliveryGuaranteeInvariant{
	{"resolve-before-initial-reads", "before the first `mem_context`, `mem_search`, or `mem_review`, call `mem_current_project` and wait for it"},
	{"cwd-only-when-schema-supports", "pass the runtime workspace as `cwd` only if the tool schema accepts it"},
	{"workspace-guard-terminal", "apply the first matching rule and stop: (1) runtime workspace unknown, returned `cwd` missing or different, or the call fails: skip those initial reads"},
	{"ambiguity-exact-choice-not-key", "(2) `available_projects` non-empty or `project_source` `ambiguous`: ask the user to choose exactly one value, never pass it as `project`"},
	{"ambiguous-reresolve-or-skip", "re-resolve only via `cwd` with that repository's directory, else skip those reads"},
	{"empty-project-skips", "(3) empty `project`: skip those reads"},
	{"unique-after-guards", "(4) otherwise pass the exact returned `project` as `project`"},
	{"no-invented-or-broad-fallback", "never invent a name or fall back to all projects unless the user explicitly asks to recall across projects or from a named other project"},
}

func missingStartupResolution(content string, contract []deliveryGuaranteeInvariant) []string {
	normalized := normalizeForSemanticMatch(content)
	missing := make([]string, 0)
	for _, inv := range contract {
		if !strings.Contains(normalized, normalizeForSemanticMatch(inv.phrase)) {
			missing = append(missing, inv.name)
		}
	}
	return missing
}

// assertInOrder fails unless every phrase occurs in content, in the given order.
func assertInOrder(t *testing.T, surface, content string, phrases ...string) {
	t.Helper()
	normalized := normalizeForSemanticMatch(content)
	last := -1
	for _, phrase := range phrases {
		idx := strings.Index(normalized, normalizeForSemanticMatch(phrase))
		if idx <= last {
			t.Errorf("surface %q: %q missing or out of order (index %d, previous %d)", surface, phrase, idx, last)
			return
		}
		last = idx
	}
}

func injectedPrompt(t *testing.T, path string, inject func(home string) error) string {
	t.Helper()
	home := t.TempDir()
	if err := inject(home); err != nil {
		t.Fatalf("inject error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, path))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func TestStartupProjectResolution_RenderedAndInjectedSurfaces(t *testing.T) {
	validCodexRuntime(t)
	injectClaude := func(version string) func(string) error {
		return func(home string) error {
			_, err := InjectWithOptions(home, claudeAdapter(), InjectOptions{Version: version})
			return err
		}
	}
	injectDefault := func(newAdapter func() agents.Adapter) func(string) error {
		return func(home string) error { _, err := Inject(home, newAdapter()); return err }
	}

	surfaces := []struct {
		name     string
		content  string
		contract []deliveryGuaranteeInvariant
	}{
		{"full", protocolFull(), fullStartupResolutionContract},
		{"slim", protocolSlim(), slimStartupResolutionContract},
		{"codex-instructions", codexInstructions(), fullStartupResolutionContract},
		{"injected claude slim", injectedPrompt(t, ".claude/CLAUDE.md", injectClaude("1.18.0")), slimStartupResolutionContract},
		{"injected claude below floor", injectedPrompt(t, ".claude/CLAUDE.md", injectClaude("")), fullStartupResolutionContract},
		{"injected opencode", injectedPrompt(t, ".config/opencode/AGENTS.md", injectDefault(opencodeAdapter)), fullStartupResolutionContract},
		{"injected codex", injectedPrompt(t, ".codex/engram-instructions.md", injectDefault(codexAdapter)), fullStartupResolutionContract},
	}

	for _, s := range surfaces {
		for _, missing := range missingStartupResolution(s.content, s.contract) {
			t.Errorf("surface %q violates startup project-resolution invariant %q", s.name, missing)
		}
	}
}

func TestStartupProjectResolution_OrderingAndSessionAuthority(t *testing.T) {
	full := protocolFull()
	assertInOrder(t, "full", full,
		"1. **Resolve Project Before Initial Memory Access**",
		"**Workspace check (terminal)**", "**Ambiguous**", "**Unique**", "**No project**",
		"2. **Consume Runtime Session Identity**",
		"3. **Persist State**", "**Cross-project recall exception (independent of the step 1 branches and their outcome)**",
		"### WHEN TO SEARCH MEMORY",
		"Every search below uses the project resolved at session start",
	)
	assertInOrder(t, "slim", protocolSlim(),
		"call `mem_current_project` and wait for it",
		"(1) runtime workspace unknown",
		"(2) `available_projects` non-empty",
		"(3) empty `project`",
		"(4) otherwise pass the exact returned `project`",
	)

	normalized := normalizeForSemanticMatch(full)
	start := strings.Index(normalized, normalizeForSemanticMatch("1. **Resolve Project Before Initial Memory Access**"))
	end := strings.Index(normalized, normalizeForSemanticMatch("2. **Consume Runtime Session Identity**"))
	if start < 0 || end < start {
		t.Fatal("full protocol missing bounded startup resolution step")
	}
	if strings.Contains(normalized[start:end], "mem_session_start") || strings.Contains(normalized[start:end], "session_id") {
		t.Error("startup project resolution must not touch session registration or identity")
	}
	if !strings.Contains(normalized, "do not call `mem_session_start`") {
		t.Error("full protocol lost the top-level runtime session authority rule")
	}
}

func TestStartupProjectResolution_DetectorRejectsWeakenedBranches(t *testing.T) {
	tests := []struct {
		name, content, positive, weakened string
		contract                          []deliveryGuaranteeInvariant
	}{
		{"full ambiguous guess", protocolFull(), "ask the user to choose exactly one value from `available_projects`; that value is a workspace alternative, not a project key, so never pass it as `project`", "pick the most likely value from `available_projects` and pass it as `project`", fullStartupResolutionContract},
		{"full broad fallback", protocolFull(), "never fall back to an omitted `project` or `all_projects` search", "fall back to an `all_projects` search", fullStartupResolutionContract},
		{"full workspace mismatch accepted", protocolFull(), "stop: skip initial project-scoped reads, trust neither `project` nor `available_projects` from that result", "continue with the returned `project`", fullStartupResolutionContract},
		{"full work request as recall consent", protocolFull(), "asks to recall memory across projects or from a named other project", "asks for another project", fullStartupResolutionContract},
		{"slim ambiguous guess", protocolSlim(), "ask the user to choose exactly one value, never pass it as `project`", "pick the most likely value and pass it as `project`", slimStartupResolutionContract},
		{"slim workspace mismatch falls through", protocolSlim(), "or the call fails: skip those initial reads", "or the call fails: continue to the next rule", slimStartupResolutionContract},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := normalizeForSemanticMatch(tt.content)
			mutated := strings.Replace(normalized, normalizeForSemanticMatch(tt.positive), normalizeForSemanticMatch(tt.weakened), 1)
			if mutated == normalized {
				t.Fatalf("positive branch %q not found", tt.positive)
			}
			if len(missingStartupResolution(mutated, tt.contract)) == 0 {
				t.Fatal("weakened startup branch was incorrectly accepted")
			}
		})
	}
}
