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

// requiredLedgerClauses is the OpenCode binding of the shared clause set: the
// only consumer is the preserved OpenCode orchestrator prompt.
var requiredLedgerClauses = boundedReviewRequiredClausesFor(model.AgentOpenCode)

const requiredOrchestratorMergeModeClause = "Parent orchestrator and native CLI only"

func TestBoundedReviewContractLeavesCanonicalizationToNativeGo(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"Native Go owns validation, canonicalization, persistence, hashing, reopening, and binding",
		"Only candidate-caused severe findings block",
		"Claude Code, OpenCode, and Codex advertise immutable reviewer execution",
		"Kilo remains dormant",
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
				content := renderBoundedReviewAsset(agentForAssetPath(t, path), path)
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
		if !strings.Contains(frontmatter, "tools: []") {
			t.Errorf("%s grants live reviewer tools: %s", path, frontmatter)
		}
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
		assertNoReviewerLifecycleInstructions(t, path, renderBoundedReviewAsset(agentForAssetPath(t, path), path))
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
			expandOpenCodeBoundedReviewAgents(agentsMap)
			for _, name := range []string{"review-risk", "review-readability", "review-reliability", "review-resilience"} {
				agent := agentsMap[name].(map[string]any)
				prompt := agent["prompt"].(string)
				assertTextContainsClauses(t, path+" "+name, prompt, []string{"## Scope", "## Candidate-Causal Admission", "## Severity", "## Evidence", "## Output"})
				assertNoReviewerLifecycleInstructions(t, path+" "+name, prompt)
				assertOpenCodeReadOnlyTools(t, path+" "+name, agent["tools"].(map[string]any), false, false)
				assertOpenCodeProviderInjectedReviewer(t, path+" "+name, agent)
			}
			for _, name := range []string{"jd-judge-a", "jd-judge-b"} {
				agent := agentsMap[name].(map[string]any)
				prompt := agent["prompt"].(string)
				if prompt != judgmentDayReviewerContract() {
					t.Errorf("%s %s does not use the native role-only judgment contract", path, name)
				}
				assertNoReviewerLifecycleInstructions(t, path+" "+name, prompt)
				assertOpenCodeReadOnlyTools(t, path+" "+name, agent["tools"].(map[string]any), true, false)
			}
			refuter := agentsMap[opencode.ReviewRefuterAgent].(map[string]any)
			refuterPrompt := refuter["prompt"].(string)
			if !strings.Contains(refuterPrompt, "exactly ONE transaction-wide inferential batch") || !strings.Contains(refuterPrompt, "terminate") {
				t.Errorf("%s refuter prompt is not bounded: %s", path, refuterPrompt)
			}
			assertNoReviewerLifecycleInstructions(t, path+" refuter", refuterPrompt)
			assertOpenCodeReadOnlyTools(t, path+" refuter", refuter["tools"].(map[string]any), true, false)
		})
	}
}

// assertOpenCodeProviderInjectedReviewer proves the genuinely restored
// shape: the reviewer prompt names the provider-injected context block
// (never the disabled "unsupported-capability" refusal) and its permission
// map denies bash and edit outright, with no wildcarded allow list — the
// dynamic-binding problem the wildcard existed for cannot exist when there
// is nothing left to allow.
func assertOpenCodeProviderInjectedReviewer(t *testing.T, label string, agent map[string]any) {
	t.Helper()
	prompt, _ := agent["prompt"].(string)
	if strings.Contains(prompt, "unsupported-capability") {
		t.Fatalf("%s prompt still refuses immutable inspection as unsupported: %s", label, prompt)
	}
	if !strings.Contains(prompt, "GENTLE_AI_REVIEW_CONTEXT") || !strings.Contains(prompt, "You have no execution tools") {
		t.Fatalf("%s prompt does not name the provider-injected context block: %s", label, prompt)
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

// TestReviewerBashPromptIsNativeAndWindowsPortable pins the shared
// bash-command reviewer prompt (reviewerPrompt) still used by markdown-based
// runtimes that keep their own shell (kiro, kimi, cursor). OpenCode and
// Kilocode no longer use this prompt or a Bash permission wildcard: they get
// openCodeProviderInjectedReviewerPrompt with no bash and no read tool
// instead (see TestOpenCodeOverlaysRenderBoundedReadOnlyReviewRoles).
func TestReviewerBashPromptIsNativeAndWindowsPortable(t *testing.T) {
	prompt, ok := reviewerPrompt("review-reliability")
	if !ok {
		t.Fatal("review-reliability prompt missing")
	}
	for _, forbidden := range []string{"env -i", " git ", "--text", "PowerShell", "cmd /", "Git Bash"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("review inspection still depends on %q", forbidden)
		}
	}
	for _, operation := range []string{"name-status", "numstat", "stat", "patch", "object"} {
		if !strings.Contains(prompt, "gentle-ai review inspect-candidate") || !strings.Contains(prompt, "--operation "+operation) {
			t.Errorf("review prompt omits native %s inspection recipe", operation)
		}
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
	// Corrective verify cycle 5, CRITICAL-D: review-ledger-contract.md's
	// Delivery section archive-gate sentence was corrected (see
	// TestOpenCodeRenderedReviewProtocolCost's changelog comment above for
	// the full reason); Kilocode embeds the same shared contract, so its
	// rendered settings hash moved too. Deliberate, not drift.
	//
	// Exit-naming audit fix #1: review-ledger-contract.md's stop clause
	// stopped telling the orchestrator to surface a bare `reason_code`
	// ("never from status prose") and gained the embedded "Continue after a
	// stop reason code" table (16 rows, one per reviewStopTransition code,
	// each naming its real continuation and `gentle-ai review mode disable`
	// as the self-service fallback where no more specific exit exists).
	// Kilocode embeds the same shared contract, so its rendered settings hash
	// moved again. Deliberate, not drift.
	//
	// Second pass fixing adversarial verification findings F1/F4/F6/F7 (see
	// TestOpenCodeRenderedReviewProtocolCost's changelog comment above) moved
	// it a third time. Deliberate, not drift.
	//
	// Prerelease resume fix: the provider-defect handoff's resume clause used
	// to require a `released` fix, which orchestrators read as stable-only and
	// used to refuse resuming on an installed release candidate. It now says
	// "an installed published fix", states that an installed published
	// prerelease or release candidate satisfies it, and draws the real
	// boundary at unpublished code. Kilocode embeds the same orchestrator
	// contract in `agent.gentle-orchestrator.prompt`, and that key is the only
	// difference in the rendered settings, so the hash moved a fourth time.
	// Deliberate, not drift.
	//
	// SDD edit-authority consent relay (#2570, S6 of #2540): the orchestrator
	// contract gained the byte-identical "SDD Edit-Authority Consent Relay
	// (MANDATORY)" clause teaching the lossless relay of the typed
	// gentle-ai.sdd-integration.consent/v1 envelope. Kilocode embeds the same
	// orchestrator contract in `agent.gentle-orchestrator.prompt`, so the
	// hash moved a fifth time. Deliberate, not drift.
	//
	// OpenCode Desktop delegation visibility (#633): Kilocode renders the same
	// OpenCode orchestrator asset in `agent.gentle-orchestrator.prompt`, so the
	// new assistant-visible native delegation status lines move this hash too.
	// Deliberate, not drift.
	//
	// Empty SDD task results now carry a versioned terminal handoff and the
	// orchestrator must run its supplied sdd-status continuation exactly once.
	// Kilocode embeds the shared orchestrator contract, so its rendered settings
	// hash moves with that required fail-closed protocol. Deliberate, not drift.
	//
	// This baseline combines #2485's answer-validation contract, #2417's
	// provider-injected reviewer shape, #2440's runtime-bound identity, and
	// #2207's executor-boundary wording. It is recomputed from the merged tree.
	//
	// RDD shared advisory transport, Slice B: claudeReviewerPrompt and
	// openCodeProviderInjectedReviewerPrompt were unified into one shared
	// template (runtimeReviewerPrompt) so the reviewer input contract exists
	// exactly once instead of once per runtime. Kilocode embeds the unified
	// OpenCode-shaped reviewer prompt, so the hash moved again. Deliberate,
	// not drift.
	//
	// RDD shared advisory transport, Slice E: Codex was activated as an
	// advertised immutable-reviewer-execution runtime once its organic proof
	// passed (TestRealCodexReviewerOrdinarySessionAdmitsRawOutput,
	// e2e/organicruntime). Kilocode embeds the shared orchestrator contract's
	// executor-boundary paragraph, which now names Codex's CodexAdapter
	// boundary too, so the hash moved again. Deliberate, not drift.
	//
	// RDD shared advisory transport, Slice D (retirement pass): the
	// executor-boundary paragraph's OpenCode clause used to claim OpenCode's
	// provider plugin "requires process-isolation controls before launch",
	// which the shared advisory transport made false the moment slices A-C
	// landed (rdd-advisory-transport SKILL.md: "No OpenCode restart, child
	// isolation, special session, or OPENCODE_DISABLE_* variables. An
	// ordinary running session is sufficient."). It now names that ordinary-
	// session boundary explicitly. Kilocode embeds the same paragraph, so the
	// hash moved again. Deliberate, not drift.
	//
	// Provider defect handoff recovery fix: an installed published fix remains
	// valid, but an explicit maintainer-authorized native recovery or reset that
	// the runtime supports is no longer overridden. Kilocode renders that shared
	// orchestrator contract, so the hash moved again. Deliberate, not drift.
	//
	// The canonical artifact language contract is appended to all eight agent
	// prompts (+458 characters each); no key is added, removed, or otherwise
	// changed. The hash is recomputed from the rebased tree. Deliberate, not
	// drift.
	// The provider-defect handoff now has three candidate-scoped outcomes;
	// Kilocode embeds it in the orchestrator prompt, so the hash moved.
	const want = "f259d91524ee1b893e3ec06fbfc8391d3c629add488c11a7bf5103668e0c7146"
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
		// +285 (13,729 -> 14,014 / 21,487 -> 21,772): corrective verify cycle 5,
		// CRITICAL-D. The Delivery section's archive-gate sentence still said
		// "reviewGate.result: allow ... or reviewGate.delivery: disabled/unmanaged
		// while the kill switch is off", the pre-Wave-4 contract the wave's own
		// runtime fixes (cycles 2-4) superseded three times over: the kill switch
		// off now yields reviewGate structurally ABSENT (never a populated
		// disabled/unmanaged value), and the switch on with no receipt is now also
		// decline-by-absence-of-action with reviewGate absent (BLOCKER-1). Both
		// the archive skill and this shared contract would have refused exactly
		// the states sdd-status now reports as archive-ready. This is a deliberate
		// contract correction, not drift.
		//
		// wantChars grew by 4,392 per row (14,014 -> 18,406 / 21,772 -> 26,164)
		// when the Route section gained the "Continue after a stop reason code"
		// table (exit-naming audit fix #1): the shipped contract previously told
		// the orchestrator to "surface its reason_code" and explicitly forbade
		// reading anything else, converting all 16 documented, correct stop
		// continuations (docs/review-integration.md's own table, which docs/ is
		// never embedded to ship) into dead ends on the one channel a consuming
		// orchestrator may route from. The table names every reason code's real
		// continuation plus `gentle-ai review mode disable` as the self-service
		// fallback wherever no more specific exit exists. The standard ceiling
		// moves with it (18,500 -> 21,200) to restore the ~15% margin below; the
		// full-4R ceiling already had enough headroom and is unchanged.
		// wantChars grew again by 870 per row (18,406 -> 19,276 / 26,164 ->
		// 27,034) fixing adversarial verification findings against exit-naming
		// audit fix #1: F1 completed two abbreviated `review status
		// --next-transition` invocations to their real required form
		// (--contract gentle-ai.review-integration/v2 --agent claude-code,
		// verified by execution -- the bare form is refused), F4 disclosed
		// that `review start` on an unchanged candidate only resumes the same
		// review rather than starting a fresh one (also verified by
		// execution), F6 switched every `review mode disable` mention to the
		// clone-scoped `--scope clone --cwd <repo>` form plus a one-line
		// disclosure that omitting --scope disables review machine-wide
		// (--scope defaults to global; verified by execution), and F7
		// completed the `review reopen-results` invocation to its six
		// required flags (also verified by execution). The standard ceiling
		// moves with it (21,200 -> 22,200) to restore the ~15% margin below;
		// full-4R still has headroom and is unchanged.
		//
		// #2207 advertises only Claude Code and OpenCode after their fresh-reviewer
		// constraints are made explicit in the shared contract.
		//
		// wantChars grew by 59 per lens (19,569 -> 19,628 / 29,970 -> 30,206)
		// when claudeReviewerPrompt and openCodeProviderInjectedReviewerPrompt
		// were unified into runtimeReviewerPrompt, one shared template whose
		// only runtime-specific input is the context marker and the supplying
		// process. The union of both runtimes' forbidden-tool wording ("Bash,
		// Git, Read") and the "Never read the live worktree" clause now render
		// identically for every runtime; this is a deliberate contract
		// unification, not drift. Ceilings are unchanged: both rows keep more
		// than 15% headroom.
		//
		// RDD shared advisory transport, Slice E: wantChars grew by 291 per
		// row (19,628 -> 19,919 / 30,206 -> 30,497) when the shared
		// executor-boundary paragraph was extended to describe Codex's
		// CodexAdapter boundary alongside Claude's and OpenCode's, once
		// Codex's organic proof passed
		// (TestRealCodexReviewerOrdinarySessionAdmitsRawOutput,
		// e2e/organicruntime). Ceilings are unchanged: both rows keep more
		// than 15% headroom.
		//
		// RDD shared advisory transport, Slice D (retirement pass): wantChars
		// grew by 143 per row (19,919 -> 20,062 / 30,497 -> 30,640) when the
		// same paragraph's OpenCode clause was corrected from claiming its
		// provider plugin "requires process-isolation controls before launch"
		// (false since slices A-C landed) to naming the actual ordinary-
		// session boundary: no restart, child process, special session, or
		// OPENCODE_DISABLE_* variable. Ceilings are unchanged: both rows keep
		// more than 15% headroom.
		//
		// +458 per lens injects the canonical artifact language contract into every
		// rendered sub-agent prompt, so executors no longer depend on the orchestrator
		// remembering to forward it. The ceilings move to preserve the required 15%
		// headroom after that deliberate increase.
		// Root 7 (#2471) removed two stop-reason rows from the shipped contract
		// because the machine now routes them as collect transitions, so the
		// rendered protocol got 372 characters cheaper. The pins move DOWN,
		// which is the direction this table exists to protect.
		{name: "standard", agents: []string{"review-reliability"}, beforeChars: 42_301, wantChars: 20_878, maxCharacters: 24_300},
		{name: "full-4R", agents: []string{"review-risk", "review-resilience", "review-readability", "review-reliability"}, beforeChars: 106_998, wantChars: 33_223, maxCharacters: 38_500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Measure what an OpenCode user actually installs, not the
			// shared source: the contract now carries the runtime-identity
			// substitution placeholder, and only the bound form is ever
			// written to disk (issue #2440).
			chars, _ := measurePromptCost(bindRuntimeAgentIdentity(boundedReviewContract(), model.AgentOpenCode))
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

func assertOpenCodeReadOnlyTools(t *testing.T, label string, tools map[string]any, read, bash bool) {
	t.Helper()
	want := map[string]bool{"*": false, "read": read, "write": false, "edit": false, "bash": bash, "task": false}
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
	assertOpenCodeReadOnlyTools(t, label, tools, true, false)
}
