package sdd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

var requiredLedgerClauses = boundedReviewRequiredClauses

const requiredOrchestratorMergeModeClause = "Parent orchestrator and native CLI only"

func TestBoundedReviewContractLeavesCanonicalizationToNativeGo(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"Native Go owns validation, canonicalization, persistence, hashing, reopening, and binding",
		"Only candidate-caused severe findings block",
		"Claude Code carries immutable candidate evidence directly in the reviewer task prompt",
		"read-only native Git commands",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator contract missing %q", want)
		}
	}
	for _, forbidden := range []string{"canonical empty ledger bytes are exactly", "MUST NOT serialize", "Unknown native finding fields remain rejected"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("orchestrator contract retains model-facing canonicalization rule %q", forbidden)
		}
	}
}

func TestDedicatedReviewAndJudgmentAssetsRenderRoleContracts(t *testing.T) {
	assetsByFamily := map[string][]string{
		"claude": {
			"claude/agents/review-risk.md", "claude/agents/review-readability.md",
			"claude/agents/review-reliability.md", "claude/agents/review-resilience.md",
			"claude/agents/jd-judge-a.md", "claude/agents/jd-judge-b.md",
		},
		"cursor": {
			"cursor/agents/review-risk.md", "cursor/agents/review-readability.md",
			"cursor/agents/review-reliability.md", "cursor/agents/review-resilience.md",
		},
		"kimi": {
			"kimi/agents/review-risk.md", "kimi/agents/review-readability.md",
			"kimi/agents/review-reliability.md", "kimi/agents/review-resilience.md",
		},
		"kiro": {
			"kiro/agents/review-risk.md", "kiro/agents/review-readability.md",
			"kiro/agents/review-reliability.md", "kiro/agents/review-resilience.md",
			"kiro/agents/jd-judge-a.md", "kiro/agents/jd-judge-b.md",
		},
	}
	for family, paths := range assetsByFamily {
		for _, path := range paths {
			t.Run(family+"/"+path, func(t *testing.T) {
				content := renderBoundedReviewAsset(path)
				assertTextContainsClauses(t, path, content, []string{"candidate", "BLOCKER", "CRITICAL", "causal", "proof"})
				if !strings.Contains(content, "read-only") && !strings.Contains(content, "Never edit") {
					t.Errorf("%s does not state its non-mutating role", path)
				}
				assertNoReviewerLifecycleInstructions(t, path, content)
			})
		}
	}
}

func TestDedicatedReviewersAndRefutersAreStructurallyReadOnly(t *testing.T) {
	for _, path := range []string{
		"claude/agents/review-risk.md", "claude/agents/review-readability.md",
		"claude/agents/review-reliability.md", "claude/agents/review-resilience.md",
	} {
		frontmatter := markdownFrontmatter(t, path)
		if strings.Contains(frontmatter, "Bash") {
			t.Errorf("%s grants unrestricted Bash without a per-command policy", path)
		}
		for _, forbidden := range []string{"Write", "Edit"} {
			if strings.Contains(frontmatter, forbidden) {
				t.Errorf("%s frontmatter grants %s", path, forbidden)
			}
		}
	}
	if frontmatter := markdownFrontmatter(t, "claude/agents/review-refuter.md"); strings.Contains(frontmatter, "Bash") || strings.Contains(frontmatter, "Write") || strings.Contains(frontmatter, "Edit") {
		t.Errorf("Claude refuter grants an execution or mutation tool: %s", frontmatter)
	}
	for _, path := range []string{
		"kiro/agents/review-risk.md", "kiro/agents/review-readability.md",
		"kiro/agents/review-reliability.md", "kiro/agents/review-resilience.md",
	} {
		if frontmatter := markdownFrontmatter(t, path); !strings.Contains(frontmatter, `tools: ["read"]`) || strings.Contains(frontmatter, "shell") {
			t.Errorf("%s does not fail closed without a narrow shell policy:\n%s", path, frontmatter)
		}
	}
	for _, path := range []string{"kiro/agents/review-refuter.md", "kiro/agents/jd-judge-a.md", "kiro/agents/jd-judge-b.md"} {
		if frontmatter := markdownFrontmatter(t, path); !strings.Contains(frontmatter, `tools: ["read"]`) {
			t.Errorf("%s is not read-only:\n%s", path, frontmatter)
		}
	}
	for _, path := range []string{
		"cursor/agents/review-risk.md", "cursor/agents/review-readability.md",
		"cursor/agents/review-reliability.md", "cursor/agents/review-resilience.md",
		"cursor/agents/review-refuter.md",
	} {
		if frontmatter := markdownFrontmatter(t, path); !strings.Contains(frontmatter, "readonly: true") {
			t.Errorf("%s is not read-only", path)
		}
	}
	for _, path := range []string{
		"claude/agents/review-refuter.md", "cursor/agents/review-refuter.md",
		"kimi/agents/review-refuter.md", "kiro/agents/review-refuter.md",
	} {
		assertNoReviewerLifecycleInstructions(t, path, renderBoundedReviewAsset(path))
	}
	for _, path := range []string{
		"kimi/agents/review-risk.yaml", "kimi/agents/review-readability.yaml",
		"kimi/agents/review-reliability.yaml", "kimi/agents/review-resilience.yaml",
	} {
		content := assets.MustRead(path)
		for _, excluded := range []string{"multiagent:Task", "shell:Shell", "file:WriteFile", "file:StrReplaceFile"} {
			if !strings.Contains(content, excluded) {
				t.Errorf("%s does not exclude %s", path, excluded)
			}
		}
	}
	refuter := assets.MustRead("kimi/agents/review-refuter.yaml")
	for _, excluded := range []string{"multiagent:Task", "shell:Shell", "file:WriteFile", "file:StrReplaceFile"} {
		if !strings.Contains(refuter, excluded) {
			t.Errorf("Kimi refuter does not exclude %s", excluded)
		}
	}
}

func TestOpenCodeOverlaysRenderBoundedReadOnlyReviewRoles(t *testing.T) {
	for _, path := range []string{"opencode/sdd-overlay-single.json", "opencode/sdd-overlay-multi.json"} {
		t.Run(path, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal([]byte(assets.MustRead(path)), &root); err != nil {
				t.Fatal(err)
			}
			agentsMap := root["agent"].(map[string]any)
			expandOpenCodeBoundedReviewAgents(agentsMap, model.AgentOpenCode)
			for _, name := range []string{"review-risk", "review-readability", "review-reliability", "review-resilience"} {
				agent := agentsMap[name].(map[string]any)
				prompt := agent["prompt"].(string)
				assertTextContainsClauses(t, path+" "+name, prompt, []string{"## Scope", "## Candidate-Causal Admission", "## Severity", "## Evidence", "## Output"})
				assertNoReviewerLifecycleInstructions(t, path+" "+name, prompt)
				assertOpenCodeReadOnlyTools(t, path+" "+name, agent["tools"].(map[string]any), false)
				assertOpenCodeUnsupportedReviewer(t, path+" "+name, agent)
			}
			for _, name := range []string{"jd-judge-a", "jd-judge-b"} {
				agent := agentsMap[name].(map[string]any)
				prompt := agent["prompt"].(string)
				if prompt != judgmentDayReviewerContract() {
					t.Errorf("%s %s does not use the native role-only judgment contract", path, name)
				}
				assertNoReviewerLifecycleInstructions(t, path+" "+name, prompt)
				assertOpenCodeReadOnlyTools(t, path+" "+name, agent["tools"].(map[string]any), false)
			}
			refuter := agentsMap[opencode.ReviewRefuterAgent].(map[string]any)
			refuterPrompt := refuter["prompt"].(string)
			if !strings.Contains(refuterPrompt, "exactly ONE transaction-wide inferential batch") || !strings.Contains(refuterPrompt, "terminate") {
				t.Errorf("%s refuter prompt is not bounded: %s", path, refuterPrompt)
			}
			assertNoReviewerLifecycleInstructions(t, path+" refuter", refuterPrompt)
			assertOpenCodeReadOnlyTools(t, path+" refuter", refuter["tools"].(map[string]any), false)
		})
	}
}

func assertOpenCodeUnsupportedReviewer(t *testing.T, label string, agent map[string]any) {
	t.Helper()
	prompt, _ := agent["prompt"].(string)
	if !strings.Contains(prompt, "unsupported-capability") || strings.Contains(prompt, "inspect-candidate") {
		t.Fatalf("%s prompt does not stop unsupported inspection: %s", label, prompt)
	}
	permission, ok := agent["permission"].(map[string]any)
	if !ok || permission["bash"] != "deny" || permission["edit"] != "deny" || len(permission) != 2 {
		t.Fatalf("%s permission = %#v, want bash/edit deny only", label, agent["permission"])
	}
}

func TestReviewerInspectionCommandsReturnIndependentValues(t *testing.T) {
	first := reviewerInspectionCommands()
	second := reviewerInspectionCommands()
	if len(first) == 0 || len(second) != len(first) {
		t.Fatalf("inspection commands = %#v / %#v", first, second)
	}
	first[0] = "mutated"
	if second[0] == "mutated" || reviewerInspectionCommands()[0] == "mutated" {
		t.Fatal("reviewer inspection commands share mutable backing storage")
	}
}

func TestKilocodeReviewInspectionIsNativeAndWindowsPortable(t *testing.T) {
	prompt, ok := reviewerPrompt("review-reliability")
	if !ok {
		t.Fatal("review-reliability prompt missing")
	}
	permission := openCodeReviewerPermission()
	encoded, err := json.Marshal(permission)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"env -i", " git ", "--text", "PowerShell", "cmd /", "Git Bash"} {
		if strings.Contains(prompt, forbidden) || strings.Contains(string(encoded), forbidden) {
			t.Errorf("review inspection still depends on %q", forbidden)
		}
	}
	for _, operation := range []string{"name-status", "numstat", "stat", "patch", "object"} {
		if !strings.Contains(prompt, "gentle-ai review inspect-candidate") || !strings.Contains(prompt, "--operation "+operation) {
			t.Errorf("review prompt omits native %s inspection recipe", operation)
		}
	}
	bash := permission["bash"].(map[string]any)
	if bash["*"] != "deny" || bash["gentle-ai review inspect-candidate *"] == "allow" {
		t.Fatalf("reviewer permission is not deny-by-default and exact: %#v", bash)
	}
}

func TestKilocodeReviewSettingsMatchCurrentMainBaseline(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, kilocodeAdapter(), model.SDDModeMulti); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "kilo", "plugins", "review-result-artifacts.ts")); !os.IsNotExist(err) {
		t.Fatalf("Kilo installed OpenCode-only review plugin: %v", err)
	}
	settings, err := os.ReadFile(filepath.Join(home, ".config", "kilo", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(settings))
	const want = "7de1c9318bd3acfe763c2705bb3f03a918c1b2944cfef4984615eb0e1838877c"
	if got != want {
		t.Fatalf("Kilocode settings SHA-256 = %s, want current-main baseline %s", got, want)
	}
}

func TestOpenCodeRenderedReviewProtocolCost(t *testing.T) {
	home := t.TempDir()
	if _, err := Inject(home, opencodeAdapter(), ""); err != nil {
		t.Fatalf("Inject(opencode) error = %v", err)
	}
	settingsPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Agent map[string]struct {
			Prompt string `json:"prompt"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(payload, &settings); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		agents        []string
		beforeChars   int
		wantChars     int
		maxCharacters int
	}{
		// wantChars grew by 110 (7,085 -> 7,195 / 14,078 -> 14,188) when the
		// review-ledger-contract.md GENTLE_AI_REVIEW_BINDING sentence was
		// corrected: it previously claimed START emits that field verbatim,
		// which no emitter does; it now says how to assemble it from START's
		// own lineage_id/target_identity/lens_bindings fields (issue: docs vs
		// emitter mismatch reported alongside the review-start hint gap).
		//
		// wantChars then grew by 737 per lens (7,195 -> 7,932 / 14,188 ->
		// 17,136) when each lens prompt gained its own input and output
		// contract. Two field reports cost a review each because the prompt
		// left both unsaid: one lens returned findings/evidence with no
		// subject_hash and no inspection, and one reported inspection.status
		// "access_failure" after trying to inspect the candidate and
		// verify its SHA-256 itself, which its declared read-only tools never
		// permitted. The prompt now names GENTLE_AI_REVIEW_BINDING as the only
		// source of subject_hash, forbids inventing it, says the diff and
		// manifest arrive in the prompt, and states that there are no
		// execution tools. This is a deliberate contract change, not drift.
		//
		// wantChars then grew by 871 (7,932 -> 8,803 / 17,136 -> 18,007) when
		// review-ledger-contract.md gained the per-candidate consent relay
		// paragraph (the negotiated `--consent relay` question is a Lossless
		// Blocking Prompt the orchestrator must relay losslessly, and a
		// decline is not the kill switch) and the 4R cost-forecast rule (a
		// canonical four-lens selection is long work: one forecast — four
		// reviewer model runs, the frozen correction budget, the bounded
		// correction — before the first lens). Both close field gaps where an
		// agent launched a 4R review with no consent relayed and no cost
		// warning. This is a deliberate contract change, not drift.
		//
		// wantChars then grew by 518 (8,803 -> 9,321 / 18,007 -> 18,525)
		// when reviewer context transport was hardened after an orchestrator
		// passed frozen input through /tmp and another supplied the provider-owned
		// frozen-context token itself. The contract now pins the exact leading
		// binding prefix, rejects equals-delimited bindings and path handoffs, and
		// leaves frozen context injection to native preflight plus the plugin.
		//
		// wantChars then grew by 1,107 (9,321 -> 10,428 / 18,525 -> 19,632)
		// when issue #1916 moved generated controllers from a hardcoded START to
		// provider-owned STATUS routing. The shared contract now covers execute,
		// collect, and stop, and derives reviewer bindings from the exact
		// collection input so resumed reviews never depend on a prior START reply.
		//
		// One authority-bound native reader replaces per-lens bare-repository setup
		// and plumbing instructions (12,307 -> 11,597 / 25,954 -> 23,270). The
		// command is runtime-independent and candidate bytes remain absent.
		//
		// Retiring `review read-diff` for direct read-only native Git against the
		// frozen trees grew the recipe (11,597 -> 12,316 / 23,270 -> 25,873): the
		// prompt now carries compact discovery plus selective literal-pathspec
		// commands, environment hygiene, the no---binary rule, oversized-path
		// triage, and the Git-unavailable incomplete result. Candidate bytes
		// remain absent and prompt size still scales with path count, not patch
		// size.
		//
		// Checkout independence grew both surfaces (12,316 -> 13,074 / 25,873 ->
		// 27,981): reviewers now run the recipe in their session working
		// directory because frozen trees resolve through the shared object
		// store, orchestrators never send reviewers into another checkout, and
		// a denied optional preparatory read no longer aborts inspection. This
		// closes the cross-checkout regression where a main-repo session
		// reviewing a worktree candidate denied every subagent tool call.
		//
		// The numstat-vs-manifest suspicion rule (13,074 -> 13,294 / 27,981 ->
		// 28,861) came out of the first admitted resilience finding: a mutable
		// Git attribute can reclassify changed text as binary and silently
		// suppress its hunk, so a path numstat calls binary while its manifest
		// entry is an ordinary text-mode modification must be named in
		// evidence, never metadata-triaged in silence.
		//
		// Naming the fix validator's capability added 379 shared-contract
		// characters to both rows (13,294 -> 13,673 / 28,861 -> 29,240). It cost
		// a real correction attempt to learn: the contract said "run one
		// read-only scoped fix validator" without naming who, an orchestrator
		// routed targeted validation to the refuter (no shell, by design), and
		// that inconclusive answer was submitted as a failed check, escalating
		// the lineage irreversibly.
		//
		// Candidate-scoped consent and faithful conversation-language projection
		// added 1,151 shared-contract characters to both rows (13,536 -> 14,687 /
		// 28,956 -> 30,107). The paragraph now distinguishes global permission
		// from per-candidate consent, requires explicit benefit/consequence
		// projection, and preserves machine tokens while native UI labels are
		// localized. This is a deliberate contract change, not drift.
		//
		// The ceilings move with it (15,700 -> 17,000 / 33,600 -> 35,000) to
		// restore the ~15% margin below. This is not slackening the guard: a
		// ceiling left fixed while the pin legitimately grows converges on the
		// pin and becomes the second copy the paragraph below forbids. Both new
		// ceilings still fail loudly on the regression they exist for — one
		// agent falling through to the un-rendered contract adds roughly 28,600
		// (standard) or 19,400 (per agent, full-4R), far above either ceiling.
		//
		// maxCharacters is NOT a second copy of wantChars. wantChars catches
		// every byte of change and must be updated by hand with a reason; the
		// ceiling exists only to catch the rendering silently giving up on
		// compression — the fall-through branch in renderBoundedReviewAsset
		// injects the whole orchestrator contract into an agent it does not
		// recognize, which is what beforeChars measures (42,301 / 106,998, the
		// un-rendered protocol). The ceilings below sit ~15% above the pins so
		// an ordinary wording fix never touches them, and 4-5x below the
		// un-rendered sizes so a renderer regression still fails loudly.
		// Provider-bound preflight and the immutable Git recipe initially pushed
		// the pins too close to those ceilings. Removing repeated prose restores
		// more than 15% headroom without weakening either contract.
		// +75 over the previous pin: the archive rule now names the
		// disabled/unmanaged path, without which the agent blocks archive where
		// the native gate already defers — a deadlock the operator cannot exit.
		// Deliberately one clause: the standard tier sits close to its 15%
		// headroom rule, so the reasoning lives in sdd-archive/SKILL.md, which
		// is loaded per phase rather than always-on.
		// +457 defines STATUS-mediated recollection without adding retry state.
		// Native inspect-candidate removes repeated shell hardening prose and operands.
		// Reviewer prompts no longer expose native Git flags owned by that capability.
		// #2221 removes OpenCode reviewer transport while v2.1 pins Claude Code
		// as the sole explicit runtime. The combined generated sizes are derived
		// from the canonical rendered assets below.
		{name: "standard", agents: []string{"review-reliability"}, beforeChars: 42_301, wantChars: 13_729, maxCharacters: 18_500},
		{name: "full-4R", agents: []string{"review-risk", "review-resilience", "review-readability", "review-reliability"}, beforeChars: 106_998, wantChars: 21_487, maxCharacters: 36_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chars, _ := measurePromptCost(boundedReviewContract())
			for _, agent := range tt.agents {
				promptChars, _ := measurePromptCost(settings.Agent[agent].Prompt)
				chars += promptChars
			}
			tokens := chars / 4
			t.Logf("before=%d characters/%d estimated tokens after=%d/%d", tt.beforeChars, tt.beforeChars/4, chars, tokens)
			if chars != tt.wantChars {
				t.Fatalf("rendered protocol cost = %d characters / %d estimated tokens, want deterministic total %d / %d", chars, tokens, tt.wantChars, tt.wantChars/4)
			}
			if chars > tt.maxCharacters {
				t.Fatalf("rendered protocol cost = %d characters / %d estimated tokens, target <= %d / %d", chars, tokens, tt.maxCharacters, tt.maxCharacters/4)
			}
			if chars*115 > tt.maxCharacters*100 {
				t.Fatalf("rendered protocol cost = %d characters leaves less than 15%% headroom below ceiling %d", chars, tt.maxCharacters)
			}
		})
	}
}

func measurePromptCost(prompt string) (characters, estimatedTokens int) {
	characters = utf8.RuneCountInString(prompt)
	return characters, characters / 4
}

func markdownFrontmatter(t *testing.T, path string) string {
	t.Helper()
	parts := strings.SplitN(assets.MustRead(path), "---", 3)
	if len(parts) != 3 {
		t.Fatalf("%s missing frontmatter", path)
	}
	return parts[1]
}

func assertOpenCodeReadOnlyTools(t *testing.T, label string, tools map[string]any, bash bool) {
	t.Helper()
	want := map[string]bool{"*": false, "read": true, "write": false, "edit": false, "bash": bash, "task": false}
	if len(tools) != len(want) {
		t.Fatalf("%s tools = %#v", label, tools)
	}
	for name, expected := range want {
		if got, ok := tools[name].(bool); !ok || got != expected {
			t.Errorf("%s tool %s = %v, want %v", label, name, tools[name], expected)
		}
	}
}

func assertTextContainsClauses(t *testing.T, label, content string, clauses []string) {
	t.Helper()
	for _, clause := range clauses {
		if !strings.Contains(content, clause) {
			t.Errorf("%s missing required clause %q", label, clause)
		}
	}
}

func assertNoReviewerLifecycleInstructions(t *testing.T, label, content string) {
	t.Helper()
	forbidden := regexp.MustCompile(`(?i)\b(bundle|receipt|mirror|release|gate)s?\b`)
	if match := forbidden.FindString(content); match != "" {
		t.Errorf("%s reviewer prompt contains lifecycle instruction term %q", label, match)
	}
	lower := strings.ToLower(content)
	for _, phrase := range []string{"ordinary 4r", "fix/re-judgment", "launches review-refuter", "review/start", "review-resume", "correction transaction", "scoped validator"} {
		if strings.Contains(lower, phrase) {
			t.Errorf("%s reviewer prompt contains lifecycle routing phrase %q", label, phrase)
		}
	}
}

func readGentleOrchestratorPrompt(t *testing.T, settingsPath string) string {
	t.Helper()
	payload, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	agentsMap := root["agent"].(map[string]any)
	orchestrator := agentsMap["gentle-orchestrator"].(map[string]any)
	return orchestrator["prompt"].(string)
}

func assertOpenCodeRefuterToolsReadOnly(t *testing.T, label string, tools map[string]any) {
	t.Helper()
	assertOpenCodeReadOnlyTools(t, label, tools, false)
}
