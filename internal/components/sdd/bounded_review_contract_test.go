package sdd

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// boundedReviewRequiredClausesFor is agent-parameterized because two of these
// clauses state the runtime identity the negotiated route must carry. Pinning
// them to a constant is what let issue #2440 ship: every runtime's generated
// instructions claimed to be claude-code, and the test suite agreed.
func boundedReviewRequiredClausesFor(agent model.AgentID) []string {
	return []string{
		"Native Compact Review Orchestration",
		"gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + string(agent) + " --next-transition",
		"Selectorless STATUS only preflights the current worktree candidate",
		"START freezes one compact atomic transaction",
		"exact captured lineage, revision, and target tokens",
		"Route only from that transaction's returned `next_transition`",
		"Forecast is informational; route only from `next_transition`",
		"exact literal prefix `GENTLE_AI_REVIEW_BINDING `",
		"one-line JSON assembled only from that input",
		"`revision` from `expected-revision`",
		"`subject_hash` from `artifact_subject.subject_hash`",
		"query the same exact-lineage STATUS",
		"reoffers the same bound slot",
		"repeated `--result-artifact-file <path>`",
		"Only candidate-caused severe findings block",
		"four-lens review is long work",
		"at-most-one bounded correction",
		"typed `gentle-ai.review-integration.consent/v3` envelope",
		"Lossless Blocking Prompt",
		"Do not translate machine answer tokens (`granted`, `declined`)",
		"validator that cannot inspect the immutable trees produced no verdict",
		"Claude Code, OpenCode, Codex, and Pi use the shared Go provider contract",
		"Never hand candidate bytes through `/tmp`",
		"### Authority-First Terminal Procedure",
		"burns that exact authority and its artifacts",
		"enabled gates return `invalidated/unmanaged`",
		"disabled gates return `disabled/unmanaged`",
		"The final reviewer, refuter, or targeted-validator capture owns closure.",
		"A malformed, incomplete, or unavailable capture never burns authority: issue one retained target-bound read-only STATUS and relaunch only when it reoffers the same bound slot.",
		"Commit, push, PR, and release remain separate human decisions under ordinary repository policy.",
		"### Cross-repository lifecycle root",
		"explicit user authorization",
		"canonical B worktree root",
		"B as the lifecycle working root",
		"process cwd B",
		"Never append, remove, or rebuild provider-issued command tokens",
		"Opaque `repository_context` can capture or materialize from any process cwd",
		"Go owns repository binding; adapters never parse authorization or roots",
		"Approval burns B only; A remains untouched",
		"review lifecycle stops",
		"Unsupported runtimes remain unavailable",
		"### Research and Pre-Proposal Gate (MANDATORY)",
		"immediately after `sdd-explore`",
		"selected research is `done` or research is unselected",
		"product decisions are `confirmed`",
		"evidence references are valid",
		"one lossless grouped prompt",
		"persist the pending state before prompting",
		"STOP without invoking `sdd-propose`",
		"Native `gentle-ai.sdd-status/v1` remains unchanged",
	}
}

func TestReviewLifecycleContractRequiresAtomicBurnAndNonDecidingDelivery(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"Selectorless STATUS only preflights the current worktree candidate",
		"START freezes one compact atomic transaction",
		"burns that exact authority and its artifacts",
		"enabled gates return `invalidated/unmanaged`",
		"disabled gates return `disabled/unmanaged`",
		"The final reviewer, refuter, or targeted-validator capture owns closure.",
		"A malformed, incomplete, or unavailable capture never burns authority: issue one retained target-bound read-only STATUS and relaunch only when it reoffers the same bound slot.",
		"Commit, push, PR, and release remain separate human decisions under ordinary repository policy.",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("atomic lifecycle contract missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"reconcile-terminal-mirrors",
		"reviewGate.result: allow",
		"staged_delivery_candidate_required",
		"Reuse a valid receipt",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("atomic lifecycle contract retains obsolete clause %q", forbidden)
		}
	}
}

func TestBoundedReviewStopInventoryIsCompleteWithoutRepeatingStatus(t *testing.T) {
	content := boundedReviewContract()
	const start = "### Continue after a stop reason code"
	const end = "## Delivery follows ordinary repository policy"
	startIndex := strings.Index(content, start)
	endIndex := strings.Index(content, end)
	if startIndex < 0 || endIndex < startIndex {
		t.Fatal("bounded review contract has no bounded stop inventory")
	}
	inventory := content[startIndex:endIndex]

	for _, code := range []string{
		"captured_artifacts_unverifiable",
		"captured_result_selection_unavailable",
		"missing_authority_binding",
		"corrupted_or_unverifiable_authority",
		"manual_intervention_required",
		"native_stop_required",
		"empty_base_diff_bootstrap_required",
		"lens_context_budget_exceeded",
		"staged_workspace_overlay_recovery_unavailable",
		"corrected_candidate_unavailable",
		"recovery_scope_unchanged",
		"rdd_disabled",
	} {
		if got := strings.Count(inventory, "`"+code+"`"); got != 1 {
			t.Errorf("stop inventory contains %d occurrences of %q, want exactly one", got, code)
		}
	}

	for _, group := range []string{
		"| `corrupted_or_unverifiable_authority`, `manual_intervention_required`, `native_stop_required` |",
	} {
		if strings.Count(inventory, group) != 1 {
			t.Errorf("stop inventory lost grouped continuation %q", group)
		}
	}

	for _, want := range []string{
		"`D` means `gentle-ai review mode disable --scope clone --cwd <B>`",
		"`S` means re-query the exact captured target-root STATUS command with lineage and target.",
		"then `S`; do not reuse the pre-correction target",
		"then `S`.",
	} {
		if !strings.Contains(inventory, want) {
			t.Errorf("stop inventory missing continuation alias rule %q", want)
		}
	}
	if strings.Contains(inventory, "gentle-ai review status --cwd") {
		t.Fatal("stop inventory repeats the canonical STATUS command instead of using S")
	}
	canonicalStatus := "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent " + runtimeAgentIDPlaceholder + " --next-transition"
	if got := strings.Count(content, canonicalStatus); got != 1 {
		t.Fatalf("bounded review contract contains %d canonical STATUS commands, want exactly one", got)
	}
}

func TestBoundedReviewConsentLocalizationPreservesMachineDomain(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"relay it as a Lossless Blocking Prompt",
		"faithfully translate the headline, reason, `value`, risk evidence, choice labels, every choice `effect`, and the off-path note",
		"Project `value` as benefits and every `effect` as consequences",
		"Do not translate machine answer tokens (`granted`, `declined`)",
		"Run exactly the invocation selected by the human",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator contract missing localized consent rule %q", want)
		}
	}
}

func TestBoundedReviewContractRequiresRuntimeBoundReviewerContext(t *testing.T) {
	content := boundedReviewContract()
	for _, want := range []string{
		"Claude Code, OpenCode, Codex, and Pi use the shared Go provider contract",
		"Go owns frozen evidence, binding, schema, byte bounds, validation, admission, and capture",
		"adapters transport opaque provider output",
		"Never hand candidate bytes through `/tmp`",
		"an external file",
		"a repository scratch file",
		"`GENTLE_AI_FROZEN_CANDIDATE_CONTEXT`",
		"Reviewers inspect only the provider-bound immutable trees",
		"Never pass `--binary`",
		"live worktree, index, `HEAD`, or an unbound revision",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("orchestrator contract missing reviewer context rule %q", want)
		}
	}
	if strings.Contains(content, "`GENTLE_AI_REVIEW_BINDING=") {
		t.Fatal("orchestrator contract permits equals-delimited reviewer bindings")
	}
}

func TestGeneratedOpenCodeReviewControllersUseNegotiatedStatusRouting(t *testing.T) {
	controllers := map[string][]string{
		"orchestrator": {
			"Selectorless STATUS only preflights the current worktree candidate",
			"Invoke only the returned START operation and its ordered tokens unchanged",
			"Every later STATUS and collection call",
			"For `execute`", "For `collect`", "For `stop`",
		},
		"post-apply": {
			"exact returned START",
			"exact-lineage STATUS and collect",
			"native readback, exact authority/artifact burn, then `approved`",
		},
	}
	for name, required := range controllers {
		content := renderSDDOrchestratorAsset(model.AgentOpenCode)
		if name == "post-apply" {
			content = renderBoundedReviewAsset(model.AgentOpenCode, "opencode/commands/sdd-apply.md")
		}
		t.Run(name, func(t *testing.T) {
			clauses := append([]string{"lineage, revision, and target"}, required...)
			if name == "orchestrator" {
				clauses = append(clauses, "gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent "+string(model.AgentOpenCode)+" --next-transition")
			} else if strings.Contains(content, "gentle-ai review status --cwd <repo>") {
				t.Error("generated OpenCode post-apply controller repeats the canonical STATUS command")
			}
			for _, clause := range clauses {
				if !strings.Contains(content, clause) {
					t.Errorf("generated OpenCode %s controller missing atomic routing clause %q", name, clause)
				}
			}
			for _, stale := range []string{
				"Call `gentle-ai review start` once.",
				"runs `gentle-ai review start --cwd <repo>`",
				"| 01 | `gentle-ai review start`",
				"reconcile-terminal-mirrors",
			} {
				if strings.Contains(content, stale) {
					t.Errorf("generated OpenCode %s controller retains obsolete route %q", name, stale)
				}
			}
		})
	}
}

func TestSharedReviewLifecycleRendersOnlyForAdvertisedRuntimes(t *testing.T) {
	const wantExposed = 4
	const lifecycleSentinel = "### Authority-First Terminal Procedure"

	exposed := 0
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			want := agent.ID == model.AgentClaudeCode ||
				agent.ID == model.AgentOpenCode ||
				agent.ID == model.AgentCodex ||
				agent.ID == model.AgentPi
			if got := strings.Contains(content, lifecycleSentinel); got != want {
				t.Fatalf("shared review lifecycle rendered = %t, want %t", got, want)
			}
			if !want {
				if !strings.Contains(content, "## SDD Workflow") {
					t.Fatal("non-RDD runtime lost its normal SDD workflow")
				}
				for _, forbidden := range []string{
					"Native Compact Review Orchestration",
					"Selectorless STATUS only preflights the current worktree candidate",
					"Clean FINALIZE success stops with no terminal STATUS.",
					"### Cross-repository lifecycle root",
				} {
					if strings.Contains(content, forbidden) {
						t.Fatalf("non-RDD runtime received lifecycle promise %q", forbidden)
					}
				}
			}
			if want {
				exposed++
			}
		})
	}
	if exposed != wantExposed {
		t.Fatalf("shared lifecycle runtimes = %d, want %d", exposed, wantExposed)
	}
}

func TestBoundedReviewContractRendersForAdvertisedRuntimes(t *testing.T) {
	agents := catalog.AllAgents()
	rendered := 0
	for _, agent := range agents {
		if !expectedReviewLifecycleRuntime(agent.ID) {
			continue
		}
		rendered++
		t.Run(string(agent.ID), func(t *testing.T) {
			content := renderSDDOrchestratorAsset(agent.ID)
			assertTextContainsClauses(t, string(agent.ID), content, boundedReviewRequiredClausesFor(agent.ID))
			if strings.Count(content, researchLifecycleContract()) != 1 {
				t.Fatal("rendered orchestrator must contain one canonical research lifecycle")
			}
			// The retired WorkRun commands are gone from the assets, so nothing
			// here may require them. internal/assets/assets_test.go owns the
			// inverse assertion that they never come back.
			if strings.Contains(content, runtimeAgentIDPlaceholder) {
				t.Errorf("rendered %s retains runtime agent placeholder", agent.ID)
			}
			for _, forbidden := range []string{"review-start", "review-step", "review-resume", "review-validate", "review-bundle-export", "review-bundle-import"} {
				if strings.Contains(content, forbidden) {
					t.Errorf("rendered %s exposes lower-level compatibility command %q", agent.ID, forbidden)
				}
			}
			for _, forbidden := range []string{
				"exactly THREE refuters total",
				"3 total for full-4R",
				"run at most 2 sweeps per lens",
				"standard review or three lens passes sequentially",
				"verifies fix-touched lines",
				"may append fix-caused defects",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("rendered %s retains obsolete review clause %q", agent.ID, forbidden)
				}
			}
		})
	}
	if rendered != 4 {
		t.Fatalf("review lifecycle runtime count = %d, want 4", rendered)
	}
	for _, forbidden := range []string{"review-start", "review-step", "review-resume", "review-validate", "review-bundle-export", "review-bundle-import"} {
		if strings.Contains(boundedReviewContract(), forbidden) {
			t.Errorf("orchestrator contract exposes lower-level compatibility command %q", forbidden)
		}
	}
	if got := sddOrchestratorAsset(model.AgentPi); got != "generic/sdd-orchestrator.md" {
		t.Fatalf("Pi orchestrator asset = %q, want generic adapter", got)
	}
}

func TestOpenCodeOrchestratorAddsOnlyOneConcurrentReviewerGroupContract(t *testing.T) {
	const openCodeConcurrentReviewerGroupContract = "### OpenCode Concurrent Reviewer Group (MANDATORY)\n\n" +
		"When one fresh `collect.inputs` set contains multiple distinct independent `review.capture-result` reviewer slots, emit one grouped OpenCode `task` tool-call response with one foreground task per input in provider order. For canonical 4R, preserve `review-risk`, `review-resilience`, `review-readability`, `review-reliability` order.\n\n" +
		"Each task submits only its own provider-issued `review.capture-result` binding, exact lens as `subagent_type`, and exact binding prompt prefix. Do not set a `background` flag. Do not wait between launches; wait for every foreground task result. Completion order is not authority: shared Go admission/election owns reduction and semantics. The final admitted capture owns reduction and closure. On `approved`, authority is already burned: do not FINALIZE or issue a trailing STATUS. On `correction_required`, continue only through exact bound STATUS and the provider-issued `review.capture-correction-plan` binding. After a malformed or nonterminal capture, reconcile through exact bound STATUS and retry only an identically reoffered slot."

	for _, test := range []struct {
		name  string
		agent model.AgentID
		count int
	}{
		{name: "opencode", agent: model.AgentOpenCode, count: 1},
		{name: "claude", agent: model.AgentClaudeCode},
		{name: "codex", agent: model.AgentCodex},
		{name: "kilocode", agent: model.AgentKilocode},
		{name: "generic", agent: model.AgentPi},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := strings.Count(renderSDDOrchestratorAsset(test.agent), openCodeConcurrentReviewerGroupContract); got != test.count {
				t.Fatalf("rendered %s concurrent reviewer group contract count = %d, want %d", test.name, got, test.count)
			}
		})
	}

	entries, err := assets.FS.ReadDir("opencode/commands")
	if err != nil {
		t.Fatalf("read OpenCode command assets: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		t.Run("command/"+entry.Name(), func(t *testing.T) {
			content := renderBoundedReviewAsset(model.AgentOpenCode, "opencode/commands/"+entry.Name())
			if got := strings.Count(content, openCodeConcurrentReviewerGroupContract); got != 0 {
				t.Fatalf("rendered OpenCode command concurrent reviewer group contract count = %d, want 0", got)
			}
		})
	}

	preserved := "Operator-owned prompt.\n\n#### Review Execution Contract\n\n" +
		"Legacy managed review contract.\n\n" + openCodeConcurrentReviewerGroupContract +
		"\n\n#### Cost and Context Balance\n\nOperator-owned balance guidance.\n"
	once := renderPreservedOpenCodeOrchestratorPrompt(preserved, model.AgentOpenCode)
	twice := renderPreservedOpenCodeOrchestratorPrompt(once, model.AgentOpenCode)
	if got := strings.Count(once, openCodeConcurrentReviewerGroupContract); got != 1 {
		t.Fatalf("preserved OpenCode prompt concurrent reviewer group contract count after first render = %d, want 1", got)
	}
	if got := strings.Count(twice, openCodeConcurrentReviewerGroupContract); got != 1 {
		t.Fatalf("preserved OpenCode prompt concurrent reviewer group contract count after second render = %d, want 1", got)
	}
	if twice != once {
		t.Fatal("preserved OpenCode prompt rendering is not idempotent")
	}
}

func TestPreservedSharedOrchestratorSubstitutesRuntimeAgentIdentity(t *testing.T) {
	t.Parallel()

	preserved := strings.Join([]string{
		"Bind this to the dedicated `sdd-orchestrator` agent only.",
		runtimeAgentIDPlaceholder,
	}, "\n")
	rendered := renderPreservedOpenCodeOrchestratorPrompt(
		preserved,
		model.AgentKilocode,
	)
	if !strings.Contains(rendered, "Bind this to the dedicated `gentle-orchestrator` agent only.") {
		t.Fatalf("preserved prompt lost its migration:\n%s", rendered)
	}
	if strings.Contains(rendered, runtimeAgentIDPlaceholder) {
		t.Fatal("preserved prompt retained runtime agent placeholder")
	}
	if !strings.Contains(rendered, string(model.AgentKilocode)) {
		t.Fatalf("preserved prompt missing runtime agent identity:\n%s", rendered)
	}
}

// The retired WorkRun commands no longer exist, so a preserved prompt that
// still mentions them must pass through migration untouched instead of being
// rewritten into a better-formed invocation of a deleted command.
func TestPreservedSharedOrchestratorLeavesRetiredWorkCommandsUntouched(t *testing.T) {
	t.Parallel()

	retired := []string{
		"gentle-ai work-capabilities --cwd <repo> --contract gentle-ai.work-capabilities/v2 --json",
		"gentle-ai work-start --cwd <repo> --contract gentle-ai.work-start/v1 --json",
	}
	rendered := renderPreservedOpenCodeOrchestratorPrompt(
		strings.Join(retired, "\n"),
		model.AgentKilocode,
	)
	for _, command := range retired {
		if !strings.Contains(rendered, command) {
			t.Fatalf("preserved prompt rewrote retired command %q:\n%s", command, rendered)
		}
	}
	if strings.Contains(rendered, "--agent "+string(model.AgentKilocode)) {
		t.Fatalf("preserved prompt injected an adapter identity into a retired command:\n%s", rendered)
	}
}

func TestRenderedReviewersAreReadOnlyAndSingleResult(t *testing.T) {
	for _, family := range []string{"claude", "cursor", "kimi", "kiro"} {
		for _, lens := range []string{"risk", "readability", "reliability", "resilience"} {
			path := family + "/agents/review-" + lens + ".md"
			t.Run(family+"/"+lens, func(t *testing.T) {
				content := renderBoundedReviewAsset(agentForAssetPath(t, path), path)
				for _, want := range []string{"Review once", "changed_path_manifest", "base_tree", "candidate_tree", "incomplete inspection", "Never read the live worktree", "## Candidate-Causal Admission", "Return one JSON object and no prose", `"subject_hash":"<artifact_subject.subject_hash>"`, "GENTLE_AI_REVIEW_BINDING.subject_hash", `"inspection":{"status":"completed","paths":["<complete unique unordered set>"]}`, "path:line or path:start-end", "complete unique unordered manifest set", "lens triage", "Emit no unknown fields"} {
					if !strings.Contains(content, want) {
						t.Errorf("%s missing %q", path, want)
					}
				}
				if family == "claude" {
					// "provider-injected context" replaced the Claude-only
					// "prompt-carried immutable context" wording when
					// claudeReviewerPrompt and openCodeProviderInjectedReviewerPrompt
					// were unified into one shared template
					// (runtimeReviewerPrompt): the two runtimes now render
					// identical wording and differ only in which process
					// supplies the block, so the reviewer input contract no
					// longer names a Claude-specific nature for the context.
					for _, want := range []string{"GENTLE_AI_CLAUDE_REVIEW_CONTEXT", "provider-injected context", "path evidence for every manifest index", "Missing, partial, reordered, mismatched, or unavailable evidence", "no execution tools"} {
						if !strings.Contains(content, want) {
							t.Errorf("%s missing Claude transport clause %q", path, want)
						}
					}
					for _, forbidden := range []string{"OpenCode tasks begin", "gentle-ai review inspect-candidate"} {
						if strings.Contains(content, forbidden) {
							t.Errorf("%s retains provider-only instruction %q", path, forbidden)
						}
					}
					return
				}
				for _, want := range []string{"GENTLE_AI_REVIEW_CONTEXT", "sole source of artifact_subject", "gentle-ai review inspect-candidate", "--operation name-status", "--operation numstat", "--operation stat --path-index", "--operation patch --path-index", "--operation object --path-index", "--side base", "--side candidate", "provider binding", "zero-based changed_path_manifest index", "never pass --binary"} {
					if !strings.Contains(content, want) {
						t.Errorf("%s missing provider transport clause %q", path, want)
					}
				}
			})
		}
	}
}

func TestJudgmentDayReviewersUseNativeResultSchema(t *testing.T) {
	for name, content := range map[string]string{
		"rendered contract": judgmentDayReviewerContract(),
		"skill reference":   assets.MustRead("skills/judgment-day/references/prompts-and-formats.md"),
	} {
		for _, want := range []string{nativeReviewerResultSchema, "Never emit", "skill_resolution", "unknown field", "orchestration metadata outside the native result JSON", `{"findings":[],"evidence":["what was inspected"]}`} {
			if !strings.Contains(content, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
}

func TestBoundedReviewContractDoesNotEnforceModelPolicy(t *testing.T) {
	content := boundedReviewContract()
	for _, forbidden := range []string{"MUST use model", "required provider", "enforced effort", "mandatory profile"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("bounded review contract enforces model policy with %q", forbidden)
		}
	}
}

func TestBoundedReviewContractMakesCompatibilityGatesNonDeciding(t *testing.T) {
	content := boundedReviewContract()
	for _, clause := range []string{
		"Shipped `review validate` and gate commands are compatibility/informational only",
		"enabled gates return `invalidated/unmanaged`",
		"disabled gates return `disabled/unmanaged`",
		"They never allow, approve, block, commit, push, open a PR, or govern release",
	} {
		if !strings.Contains(content, clause) {
			t.Errorf("contract missing non-deciding gate clause %q", clause)
		}
	}
	for _, forbidden := range []string{"reviewGate.result: allow", "approved receipt", "reconcile-terminal-mirrors", "staged_delivery_candidate_required"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("contract retains obsolete delivery gate clause %q", forbidden)
		}
	}
}

func TestAuthorityFirstTerminalProcedureIsStructuredAndAtomic(t *testing.T) {
	rows := parseAuthorityFirstRows(t, authorityFirstTerminalProcedure())
	want := []authorityFirstRow{
		{order: 1, operation: "canonical initial STATUS above", result: "exactly one current-worktree START preflight; no authority discovery"},
		{order: 2, operation: "exact returned START", result: "one compact lineage/worktree/target binding; retain lineage, revision, and target"},
		{order: 3, operation: "exact-lineage STATUS and collect", result: "only returned transaction actions; no ambient resume, reuse, or delivery gate"},
		{order: 4, operation: "final admitted capture", result: "native readback, exact authority/artifact burn, then `approved`"},
		{order: 5, operation: "terminal lifecycle stop", result: "ordinary repository policy owns any later delivery decision"},
	}
	if len(rows) != len(want) {
		t.Fatalf("authority-first rows = %d, want %d", len(rows), len(want))
	}
	for index, expected := range want {
		if rows[index] != expected {
			t.Fatalf("authority-first row[%d] = %#v, want %#v", index, rows[index], expected)
		}
	}
}

func TestAuthorityFirstLifecycleRendersForAdvertisedRuntimes(t *testing.T) {
	rendered := 0
	for _, agent := range catalog.AllAgents() {
		if !expectedReviewLifecycleRuntime(agent.ID) {
			continue
		}
		rendered++
		t.Run(string(agent.ID), func(t *testing.T) {
			procedure := bindRuntimeAgentIdentity(authorityFirstTerminalProcedure(), agent.ID)
			content := renderSDDOrchestratorAsset(agent.ID)
			if strings.Count(content, procedure) != 1 {
				t.Fatal("rendered orchestrator does not contain exactly one canonical terminal procedure")
			}
			for _, want := range []string{"Selectorless STATUS only preflights the current worktree candidate", "Route only from that transaction's returned `next_transition`", "Forecast is informational; route only from `next_transition`", "The final reviewer, refuter, or targeted-validator capture owns closure."} {
				if !strings.Contains(content, want) {
					t.Errorf("rendered orchestrator missing forecast contract %q", want)
				}
			}
		})
	}
	if rendered != 4 {
		t.Fatalf("authority-first lifecycle runtime count = %d, want 4", rendered)
	}
}

func TestOpenCodeAndClaudeApplyCommandsUseTheAtomicLifecycle(t *testing.T) {
	for _, path := range []string{"opencode/commands/sdd-apply.md", "claude/commands/sdd-apply.md"} {
		t.Run(path, func(t *testing.T) {
			raw := assets.MustRead(path)
			if strings.Count(raw, authorityFirstProcedurePlaceholder) != 1 {
				t.Fatalf("%s must reference the centralized terminal procedure exactly once", path)
			}
			agent := agentForAssetPath(t, path)
			content := renderBoundedReviewAsset(agent, path)
			procedure := bindRuntimeAgentIdentity(authorityFirstTerminalProcedure(), agent)
			if strings.Contains(content, authorityFirstProcedurePlaceholder) || strings.Count(content, procedure) != 1 {
				t.Fatalf("%s did not render the centralized terminal procedure", path)
			}
			if !strings.Contains(content, "canonical initial STATUS above") {
				t.Fatalf("%s does not reference the canonical initial STATUS", path)
			}
			if strings.Contains(content, "gentle-ai review status --cwd <repo>") {
				t.Fatalf("%s duplicates the canonical STATUS command", path)
			}
			for _, forbidden := range []string{"runs `gentle-ai review start --cwd <repo>`", "Reuse a valid receipt", "reviewGate.result: allow"} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s retains obsolete review routing %q", path, forbidden)
				}
			}
		})
	}
}

func TestOpenCodeAndClaudeArchiveInstructionsDoNotGateOnReviewAuthority(t *testing.T) {
	for _, path := range []string{"opencode/commands/sdd-archive.md", "claude/commands/sdd-archive.md"} {
		t.Run(path, func(t *testing.T) {
			content := assets.MustRead(path)
			for _, required := range []string{
				"`reviewOffer` is optional and never an archive or delivery gate",
				"Review approval is terminal and burns its authority",
				"archive never requires `reviewGate`, a receipt, a ledger, or gate-context artifacts",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("%s missing archive non-gate rule %q", path, required)
				}
			}
			for _, forbidden := range []string{
				"reviewGate.result: allow",
				"Only a PRESENT `reviewGate`",
				"native receipt and task completion gates",
				"exact transaction/ledger/receipt/gate-context references",
			} {
				if strings.Contains(content, forbidden) {
					t.Errorf("%s retains obsolete archive review gate %q", path, forbidden)
				}
			}
		})
	}
}

type authorityFirstRow struct {
	order     int
	operation string
	result    string
}

func parseAuthorityFirstRows(t *testing.T, content string) []authorityFirstRow {
	t.Helper()
	rows := make([]authorityFirstRow, 0, 15)
	for _, line := range strings.Split(content, "\n") {
		if len(line) < 4 || line[0] != '|' || line[2] < '0' || line[2] > '9' {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 5 {
			t.Fatalf("malformed authority-first table row %q", line)
		}
		var order int
		if _, err := fmt.Sscanf(strings.TrimSpace(fields[1]), "%d", &order); err != nil {
			t.Fatalf("parse authority-first order: %v", err)
		}
		rows = append(rows, authorityFirstRow{
			order: order, operation: strings.Trim(strings.TrimSpace(fields[2]), "`"),
			result: strings.TrimSpace(fields[3]),
		})
	}
	return rows
}
