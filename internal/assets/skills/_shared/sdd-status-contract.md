# SDD Status and Instructions Contract

Shared OpenSpec-style contract for SDD commands and phase skills. Use it before acting on a change so orchestration does not guess state, paths, or edit scope.

## Purpose

Commands that select, continue, apply, verify, or archive an SDD change MUST first produce or consume structured status. The status is the handoff between orchestrator and phase executor.

## Change Selection

- If a change name is provided, use that exact change after confirming it exists in the selected artifact store.
- If no change name is provided, infer only when the active change is unambiguous from session state or there is exactly one active change.
- If multiple active changes match or the active change is unclear, ask the user to choose. Do not guess.
- If no active changes exist, report that no SDD change is active and suggest `/sdd-new <change>`.

## Native Engine

Native `gentle-ai.sdd-status/v2` is the sole status contract. A request for v1 or another prior contract fails read-only with one instruction: start a fresh implementation state and rerun `gentle-ai sdd-status --contract gentle-ai.sdd-status/v2`. When status recommends `propose`, the orchestrator-owned pre-proposal gate separately requires confirmed decisions, valid evidence references, and matching hybrid state; selected research must be `done`.

- When the session artifact store is `openspec` or `hybrid` and the `gentle-ai` binary is available, prefer `gentle-ai sdd-status [change] --cwd <repo> --json --instructions` for read-only status and `gentle-ai sdd-continue [change] --cwd <repo>` for dispatcher output. When the store is `engram`, do not invoke those OpenSpec dispatcher commands.
- The native dispatcher reads only OpenSpec file artifacts and emits `artifactStore: openspec`; it cannot observe Engram-backed changes. Treat dispatcher status as authoritative only when the selected artifact store is `openspec` or `hybrid`. When the selected store is `engram`, resolve artifact status from Engram (`mem_search` + `mem_get_observation` on the change topic keys) using the manual schema below.
- Runtime-attempt authority is different from artifact dispatch: normal runtime-bearing OpenSpec and Engram continuations MUST bracket external execution with `gentle-ai sdd-attempt acquire|settle --cwd <repo> --change <change>`. Their bounded result contains only `proceed`, `blocked`, or `complete` plus an opaque continuation token when required, and MAY carry `settle_obligation` on a `proceed`. The Git-common-dir immutable chain remains the sole authority for ordinals, cumulative attempt/line budgets, runtime evidence, and ordinary SDD failed-evidence remediation. Full `status|begin|finish|reset` payloads MUST NOT be embedded in the SDD v2 status document. Never create OpenSpec attempt-ledger files or Engram attempt-ledger topics.
- A phase actor launched by a parent that already holds a `proceed` acquire for that exact work unit authenticates as that same attempt with the returned `--token`; it MUST NOT acquire again blind.
- When `sdd-attempt status` carries a `gentle-ai.sdd-integration.consent/v1` consent block, the ledger is ASKING, not reporting. Treat it as a Lossless Blocking Prompt: relay the complete envelope in order, preserve answer tokens and invocations, and never answer on their behalf. In a non-interactive runtime, emit the complete envelope and STOP. Attempts that never ran the work are not evidence about the candidate.
- For `openspec` and `hybrid` stores, treat native status JSON as authoritative over prompt inference or manually reconstructed state.
- When `blockedReasons` is non-empty, do not proceed to terminal, archive, or apply work. Return or report `blockedReasons` and stop unless `nextRecommended` is `verify`, in which case verification may run only to remediate or refresh evidence for the blockers. When `nextRecommended` is `resolve-blockers`, always report `blockedReasons` and stop. When `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase — missing planning artifacts are the expected output of those phases, not genuine blockers.
- `nextRecommended` is a bounded machine token for routing, not human prose. Route only by `nextRecommended` and dependency states. Human-readable explanation belongs in `blockedReasons`.
- If the binary is unavailable, fall back to this prompt contract and the manual status schema below. Manual fallback status MUST stay shape-compatible with native `gentle-ai.sdd-status` JSON even when values are reconstructed manually.

## Status Schema

Return status as markdown with these fields, or as equivalent JSON when the host supports it. This is the exact frozen external `StatusV2Projection`, not the extensible internal aggregate:

```yaml
schemaName: gentle-ai.sdd-status
schemaVersion: 2
changeName: <change-name-or-null>
artifactStore: openspec | engram | none
planningHome:
  mode: repo-local
  path: <absolute path to openspec>
changeRoot: <absolute path to openspec/changes/<change> or null>
artifactPaths:
  proposal: [<absolute path>]
  specs: [<absolute paths>]
  design: [<absolute path>]
  tasks: [<absolute path>]
  applyProgress: [<absolute path>]
  verifyReport: [<absolute path>]
contextFiles:
  proposal: [<absolute readable files>]
  specs: [<absolute readable files>]
  design: [<absolute readable files>]
  tasks: [<absolute readable files>]
  applyProgress: [<absolute readable files>]
  verifyReport: [<absolute readable files>]
artifacts:
  proposal: missing | done | partial
  specs: missing | done | partial
  design: missing | done | partial
  tasks: missing | done | partial
  applyProgress: missing | done | partial
  verifyReport: missing | done | partial
taskProgress:
  total: 0
  completed: 0
  pending: 0
  allComplete: false
dependencies:
  proposal: blocked | ready | all_done
  specs: blocked | ready | all_done
  design: blocked | ready | all_done
  tasks: blocked | ready | all_done
  apply: blocked | ready | all_done
  verify: blocked | ready | all_done
  archive: blocked | ready | all_done
applyState: blocked | all_done | ready
actionContext:
  mode: repo-local
  workspaceRoot: <absolute path>
  allowedEditRoots: [<absolute paths>]
relationships:
  dependsOn: []
  supersedes: []
  amends: []
  conflictsWith: []
  sameDomainActiveChanges: []
remediationState:
  required: false
  complete: false
  failedEvidenceRevision: ""
  reason: ""
reviewOffer:
  available: true
  invocation: <fresh review start command>
consent: <optional exact gentle-ai.sdd-integration.consent/v1 envelope>
phaseInstructions:
  apply: [<instruction strings>]
  verify: [<instruction strings>]
  remediate: [<instruction strings>]
  archive: [<instruction strings>]
nextRecommended: propose | spec | design | tasks | apply | verify | remediate | archive | sdd-new | select-change | resolve-blockers
blockedReasons: []
```

`reviewOffer` is optional and appears only after strict independent verification passes while review mode is enabled. It is a fresh mode-only offer with exactly `available` and `invocation`; it carries no lineage, receipt, binding, successor, gate, transaction, or previous review result. Disabled review mode is structural absence. Repeated status reads may present the same fresh offer and no offered, declined, burned, or historical authority changes archive readiness.

`phaseInstructions` is optional and appears only when instructions are requested. It carries execution-phase keys (`apply`, `verify`, `remediate`, `archive`); planning-phase instructions (`propose`, `spec`, `design`, `tasks`) are surfaced in dispatcher markdown. `consent` is structurally absent everywhere except an OpenSpec-backed native status that reports `blocked(edit_authority_missing)`; manual fallback MUST NOT reconstruct it. Empty path fields MUST be arrays, not null. `changeName` and `changeRoot` are nullable; all other non-optional sections should be present in fallback output so consumers can parse native and manual status the same way.

## Apply State

- `blocked`: Required apply artifacts are missing, task selection is ambiguous, or action context makes edits unsafe.
- `all_done`: Tasks artifact exists and every implementation task is checked `[x]`.
- `ready`: Tasks artifact exists, at least one implementation task remains unchecked, and edit scope is safe.

## Dependency States

- `proposal`, `specs`, `design`, and `tasks` report whether prerequisite artifacts are blocked, ready, or all done.
- `apply` is `ready` only when specs, design, and tasks are available and task progress is not all done.
- `verify` is `ready` only when every implementation task is complete and required planning/apply evidence is available. Review presence, absence, or non-allow state is informational: it never routes status to `review`, suppresses test/build execution, or blocks verification. Apply-progress and focused work-unit checks support implementation evidence but never replace the independent final SDD verification.
- Verify routing parses only the strict leading `gentle-ai.verify-result/v1` envelope. It compares measured requirement/scenario totals with actual specs and requires current test/build commands, zero passing exit codes, and output hashes. Human prose never controls readiness.
- Failed evidence may route to `remediate` only through ordinary SDD failed-evidence accounting for the same failed evidence revision. Remediation completion requires concrete focused-test, runtime-harness (or justified N/A), and rollback evidence; a bare envelope never passes.
- `archive` is `ready` only when tasks are complete and strict SDD verification passes. A `reviewOffer` never authorizes, blocks, or governs archive or delivery.
- A passing remediation settlement requires a fresh verification report before archive. The historical failed report is preserved and never erased, no PASS is fabricated, and archive stays blocked until a current passing report exists.
- Before a runtime-bearing continuation, call compact `sdd-attempt acquire` with `<acquire-id>` and launch only for `state: proceed`; retain its opaque token and call compact `sdd-attempt settle` after the external run with a distinct `<settle-id>`. Reuse each operation's own request ID only for its idempotent replay. `blocked` or `complete` stops the launch, and settle's three states alone control whether another bounded acquire is allowed. When acquire returns `settle_obligation`, RELAY IT TO THE HUMAN VERBATIM BEFORE LAUNCHING THE WORK UNIT, and carry it into the settle. It is never a block — the token is real and the launch proceeds. Reset remains an explicit maintainer scope decision and never occurs automatically.
- Planning and apply phases never auto-launch ordinary 4R or Judgment Day. Only after independent SDD verification passes may status present the optional review offer. Pre-commit, pre-push, pre-PR, and release follow ordinary repository policy; review outcomes never create a delivery gate or a new review budget.

## Action Context Guard

The orchestrator MUST carry `actionContext` into any phase launch.

- If manually reconstructed context cannot prove edit ownership or allowed edit roots, stop before editing.
- If `allowedEditRoots` is present, only edit files within those roots.
- If a command cannot prove a file is inside the authoritative workspace or allowed edit roots, stop and ask for clarification.

## Edit Authority Consent

A change whose tasks.md work units target paths outside `allowedEditRoots` never reports apply ready. Native status reports `applyState: blocked` and `blockedReasons` carries a `blocked(edit_authority_missing)` reason naming each unauthorized edit root and both exits: edit tasks.md so every work unit stays inside the authorized edit roots, or grant this change edit authority for the named edit roots.

- Detection is conservative prose inspection: backticked path-like tokens inside markdown checkbox lines that resolve to a path in a Git repository outside the authorized roots. A different repository is named by its Git root; a same-repository target is narrowed to its containing edit root. A context reference can raise a false consent question; the consequence is a question, never silent authority.
- An OpenSpec-backed native status that reports `blocked(edit_authority_missing)` also carries the typed `gentle-ai.sdd-integration.consent/v1` envelope as the optional `consent` block: headline, reason, `value`, the missing roots as evidence, exactly two choices with answer tokens `granted` and `declined` (each with label, effect, and an exact invocation), and an off-path note.
- Answer flow: the orchestrator relays the COMPLETE envelope losslessly as a blocking prompt. Only on the human's explicit `granted` answer does the agent execute the envelope's named grant invocation, verbatim and exactly once, then re-enter through native status. The agent NEVER runs the grant unprompted and NEVER answers on the human's behalf.
- Decline stays blocked: the agent runs the envelope's decline invocation, nothing is persisted, the change stays `blocked(edit_authority_missing)`, and the reason names both exits.

## Status Output

Every command that acts on a change MUST show status before launching an executor or performing archive work:

- Active change selection and schemaName.
- Artifact statuses and paths/topics used as context.
- Task progress and unchecked task list when tasks exist.
- Next recommended action.
- `blockedReasons` when `nextRecommended` is not `verify`, plus any edit-root blockers.
