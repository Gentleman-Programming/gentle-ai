## SDD Workflow (Spec-Driven Development)

SDD is the structured planning layer for substantial changes. This file is the lazy-loaded Claude Code workflow surface; read it before handling `/sdd-*`, SDD meta-commands, SDD/Judgment-Day phase delegation, or SDD continuation/routing.

### Artifact Store Policy

- `engram` — default when available; persistent memory across sessions.
- `openspec` — file-based artifacts; use only when the user explicitly requests it or a change already exists there.
- `hybrid` — both backends; useful for team-shareable files plus cross-session recovery.
- `none` — return results inline only; recommend enabling engram or openspec.

### Commands

Skills and slash commands:

- `/sdd-init` → initialize SDD context; detects stack and testing capabilities.
- `/sdd-explore <topic>` → investigate an idea; no implementation.
- `/sdd-status [change]` → read-only structured status.
- `/sdd-apply [change]` → implement pending tasks in batches.
- `/sdd-verify [change]` → validate implementation against specs/tasks.
- `/sdd-archive [change]` → close a completed change.
- `/sdd-onboard` → guided end-to-end walkthrough.

Meta-commands are handled by the orchestrator directly and do not appear in autocomplete:

- `/sdd-new <change>` → run exploration then proposal.
- `/sdd-continue [change]` → run the next dependency-ready phase.
- `/sdd-ff <name>` → fast-forward proposal → specs → design → tasks.

### Native SDD Dispatcher Guard

Before routing, continuing, applying, verifying, or archiving an SDD change, first determine this session's artifact store. The native dispatcher (`gentle-ai sdd-continue [change] --cwd <repo>` or `gentle-ai sdd-status [change] --cwd <repo> --json --instructions`) reads only OpenSpec file artifacts and always emits `artifactStore: openspec`; it cannot observe Engram-backed changes.

- For `engram`, do NOT invoke the dispatcher. Resolve status from Engram topic keys with `mem_search` followed by `mem_get_observation`.
- For `openspec` or `hybrid`, use the dispatcher when available and treat its JSON as authoritative over prompt inference.
- Route only by structured `nextRecommended`, dependency states, and `blockedReasons`; never infer from free text.
- If blocked, stop and report the blocker. Do not proceed to apply, archive, or terminal work.

### SDD Session Preflight (HARD GATE)

Before executing ANY SDD command or natural-language SDD request, ensure this session has an explicit `SDD Session Preflight` decision block.

This applies to `/sdd-new`, `/sdd-ff`, `/sdd-continue`, `/sdd-explore`, `/sdd-status`, `/sdd-apply`, `/sdd-verify`, `/sdd-archive`, and natural-language equivalents such as "use SDD to add dark mode" / "do it with SDD".

Required preflight choices:

1. **Execution mode**: `interactive` or `auto`.
2. **Artifact store**: `openspec`, `engram`, or `hybrid` when Engram is callable. If Engram is unavailable, offer only file/inline-safe choices.
3. **Chained PR strategy**: `auto-forecast`, `ask-always`, `single-pr-default`, or `force-chained`.
4. **Review budget**: maximum changed lines before stopping for reviewer-burden approval.

User-facing preflight question format:

Use the built-in `AskUserQuestion` tool for SDD Session Preflight. Do NOT render the full preflight menu as plain chat text.

Ask all four preflight groups in one single `AskUserQuestion` tool call so Claude Code can render the groups as one interactive prompt. Do NOT run this as a sequential wizard. Do NOT issue four separate `AskUserQuestion` tool calls.

The single `AskUserQuestion` tool call must contain these four localized groups in this order:

1. Pace: Interactive, Automatic.
2. Artifacts: OpenSpec, Engram, Both.
3. PRs: Ask me, Single PR, Chained, Auto.
4. Review: 400 lines, 800 lines, Other.

Match the user's current language and active persona for question labels and descriptions. Treat the preflight UI as direct orchestrator conversation, not as a generated technical artifact. Technical artifacts still default to English, but this UI follows the user's conversation language/persona. Do NOT mix languages inside one grouped question.

Do NOT show option codes in the interactive UI. Do NOT show canonical values or other internal values in the interactive UI labels or descriptions.

After the single grouped `AskUserQuestion` tool call returns, map the selected human labels to canonical values internally. Do not reveal the canonical values in the UI.

If Other is selected for review budget, ask one follow-up question for the numeric budget.

Only after all four preflight choices are collected, summarize them as the `SDD Session Preflight` decision block and continue with the SDD init guard/requested phase.

Map answers to canonical values:

- Pace: Interactive -> `interactive`; Automatic -> `auto`.
- Artifacts: OpenSpec -> `openspec`; Engram -> `engram`; Both -> `hybrid`.
- PRs: Ask me -> `ask-always`; Single PR -> `single-pr-default`; Chained -> `force-chained`; Auto -> `auto-forecast`.
- Review: 400 lines -> `review_budget_lines: 400`; 800 lines -> `review_budget_lines: 800`; Other -> ask one follow-up for the number.

Hard gate rules:

- `openspec/config.yaml`, existing SDD artifacts, previous `sdd-init` results, or installed SDD assets do NOT satisfy session preflight.
- If the session has no preflight block, ask the single grouped `AskUserQuestion` preflight above. Do not run init, delegate phases, edit files, or apply tasks until all four choices are collected.
- Cache the choices for this session and include them in later phase prompts.
- If the user explicitly provided all four choices in the current conversation, summarize them as the session preflight block and continue.

### SDD Entry Routing (MANDATORY)

For a new product/code change request that says to use SDD, start at preflight -> init guard -> explore/proposal (`/sdd-new` equivalent). Never launch `sdd-apply` just because the user asked to implement a feature.

Only launch `sdd-apply` when all are true:

1. Session preflight is complete.
2. The active change has existing spec, design, and tasks artifacts.
3. The user explicitly asked to apply/continue implementation, or the prior SDD planning phase completed and the orchestrator has passed the review workload guard.

If any dependency is missing, STOP and propose `/sdd-new` or `/sdd-ff`; do not implement.

### SDD Init Guard (MANDATORY)

Before executing any SDD command or meta-command, check whether `sdd-init` has run for this project:

1. Search Engram: `mem_search(query: "sdd-init/{project}", project: "{project}")`.
2. If found, proceed normally.
3. If not found, run `sdd-init` first, then continue with the requested command.

This ensures testing capabilities, Strict TDD mode, and project context are available to later phases.

### Execution Mode

This is collected by `SDD Session Preflight`. If missing, enforce the hard gate before any phase work. Cache the collected mode for the session:

- **Automatic** (`auto`): phases run back-to-back without pausing, but the orchestrator gatekeeper validates after each phase before launching the next.
- **Interactive** (`interactive`): after each phase, show a concise summary and ask whether to adjust or continue.

Default to **Interactive** when unspecified. Interactive approval is phase-scoped; words like "continue", "dale", or "go on" approve only the immediate next phase.

Before the `sdd-propose` phase in interactive mode, offer the user a proposal question round focused on business/product understanding, business problem, business rules, outcomes, implications and impact, edge cases, scope boundaries, non-goals, constraints, and product tradeoffs. Do not ask about test commands, PR shape, changed-line budget, or other harness mechanics unless the user explicitly asks.

### Automatic Mode Gatekeeper (MANDATORY)

In Automatic mode, the orchestrator validates every delegated phase result before launching the next phase. The gatekeeper runs after every phase and before launching the next sub-agent.

Gate checks:

- **Contract conformance:** returned `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`; status is not partial/failed/blocked.
- **Artifact existence:** declared artifact is readable in the active backend.
- **No hallucination:** claimed files, symbols, commands, and artifacts exist.
- **No drift from inputs:** proposal/spec/design/tasks/apply outputs stay consistent with their dependencies.
- **Routing coherence:** `next_recommended` follows the dependency graph and no unaddressed CRITICAL risk remains.

Hybrid validation:

- Inline for low-risk phases: `sdd-explore`, `sdd-spec`, `sdd-tasks`, `sdd-archive`.
- Fresh-context phase-contract validator for `sdd-design` and `sdd-apply`: validate only the phase artifact against its inputs. This is not adversarial implementation review, inspects no code diff, and creates no 4R/Judgment-Day budget.
- Escalate to fresh-context review when an inline gate smells wrong.

On gate failure, re-run the same phase exactly once with specific corrective feedback. If the second result fails, STOP the automatic chain and report; do not advance dependent phases.

### Artifact Store Mode

This is collected by `SDD Session Preflight`. If missing, enforce the hard gate before any phase work. Cache the collected store (`engram`, `openspec`, `hybrid`, or `none`) for the session. If unspecified, default to `engram` when Engram is available; otherwise use `none` and explain the persistence limitation.

Pass the artifact store mode to every SDD phase agent.

### Delivery Strategy

On the first SDD chain request in a session, ask once for delivery strategy and cache it:

- `ask-on-risk` — default; ask only when the tasks forecast detects review-budget risk.
- `auto-chain` — automatically split into chained/stacked PR slices when needed.
- `single-pr` — proceed as one PR only if the size is within budget.
- `exception-ok` — user accepts `size:exception` when over budget.

Pass `delivery_strategy` to `sdd-tasks` and `sdd-apply`.

### Chain Strategy

When delivery planning yields chained PRs, ask once for chain strategy and cache it:

- `stacked-to-main` — each PR targets the previous PR branch or main in sequence.
- `feature-branch-chain` — PR #1 targets the tracker branch; child PRs target the immediate previous PR branch; only the tracker merges to main.

When chained PRs are selected, treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match. Resolve and forward it by registry path to `sdd-tasks` and `sdd-apply`; do not hardcode its path.

Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`.

### Dependency Graph

```text
proposal -> specs --> tasks -> apply -> verify -> archive
             ^
             |
           design
```

### Result Contract

Every SDD phase returns: `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`.

### Review Workload Guard (MANDATORY)

After `sdd-tasks` completes and before launching `sdd-apply`, inspect `Review Workload Forecast`.

If it says `Chained PRs recommended: Yes`, `400-line budget risk: High`, estimated changed lines exceed 400, or `Decision needed before apply: Yes`, apply cached `delivery_strategy`:

- `ask-on-risk`: stop and ask whether to split or proceed with `size:exception`.
- `auto-chain`: split automatically; ask for `chain_strategy` only if missing.
- `single-pr`: stop and require/record `size:exception` before apply.
- `exception-ok`: continue and tell `sdd-apply` this run uses `size:exception`.

Always pass the resolved `delivery_strategy`, `chain_strategy`, and PR boundary/exception to `sdd-apply`.

When launching `sdd-apply`, always include the resolved `delivery_strategy`, `chain_strategy`, and any chosen PR boundary/exception in the prompt.

<!-- gentle-ai:sdd-model-assignments -->

## Model Assignments

Read this table before the first SDD/Judgment-Day delegation in a session, cache it, and use the mapped alias only for SDD/Judgment-Day phase agents. If a phase is missing, use `default`. If the assigned model is unavailable, substitute `sonnet` and continue.

The Claude Code session model is controlled by Claude Code itself; Gentle AI does not configure the main orchestrator model. This table applies only to Agent tool calls for SDD/Judgment-Day phase sub-agents, not generic delegation.

**Mandatory phase model gate:** Agent tool calls for SDD/Judgment-Day phase agents MUST include `model`. Generic/non-SDD delegation MUST NOT use this table; omit `model` unless the user explicitly requested an override.

| Phase       | Default Model | Effort  | Reason                                     |
| ----------- | ------------- | ------- | ------------------------------------------ |
| sdd-explore | sonnet        | default | Reads code, structural - not architectural |
| sdd-propose | opus          | default | Architectural decisions                    |
| sdd-spec    | sonnet        | default | Structured writing                         |
| sdd-design  | opus          | default | Architecture decisions                     |
| sdd-tasks   | sonnet        | default | Mechanical breakdown                       |
| sdd-apply   | sonnet        | default | Implementation                             |
| sdd-verify  | sonnet        | default | Validation against spec                    |
| sdd-archive | haiku         | default | Copy and close                             |
| default     | sonnet        | default | SDD/JD phase fallback                      |

<!-- /gentle-ai:sdd-model-assignments -->

### Sub-Agent Launch Deduplication (MANDATORY)

Maintain a session-scoped launch log of `(phase, task-fingerprint)` pairs. If the same pair already exists, do NOT launch again. Emit exactly one launch per distinct task and append the pair after launch.

### Sub-Agent Launch Protocol

ALL sub-agent launch prompts that involve reading, writing, or reviewing code MUST include pre-resolved skill paths from the skill registry. Follow `~/.claude/skills/_shared/skill-resolver.md`.

Pre-flight before every SDD/Judgment-Day Agent call:

1. Identify the phase key (`sdd-apply`, `sdd-verify`, `jd-judge-a`, etc.).
2. Look up the model alias in the Model Assignments table.
3. Include `model: "<alias>"` in SDD/Judgment-Day Agent calls.
4. For generic/non-SDD delegation, omit `model` unless the user explicitly requested one.

Resolve skills once per session, cache the registry, and pass exact `SKILL.md` paths. If a delegated result reports `skill_resolution` as `fallback-registry`, `fallback-path`, or `none`, re-read the registry before subsequent delegations.

### Context Protocol

Sub-agents start with fresh context. The orchestrator controls what context they receive.

For non-SDD delegation:

- Orchestrator searches Engram for relevant prior context and passes it in the prompt.
- Sub-agent saves significant discoveries, decisions, and bug fixes to Engram before returning.
- Orchestrator forwards exact skill paths.

For SDD phases, sub-agents read/write the active backend directly using artifact references, not copied artifact bodies.

| Phase         | Reads                                                  | Writes           |
| ------------- | ------------------------------------------------------ | ---------------- |
| `sdd-explore` | nothing                                                | `explore`        |
| `sdd-propose` | exploration (optional)                                 | `proposal`       |
| `sdd-spec`    | proposal (required)                                    | `spec`           |
| `sdd-design`  | proposal (required)                                    | `design`         |
| `sdd-tasks`   | spec + design (required)                               | `tasks`          |
| `sdd-apply`   | tasks + spec + design + apply-progress if present      | `apply-progress` |
| `sdd-verify`  | spec + tasks + apply-progress                          | `verify-report`  |
| `sdd-archive` | all artifacts                                          | `archive-report` |

### Strict TDD Forwarding (MANDATORY)

When launching `sdd-apply` or `sdd-verify`, search for testing capabilities (`sdd-init/{project}`). If `strict_tdd: true`, add: `STRICT TDD MODE IS ACTIVE. Test runner: {test_command}. You MUST follow strict-tdd.md. Do NOT fall back to Standard Mode.`

### Apply-Progress Continuity (MANDATORY)

When launching `sdd-apply` after prior batches, search for `sdd/{change-name}/apply-progress`. If it exists, tell the sub-agent to read it first, merge new progress into it, and save the combined result. Do not overwrite.

### Topic Keys

| Artifact        | Topic Key                          |
| --------------- | ---------------------------------- |
| Project context | `sdd-init/{project}`               |
| Exploration     | `sdd/{change-name}/explore`        |
| Proposal        | `sdd/{change-name}/proposal`       |
| Spec            | `sdd/{change-name}/spec`           |
| Design          | `sdd/{change-name}/design`         |
| Tasks           | `sdd/{change-name}/tasks`          |
| Apply progress  | `sdd/{change-name}/apply-progress` |
| Verify report   | `sdd/{change-name}/verify-report`  |
| Archive report  | `sdd/{change-name}/archive-report` |
| DAG state       | `sdd/{change-name}/state`          |

Sub-agents retrieve full Engram content in two steps: `mem_search(query: "{topic_key}", project: "{project}")`, then `mem_get_observation(id)`.

### State and Conventions

Convention files live under the agent's global skills directory, including `engram-convention.md`, `persistence-contract.md`, and `openspec-convention.md`.

### Recovery

- `engram` → `mem_search(...)` → `mem_get_observation(...)`.
- `openspec` → read `openspec/changes/*/state.yaml` and artifacts.
- `none` → state is not persisted; explain the limitation.
