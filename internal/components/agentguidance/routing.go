// Package agentguidance owns the always-installed organic routing guidance
// projected into every configured agent. Routing is unconditional: it must not
// depend on the optional SDD component, on SDD mode, or on any SDD asset being
// present, because an agent without routing guidance has no way to choose
// between direct, delegated, and proposed work.
package agentguidance

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// ErrUnknownRoutingPolicy fails closed when the canonical manifest carries a
// selection policy this renderer was not written for. Rendering stale prose
// over a changed policy would silently misinform every installed agent.
var ErrUnknownRoutingPolicy = errors.New("unknown sdd selection policy")

// RenderRouting projects the canonical implementation-routing facts of one
// supported adapter into its guidance block.
//
// The rendered block deliberately carries routing semantics only. It states no
// runtime observation, issues no lifecycle authority, and activates no remote
// mechanism, so an offline agent reading it can still route work correctly.
func RenderRouting(agent model.AgentID) (string, error) {
	manifest, err := capabilitymanifest.ForAgent(agent)
	if err != nil {
		return "", fmt.Errorf("render routing guidance for %q: %w", agent, err)
	}

	routing := manifest.ImplementationRouting
	if routing.SDD.SelectionPolicy != capabilitymanifest.SDDSelectionExplicitRequestOrAcceptedProposal {
		return "", fmt.Errorf("%w: %q", ErrUnknownRoutingPolicy, routing.SDD.SelectionPolicy)
	}

	var output strings.Builder
	output.WriteString("## Implementation Routing\n\n")
	output.WriteString("Organic Driven Development (ODD) is the predefined workflow of this orchestrator. Every request enters it, on every runtime, without the user asking for a workflow, a plan, or task tracking. SDD is a branch inside ODD, entered only by an explicit request or an accepted proposal. Never describe this workflow only when asked about it: run it.\n\n")
	output.WriteString("### ODD protocol (MANDATORY, in this order, on every request)\n\n")
	output.WriteString("1. **Authorize.** First establish whether the requested outcome explicitly authorizes a change. Investigation, explanation, review, audit, comparison, and solution-proposal or planning-only requests are read-only unless the user explicitly requests implementation or another mutation.\n")
	output.WriteString("   - Read-only work may inspect, explain, compare, and recommend, but must not write or edit files, delegate a writer, invoke apply, or create implementation artifacts.\n")
	output.WriteString("   - If change intent is ambiguous or conditional, ask one clarification and remain read-only until answered.\n")
	output.WriteString("2. **Explore.** Explore the existing code and requirements first, proportionately to the request, before proposing or writing anything.\n")
	output.WriteString("3. **Resolve uncertainty.** Recommend optional research only for a named uncertainty; ask one focused user question only for a real unresolved product decision, then stop and wait; use at most one scoped read-only assumption challenge for a high-consequence unproven premise.\n")
	output.WriteString("4. **Classify.** The work is substantial when exploration yields two or more meaningful implementation steps, or progress worth recovering after an interruption. Small, understood work stays small and creates no durable task artifacts.\n")
	output.WriteString("5. **Track before the first write.** For substantial authorized implementation, create `odd/tasks/<feature-name>.md` and its Engram mirror `odd/<feature-name>/tasks` automatically, before the first source write, without asking permission for tasks or storage. Tell the user in one line which feature document was created and how many tasks it holds.\n")
	output.WriteString("6. **Implement task by task.** Route each task through the smallest useful topology below, honoring its mandatory delegation triggers, with the configured TDD mode and applicable checks. Check an item off only after its outcome and checks were observed; update the file and the mirror after each task. Every task closes with at least one work-unit commit on the feature branch, branch first when on the default branch, with tests and docs alongside the behavior, using a Conventional Commit message; record the commit identity in the feature document as evidence. Work-unit commits on the feature branch are part of authorized substantial ODD implementation; push, pull request creation, and merge remain the user's decisions under ordinary repository policy.\n")
	output.WriteString("7. **Close.** Report the verified outcome, every failed, skipped, or pending check, and the next step. The native review candidate is a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch; native review runs only under the user-owned receipt-driven development switch.\n\n")
	output.WriteString("Resume an interrupted feature with `mem_context`, then project- and feature-scoped `mem_search`, then `mem_get_observation` for the full document, then the task file itself; reconcile before continuing the next unfinished task.\n\n")
	output.WriteString("After explicit change intent is established, route work for the requested outcome with the smallest useful topology. Every authorized change takes exactly one implementation route: direct inline, delegated direct, or optional SDD.\n\n")

	_, _ = fmt.Fprintf(
		&output,
		"- **Direct inline:** decide or verify from %d–%d files inline. Keep one mechanical, already-understood file change inline only when it needs no research and has no unresolved design decision.\n",
		routing.DirectInline.MinUnderstandingFiles,
		routing.DirectInline.MaxUnderstandingFiles,
	)
	_, _ = fmt.Fprintf(
		&output,
		"- **Delegated direct:** delegate one narrow exploration when understanding needs %d+ files; delegate one writer for %d+ non-trivial files. Reading that prepares a write and broad research also delegate.\n",
		routing.DelegatedDirect.MappingMinUnderstandingFiles,
		routing.DelegatedDirect.WriterMinNonTrivialFiles,
	)
	output.WriteString("- **Optional SDD:** retain explicitly selected SDD workflows. SDD is selected only by an explicit request or an accepted proposal. Do not recommend SDD merely to resolve ambiguity; use the organic flow below.\n")
	output.WriteString("- File count, changed lines, size, or perceived risk alone never selects SDD and never forces a heavier route.\n")
	output.WriteString("- Automatic SDD pace is not mutation authorization; once implementation is explicitly authorized, it continues under the selected route.\n")
	output.WriteString("- These are implementation routes, not a ban on per-action delegation. Tests, builds, installs, and review actors may still use fresh workers without changing the selected route.\n")
	output.WriteString("- Direct and delegated work never create SDD artifacts, prompts, phase attempts, or synthetic SDD runs.\n")

	// Mandatory delegation triggers: delegation must be behavioral, not
	// advisory. Rendered immediately after the route descriptions so the
	// trigger table competes with the permissive "smallest useful topology"
	// framing instead of being buried pages away from it.
	output.WriteString("\n### Mandatory Delegation Triggers\n\n")
	output.WriteString("These triggers are mandatory, not advisory. When one fires, stop and delegate through the runtime's subagent mechanism before continuing; executing past a fired trigger inline is a routing defect even if the work succeeds. Delegation keeps the parent context thin enough to orchestrate; it does not slow the work down.\n\n")
	_, _ = fmt.Fprintf(&output, "- **Mapping trigger:** when understanding the work requires %d or more files, delegate one narrow exploration or mapping task before deciding or writing anything.\n", routing.DelegatedDirect.MappingMinUnderstandingFiles)
	_, _ = fmt.Fprintf(&output, "- **Writer trigger:** when implementation touches %d or more non-trivial files, delegate one bounded writer instead of editing them inline. A mechanical second-file edit does not fire this trigger solely because an earlier file was touched; count non-trivial files in the current work.\n", routing.DelegatedDirect.WriterMinNonTrivialFiles)
	output.WriteString("- **Preparation trigger:** reading that prepares a write, and broad research or context compression, delegate together with or ahead of the write instead of filling the parent context.\n")
	output.WriteString("- **Long-session backstop:** after about 20 tool calls, 5 exploratory reads, or 2 non-mechanical edits without any delegation, pause and delegate the next bounded unit of work.\n")
	output.WriteString("- **Route declaration:** for substantial work, record the chosen route per task (inline or delegated) and the trigger evidence in the feature document, so skipped delegation is observable instead of silent.\n")
	output.WriteString("- These triggers never select SDD and never create SDD artifacts; they only choose between direct inline and delegated direct inside the organic flow.\n")

	output.WriteString("\n### Organic Driven Development\n\n")
	output.WriteString("Use this flow for direct and delegated organic work, not explicitly selected SDD. Explore the existing code and requirements first, proportionately to the request; the mutation-authorization guard above still applies.\n\n")
	output.WriteString("This section is the reference detail for the protocol above.\n\n")
	output.WriteString("- Recommend optional research only for a named uncertainty. If declined, continue within authorized scope only where safe without the missing evidence; disclose unresolved uncertainty and pause affected unsafe decisions. Offer a concise proposal only when a real scope or product decision needs it. Neither research nor a proposal is mandatory.\n")
	output.WriteString("- Establish the problem, intended outcome, constraints, and current evidence; inspect relevant code. Adapt depth to uncertainty and consequence, not a fixed questionnaire or mandatory rounds. The parent owns product decisions: ask one focused user question only for a real unresolved product decision, then stop and wait. Workers return gaps to the parent rather than assuming choices.\n")
	output.WriteString("- When the question needs external evidence, use available authorized documentation/web tools and prefer primary sources. Attribute material claims to source URLs or code locations; distinguish verified facts, assumptions, contradictions, freshness, and gaps. If tools are unavailable, disclose limitations without inventing access or evidence; pause only unsafe decisions dependent on missing evidence.\n")
	output.WriteString("- Return concise findings, recommendation, tradeoffs, open questions, and implementation implications. When delegating research, forward these research instructions to a fresh general exploration/research worker through existing delegation; do not create a specialized agent or invoke sdd-research. Keep research read-only; its findings do not authorize implementation or require new persistence or readiness machinery.\n")
	output.WriteString("- Use at most one scoped independent read-only assumption challenge for a high-consequence unproven premise, even in a small security-critical change. Name the premise, evidence, and consequence; do not start a debate loop. Deterministic failures need fixes, not model debate. The native RDD refuter owns native review claims; never duplicate or bypass it.\n")
	output.WriteString("- Validate consequential premises against available evidence before building; reuse relevant sibling investigation instead of repeating it. Run focused checks during iteration and all applicable full checks at task closure. Keep spend proportional without a hard spend or line gate; preserve configured TDD, native RDD, safety, and consent requirements.\n")
	output.WriteString("- Small, understood work creates no durable task artifacts. Substantial means coordinated steps or progress worth recovering, not a line-count threshold. For substantial authorized implementation, automatically create the feature document after exploration, without a task or storage permission prompt.\n")
	output.WriteString("- Use about 400 authored changed lines per ODD task only as a planning heuristic, counting additions plus deletions; prefer the smallest coherent behavior with its tests and docs. This is not a task acceptance criterion, hard cap, counter-trigger, automatic stop, forced split, or RDD trigger. If the correct, clear solution naturally exceeds it, briefly explain why and continue without size-only rework loops. Never delete spaces, blank lines, or comments for cosmetic line savings; never omit tests, minify, add gratuitous abstractions, or split artificially to fit the heuristic. Forward this same advisory-only instruction when delegating tasks to subagents. The delivery budget below reads the accumulated branch, not this per-task heuristic. Existing PR size gates remain unchanged; continue under existing repository policy.\n")
	output.WriteString("- Keep `odd/tasks/<feature-name>.md` and an Engram recovery copy under topic `odd/<feature-name>/tasks`, scoped to the current project. Use a descriptive filename-safe feature name. Reuse the same feature identity; never overwrite another feature. Keep one feature document, not a separate plan file or topic: objective, problem, why, scope, constraints, actionable checklist with stable task IDs, authorized scope, acceptance criteria, and applicable checks. Include progress, verification evidence, and next step, plus concise rationale for meaningful accepted changes. Routine corrections stay with their tasks; no exhaustive decision journal. Mirror the full current document and repository-relative file locator, not just a summary or completion notice.\n")
	output.WriteString("- Accepted user, review, or verification changes automatically update affected intent and TODOs: preserve valid completed and unrelated work; add genuinely new tasks or reopen invalidated items with a reason, and revise their checks. Findings alone never authorize scope expansion or automatic acceptance. Business scope changes still require user authorization. Check off only observed outcomes with applicable proof; record failed, unavailable, skipped, or pending checks honestly. Checkboxes grant no approval or receipt.\n")
	output.WriteString("- Read back both writes; they are not atomic. If Engram is unavailable, preserve local progress and explicitly mark the mirror pending; do not claim success or block unrelated safe work. Resynchronize when available. If a file write is unsafe or unavailable, preserve existing state and report the limitation. Preserve both versions on irreconcilable edits and ask only about the real conflict.\n")
	output.WriteString("- On resume, use `mem_context`, then `mem_search` scoped to the current project and feature, and `mem_get_observation` for the full saved document; read the actual task file. Do not infer active work from the newest global memory. Reconcile current requirements, code, and proof before resuming the next unfinished task; preserve pending mirrors and conflicting edits.\n")
	output.WriteString("- Before implementation or resume, the parent reads both the actual file and full observation, reconciles them, and passes the locator and relevant context; workers read the document before edits. Small work without a document still receives its authorized scope and checks.\n")
	output.WriteString("- Resolve effective TDD on/off from existing project/session configuration or explicit user choice; retain its source and exact test runner. Record resolved mode, source, and runner in the feature document when present. Tests or frameworks being present does not enable TDD. Forward mode, source, and runner on every implementation delegation; refresh on resume. When enabled, require observed RED before implementation, GREEN, then REFACTOR; never invent evidence. When disabled, run ordinary functional checks, not no checks. If mode is unknown/conflicting or the runner is missing, disclose and resolve only the ambiguity affecting the next action; never invent precedence or a command, and never invoke sdd-init to determine ODD TDD.\n")
	output.WriteString("- Preserve existing native risk selection and applicable functional verification. Run applicable functional checks per task, not a review cycle per TODO checkbox. The native review candidate is a work-unit commit or a PR slice, never a TODO checkbox and never the accumulated feature branch. After each work-unit commit, when RDD is enabled, run `gentle-ai review assess --cwd <repo> --agent <runtime> --base-ref <last reviewed boundary> --committed-only --json` on that commit and read `review_due` and `review_due_reason`. When `review_due` is true (`high_risk`, or `slice_budget_reached` for a medium range that reached the delivery budget of about 400 authored changed lines), execute the returned `next_transition.command` verbatim: it is the exact preflight STATUS for the same `--base-ref`/`--committed-only` selectors; follow the transitions it returns, and the reviewed boundary advances to this commit once that review is acknowledged. When `review_due` is false, record `review_due_reason` and continue: `passive` needs no review and the boundary advances; `under_budget` stays pending in the slice until a later commit reaches the budget; `already_reviewed` means this exact range is already covered by terminal authority. The first boundary is the branch point, and every reviewed boundary becomes the next base. Record per task the assessed tier and outcome: granted, declined, passive, under budget, already reviewed, or unavailable. An unavailable or failed assessment never lowers the tier: treat the commit as due and run the preflight STATUS with `--base-ref <last reviewed boundary> --committed-only`. Never infer low risk from a failed assessment, and keep existing risk, consent, and authority unchanged. Never skip an existing delivery gate. A task list or assumption challenge never enables RDD, replaces its roles or candidate consent, or adds an execution harness.\n")
	output.WriteString("- Delivery follows work units. At feature-document creation, forecast authored changed lines (additions plus deletions, generated files excluded) from the task list, and keep a running count from work-unit commits. Choose one delivery strategy per feature from the SDD vocabulary: `ask-on-risk` (default), `auto-chain`, `single-pr`, or `exception-ok`. When the forecast or the running count exceeds about 400 authored changed lines, apply the chosen strategy before the next commit: `ask-on-risk` asks once for the chain strategy, `stacked-to-main` or `feature-branch-chain`; `auto-chain` asks only for a missing chain strategy and slices automatically. Cache both choices, and record slice boundaries, which commits each pull request holds, in the feature document. Resolve the `work-unit-commits` and `chained-pr` skills by registry name before planning or creating any pull request, never hardcode their paths.\n")
	output.WriteString("- When RDD is enabled, first use the existing native candidate risk assessment (`gentle-ai review assess --cwd <repo> --json`). Passive/low uses silent structural checks with no reviewer or consent ceremony. Medium/high relays the existing candidate consent and follows the native plan: native review runs only on grant; a decline continues under ordinary policy. Do not substitute model judgment, task size, or defect severity for prospective candidate risk; never infer low risk from a failed assessment. Follow native continuations and authority without bypassing gates. When RDD is disabled, do not start or prompt for RDD; ordinary checks remain.\n")

	// The kill switch ships in the routing block, not in the optional SDD
	// assets, for the same reason routing itself is unconditional: it is
	// installed for every configured agent. A switch the agent cannot name does
	// not exist for the user, who would otherwise ask to stop using
	// receipt-driven development and be argued with instead of obeyed.
	output.WriteString("\n### Receipt-driven development is user-owned\n\n")
	output.WriteString("The user controls receipt-driven development with a switch: `gentle-ai review mode enable|disable|status`.\n\n")
	output.WriteString("- It is **on by default and opt-out**. An unset preference permits review without recording a user decision; explicit global or clone-local OFF still wins. Candidate consent remains separate from the mode default.\n")
	output.WriteString("- `status` is read-only. It reports the deciding source and the effective mode, and changes nothing. A `default` deciding source means nobody has chosen, so the effective mode is on.\n")
	output.WriteString("- When the user asks to stop using receipt-driven development, run `disable`. Do not argue, do not work around it, and do not propose alternatives first.\n")
	output.WriteString("- While it is disabled, keep implementing organically through direct inline, delegated direct, or optional SDD: do not start reviews, do not retry, do not reactivate it, and do not fall back to any retired path.\n")
	output.WriteString("- Delivery under a disabled switch follows ordinary repository policy and reports `disabled/unmanaged`, never a fabricated approval.\n")
	output.WriteString("- Never toggle the mode automatically or persist a preference just because the default is on. Never enable receipt-driven development on the user's behalf unless the user explicitly asks for it.\n")

	// The remote execution boundary ships in the routing block, not only in the
	// optional permission profiles, for the same reason as the kill switch above:
	// it is unconditional for every configured agent, including agents with no
	// permission profile at all. Ambient authenticated sessions (SSH
	// ControlMaster sockets) let a local agent reach remote infrastructure
	// without any permission rule firing (#4324).
	output.WriteString("\n### Remote execution boundary\n\n")
	output.WriteString("Remote systems are outside every authorized workspace. Never execute commands on remote infrastructure — `ssh`, `scp`, `sftp`, `rsync`, or equivalents — unless the user explicitly requested that remote action in this conversation.\n")
	output.WriteString("- Never discover, enumerate, or attach to authenticated sessions or Unix domain sockets outside the workspace, including SSH ControlMaster sockets under `/tmp` or `~/.ssh` reached through the process table or socket files.\n")
	output.WriteString("- Local problems stay local: when a task appears to need remote access (database outages, service checks), report findings and ask the user to run the remote step or grant explicit direction first.\n")

	return output.String(), nil
}
