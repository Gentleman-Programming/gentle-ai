package agentguidance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestRoutingIncludesApplicableTestFirstPolicy(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode, model.AgentKimi} {
		rendered, err := RenderRouting(agent)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"relevant runnable deterministic test", "observe RED before implementation", "GREEN", "refactor", "passive documentation", "no meaningful runnable RED", "Tests or frameworks being present alone"} {
			if !strings.Contains(rendered, want) {
				t.Errorf("%s missing %q", agent, want)
			}
		}
		if strings.Contains(rendered, "configured TDD mode") || strings.Contains(rendered, "Resolve effective TDD on/off") {
			t.Errorf("%s retains toggle-gated ODD guidance", agent)
		}
	}
}

// C7: blind reviewers flagged test scratch and tool caches in delivered diffs,
// so the shared test-first policy keeps scratch outside the worktree and
// forwards that rule to workers with the rest of the test discipline.
func TestRenderRoutingKeepsTestScratchOutOfTheWorktree(t *testing.T) {
	t.Parallel()

	const rule = "Keep test scratch state (temporary directories, `HOME`, `TMPDIR`, tool caches) outside the worktree in a `mktemp -d` directory; when it must live inside the worktree, use a path git excludes and remove it before finishing, so no tool cache or scratch path remains in the change. Subagents never delete scratch: they report its exact paths, and the parent removes outside scratch after use, including the paths a worker or verifier reports. Forward the applicable runner, this test discipline"
	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(rendered, rule) {
			t.Errorf("%s routing is missing the test scratch rule", agent.ID)
		}
		for _, retired := range []string{".test-tmp", "TMPDIR=$PWD"} {
			if strings.Contains(rendered, retired) {
				t.Errorf("%s routing still points scratch into the worktree: %q", agent.ID, retired)
			}
		}
	}
}

// pain-point-iteration S14: commit per reviewable slice, not per task. A
// per-task commit rule made models commit every small task (branch plus seven
// commits on a six-task feature); about 400-line slices keep the native review
// candidate inside the reviewer context, and a high-risk unit still gets its
// own commit so review can take it alone. RDD runtimes carry the canon byte
// for byte; runtimes without RDD name verification instead of native review,
// as step 7 already does.
func TestRenderRoutingCommitsWorkUnitsInReviewableSlices(t *testing.T) {
	t.Parallel()

	const rule = "Commit work units in reviewable slices of about 400 authored changed lines on the feature branch (branch first when on the default branch), with tests and docs alongside the behavior and a Conventional Commit message; a unit whose Risk line names a high-risk item gets its own commit so native review can take it alone, and the last slice closes the feature. Record the commit that holds each task in the feature document as evidence."
	oddRule := strings.Replace(rule, "so native review can take it alone", "so verification can take it alone", 1)
	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := oddRule
		if model.SupportsReceiptDrivenDevelopment(agent.ID) {
			want = rule
		}
		if !strings.Contains(rendered, want) {
			t.Errorf("%s routing is missing the reviewable-slice commit rule", agent.ID)
		}
		if strings.Contains(rendered, "Every tracked task closes with at least one work-unit commit") {
			t.Errorf("%s routing still commits per task", agent.ID)
		}
	}
}

// supportedAgentCount guards the catalog itself: routing renders for every
// supported adapter (including the catalog-only Conductor, which simply never
// gets written), so a silently shrinking catalog must fail here instead of
// quietly reducing coverage of the table-driven tests below.
const supportedAgentCount = 18

// retiredRemoteControlPlaneVocabulary is the wire and ceremony vocabulary the
// organic routing projection must never carry. Rendering any of these would
// reintroduce the removed remote control plane into always-on adapter guidance.
var retiredRemoteControlPlaneVocabulary = []string{
	"work-capabilities",
	"work-start",
	"work-advance",
	"work-route",
	"work-status",
	"work-transition",
	"work-reconcile",
	"work-verification-decide",
	"workrun",
	"connector",
	"GENTLE_AI_PRODUCTIVE_RUNTIME",
	"dormant",
	"advertised",
	"exposure",
	"authorizedTransition",
	"expected-revision",
	"--contract",
	"http://",
	"https://",
	"token",
	"certificate authority",
	"daemon",
}

// Every adapter must receive only the ODD workflow, not a selectable legacy route.
func TestRenderRoutingOffersOnlyODD(t *testing.T) {
	t.Parallel()
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()
			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rendered, "Organic Driven Development (ODD) is the predefined workflow") {
				t.Fatal("ODD workflow is missing")
			}
			if strings.Contains(strings.ToLower(rendered), "sdd") {
				t.Fatalf("agent %q still offers SDD routing", agent.ID)
			}
		})
	}
}

func TestRenderRoutingSucceedsForEverySupportedAgent(t *testing.T) {
	t.Parallel()

	all := catalog.AllAgents()
	if len(all) != supportedAgentCount {
		t.Fatalf("catalog.AllAgents() returned %d agents, want %d", len(all), supportedAgentCount)
	}

	routing := capabilitymanifest.CanonicalImplementationRouting()

	for _, agent := range all {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			if strings.TrimSpace(rendered) == "" {
				t.Fatalf("RenderRouting(%q) returned blank guidance", agent.ID)
			}

			for _, want := range []string{
				"Direct inline",
				"Delegated direct",
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing route %q:\n%s", agent.ID, want, rendered)
				}
			}

			// The rendered thresholds must come from the canonical manifest, not
			// from prose invented by the renderer.
			for _, want := range []string{
				fmt.Sprintf("one parallel batch (%d maximum)", routing.DirectInline.MaxEvidenceBatches),
				fmt.Sprintf("at most %d calls", routing.DirectInline.MaxEvidenceCalls),
				fmt.Sprintf("approximately %dk tokens", routing.DirectInline.ApproxEvidenceTokens/1000),
				fmt.Sprintf("more than approximately %d sequential lookups", routing.DelegatedDirect.ApproxSequentialLookupLimit),
				fmt.Sprintf("at most approximately %dk tokens", routing.DelegatedDirect.ApproxHandoffTokens/1000),
				fmt.Sprintf("one parent spot check (%d maximum)", routing.DelegatedDirect.MaxParentSpotChecks),
				fmt.Sprintf("approximately %dk parent-context tokens", routing.DelegatedDirect.ApproxParentContextTokens/1000),
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing canonical threshold %q:\n%s", agent.ID, want, rendered)
				}
			}
		})
	}
}

// These tests prove the delivered instruction contract, not model compliance.
// The existing injector tests separately read back this complete rendered block.
func TestRenderRoutingOrganicTaskContinuity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		clauses []string
	}{
		{"authorized large work", []string{
			"Explore the existing code and requirements first",
			"For large authorized implementation, automatically create",
			"without a task or storage permission prompt",
			"small work creates no durable task artifacts",
		}},
		{"optional research within ODD", []string{
			"Recommend optional research only for a named uncertainty",
			"If declined, continue within authorized scope only where safe without the missing evidence",
			"disclose unresolved uncertainty and pause affected unsafe decisions",
			"Neither research nor a proposal is mandatory",
		}},
		{"adaptive research and product questions", []string{
			"Establish the problem, intended outcome, constraints, and current evidence; inspect relevant code",
			"Adapt depth to uncertainty and consequence, not a fixed questionnaire or mandatory rounds",
			"The parent owns product decisions",
			"ask one focused user question only for a real unresolved product decision, then stop and wait",
			"Workers return gaps to the parent rather than assuming choices",
			"forward these research instructions to a fresh general exploration/research worker through existing delegation",
			"do not create a specialized research agent",
		}},
		{"external evidence and useful research handoff", []string{
			"use available authorized documentation/web tools and prefer primary sources",
			"Attribute material claims to source URLs or code locations",
			"distinguish verified facts, assumptions, contradictions, freshness, and gaps",
			"Return concise findings, recommendation, tradeoffs, open questions, and implementation implications",
			"If tools are unavailable, disclose limitations without inventing access or evidence",
			"pause only unsafe decisions dependent on missing evidence",
		}},
		{"durable feature identity", []string{
			"odd/tasks/<feature-name>.md",
			"odd/<feature-name>/tasks",
			"current project",
			"full current document and repository-relative file locator",
			"stable task IDs",
			"authorized scope, acceptance criteria, and applicable checks",
			"Reuse the same feature identity; never overwrite another feature",
		}},
		{"unified intent and implementation handoff", []string{
			"one feature document, not a separate plan file or topic",
			"verification evidence, progress, and next step",
			"rationale for meaningful accepted changes",
			"Routine corrections stay brief; no exhaustive decision journal",
			"Accepted user, review, or verification changes",
			"add genuinely new tasks or reopen invalidated items with a reason",
			"Findings alone never authorize scope expansion or automatic acceptance",
			"Before implementation or resume, the parent reads both the actual file and full observation",
			"passes the locator, task IDs, and linked `S#`; workers read the document until `## Log` before edits",
		}},
		// Gentleman-Programming/gentle-shell#1713: handoffs paraphrased the user's
		// request and the feature document summarized it. The document is now the
		// specification subagents read by reference, in a fixed order.
		{"feature document is the verbatim specification read by reference", []string{
			"specification subagents read by reference",
			"stable content first and the growing log last",
			"`## Specs`",
			"`## Tasks`",
			"`## Log`",
			"exact strings, error messages, and examples verbatim",
			"never summarize or reword those fragments",
			"Do not add requirements the user never asked for",
			"`L1` holds the user's original request verbatim",
			"rewrites only the affected `S#`, and reopens only its linked task",
			"Hand off by reference, never by paraphrase",
			"read until `## Log`",
			"Without a feature document, include the user's request verbatim",
			"a verdict per `S#`",
			"runs the spec's examples the parent authorized, against isolated state when they mutate data",
			"reproduce it before deciding it already works",
		}},
		{"default applicable test-first policy", []string{
			"relevant runnable deterministic test and clear expected outcome",
			"observe RED before implementation, implement GREEN, then refactor",
			"Tests or frameworks being present alone do not establish applicability",
			"passive documentation, unavailable runners, or no meaningful runnable RED",
			"explain the exception and run proportionate functional or structural checks",
			"never invent RED/GREEN evidence or a runner",
		}},
		{"updates require proof", []string{
			"automatically update affected intent and TODOs",
			"preserve valid completed and unrelated work",
			"reopen invalidated items",
			"Business scope changes still require user authorization",
			"Check off only observed outcomes with applicable proof",
			"failed, unavailable, skipped, or pending checks",
			"Checkboxes grant no approval",
		}},
		{"advisory coherent task size", []string{
			"Use about 400 authored changed lines per ODD task only as a planning heuristic, counting additions plus deletions",
			"smallest coherent behavior with its tests and docs",
			"not a task acceptance criterion, hard cap, counter-trigger, automatic stop",
			"naturally exceeds it, briefly explain why and continue without size-only rework loops",
			"Never delete spaces, blank lines, or comments for cosmetic line savings",
			"omit tests, minify, add gratuitous abstractions, or split artificially to fit the heuristic",
			"Forward this same advisory-only instruction when delegating tasks to subagents",
			"Existing PR size gates remain unchanged",
		}},
		{"recover the right feature", []string{
			"mem_context",
			"mem_search",
			"mem_get_observation",
			"read the actual task file",
			"Do not infer active work from the newest global memory",
			"Reconcile current requirements, code, and proof before resuming",
		}},
		{"partial persistence and conflict", []string{
			"Read back both writes; they are not atomic",
			"Engram is unavailable, preserve local progress and explicitly mark the mirror pending",
			"do not claim success or block unrelated safe work",
			"Preserve both versions on irreconcilable edits and ask only about the real conflict",
		}},
		{"selective independent challenge", []string{
			"at most one scoped independent read-only assumption challenge",
			"high-consequence unproven premise, even in a small security-critical change",
			"Deterministic failures need fixes, not model debate",
		}},
		{"spend-proportional implementation and verification", []string{
			"Validate consequential premises against available evidence before building",
			"reuse relevant sibling investigation instead of repeating it",
			"Run focused checks during iteration and all applicable full checks at task closure",
			"without a hard spend or line gate",
			"preserve the applicable test-first policy",
		}},
		{"existing checks", []string{
			"applicable functional verification",
			"Run applicable functional checks per task",
			"Never skip an existing delivery gate",
		}},
	}
	// rddTests hold the receipt-driven development clauses: only runtimes in
	// model.SupportsReceiptDrivenDevelopment receive them.
	rddTests := []struct {
		name    string
		clauses []string
	}{
		{"receipt-bound checkboxes and task size", []string{
			"Checkboxes grant no approval or receipt",
			"not a task acceptance criterion, hard cap, counter-trigger, automatic stop, forced split, or RDD trigger",
		}},
		{"refuter owns native review claims", []string{
			"The native RDD refuter owns native review claims; never duplicate or bypass it",
			"preserve the applicable test-first policy, native RDD, safety, and consent requirements",
		}},
		{"existing checks and ownership", []string{
			"Preserve existing native risk selection and applicable functional verification",
			"Run applicable functional checks per task, not a review cycle per TODO checkbox",
			"The native review candidate is a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch",
			"existing risk, consent, and authority",
			"Never skip an existing delivery gate",
			"A task list or assumption challenge never enables RDD",
			"Never enable receipt-driven development on the user's behalf",
		}},
		{"native risk before candidate consent", []string{
			"When RDD is enabled, first use the existing native candidate risk assessment",
			"gentle-ai review assess --cwd <repo> --agent <runtime> --json",
			"Passive/low uses silent structural checks with no reviewer or consent ceremony",
			"Medium/high relays the existing candidate consent",
			"native review runs only on grant",
			"a decline continues under ordinary policy",
			"Do not substitute model judgment, task size, or defect severity to lower prospective candidate risk",
			"never infer low risk from a failed assessment",
			"When RDD is disabled, do not start or prompt for RDD; ordinary checks remain",
		}},
	}
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()
			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			guard := strings.Index(rendered, piStep(agent.ID, "First establish whether the requested outcome explicitly authorizes a change."))
			organic := strings.Index(rendered, "### Organic Driven Development")
			if guard < 0 || organic <= guard {
				t.Fatal("ODD must follow the mutation-authorization guard")
			}
			applicable := tests
			if model.SupportsReceiptDrivenDevelopment(agent.ID) {
				applicable = append(append(applicable[:0:0], tests...), rddTests...)
			}
			for _, tt := range applicable {
				t.Run(tt.name, func(t *testing.T) {
					for _, clause := range tt.clauses {
						clause = strings.ReplaceAll(clause, "<runtime>", string(agent.ID))
						if !strings.Contains(rendered, clause) {
							t.Errorf("missing organic instruction %q", clause)
						}
					}
				})
			}
		})
	}
}

// TestRenderRoutingClosesEachTaskWithAWorkUnitCommitAndReviewsIt pins the
// work-unit commit contract: work units are committed in reviewable slices on
// the feature branch, and the native review candidate is that commit or the PR slice it
// belongs to, bounded to the previous reviewed boundary, rather than a TODO
// checkbox or the accumulated branch. It also pins the per-tier assessment
// contract (passive/low stays silent, high or an unavailable assessment
// reviews the commit immediately, medium defers to the PR slice bounded by
// the delivery budget) and delivery follows the running authored line count
// using the shared delivery-strategy vocabulary and named skill registry
// lookups.
func TestRenderRoutingClosesEachTaskWithAWorkUnitCommitAndReviewsIt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		clauses []string
	}{
		// S14: commit per reviewable slice, not per task.
		{"work units are committed in reviewable slices", []string{
			"Commit work units in reviewable slices of about 400 authored changed lines on the feature branch",
			"branch first when on the default branch",
			"tests and docs alongside the behavior",
			"a Conventional Commit message",
			"a unit whose Risk line names a high-risk item gets its own commit so",
			"Record the commit that holds each task in the feature document as evidence",
			"Work-unit commits on the feature branch are part of authorized large ODD implementation",
			"push, pull request creation, and merge remain the user's decisions under ordinary repository policy",
		}},
		{"delivery strategy vocabulary and skill resolution", []string{
			"Delivery follows work units",
			"forecast authored changed lines (additions plus deletions, generated files excluded) from the task list",
			"keep a running count from work-unit commits",
			"`ask-on-risk` (default), `auto-chain`, `single-pr`, or `exception-ok`",
			"apply the chosen strategy before the next commit",
			"`ask-on-risk` asks once for the chain strategy, `stacked-to-main` or `feature-branch-chain`",
			"`auto-chain` asks only for a missing chain strategy and slices automatically",
			"record slice boundaries, which commits each pull request holds, in the feature document",
			"Resolve the `work-unit-commits` and `chained-pr` skills by registry name before planning or creating any pull request, never hardcode their paths",
		}},
	}
	// rddTests hold the native review assessment clauses: only runtimes in
	// model.SupportsReceiptDrivenDevelopment receive them.
	rddTests := []struct {
		name    string
		clauses []string
	}{
		{"native review candidate is a commit or a slice, never the checkbox or branch", []string{
			"The native review candidate is a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch",
		}},
		{"assess each commit against the last reviewed boundary", []string{
			"Run applicable functional checks per task, not a review cycle per TODO checkbox",
			"run `gentle-ai review assess --cwd <repo> --agent <runtime> --base-ref <last reviewed boundary> --committed-only --json` on that commit and read `review_due` and `review_due_reason`",
		}},
		{"a due assessment hands over the exact preflight transition", []string{
			"When `review_due` is true (`high_risk`), or when the user explicitly asks for a review, execute the returned `next_transition.command` verbatim with your `--lenses`/`--lenses-reason` selection appended",
			"it is the exact preflight STATUS for the same `--base-ref`/`--committed-only` selectors",
			"the reviewed boundary advances to this commit once that review is acknowledged",
		}},
		{"a non-due assessment records its reason and continues", []string{
			"When `review_due` is false, record `review_due_reason` and continue: `passive` needs no review and the boundary advances",
			"`under_budget` stays pending in the slice until a later commit reaches the budget",
			"`already_reviewed` means this exact range is already covered by terminal authority",
		}},
		// S14: commit per reviewable slice; a high-risk commit is reviewed
		// alone only after the open medium slice before it is closed.
		{"a high-risk unit closes the open slice first", []string{
			"When a high-risk unit follows pending `under_budget` commits, close the open slice first, assessing and reviewing it as the slice, so the high-risk commit is then reviewed alone against the new boundary",
		}},
		// S14: commit per reviewable slice; a commit may hold several tasks,
		// so the assessment outcome is recorded per commit.
		{"boundaries advance and outcomes are recorded per commit", []string{
			"The first boundary is the branch point, and every reviewed boundary becomes the next base",
			"Record per commit the assessed tier and outcome: granted, declined, passive, under budget, already reviewed, or unavailable",
			"An unavailable or failed assessment never lowers the tier: treat the commit as due and run the preflight STATUS with `--base-ref <last reviewed boundary> --committed-only`",
			"Never infer low risk from a failed assessment",
		}},
	}
	// Runtimes without receipt-driven development verify each work unit by
	// risk instead of assessing it for native review. pain-point-iteration S9
	// C1 changed the verifier clauses: only a named high-risk item adds the
	// independent verifier, and an unclear change no longer counts as high.
	oddTests := []struct {
		name    string
		clauses []string
	}{
		{"verify each work unit by risk", []string{
			"Verification covers a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch.",
			"Verify each work-unit commit in proportion to its risk",
			"an added independent verifier only when a named Task Size high-risk item applies",
			"Record per task the risk tier you applied and the checks you observed",
			"an unclear change never adds a verifier by itself",
		}},
	}

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			applicable := append(tests[:0:0], tests...)
			if model.SupportsReceiptDrivenDevelopment(agent.ID) {
				applicable = append(applicable, rddTests...)
			} else {
				applicable = append(applicable, oddTests...)
			}
			for _, tt := range applicable {
				t.Run(tt.name, func(t *testing.T) {
					for _, clause := range tt.clauses {
						clause = piStep(agent.ID, strings.ReplaceAll(clause, "<runtime>", string(agent.ID)))
						if !strings.Contains(rendered, clause) {
							t.Errorf("missing work-unit commit instruction %q", clause)
						}
					}
				})
			}
		})
	}
}

func TestRenderRoutingAuthorizesOutcomesBeforeSelectingTopology(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}

			guard := piStep(agent.ID, "First establish whether the requested outcome explicitly authorizes a change.")
			guardOffset := strings.Index(rendered, guard)
			topologyOffset := strings.Index(rendered, "**Direct inline:**")
			if guardOffset < 0 || topologyOffset < 0 || guardOffset > topologyOffset {
				t.Fatalf("RenderRouting(%q) must place outcome authorization before topology:\n%s", agent.ID, rendered)
			}

			for _, want := range []string{
				"Investigation, explanation, review, audit, comparison, and solution-proposal or planning-only requests are read-only",
				"must not write or edit files, delegate a writer, invoke apply, or create implementation artifacts",
				"If change intent is ambiguous or conditional, ask one clarification and remain read-only until answered.",
				"After explicit change intent is established",
				"Every authorized change takes exactly one implementation route: direct inline or delegated direct.",
			} {
				if want = piStep(agent.ID, want); !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing outcome-authorization clause %q:\n%s", agent.ID, want, rendered)
				}
			}
		})
	}
}

// TestRenderRoutingMakesTheReviewKillSwitchDiscoverable guards the product
// promise that configuring an agent tells it what it may do. The kill switch is
// only real for the user if every RDD runtime can name it, so the exact command
// surface must be projected into its routing block. A runtime without
// receipt-driven development has no switch to name and receives none.
func TestRenderRoutingMakesTheReviewKillSwitchDiscoverable(t *testing.T) {
	t.Parallel()

	all := catalog.AllAgents()
	if len(all) != supportedAgentCount {
		t.Fatalf("catalog.AllAgents() returned %d agents, want %d", len(all), supportedAgentCount)
	}

	for _, agent := range all {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			if !model.SupportsReceiptDrivenDevelopment(agent.ID) {
				if strings.Contains(rendered, "gentle-ai review mode") {
					t.Fatalf("RenderRouting(%q) names the RDD switch on a runtime without RDD:\n%s", agent.ID, rendered)
				}
				return
			}

			for _, want := range []string{
				"gentle-ai review mode enable|disable|status",
				"`status` is read-only",
				"deciding source and the effective mode",
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) hides the kill switch, missing %q:\n%s", agent.ID, want, rendered)
				}
			}
		})
	}
}

// TestRenderRoutingObeysTheUserOnReviewMode asserts the specific instructional
// phrases, not merely the topic, so that prose drift which softens "run disable"
// into a negotiation, or which lets an agent switch review back on unbidden,
// fails here instead of shipping to every configured agent.
func TestRenderRoutingObeysTheUserOnReviewMode(t *testing.T) {
	t.Parallel()

	rendered, err := RenderRouting(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("RenderRouting error = %v", err)
	}

	for _, want := range []string{
		"It is **on by default and opt-out**.",
		"A `default` deciding source means nobody has chosen, so the effective mode is on.",
		"Never toggle the mode automatically",
		"When the user asks to stop using receipt-driven development, run `disable`.",
		"Do not argue, do not work around it, and do not propose alternatives first.",
		"Never enable receipt-driven development on the user's behalf unless the user explicitly asks for it.",
		"do not start reviews, do not retry, do not reactivate it, and do not fall back to any retired path",
		"reports `disabled/unmanaged`",
		"never a fabricated approval",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered routing is missing the exact instruction %q:\n%s", want, rendered)
		}
	}
}

func routingVocabularyForGuard(block, forbidden string) string {
	if forbidden == "token" {
		return routingBlockWithoutEvidenceSizes(block)
	}
	return block
}

func TestRenderRoutingOmitsRetiredRemoteControlPlaneVocabulary(t *testing.T) {
	t.Parallel()

	for _, forbidden := range retiredRemoteControlPlaneVocabulary {
		t.Run(forbidden, func(t *testing.T) {
			t.Parallel()

			needle := strings.ToLower(forbidden)
			for _, agent := range catalog.AllAgents() {
				rendered, err := RenderRouting(agent.ID)
				if err != nil {
					t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
				}
				// Evidence sizes are not retired remote authorization tokens.
				checked := routingVocabularyForGuard(rendered, forbidden)
				if strings.Contains(strings.ToLower(checked), needle) {
					t.Fatalf("RenderRouting(%q) leaks retired vocabulary %q:\n%s", agent.ID, forbidden, rendered)
				}
			}
		})
	}
}

func TestRenderRoutingIsSemanticallyEqualAcrossAgents(t *testing.T) {
	t.Parallel()

	// Routing is semantically equal across every runtime that shares an RDD
	// capability: RDD runtimes carry the review clauses, the rest do not.
	// Pi renders Gentle Shell's own ODD steps, so it forms its own group.
	type routingGroup struct{ rdd, pi bool }
	referenceAgent := map[routingGroup]model.AgentID{}
	references := map[routingGroup][]string{}

	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}

		// Each runtime declares its own identity on review commands; that is
		// the one intended difference between runtimes of the same group.
		rendered = strings.ReplaceAll(rendered, "--agent "+string(agent.ID)+" ", "--agent <runtime> ")
		semantics := routingSemantics(rendered)
		if len(semantics) == 0 {
			t.Fatalf("RenderRouting(%q) carries no routing semantics", agent.ID)
		}

		group := routingGroup{rdd: model.SupportsReceiptDrivenDevelopment(agent.ID), pi: agent.ID == model.AgentPi}
		reference := references[group]
		if reference == nil {
			referenceAgent[group] = agent.ID
			references[group] = semantics
			continue
		}
		if len(semantics) != len(reference) {
			t.Fatalf("agent %q renders %d routing facts, agent %q renders %d",
				agent.ID, len(semantics), referenceAgent[group], len(reference))
		}
		for i := range semantics {
			if semantics[i] != reference[i] {
				t.Fatalf("agent %q drifted from %q:\n got: %q\nwant: %q",
					agent.ID, referenceAgent[group], semantics[i], reference[i])
			}
		}
	}
}

func TestRenderRoutingIsDeterministic(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		first, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		second, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) second call error = %v", agent.ID, err)
		}
		if first != second {
			t.Fatalf("RenderRouting(%q) is not deterministic", agent.ID)
		}
	}
}

// TestRenderRoutingOpensWithTheODDProtocol pins Organic Driven Development
// (ODD) as the orchestrator's predefined, mandatory default workflow: the
// ordered protocol must render before topology selection, in step order, for
// every supported agent, and the reference detail section must still follow
// it rather than duplicate or replace it.
func TestRenderRoutingOpensWithTheODDProtocol(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}

			orderedSubstrings := []string{
				"Organic Driven Development (ODD) is the predefined workflow of this orchestrator",
				"### ODD protocol (MANDATORY, in this order, on every request)",
				"1. **Authorize.** First establish whether the requested outcome explicitly authorizes a change.",
				"2. **Explore.**",
				"3. **Resolve uncertainty.**",
				"4. **Classify.**",
				"5. **Track before the first write.**",
				"6. **Implement task by task.**",
				"7. **Close.**",
				"**Direct inline:**",
			}
			previous := -1
			for _, want := range orderedSubstrings {
				want = piStep(agent.ID, want)
				at := strings.Index(rendered, want)
				if at < 0 {
					t.Fatalf("RenderRouting(%q) is missing %q:\n%s", agent.ID, want, rendered)
				}
				if at <= previous {
					t.Fatalf("RenderRouting(%q) has %q out of order (at %d, previous %d):\n%s", agent.ID, want, at, previous, rendered)
				}
				previous = at
			}

			for _, want := range []string{
				"Size the task by the Task Size section below",
				"before the first source write",
				"Tell the user in one line which feature document was created and how many tasks it holds",
				"Never describe this workflow only when asked about it: run it.",
				"Resume an interrupted feature with `mem_context`",
			} {
				if want = piStep(agent.ID, want); !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing ODD default-workflow clause %q:\n%s", agent.ID, want, rendered)
				}
			}

			protocolHeading := strings.Index(rendered, "### ODD protocol")
			detailHeading := strings.Index(rendered, "### Organic Driven Development")
			if protocolHeading < 0 || detailHeading < 0 || protocolHeading >= detailHeading {
				t.Fatalf("RenderRouting(%q) must render the ODD protocol heading before the detail section:\n%s", agent.ID, rendered)
			}
		})
	}
}

// TestRenderRoutingMakesDelegationMandatory pins the ODD delegation contract
// as behavioral, not advisory: the rendered block must carry a mandatory
// trigger table (mapping, writer, preparation, long-session backstop), a
// per-task route declaration recorded in the feature document, and the
// explicit statement that executing past a fired trigger inline is a routing
// defect. Without this, the permissive "smallest useful topology" framing
// wins and the orchestrator executes everything inline.
func TestRenderRoutingMakesDelegationMandatory(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}

			for _, want := range []string{
				// pain-point-iteration S9 C4: a trigger is mandatory only once its
				// condition truly holds; none holds by default, inline when in doubt.
				"### Delegation Triggers",
				"Each trigger holds only when its condition truly holds, and only then is it mandatory, not advisory; no trigger holds by default, and when in doubt work inline.",
				"stop and delegate through the runtime's subagent mechanism",
				"executing past a fired trigger inline is a routing defect",
				"**Mapping trigger:** when understanding exceeds the inline batch budget",
				"**Writer trigger:** a writer is delegated only for a named reason",
				"file count never fires this trigger",
				"**Preparation trigger:**",
				"**Long-session backstop:**",
				"pause and delegate the next bounded unit",
				"**Route declaration:**",
				"record the chosen route per task",
				"so skipped delegation is observable instead of silent",
				"These triggers only choose between direct inline and delegated direct inside the organic flow",
				"honoring its delegation triggers when their conditions truly hold",
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing mandatory delegation clause %q:\n%s", agent.ID, want, rendered)
				}
			}
		})
	}
}

func TestRenderRoutingRejectsUnregisteredAgent(t *testing.T) {
	t.Parallel()

	rendered, err := RenderRouting(model.AgentID("totally-unregistered-agent"))
	if err == nil {
		t.Fatalf("RenderRouting accepted an unregistered agent and rendered:\n%s", rendered)
	}
	if !errors.Is(err, capabilitymanifest.ErrUnsupportedAgent) {
		t.Fatalf("RenderRouting error = %v, want it to wrap ErrUnsupportedAgent", err)
	}
	if rendered != "" {
		t.Fatalf("RenderRouting invented guidance for an unregistered agent:\n%s", rendered)
	}
}

// routingSemantics strips markdown formatting so two adapters may present the
// same routing facts differently without the parity check reading formatting
// as a semantic difference.
func routingSemantics(rendered string) []string {
	var facts []string
	for _, line := range strings.Split(rendered, "\n") {
		stripped := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
				return unicode.ToLower(r)
			}
			return -1
		}, line)
		stripped = strings.Join(strings.Fields(stripped), " ")
		if stripped != "" {
			facts = append(facts, stripped)
		}
	}
	return facts
}

// The user-facing usage guide describes the same spec-by-reference contract the
// canonical routing renders, so it cannot drift silently (review follow-up).
func TestUsageGuideDescribesTheSpecByReferenceFeatureDocument(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "usage.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, clause := range []string{
		"`## Specs`",
		"`## Tasks`",
		"`## Log`",
		"exact strings, error messages, and examples verbatim",
		"`L1` is your original request verbatim",
		"A requirement change rewrites only the affected spec and reopens only its task",
		"never a paraphrase of your request",
		"runs the spec's examples you authorized, against isolated state when they mutate data",
		"reproduced before anyone decides it already works",
	} {
		if !strings.Contains(string(body), clause) {
			t.Errorf("docs/usage.md is missing %q", clause)
		}
	}
}

// gentle-shell#1494: task size is decided by understanding, contained risk,
// and whether the work can be resumed from the diff, never by counting files,
// commands, fixes, or a requested todo list. Each mechanism turns on only by
// its own trigger, and the agent may raise assess risk but never lower it.
func TestRenderRoutingSizesTasksByUnderstandingRiskAndResumability(t *testing.T) {
	t.Parallel()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, want := range []string{
				"### Task Size",
				"**Understood**",
				"no product or design decision is open",
				"**Contained risk**",
				"**Resumable from the diff**",
				"the original request and `git diff` alone",
				"A task is **large** only when the resume test fails",
				"The number of files, commands or tests, fixes, or a requested todo list never decides size",
				"run the focused test and the suite inline, once each",
				"observe RED inline before the fix",
				"No explore, worker, or verifier",
				"**High risk**",
				"(1) data or irreversible effects",
				"(6) no test would catch a regression",
				"only when a bounded look cannot tell whether (1)-(5) apply",
				"**Ask trigger:**",
				"**Verification trigger:**",
				"**Tracking trigger:**",
				"file count never fires this trigger",
				"re-evaluate task size",
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing task-size clause %q", agent.ID, want)
				}
			}
			for _, retired := range []string{
				"two or more meaningful implementation steps",
				"non-trivial files",
				"Substantial means coordinated steps",
			} {
				if strings.Contains(rendered, retired) {
					t.Fatalf("RenderRouting(%q) keeps retired count-based sizing %q", agent.ID, retired)
				}
			}
			if model.SupportsReceiptDrivenDevelopment(agent.ID) {
				for _, want := range []string{
					"--escalate-item <1-6> --escalate-reason",
					"never lower it",
					// pain-point-iteration S9 C1: the native tier decides native
					// review only; it no longer "wins" for the independent verifier.
					"When RDD is on, the native assess tier decides native review and you may only raise it; it never adds an independent verifier by itself, only a named high-risk item does.",
				} {
					if !strings.Contains(rendered, want) {
						t.Fatalf("RenderRouting(%q) is missing agent escalation clause %q", agent.ID, want)
					}
				}
			}
			if strings.Contains(rendered, "that tier wins") {
				t.Fatalf("RenderRouting(%q) keeps the retired native-tier-wins clause", agent.ID)
			}
		})
	}
}

// gentle-shell#1731: a writer is delegated for a named reason, never for size;
// parallel writers stay safe without a single-writer ban; every code change
// closes with a risk line; a tracked task never pauses on hedged wording; and
// exploration exists only to decide or route.
func TestRenderRoutingDelegatesForReason(t *testing.T) {
	t.Parallel()

	specs := []struct {
		name     string
		required []string
		retired  []string
	}{
		{"S1 writer by reason", []string{
			"**Writer trigger:** a writer is delegated only for a named reason",
			// pain-point-iteration S9 C4: a unit is an independent behavior with
			// several minutes of work, never a test/doc/code split of one behavior.
			"(a) parallelism: two or more independent units of behavior with disjoint edit surfaces, each needing several minutes of work (several files or roughly 100+ changed lines; a subagent spends one to two minutes reading before it writes), launched together",
			"A unit is one independent piece of behavior with its code, tests, and docs together: never split one behavior into test, docs, and code writers, and a delivery that depends on another is not independent; several small independent changes run inline, one after another.",
			"(b) context: the long-session backstop below",
			"Never delegate a writer for size, a large task alone, file count, or a price ratio",
			"Without a reason, the parent works inline, following its logbook",
			"large tasks without a Writer trigger reason, which follow their logbook",
		}, []string{
			"a large task delegates one writer per task",
			"a large task delegates one bounded writer per task",
			"each clearly heavier than starting a subagent",
		}},
		{"S2 parallel writers", []string{
			"**Parallel writers:** work in another repository goes in a fresh worktree based on that repository's main",
			"Parallel units in the same local repository run in the same tree, each with declared disjoint edit surfaces",
			"the parent owns git",
			"writers run no repository-wide formatters, generators, or installs",
			"a shared file has one owner or the parent edits it at the end",
			"the parent checks each writer's diff against its surface before accepting it",
			"one seam check runs at the end",
			"Units that need the same file use isolated worktrees",
		}, []string{
			"single writer",
			"single-threaded",
		}},
		{"S3 risk line", []string{
			"**Risk line**: close every code change, small path or delegated, with `Risk: item N (reason)` or `Risk: none`",
			"(3) contracts others consume: changing or removing public API, CLI flags, config formats, published exports, or mirrored prompts or contracts others already consume (not adding a flag, command, or optional field; requested changes are not high risk by themselves; unrequested breaks in shared code are)",
			"Before writing `Risk: none`, check whether the diff changes code that existing behavior the request did not mention also uses",
		}, []string{
			"deleting or rewriting persisted data",
		}},
		{"S4 no self-pause", []string{
			"Hedged wording is not a stop",
			"A tracked task ends pending only with the user's explicit stop or one **Needs your decision** result naming the open blockers or missing proof",
		}, nil},
		{"S5 explore only to decide or route", []string{
			"Explore only for a map you need to decide or route, never to read files you will read anyway before writing inline",
			"when the Writer trigger delegates a write, reading that prepares it",
			"Inline writes read inline",
		}, []string{
			"On a small task the parent reads and writes inline",
		}},
		{"S6 test discipline", []string{
			"Write one RED test per requested rule",
			"For every existing command or option the change touches, add one test proving its previous behavior still holds",
			"add no other cases",
			"An existing behavior counts as touched when it shares the code you changed (options, parsers, helpers, validation)",
			// pain-point-iteration S9 C8: the differential check compares old
			// and new output of the commands that share the changed code.
			"Before finishing, run a differential check: for each existing command that shares the code you changed, run it with the new or changed input (for example a new option passed to a sibling subcommand) against the previous code (a copy of the base commit in a `mktemp -d` scratch directory, never committed as a test; committed tests never read git history or a fixed commit) and against your change, and compare stdout, stderr, and exit code; any difference nobody asked for is a regression to fix and pin with a test.",
			"Each test asserts every observable effect of the rule (output, exit code, persisted data), covers the cases the rule itself names, and goes through the public interface",
			"When you add or change a command, option, or message, update the help text and docs",
		}, []string{
			"configured TDD mode",
			"strict TDD is active",
		}},
		// pain-point-iteration S9 C3: parallel writers cannot see each other,
		// so each writer prompt names the helpers that keep shared invariants.
		{"S9 shared-state invariants", []string{
			"**Invariants:** each writer prompt names the existing helpers its unit must reuse for shared state (for example the function that adds an entry other commands also add) and the invariants they keep (ordering, uniqueness, validation, normalization)",
			"Whoever writes, inline or delegated, reuses the existing function for shared state and never re-implements that step with a different shape.",
		}, nil},
	}

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}
			for _, spec := range specs {
				for _, want := range spec.required {
					if want = piStep(agent.ID, want); !strings.Contains(rendered, want) {
						t.Errorf("%s: RenderRouting(%q) is missing %q", spec.name, agent.ID, want)
					}
				}
				for _, retired := range spec.retired {
					if strings.Contains(rendered, retired) {
						t.Errorf("%s: RenderRouting(%q) keeps retired %q", spec.name, agent.ID, retired)
					}
				}
			}
		})
	}
}

// weak-model-economy S4 (Log L4 compact wording): high-risk item 1 covers only
// data that cannot be rebuilt; a dev or test database a seed or backup rebuilds
// is ordinary work, so it never fires the Verification trigger.
func TestRenderRoutingHighRiskItemOneCoversOnlyUnrebuildableData(t *testing.T) {
	t.Parallel()

	const itemOne = "(1) data or irreversible effects that cannot be rebuilt: migrations, changing or deleting existing stored data, persisted format changes (not saving new records, nor a dev or test database a seed or backup rebuilds);"
	const retired = "(1) data or irreversible effects: migrations, changing or deleting existing stored data, persisted format changes (not saving new records)"
	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		if !strings.Contains(rendered, itemOne) {
			t.Errorf("RenderRouting(%q) is missing high-risk item 1 %q", agent.ID, itemOne)
		}
		if strings.Contains(rendered, retired) {
			t.Errorf("RenderRouting(%q) keeps retired high-risk item 1 %q", agent.ID, retired)
		}
	}
}

// S15 (pain-point iteration): a bench run read two wishes in one message as separate deliverables and started
// two writers for ~100-line units (USD 0.55 vs 0.20 inline); blind review found delivered tests pinning a fixed git
// commit from the differential check. Every runtime gets the three corrections.
func TestRenderRoutingKeepsSmallTasksInlineAndDifferentialChecksInScratch(t *testing.T) {
	t.Parallel()

	rules := []string{
		"separate deliverables the user wants apart (never several features asked at once)",
		"Units of about 100 changed lines or less always run inline: writing them costs less than starting workers.",
		"a copy of the base commit in a `mktemp -d` scratch directory, never committed as a test; committed tests never read git history or a fixed commit",
	}
	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range rules {
			if !strings.Contains(rendered, rule) {
				t.Errorf("%s routing is missing %q", agent.ID, rule)
			}
		}
		if strings.Contains(rendered, "(`git stash` or a copy of HEAD)") {
			t.Errorf("%s routing still suggests git stash or a HEAD copy for the differential check", agent.ID)
		}
	}
}

// Pi's ODD steps carry Gentle Shell's own clauses (the visible `todo`
// projection, routing through the orchestrator's Mechanisms, phase reporting,
// and its asset pointers). gentle-ai renders them so Gentle Shell consumes one
// source instead of adapting the canon by hand; no other runtime receives them.
func TestRenderRoutingGivesPiItsOwnODDSteps(t *testing.T) {
	t.Parallel()

	piClauses := []string{
		"then create or rebuild the visible `todo` list from the reconciled feature tasks",
		"Route each task through the orchestrator's Mechanisms",
		"update the file, mirror, and visible `todo` projection after every task transition and material plan change",
		"Never end with a tracked task pending unless you quote the user's explicit stop.",
		"native review runs only under the user-owned RDD switch.",
		"Phase reporting: the Gentle Shell prompt label is inferred automatically",
		"Detail for steps 3–7: `orchestrator-delegation.md` and `orchestrator-memory.md`.",
	}
	// The hedged-wording rule lives in Pi's step 1 and in the canonical step 7;
	// each runtime must carry it exactly once.
	const hedged = "they may stop you or resume later asks for notes and separate commits"

	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, clause := range piClauses {
			if got, want := strings.Contains(rendered, clause), agent.ID == model.AgentPi; got != want {
				t.Errorf("%s routing contains %q = %t, want %t", agent.ID, clause, got, want)
			}
		}
		if count := strings.Count(rendered, hedged); count != 1 {
			t.Errorf("%s routing carries the hedged-wording rule %d times, want 1", agent.ID, count)
		}
	}
}

// piStepWording maps a canonical ODD step clause to the wording of the same
// rule in Pi's own steps (piODDSteps), so the clause tests above hold every
// runtime to the rule while Pi keeps Gentle Shell's phrasing.
var piStepWording = map[string]string{
	"1. **Authorize.** First establish whether the requested outcome explicitly authorizes a change.":                      "1. **Authorize.** Investigation, explanation, review, comparison, and proposal-only requests stay read-only",
	"First establish whether the requested outcome explicitly authorizes a change.":                                        "Investigation, explanation, review, comparison, and proposal-only requests stay read-only",
	"Investigation, explanation, review, audit, comparison, and solution-proposal or planning-only requests are read-only": "Investigation, explanation, review, comparison, and proposal-only requests stay read-only",
	"must not write or edit files, delegate a writer, invoke apply, or create implementation artifacts":                    "no writer, apply, or implementation artifacts",
	"If change intent is ambiguous or conditional, ask one clarification and remain read-only until answered.":             "Ambiguous or conditional change intent (unclear whether a change is authorized at all) gets one clarification; stop and wait.",
	"Size the task by the Task Size section below":                                                                         "Size the task by the orchestrator's Task Size section",
	"push, pull request creation, and merge remain the user's decisions under ordinary repository policy":                  "push, pull request creation, and merge remain the user's decisions.",
	"Hedged wording is not a stop": "that result is a valid stop, hedged wording is not",
	"A tracked task ends pending only with the user's explicit stop or one **Needs your decision** result naming the open blockers or missing proof":                           "Never end with a tracked task pending unless you quote the user's explicit stop. An applicable quick check runs once; an unavailable verifier or subagent is reported as unavailable, never retried or escalated into extra ceremony. Partial, blocked, unavailable, or exhausted proof becomes one **Needs your decision** result naming the open blockers or missing proof",
	"Explore only for a map you need to decide or route, never to read files you will read anyway before writing inline":                                                       "Do not delegate exploration of files you will read anyway to work inline; explore only for a map you need to decide or route.",
	"Each test asserts every observable effect of the rule (output, exit code, persisted data), covers the cases the rule itself names, and goes through the public interface": "Each test asserts every observable effect of the rule it covers (exit code, exact stdout and stderr, and that rejected input leaves stored data and counters unchanged), covers the cases the rule itself names (its examples, boundaries, and errors), and checks through the public interface, never internal storage.",
}

func piStep(agent model.AgentID, clause string) string {
	if pi, ok := piStepWording[clause]; ok && agent == model.AgentPi {
		return pi
	}
	return clause
}
