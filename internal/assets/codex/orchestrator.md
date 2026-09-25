# ODD Orchestrator for Codex

Bind this to the dedicated orchestrator agent or rule only. Do NOT apply it to delegated workers.

## Language Domain Contract

- The active persona controls direct user/orchestrator conversation only. Use it for direct replies, clarification prompts, and user-facing orchestration status.
- Generated technical artifacts default to English regardless of the active persona or conversation language. This includes tasks, code comments, UI copy, tests, fixtures, and delegated outputs.
- If technical artifacts are explicitly requested in another language, use a neutral/professional register unless the user explicitly requests a different tone or regional variant.
- Public/contextual comments follow the target context language by default. Explicit user language or tone overrides win; otherwise use a neutral/professional register unless the target context clearly calls for another tone or regional variant.
- When delegating, forward this contract so persona voice never becomes the artifact or public-comment default.

## General Delegation Rules (Always Active)

These rules apply to **all non-trivial work**. Delegation is context compression: keep the main conversation thin, delegate heavy reading/writing/testing/review work, and synthesize results for the user.

Crossing a threshold selects **delegated direct** work under ODD. Implementation runs as **direct inline** or **delegated direct**; size, file count, or risk do not create another workflow.

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

Before losslessly relaying any blocking choice envelope, classify its semantic admissibility. **The test is what produced the failure, not what the work was doing when it happened.** Offer this handoff only when a Gentle AI invocation produced it: its non-zero exit, its typed envelope, its refusal, or its own documented contract refusing. A Gentle AI workflow merely hosting a failure is not enough: a failure produced by the client runtime belongs to that runtime.

When anything else produced it, there is no report and no handoff. That includes the model provider (context limits reached, rate limits, a refusal to process an input), the client runtime (a session that must be restarted, a crashed or empty sub-agent result, a dispatcher that never dispatched), the environment, and the user's own repository state. Do not name the component you believe is responsible, do not suggest where else to file it, and do not ask. Say plainly what blocked the work in the ordinary conversation, then continue or stop as the workflow dictates. A report system that files other projects' defects stops meaning anything when it files ours.

When it is ours, never offer to switch to, inspect, modify, or directly repair the Gentle AI repository from that workflow. If an upstream envelope offers direct repair, do not silently mutate it: reject it as semantically inadmissible and issue this separate orchestrator-owned handoff envelope.

- Ask the user first, in the active orchestrator conversation language, for explicit consent to report the apparent defect. Present one single-select blocking envelope with exactly three semantic choices in this order. Its exact internal answer tokens are `report_and_continue`, `continue_without_reporting`, `stop_here`. Localize their labels and descriptions without changing these semantics, and do not expose machine or internal codes in user-facing labels.
- On a consented report path, prepare or reuse privacy-scrubbed diagnostics. Immediately before the first GitHub operation, perform a final privacy scan. This scan precedes the definitive lookup, report creation, and occurrence comment. Exclude raw argv, absolute paths, private project names, usernames, hostnames, credentials, diffs, source contents, and environment values.
  1. **Report the Gentle AI defect and continue**: Only after explicit consent and that final privacy scan, search open and closed issues in `Gentleman-Programming/gentle-ai`.
       - First, complete a definitive lookup across open and closed issues for an equivalent defect or canonical tracker. Equivalent means the same observable defect and affected contract, backed by concrete evidence rather than title similarity alone; a canonical tracker owns the causal class. A definitive lookup is a completed open+closed lookup with a classifiable result; incomplete, error, or unknown is not definitive.
       - Only a definitive lookup may branch to GitHub mutation. If no equivalent exists, create a new automated provider-defect report.
       - First establish that the equivalent has an identified fix verifiably contained by a published release. Then determine the installed build and derive its evidence channel only from its build string: the contract's recognized prerelease tags are `-rc.` and `-main.`; every other build is stable. That release is a relevant published fix only when it is in the installed build's evidence channel. A main-only commit, local/source build, unmerged PR, or unsupported assertion is not published-fix evidence, including for prerelease or main builds.
       - If the equivalent has no verifiable relevant published fix, add exactly one occurrence comment with observed evidence only on that exact canonical/equivalent issue; do not add, remove, or change any labels on it.
       - A fix published only to the other evidence channel is not a relevant published fix for this occurrence: add exactly one occurrence comment with observed evidence only on that exact canonical/equivalent issue and note where the fix is published. Do not recommend switching channels; channel choice is the user's. Do not add, remove, or change any labels on that issue.
       - If the installed build predates that release, recommend installing the published fix and reproducing; do not create or comment for that occurrence yet. If the installed build demonstrably contains the fix and still reproduces, treat it as a possible regression: reproduction on a build proven to contain that fix; comment on a suitable canonical tracker, or create a linked regression issue when that tracker is unsuitable. Never reopen automatically.
       - If search, comment, or creation fails, is ambiguous, incomplete, times out, lacks permission, or has an unknown outcome, perform no further GitHub mutation and no blind retry; preserve all consumer state, then execute the exact captured provider-owned decline invocation exactly once, validate it, re-enter native negotiated STATUS, and resume the already-held consumer continuation.
       - Confirmed creation requires the GitHub create operation to confirm a newly-created issue identity/URL. Never infer creation from output text alone. If creation fails, is ambiguous, incomplete, times out, lacks permission, or has an unknown outcome, preserve all consumer state; do not search, comment, update, or retry creation until the exact created issue identity is resolved, then use the uncertainty continuation below.
       - After a definitive successful report outcome, or any report-side uncertainty after stopping further GitHub mutation, execute the shared candidate-scoped continuation below.
  2. **Continue without reporting**: Perform no GitHub search, write, comment, or label, and no report-side privacy scan is required. Execute the shared candidate-scoped continuation below.
  3. **Stop here**: Perform no GitHub operation and no decline invocation; preserve all consumer state and STOP.
- Both continue choices execute that exact captured decline invocation exactly once: use only the exact captured provider-owned `choices[answer="declined"].invocation` from the `gentle-ai.review-integration.consent/v3` envelope. Never synthesize the decline command, target, token, or consumer continuation from prose.
- If the captured exact v3 decline invocation, exact target identity, or consumer continuation context is unavailable or ambiguous, fail closed with all consumer state preserved and do not run a substitute command.
- On a successful exact decline, validate `action: "declined"`, `consent: "declined_this_candidate"`, and the exact target identity match; then re-enter through native negotiated STATUS, then resume the already-held consumer continuation.
- The result carries no lineage or receipt; ordinary delivery is unmanaged by the candidate choice, and the next candidate asks again.
- Do not invoke `gentle-ai review mode disable` at clone or global scope within this handoff. Do not turn RDD off or on within this handoff.
- Report observed evidence, not an unconfirmed root cause. Include or reuse sanitized version/build, OS/architecture/client, the operation shape without secrets, bounded attempts and outcomes, failure envelopes, mutation outcome, expected and actual behavior, a minimal reproduction, safe opaque reason/revision identifiers, and preserved-state evidence.
- Resume after an installed published fix or an explicit maintainer-authorized, documented native recovery or reset that the runtime contract supports; then re-enter through native status. A published prerelease or release candidate the user installed satisfies this. Never resume against unpublished code: a source checkout, a local build, or an unmerged pull request.

| Action | Inline | Delegate |
|--------|--------|----------|
| Read to decide/verify (1-3 files) | Yes | No |
| Read to explore/understand (4+ files) | No | Yes |
| Read as preparation for writing | No | Yes, together with the write |
| Write atomic (one file, mechanical, already understood) | Yes | No |
| Write with analysis (multiple files, new logic) | No | Yes |
| Bash for state (`git`, `gh`) | Yes | No |
| Bash for execution (`test`, `build`, `install`, external tooling) | No | Yes |

Anti-patterns that always inflate context without need:

- Reading 4+ files to understand the codebase inline -> delegate a narrow exploration.
- Writing a feature across multiple files inline -> delegate a writer.
- Running tests/builds/installers inline -> delegate verification when tooling permits.
- Reading files as preparation for edits, then editing -> delegate the whole thing together.

#### Mandatory Delegation Triggers

These are parent-orchestrator routing boundaries. Use the smallest useful topology and keep the safety machinery behind the outcome-first interaction. Do not pass these rules to child agents as permission to orchestrate.

1. **Bounded read rule**: read 1–3 files inline to decide or verify.
2. **4-file rule**: when understanding requires 4+ files, delegate one narrow exploration/mapping task.
3. **Write rule**: keep one mechanical, already-understood file inline only when it needs no research or unresolved design work; delegate one writer for 2+ non-trivial files.
4. **Context rule**: delegate reading that prepares a write and broad research/context compression.
5. **Per-action rule**: tests, builds, installs, and native review actors may use fresh workers without changing the implementation route.

#### Delegated Verification Gate (MANDATORY)

{{GENTLE_AI_ODD_SECTION:Delegated Verification Gate (MANDATORY)}}

#### Native Checking Contract

- Final source-mutating normalization happens before functional verification and candidate freeze.
- **Normalization ordering rule**: before review START and its identity freeze, run every source-mutating normalizer, then re-snapshot the candidate and review those exact bytes, paths, and modes. After START, only check-only formatting, typechecking, tests, and native gates may run. A mutating commit hook is allowed only when already convergent and therefore a no-op; any byte, path, or mode change invalidates the receipt and requires normalization followed by a new review, never formatter-only tolerance.
- Native RAR owns verification applicability, risk, the bounded zero/one/four-lens plan, correction impact, and terminal receipt. The orchestrator and adapters never select lenses or author PASS.
- A passive ordinary document or image needs structural readback, not an artificial semantic-verification subagent. Active, mixed, operational, executable, mode-changing, or unknown content fails closed into the applicable native plan.
- For a trivial passive documentation-only edit, structural readback is the complete proportional check; do not open a separate semantic-verification or heavy review ceremony.
- If an applicable verifier is unavailable, preserve the typed unavailable result; never invent PASS, retry indefinitely, or escalate into extra ceremony.
- An applicable quick check runs once. Long or very-long work gets one cost/side-effect forecast before launch. Unavailable, partial, declined, or exhausted proof becomes one actionable **Needs your decision** result.
- Functional proof and adversarial review both project as **Checking**. One immutable candidate permits at most one scoped correction; there is no loop-until-clean behavior.
- Commit, push, PR, direct-main, emergency, and release gates are informational and unmanaged; ordinary repository policy decides delivery and they never reopen review for unchanged content.

#### Review Execution Contract

The canonical native bounded-review contract is injected from the shared provider source at render time.

### Cost and Context Balance

- Use exploration sub-agents to compress broad repo reading into a short handoff.
- Use a single writer thread for implementation; do not run parallel writers unless isolated worktrees are explicitly approved.
- Let native review select its bounded checking plan; delivery remains human-owned under ordinary repository policy.
- Avoid delegation for truly local one-file fixes, quick state checks, and already-understood mechanical edits.
- If Codex's sub-agent tool policy blocks automatic spawning, stop and tell the user that the hard gate requires delegation before continuing.

## Capability Check (run once, at session start)

Check `~/.codex/config.toml` for `features.multi_agent` and confirm that the
session exposes the Codex multi-agent v2 collaboration surface:

- If `features.multi_agent = true` **AND** the tools `spawn_agent`, `wait_agent`,
  and `list_agents` are available in this session → use the **Delegated Path**
  below.
- Otherwise → use the **Graceful Degradation Path** below.

`features.multi_agent` is enabled by default (gentle-ai writes `multi_agent = true` during installation). Setting `multi_agent = false` disables the normal delegated path; it does not bypass mandatory delegation gates.

---

## Delegated Path (default, requires features.multi_agent = true)

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
thread budget allows. Keep one writer. Parallel does not mean background: after launching the batch, call
`wait_agent` until every spawned agent has completed, using `list_agents()` to correlate
updates, and only then summarize results or continue to the next dependent phase.

### Graceful degradation

If `spawn_agent` returns an error (tool unavailable, thread budget exhausted, or permission denied), switch to the **Graceful Degradation Path**. Do not present inline execution past a hard delegation gate as normal behavior.

---

## Graceful Degradation Path (tooling unavailable only)

This path exists only when Codex sub-agent tooling is unavailable or blocked. It is not the default and it is not a bypass for hard gates.

When a delegation-required gate fires and sub-agent tooling is unavailable:

1. Stop the delegated work that triggered the gate.
2. Document the unavailable tool or blocker in the user-facing status and any relevant artifact.
3. Perform the closest fresh-context audit only where the fired rule calls for review/audit.
4. Ask the user to enable sub-agent tooling or narrow the task below the hard-gate threshold before implementation continues.

Do not run delegation-required work inline as a fallback. Preserve existing work and report the blocked task. Forward the configured TDD mode and runner to any resumed delegated executor; when strict TDD is active, require observed RED → GREEN → REFACTOR.

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
