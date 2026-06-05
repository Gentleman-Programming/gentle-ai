# SDD Status and Instructions Contract

Shared OpenSpec-style contract for SDD commands and phase skills. Use this before acting on a change so orchestration does not guess state, paths, or edit scope.

## Purpose

Commands that select, continue, apply, verify, or archive an SDD change MUST first produce or consume structured status. The status is the handoff between orchestrator and phase executor.

## Change Selection

- If a change name is provided, use that exact change after confirming it exists in the selected artifact store.
- If no change name is provided, infer only when the active change is unambiguous from session state or there is exactly one active change.
- If multiple active changes match or the active change is unclear, ask the user to choose. Do not guess.
- If no active changes exist, report that no SDD change is active and suggest `/sdd-new <change>`.

## Native Engine

- When the `gentle-ai` binary is available, prefer `gentle-ai sdd-status [change] --cwd <repo> --json --instructions` for read-only status and `gentle-ai sdd-continue [change] --cwd <repo>` for dispatcher output.
- Treat native status JSON as authoritative over prompt inference or manually reconstructed state.
- When `blockedReasons` is non-empty, do not proceed to terminal, archive, or apply work. Return or report `blockedReasons` and stop unless `nextRecommended` is `verify`, in which case verification may run only to remediate or refresh evidence for the blockers. When `nextRecommended` is `resolve-blockers`, always report `blockedReasons` and stop.
- `nextRecommended` is a bounded machine token for routing, not human prose. Route only by `nextRecommended` and dependency states.
- Human-readable explanation belongs in `blockedReasons`, not `nextRecommended`.
- If the binary is unavailable, fall back to this prompt contract and the manual status schema below. Manual fallback status MUST stay shape-compatible with native `gentle-ai.sdd-status` JSON even when values are reconstructed manually.

## Status Schema

Return status as markdown with these fields, or as equivalent JSON when the host supports it:

```yaml
schemaName: gentle-ai.sdd-status
schemaVersion: 1
changeName: <change-name-or-null>
artifactStore: openspec
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
phaseInstructions:
  apply: [<instruction strings>]
  verify: [<instruction strings>]
  archive: [<instruction strings>]
nextRecommended: apply | verify | archive | sdd-new | select-change | resolve-blockers
blockedReasons: []
```

`phaseInstructions` is optional and appears only when instructions are requested. Empty path fields MUST be arrays, not null. `changeName` and `changeRoot` are nullable; all other sections should be present in fallback output so consumers can parse native and manual status the same way. Native status currently emits `artifactStore: openspec`; if native adds store modes later, fallback output must mirror the native token.

## Apply State

- `blocked`: Required apply artifacts are missing, task selection is ambiguous, or action context makes edits unsafe.
- `all_done`: Tasks artifact exists and every implementation task is checked `[x]`.
- `ready`: Tasks artifact exists, at least one implementation task remains unchecked, and edit scope is safe.

## Dependency States

- `proposal`, `specs`, `design`, and `tasks` report whether prerequisite artifacts are blocked, ready, or all done.
- `apply` is `ready` only when specs, design, and tasks are available and task progress is not all done.
- `verify` is `ready` when tasks exist and either apply-progress exists or the tasks artifact shows all intended implementation work complete. Incomplete tasks remain blockers for full verification.
- `archive` is `ready` only when verify-report exists, is clearly passing, and tasks are complete. A clearly passing report needs an explicit PASS/SUCCESS signal and no blocker or negation signals such as FAIL, FAILURE, BLOCKED, CRITICAL, PENDING, TODO, verification blockers, `not passed`, or `pass: no`. CRITICAL verification issues have no override. Explicit recorded exceptions are limited to non-critical partial archives or stale-checkbox reconciliation when apply-progress/verify-report prove completion.

## Action Context Guard

The orchestrator MUST carry `actionContext` into any phase launch.

- If manually reconstructed context cannot prove edit ownership or allowed edit roots, stop before editing.
- If `allowedEditRoots` is present, only edit files within those roots.
- If a command cannot prove a file is inside the authoritative workspace or allowed edit roots, stop and ask for clarification.

## Status Output

Every command that acts on a change MUST show status before launching an executor or performing archive work:

- Active change selection and how it was resolved.
- Artifact statuses and paths/topics used as context.
- Task progress and unchecked task list when tasks exist.
- Next recommended action.
- `blockedReasons` when `nextRecommended` is not `verify`, plus any edit-root blockers.
