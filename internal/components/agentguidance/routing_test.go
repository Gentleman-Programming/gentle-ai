package agentguidance

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v3/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
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

// supportedAgentCount guards the catalog itself: routing is unconditional for
// every supported adapter, so a silently shrinking catalog must fail here
// instead of quietly reducing coverage of the table-driven tests below.
const supportedAgentCount = 16

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
				fmt.Sprintf("%d–%d files", routing.DirectInline.MinUnderstandingFiles, routing.DirectInline.MaxUnderstandingFiles),
				fmt.Sprintf("%d+ files", routing.DelegatedDirect.MappingMinUnderstandingFiles),
				fmt.Sprintf("%d+ non-trivial files", routing.DelegatedDirect.WriterMinNonTrivialFiles),
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
		{"authorized substantial work", []string{
			"Explore the existing code and requirements first",
			"For substantial authorized implementation, automatically create",
			"without a task or storage permission prompt",
			"Small, understood work creates no durable task artifacts",
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
			"stable task IDs, authorized scope, acceptance criteria, and applicable checks",
			"Reuse the same feature identity; never overwrite another feature",
		}},
		{"unified intent and implementation handoff", []string{
			"one feature document, not a separate plan file or topic",
			"objective, problem, why, scope, constraints",
			"progress, verification evidence, and next step",
			"concise rationale for meaningful accepted changes",
			"Routine corrections stay with their tasks; no exhaustive decision journal",
			"Accepted user, review, or verification changes",
			"add genuinely new tasks or reopen invalidated items with a reason",
			"Findings alone never authorize scope expansion or automatic acceptance",
			"Before implementation or resume, the parent reads both the actual file and full observation",
			"passes the locator and relevant context; workers read the document before edits",
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
			"Checkboxes grant no approval or receipt",
		}},
		{"advisory coherent task size", []string{
			"Use about 400 authored changed lines per ODD task only as a planning heuristic, counting additions plus deletions",
			"smallest coherent behavior with its tests and docs",
			"not a task acceptance criterion, hard cap, counter-trigger, automatic stop, forced split, or RDD trigger",
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
			"The native RDD refuter owns native review claims; never duplicate or bypass it",
		}},
		{"spend-proportional implementation and verification", []string{
			"Validate consequential premises against available evidence before building",
			"reuse relevant sibling investigation instead of repeating it",
			"Run focused checks during iteration and all applicable full checks at task closure",
			"without a hard spend or line gate",
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
			"gentle-ai review assess --cwd <repo> --json",
			"Passive/low uses silent structural checks with no reviewer or consent ceremony",
			"Medium/high relays the existing candidate consent",
			"native review runs only on grant",
			"a decline continues under ordinary policy",
			"Do not substitute model judgment, task size, or defect severity for prospective candidate risk",
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
			guard := strings.Index(rendered, "First establish whether the requested outcome explicitly authorizes a change.")
			organic := strings.Index(rendered, "### Organic Driven Development")
			if guard < 0 || organic <= guard {
				t.Fatal("ODD must follow the mutation-authorization guard")
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					for _, clause := range tt.clauses {
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
// work-unit commit contract: every task closes with a commit on the feature
// branch, and the native review candidate is that commit or the PR slice it
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
		{"work-unit commit closes each task", []string{
			"closes with at least one work-unit commit on the feature branch",
			"branch first when on the default branch",
			"tests and docs alongside the behavior",
			"using a Conventional Commit message",
			"record the commit identity in the feature document as evidence",
			"Work-unit commits on the feature branch are part of authorized substantial ODD implementation",
			"push, pull request creation, and merge remain the user's decisions under ordinary repository policy",
		}},
		{"native review candidate is a commit or a slice, never the checkbox or branch", []string{
			"The native review candidate is a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch",
		}},
		{"assess each commit against the last reviewed boundary", []string{
			"Run applicable functional checks per task, not a review cycle per TODO checkbox",
			"run `gentle-ai review assess --cwd <repo> --agent <runtime> --base-ref <last reviewed boundary> --committed-only --json` on that commit and read `review_due` and `review_due_reason`",
		}},
		{"a due assessment hands over the exact preflight transition", []string{
			"When `review_due` is true (`high_risk`, or `slice_budget_reached` for a medium range that reached the delivery budget of about 400 authored changed lines), execute the returned `next_transition.command` verbatim",
			"it is the exact preflight STATUS for the same `--base-ref`/`--committed-only` selectors",
			"the reviewed boundary advances to this commit once that review is acknowledged",
		}},
		{"a non-due assessment records its reason and continues", []string{
			"When `review_due` is false, record `review_due_reason` and continue: `passive` needs no review and the boundary advances",
			"`under_budget` stays pending in the slice until a later commit reaches the budget",
			"`already_reviewed` means this exact range is already covered by terminal authority",
		}},
		{"boundaries advance and outcomes are recorded per task", []string{
			"The first boundary is the branch point, and every reviewed boundary becomes the next base",
			"Record per task the assessed tier and outcome: granted, declined, passive, under budget, already reviewed, or unavailable",
			"An unavailable or failed assessment never lowers the tier: treat the commit as due and run the preflight STATUS with `--base-ref <last reviewed boundary> --committed-only`",
			"Never infer low risk from a failed assessment",
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

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					for _, clause := range tt.clauses {
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

			guard := "First establish whether the requested outcome explicitly authorizes a change."
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
				if !strings.Contains(rendered, want) {
					t.Fatalf("RenderRouting(%q) is missing outcome-authorization clause %q:\n%s", agent.ID, want, rendered)
				}
			}
		})
	}
}

// TestRenderRoutingMakesTheReviewKillSwitchDiscoverable guards the product
// promise that configuring an agent tells it what it may do. The kill switch is
// only real for the user if every configured agent can name it, so the exact
// command surface must be projected into the unconditional routing block.
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
				if strings.Contains(strings.ToLower(rendered), needle) {
					t.Fatalf("RenderRouting(%q) leaks retired vocabulary %q:\n%s", agent.ID, forbidden, rendered)
				}
			}
		})
	}
}

func TestRenderRoutingIsSemanticallyEqualAcrossAgents(t *testing.T) {
	t.Parallel()

	var (
		referenceAgent model.AgentID
		reference      []string
	)

	for _, agent := range catalog.AllAgents() {
		rendered, err := RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}

		semantics := routingSemantics(rendered)
		if len(semantics) == 0 {
			t.Fatalf("RenderRouting(%q) carries no routing semantics", agent.ID)
		}

		if reference == nil {
			referenceAgent = agent.ID
			reference = semantics
			continue
		}
		if len(semantics) != len(reference) {
			t.Fatalf("agent %q renders %d routing facts, agent %q renders %d",
				agent.ID, len(semantics), referenceAgent, len(reference))
		}
		for i := range semantics {
			if semantics[i] != reference[i] {
				t.Fatalf("agent %q drifted from %q:\n got: %q\nwant: %q",
					agent.ID, referenceAgent, semantics[i], reference[i])
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
				"two or more meaningful implementation steps",
				"before the first source write",
				"Tell the user in one line which feature document was created and how many tasks it holds",
				"Never describe this workflow only when asked about it: run it.",
				"Resume an interrupted feature with `mem_context`",
			} {
				if !strings.Contains(rendered, want) {
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

	routing := capabilitymanifest.CanonicalImplementationRouting()

	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderRouting(agent.ID)
			if err != nil {
				t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
			}

			for _, want := range []string{
				"### Mandatory Delegation Triggers",
				"These triggers are mandatory, not advisory",
				"stop and delegate through the runtime's subagent mechanism",
				"executing past a fired trigger inline is a routing defect",
				fmt.Sprintf("**Mapping trigger:** when understanding the work requires %d or more files", routing.DelegatedDirect.MappingMinUnderstandingFiles),
				fmt.Sprintf("**Writer trigger:** when implementation touches %d or more non-trivial files", routing.DelegatedDirect.WriterMinNonTrivialFiles),
				"A mechanical second-file edit does not fire this trigger solely because an earlier file was touched; count non-trivial files in the current work",
				"**Preparation trigger:**",
				"**Long-session backstop:**",
				"pause and delegate the next bounded unit of work",
				"**Route declaration:**",
				"record the chosen route per task",
				"so skipped delegation is observable instead of silent",
				"These triggers only choose between direct inline and delegated direct inside the organic flow",
				"honoring its mandatory delegation triggers",
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
