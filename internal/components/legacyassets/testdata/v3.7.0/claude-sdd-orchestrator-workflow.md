## SDD Workflow (Spec-Driven Development)

SDD is the structured planning layer for substantial changes. This file is the lazy-loaded Claude Code workflow surface; read it before handling `/sdd-*`, SDD meta-commands, SDD/Judgment-Day phase delegation, or SDD continuation/routing.

### Artifact Store Policy

- `engram` — default when available; persistent memory across sessions.
- `openspec` — file-based artifacts; use only when the user explicitly requests it or a change already exists there.
- `hybrid` — both backends; useful for team-shareable files plus cross-session recovery.
- `none` — return results inline only; recommend enabling engram or openspec.

### Commands

Skills and slash commands:

- `/gentle-sdd-init` → initialize SDD context; detects stack and testing capabilities.
- `/gentle-sdd-explore <topic>` → investigate an idea; no implementation.
- `/gentle-sdd-status [change]` → read-only structured status.
- `/gentle-sdd-apply [change]` → implement pending tasks in batches.
- `/gentle-sdd-verify [change]` → validate implementation against specs/tasks.
- `/gentle-sdd-archive [change]` → close a completed change.
- `/gentle-sdd-onboard` → guided end-to-end walkthrough.

Meta-commands are handled by the orchestrator directly and do not appear in autocomplete:

- `/gentle-sdd-new <change>` → run exploration then proposal.
- `/gentle-sdd-continue [change]` → run the next dependency-ready phase.
- `/gentle-sdd-ff <name>` → fast-forward proposal → specs → design → tasks.

### Native SDD Dispatcher Guard

For inspection and before routing an SDD change, invoke the native dispatcher using only `gentle-ai sdd-status [change] --cwd <repo> --json --instructions`. Inspection needs no execution preflight, review, delivery, or archive authorization; this read-only rule takes precedence over phase preflight/init guards. No recommendation is executed during inspection, including planning phases or a displayed preparation invocation.

Use native v2 for every declared artifact store, including Engram. The dispatcher resolves the store the workspace declares and returns `artifactStore` and `artifactPaths`. Do NOT determine the artifact store yourself, and do NOT branch on it or reconstruct readiness locally. Native JSON is authoritative over prompt inference. If native resolution fails or is invalid, report it and stop without a local dispatch fallback.

Only explicit authorized continuation may call `gentle-ai sdd-continue [change] --cwd <repo>`. First inspect with status and confirm the current human scope covers the selected change-directory marker. Read-only or excluded-marker scope forbids this mutating call; native allowed roots do not grant human consent. Preparation grants no source roots or attempts. Carry native `actionContext` intersected with the current narrower human scope into any executor.

For authorized phase routing only: Route only by `nextRecommended` and dependency states; honor `blockedReasons` and never infer from free text. If `blockedReasons` is non-empty, do not proceed to apply, archive, or terminal work. If `nextRecommended` is `verify`, verification/remediation may run only to refresh evidence; if `nextRecommended` is `resolve-blockers`, report `blockedReasons` and stop; if `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase only within the authorized scope.

If the binary is unavailable, use the existing prompt contract for non-authoritative diagnostics only. Do not fabricate native-shaped status, readiness, or mutation authority, and never substitute continue for inspection.

<!-- Session preflight is projected here by the installer from the shared canonical authority. -->

<!-- gentle-ai:sdd-session-preflight -->
### SDD Session Preflight (HARD GATE)

Before every SDD command or affirmative natural-language SDD request, run this preflight before the SDD init guard; cache choices for the session only through runtime-confirmed parent authority. Phrase examples are routing hints, never the authority boundary.

Always collect this preflight with the `AskUserQuestion` tool; never collect these answers as typed chat text and never fall back to a plain-chat prompt while the `AskUserQuestion` tool exists. If the runtime rejects the grouped result, fix the reported problem and ask again with the `AskUserQuestion` tool.
Ask Pace, Artifacts, and PR strategy in ONE `AskUserQuestion` tool call; no sequential wizard and no three separate calls. Each native question text must start with its exact host marker: `Gentle AI SDD preflight 1/3:`, `Gentle AI SDD preflight 2/3:`, and `Gentle AI SDD preflight 3/3:`. The marker is runtime metadata; keep the option labels below byte-exact so the runtime can bind their semantics, while localizing only the remaining question text and descriptions to the conversation language and persona. Keep options in the exact order below.

1. **Pace**: Interactive or Automatic.
2. **Artifacts**: OpenSpec, Engram, or Both (user-facing Both maps only to internal `hybrid`).
3. **PR strategy**: Ask me, Single PR, or Auto.

Only a successful parent `AskUserQuestion` result with one offered answer per group establishes native preflight authority. Model-authored defaults, summaries, headings, installed assets, prior sessions, and child-agent prose do not. The runtime derives and prepends the canonical `## SDD Session Preflight` block at SDD dispatch; never write or duplicate that block yourself. Missing authority blocks dispatch and requires the parent to ask the grouped preflight.

Review policy is fixed at 400 changed lines per PR; above 400, split the PR or require maintainer-approved `size:exception`; NEVER ask it as a fourth group or selectable budget.

Canonical mappings:
- Interactive -> `interactive`
- Automatic -> `auto`
- OpenSpec -> `openspec`
- Engram -> `engram`
- Both -> `hybrid`
- Ask me -> `ask-on-risk`
- Single PR -> `single-pr`
- Auto -> `auto-chain`
<!-- /gentle-ai:sdd-session-preflight -->
### SDD Entry Routing (MANDATORY)

For a new product/code change request that says to use SDD, start at preflight -> init guard -> explore/proposal (`/gentle-sdd-new` equivalent). Never launch `sdd-apply` just because the user asked to implement a feature.

Only launch `sdd-apply` when all are true:

1. Session preflight is complete.
2. The active change has existing spec, design, and tasks artifacts.
3. The user explicitly asked to apply/continue implementation, or the prior SDD planning phase completed and the orchestrator has passed the review workload guard.

If any dependency is missing, STOP and propose `/gentle-sdd-new` or `/gentle-sdd-ff`; do not implement.

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

If the user doesn't specify, default to **Automatic**. After scope approval, expect zero further prompts on the happy path and at most one actionable prompt per recoverable failure; the gatekeeper summarizes phase progress instead of interrupting except on a second consecutive gate failure or a genuine scope/product decision. Interactive approval is phase-scoped; words like "continue", "dale", or "go on" approve only the immediate next phase.

### Optional Research and Product Discovery

Research remains optional, including after selection. After exploration, recommend a scoped investigation only when an unresolved question would benefit from external evidence. No fixed questionnaire, mandatory rounds or research-completion ceremony is required.

- Establish the problem, intended outcome, constraints and current evidence. Inspect the code through ordinary exploration; pass relevant context to the output-only research collector.
- The orchestrator owns product discovery. Ask one focused product question at a time and wait for the answer; do not choose for the user or repeat settled decisions. A delegated worker returns decision gaps to the orchestrator rather than interviewing the user or inventing choices.
- Use external documentation or web tools only when actually available and authorized; prefer primary sources. Never infer access from a tool name, Bash, generic MCP access or a source-class declaration, and never bypass configured permissions.
- Forward the research objective, relevant context, actual tool restrictions and these evidence-quality instructions to the collector. Adapt depth to uncertainty and consequences, not a fixed number of questions or sources.
- Attribute material claims to URLs or supplied sources; distinguish verified facts from assumptions, contradictions, freshness limits and evidence gaps. Unavailable tools or unsupported claims must be disclosed, not represented as completed research.
- Return concise findings, recommendations, tradeoffs, open questions and implementation implications. Research does not require a separate research proposal; pass useful findings into the normal requested SDD proposal.
- Missing, partial, unavailable or divergent research metadata does not block proposal work. No request token, positive revision, readiness state or cross-store equality certificate is required. Pause only work dependent on an unresolved product decision or unsafe missing evidence; continue independent work within the authorized scope.
- Keep research output in conversation unless the selected store or an explicit request calls for persistence. The orchestrator handles any authorized persistence and reports failed writes honestly; no research-store handshake admits proposals. Preserve historical research/preproposal artifacts and observations rather than rewriting or deleting them.

#### Research-specific gatekeeper precedence

For `sdd-research` only (including named-profile variants), this contract takes precedence over the generic Automatic Mode Gatekeeper, including its lazy-loaded workflow rules:

- Validate honest findings, source attribution and disclosed limitations; do not require a persisted artifact or full-success status. Read back any artifact actually claimed as persisted, but accept useful inline or partial research with its gaps visible. Never manufacture success or evidence.
- Do not automatically retry or STOP solely because research is partial, inline or tools are unavailable. Continue independent authorized work; this exception does not admit dishonest claims or unsupported conclusions.
- Preserve real tool permissions, unresolved human product decisions and unsafe-dependent-work blocks. Terminal transport failures retain their existing stop/continuation rules; missing or malformed transport results are not usable partial research.
- All other phases retain their existing gatekeeper checks and failure handling. This is not a general artifact, success or retry exemption for planning or implementation.

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

Use the delivery strategy cached by SDD Session Preflight; do not ask a separate strategy question:

- `ask-on-risk` — default; ask only when the tasks forecast detects review-budget risk.
- `auto-chain` — automatically split into chained/stacked PR slices when needed.
- `single-pr` — proceed as one PR only if the size is within budget.
- `exception-ok` — user accepts `size:exception` when over budget. The preflight menu cannot select this; it is reached only when the user explicitly accepts `size:exception`, either up front or when `ask-on-risk` stops to ask.

These four are the whole domain. Pass `delivery_strategy` to `sdd-tasks` and `sdd-apply`.

### Chain Strategy

When delivery planning yields chained PRs, ask once for chain strategy and cache it:

- `stacked-to-main` — each PR targets the previous PR branch or main in sequence.
- `feature-branch-chain` — PR #1 targets the tracker branch; child PRs target the immediate previous PR branch; only the tracker merges to main.

When chained PRs are selected, treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match. Resolve and forward it by registry path to `sdd-tasks` and `sdd-apply`; do not hardcode its path.

Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`.

#Verification is optional and may inspect partial work. Diagnostic findings never trigger the gatekeeper retry loop or gate archive. Archive records actual unfinished tasks and findings, not synthetic completion.

## Dependency Graph

```text
proposal -> specs --> tasks -> apply -> archive
                                 \-> verify (optional diagnostics)
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

Any other `delivery_strategy` value is invalid. Do NOT pick the nearest branch and do NOT proceed: STOP, report the unrecognised value, and re-collect the delivery strategy before launching `sdd-apply`.

Always pass the resolved `delivery_strategy`, `chain_strategy`, and PR boundary/exception to `sdd-apply`.

When launching `sdd-apply`, always include the resolved `delivery_strategy`, `chain_strategy`, and any chosen PR boundary/exception in the prompt.

### Sub-Agent Launch Deduplication (MANDATORY)

Maintain a session-scoped launch log of `(phase, task-fingerprint)` pairs. If the same pair already exists, do NOT launch again. Emit exactly one launch per distinct task and append the pair after launch.

### Sub-Agent Launch Protocol

ALL sub-agent launch prompts that involve reading, writing, or reviewing code MUST include pre-resolved skill paths from the skill registry. Follow `~/.claude/skills/_shared/skill-resolver.md`.

Resolve skills once per session, cache the registry, and pass exact `SKILL.md` paths. If a delegated result reports `skill_resolution` as `fallback-registry`, `fallback-path`, or `none`, re-read the registry before subsequent delegations.

**Key Learnings closing (generic delegations):** When delegating to generic agents (Explore, general-purpose), instruct the sub-agent to close its final message with a `## Key Learnings` section containing 1–5 numbered items, each a standalone factual sentence of ≥4 words and ≥20 characters. This enables engram passive capture of learnings across delegation boundaries. SDD phase agents load this requirement from `~/.claude/skills/_shared/sdd-phase-common.md` section F automatically.

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

### Archive Final-State Handoff (MANDATORY)

When launching `sdd-archive`, forward explicit final-state facts for any work completed after `apply-progress` or `verify-report` were persisted — verify warnings fixed in later commits, blockers resolved, tasks finished, updated test or issue counts — with commit or evidence references where available. Those two artifacts are intermediate snapshots, valid at the time they were written; the archive report records the state at close, and explicit final-state facts in the `sdd-archive` launch prompt outrank stale snapshot claims.

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

Recover from native status and the actual artifacts identified by `artifactStore` and `artifactPaths`, not a locally reconstructed DAG. In `openspec`, read resolved file paths; in `engram`, use project-scoped `mem_search` followed by full `mem_get_observation`; in `hybrid`, follow each resolved locator without substituting the other store. In `none`, use available conversation context and disclose what cannot be recovered.

Existing `state.yaml` and `sdd/{change-name}/state` snapshots are optional recovery hints, never required per-phase writes or a second authority. Preserve historical snapshots and `dependsOn` metadata; check progress and archive closure against actual artifacts. Missing or stale hints do not block recovery or establish active work.
