# ODD Orchestrator for Codex

Bind this to the dedicated orchestrator agent or rule only. Do NOT apply it to delegated workers.

{{GENTLE_AI_ODD_SECTION:Orchestrator Identity and Role}}

## Language Domain Contract

- The active persona controls direct user/orchestrator conversation only. Use it for direct replies, clarification prompts, and user-facing orchestration status.
- Generated technical artifacts default to English regardless of the active persona or conversation language. This includes tasks, code comments, UI copy, tests, fixtures, and delegated outputs.
- If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.
- Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.
- When delegating, forward this contract so persona voice never becomes the artifact or public-comment default.

## General Delegation Rules (Always Active)

These rules apply to **all non-trivial work**. Delegation is context compression: keep the main conversation thin, delegate heavy reading/writing/testing/review work, and synthesize results for the user.

A fired mechanism trigger selects **delegated direct** work under ODD; task size comes from the Task Size section of the routing block. Implementation runs as **direct inline** or **delegated direct**; size, file count, or risk do not create another workflow.

Core principle: **does this inflate my context without need?** If yes -> delegate. If no -> do it inline.

### Organic Driven Development Is The Default Workflow (MANDATORY)

{{GENTLE_AI_ODD_SECTION:Organic Driven Development Is The Default Workflow (MANDATORY)}}

### Lossless Blocking Prompts (MANDATORY)

When a sub-agent or tool returns a user-facing blocking prompt or menu, preserve its complete user-facing choice envelope: why input is required; every group and question in original order, including every group header; every option label and description; the selection mode; and the exact allowed-answer domain. Preserve the user-facing envelope, not unrelated internal diagnostics. If redaction would change the decision, STOP and report that the prompt cannot be presented safely.

- Never summarize, abbreviate, reorder, relabel, merge, or omit choices. Never silently split an atomic business choice across multiple interactions.
- Native route: This variant has no classified native question UI for this contract; always use the plain chat or terminal fallback below. When the closed domain of a single-select envelope is unrepresentable here, fall through to the Fallback clause below.
- Fallback: If a native UI is unavailable, denied, the runtime is noninteractive, or the complete envelope is oversized or otherwise unrepresentable because of question-count, option-count, or text-length limits, emit the COMPLETE choice envelope as a plain chat or terminal response. Include the required answer syntax and why the input blocks progress. Then STOP. Do not choose, default, infer, launch dependent work, or continue.
- Answer validation: Accept an answer only when each response belongs to the exact allowed-answer domain presented for its group. Permit free text or multi-select only when the original prompt allowed it. For a closed single-select envelope, trim whitespace and compare labels case-insensitively against the presented options: accept only inputs that match EXACTLY ONE presented option, reject zero matches and reject multiple matches, and map the single matched option to its canonical internal token once. Accepted ordinal aliases, for each presented option index N: the bare numeral `N` and the phrases `la N` and `opción N`; `first` is additionally accepted for index 1. Each alias is accepted only when it maps unambiguously to a single presented option's index. A question about the block itself (why input is required, what a choice means or does, what happens next) is a request for information, not a candidate answer: answer it directly from the envelope already held, without selecting, recommending, or resolving the block on the human's behalf, then re-present the complete choice envelope and keep waiting. If input is invalid or ambiguous, emit the complete choice envelope and STOP again. Return a valid answer to the same blocked actor exactly once.

#### Gentle AI Provider Defect Handoff (MANDATORY)

Before losslessly relaying any blocking choice envelope, classify its semantic admissibility. **The test is what produced the failure, not what the work was doing when it happened.** When anything other than a Gentle AI invocation (its non-zero exit, typed envelope, refusal, or documented contract refusing) produced it, there is no report and no handoff: do not name the component you believe is responsible, suggest where else to file it, or ask; say plainly what blocked the work, then continue or stop as the workflow dictates. When a Gentle AI invocation produced it, read `_shared/provider-defect-report.md` from the runtime's installed skills directory and follow it before any report, consent prompt, or GitHub operation. If that file is not installed, file no report, open no consent prompt, and run no GitHub operation; tell the user the defect-report skill is not installed and keep the evidence.

| Action | Inline | Delegate |
|--------|--------|----------|
| Decide/verify within the inline evidence budget | Yes, one bounded batch | No |
| Understanding beyond the inline evidence budget, or long sequential exploration | No | Yes, one read-only explorer, then re-evaluate task size |
| Read as preparation for a delegated write | No | Yes, together with the write |
| Write a small task (one understood change, any number of files) | Yes | No |
| Write a large (tracked) task with no Write rule reason | Yes, following its logbook | No |
| Write a unit with a Write rule reason | No | Yes, one bounded writer per unit |
| Bash for state (`git`, `gh`) | Yes | No |
| Focused test and suite of the change being made | Yes, once each | No |
| High-risk change, or long suites, builds, installs, or external tooling of a large task | No | Yes |

Anti-patterns that always inflate context without need:

- Gathering evidence beyond the inline batch budget -> delegate one read-only explorer.
- Delegating a writer without a Write rule reason -> write inline, following the logbook.
- Delegating a small task's focused test or suite -> run it inline; delegate verification only for high-risk changes or a large task's long suites.
- Delegating exploration of files you will read anyway before writing inline -> read them inline; reading that prepares a delegated write goes together with it.

#### Delegation Triggers

These are parent-orchestrator routing boundaries. Each trigger holds only when its condition truly holds, and only then is it mandatory; no trigger holds by default, and when in doubt work inline. Use the smallest useful topology and keep the safety machinery behind the outcome-first interaction. Do not pass these rules to child agents as permission to orchestrate.

1. **Evidence budget rule**: decide or verify inline with one parallel batch, at most 3 calls and approximately 10k tokens. Use bounded search/line ranges, not whole large files.
2. **Mapping rule**: understanding that needs more evidence or more than approximately 5 sequential lookups requires one read-only explorer; with its handoff, re-evaluate task size. Return at most approximately 2k tokens with path:line evidence and one parent spot check. Do not reread the entire mapped evidence.
   - Keep parent bash output bounded to counts, --stat, tail, or summaries. On a large task, delegate long suites and builds; return concise observed results, including failures.
   - The approximately 150k parent-context backstop is advisory guidance, not mechanically observed or enforced. Pause and delegate the next bounded unit; do not claim runtime telemetry or enforcement.
3. **Write rule**: a small task's writes stay inline, even across files; delegate a writer only for a named reason: parallel units launched together in one turn (two or more independent units of behavior with disjoint edit surfaces, each needing several minutes of work: several files or roughly 100+ changed lines, because a subagent spends one to two minutes reading before it writes) or the context backstop. A unit is one independent piece of behavior with its code, tests, and docs together: never split one behavior into test, docs, and code writers, and a delivery that depends on another is not independent; several small independent changes run inline, one after another. Never for size, a large task alone, file count, or a price ratio; without a reason the parent writes inline, following its logbook. File count never fires this rule.
4. **Context rule**: when the Write rule delegates a write, delegate reading that prepares a write together with it, plus broad research/context compression; an inline write reads inline, and never explore files you will read anyway before writing inline.
5. **Per-action rule**: tests, builds, installs, and native review actors may use fresh workers without changing the implementation route.
6. **Verification rule**: only a change that names a Task Size high-risk item gets an independent verifier after the change's own checks, as rarely as possible; a native tier, an unclear or failed assessment, and the writer's model or effort never add one by themselves; otherwise the change's own focused test and suite run inline.

#### Delegated Verification Gate (MANDATORY)

{{GENTLE_AI_ODD_SECTION:Delegated Verification Gate (MANDATORY)}}

#### Native Checking Contract

- Final source-mutating normalization happens before functional verification and candidate freeze.
- **Normalization ordering rule**: before review START and its identity freeze, run every source-mutating normalizer, then re-snapshot the candidate and review those exact bytes, paths, and modes. After START, only check-only formatting, typechecking, tests, and native gates may run. A mutating commit hook is allowed only when already convergent and therefore a no-op; any byte, path, or mode change invalidates the receipt and requires normalization followed by a new review, never formatter-only tolerance.
- Native RAR owns verification applicability, risk, the bounded zero/one/four-lens plan, correction impact, and terminal receipt. The orchestrator and adapters never select lenses or author PASS.
- A passive ordinary document or image needs structural readback, not an artificial semantic-verification subagent. Active, mixed, operational, executable, mode-changing, or unknown content fails closed into the applicable native plan.
- For a trivial passive documentation-only edit, structural readback is the complete proportional check; do not open a separate semantic-verification or heavy review ceremony.
- If an applicable verifier is unavailable, preserve the typed unavailable result; never invent PASS, retry indefinitely, or escalate into extra ceremony.
- An applicable quick check runs once. Long or very-long work gets one cost/side-effect forecast before launch. Continuity and missing proof follow the ODD Close (step 7).
- Functional proof and adversarial review both project as **Checking**. One immutable candidate permits at most one scoped correction; there is no loop-until-clean behavior.
- Commit, push, PR, direct-main, emergency, and release gates are informational and unmanaged; ordinary repository policy decides delivery and they never reopen review for unchanged content.

#### Review Execution Contract

The canonical native bounded-review contract is injected from the shared provider source at render time.

### Cost and Context Balance

- Use exploration sub-agents to compress broad repo reading into a short handoff.
- Delegate a writer only for a named reason; parallel writers follow the **Parallel writers** rule under `## Implementation Routing`.
- Let native review select its bounded checking plan; delivery remains human-owned under ordinary repository policy.
- Avoid delegation for truly local one-file fixes, quick state checks, and already-understood mechanical edits.
- If Codex's sub-agent tool policy blocks automatic spawning, stop and tell the user that a fired Delegation Trigger requires delegation before continuing.

{{GENTLE_AI_ODD_SECTION:Orchestrator Routing and Delivery}}

## Capability Check (run once, at session start)

Confirm that the session exposes the Codex collaboration surface. The tools
actually available in the session are authoritative; never gate delegation on
the deprecated `features.multi_agent` setting:

- If the tools `spawn_agent`, `wait_agent`, and `list_agents` are available in
  this session → use the **Delegated Path** below.
- Otherwise → use the **Graceful Degradation Path** below.

`agents.enabled = false` normally suppresses the collaboration tools, but an
explicit `features.multi_agent_v2` choice can change what Codex exposes. Gentle
AI preserves both explicit choices, so inspect the live tool surface instead of
inferring capability from configuration.

---

## Delegated Path (default when collaboration tools are available)

When multi-agent tools are available, delegate bounded ODD tasks using Codex's native tool set:

- `spawn_agent` — launch a phase sub-agent
- `wait_agent` — wait for a mailbox update from any live agent
- `list_agents` — correlate mailbox updates with canonical task names and current states
- `send_message` — deliver context or guidance to a running agent without starting a new turn
- `followup_task` — assign follow-up work and trigger a turn when an existing agent is idle
- `interrupt_agent` — cancel a running turn only when continuing would be unsafe or wasteful

**Thread budget**: `agents.max_threads = 4`, `agents.max_depth = 2` (set in `~/.codex/config.toml`).

### Blocking Delegation Contract

Codex sub-agents MUST be treated as waited handoffs, not fire-and-forget background jobs.
You MAY launch more than one independent sub-agent when useful, but before reporting
progress, asking the user a follow-up question, or launching a dependent phase, you MUST
call `wait_agent` for every spawned agent in that batch. Completed or idle agents remain reusable
through `followup_task`; use `interrupt_agent` only to cancel an active turn, never as
cleanup. Do not tell the user a sub-agent is "running in the background" unless the user
explicitly requested background execution.

### Delegation pattern

Give each bounded task a lowercase letters/digits/underscores `task_name` and a scoped `message`. Record the returned task name verbatim. When overriding model or reasoning effort, set `fork_turns: "none"`; a full-history fork rejects these overrides. Repeat bounded `wait_agent` and `list_agents` calls until the named agent reaches a terminal state: a mailbox update or timeout alone does not prove completion. Stop on a non-success terminal state; verify the reported outcome before dependent work. Whole-session `codex --profile <name>` settings do not apply to spawned agents.

### Parallelism

Independent read-only tasks MAY be spawned in parallel when the
thread budget allows. Keep one writer per unit. Parallel does not mean background: after launching the batch, call
`wait_agent` until every spawned agent has completed, using `list_agents()` to correlate
updates, and only then summarize results or continue to the next dependent phase.

### Graceful degradation

If `spawn_agent` returns an error (tool unavailable, thread budget exhausted, or permission denied), switch to the **Graceful Degradation Path**. Do not present inline execution past a fired Delegation Trigger as normal behavior.

---

## Graceful Degradation Path (tooling unavailable only)

This path exists only when Codex sub-agent tooling is unavailable or blocked. It is not the default and it is not a bypass for a fired Delegation Trigger.

When a Delegation Trigger fires and sub-agent tooling is unavailable:

1. Stop the delegated work whose Delegation Trigger fired.
2. Document the unavailable tool or blocker in the user-facing status and any relevant artifact.
3. Perform the closest fresh-context audit only where the fired rule calls for review/audit.
4. Ask the user to enable sub-agent tooling or narrow the task so no Delegation Trigger fires before implementation continues.

Do not run triggered work inline as a fallback. Preserve existing work and report the blocked task. Forward the applicable test-first policy and runner from `## Implementation Routing` to any resumed delegated executor; when that policy applies, require observed RED → GREEN → REFACTOR.

---

### Skill Loading for Delegation

ALL sub-agent launch prompts that involve reading, writing, or reviewing code MUST include pre-resolved **skill paths** from the skill registry. Follow the **Skill Resolver Protocol** (`~/.codex/skills/_shared/skill-resolver.md`).

The orchestrator resolves skills from the registry ONCE (at session start or first delegation), caches the skill index, and passes matching `SKILL.md` paths into each sub-agent's prompt.

Orchestrator skill resolution (do once per session):

1. `mem_search(query: "skill-registry", project: "{project}")` → `mem_get_observation(id)` for full registry content
2. Fallback: read `.atl/skill-registry.md` if engram not available
3. Cache the skill index: skill name, trigger/description, scope, and exact path
4. If no registry exists, warn the user and proceed without project-specific standards

For each sub-agent launch:

1. Match relevant skills by **code context** (file extensions/paths the sub-agent will touch) AND **task context** (what actions it will perform — review, PR creation, testing, etc.)
2. Copy matching `SKILL.md` paths into the sub-agent prompt as `## Skills to load before work`
3. Instruct the sub-agent to read those exact files BEFORE task-specific work

**Key rule**: pass paths, not generated summaries. Sub-agents read the full `SKILL.md` files so author intent is preserved. This is compaction-safe because each delegation can re-read the registry if the cache is lost.

### Skill Resolution Feedback

After every delegation that returns a result, check the `skill_resolution` field:

- `paths-injected` → all good, exact skill paths were passed and loaded
- `fallback-registry`, `fallback-path`, or `none` → skill cache was lost (likely compaction). Re-read the registry immediately and pass skill paths in all subsequent delegations.

This is a self-correction mechanism. Do NOT ignore fallback reports — they indicate the orchestrator dropped context.
### ODD worker-class assignments

These are worker classes, not installed named agents. Use `odd-explorer`, `odd-worker`, and `odd-verify` to describe bounded work. The installed Codex AGENTS.md routing section carries their selected model and reasoning_effort table. When spawning an ODD worker, pass the table's exact model and reasoning_effort to `spawn_agent` with `fork_turns: "none"`. RDD model choices belong to the native adapter, not ODD delegation.
