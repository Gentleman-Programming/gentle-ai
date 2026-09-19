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

Native `gentle-ai.sdd-status/v2` is the sole status contract. A request for v1 or another prior contract fails read-only with one instruction: start a fresh implementation state and rerun `axiom sdd status --contract gentle-ai.sdd-status/v2`. Research is optional and does not add a proposal-admission gate. Ask about real unresolved product decisions; pause only dependent work.

- When the `axiom` binary is available, prefer `axiom sdd status [change] --cwd <repo> --json --instructions` for read-only status and `axiom sdd continue [change] --cwd <repo>` only for explicit authorized continuation. This holds for every artifact store: the dispatcher resolves the declared store itself.
- The native dispatcher resolves the artifact store the workspace DECLARES in `openspec/config.yaml` and reports it in `artifactStore`. A declared store is authoritative in both directions: it selects the resolver, and an empty declared store reports as empty rather than silently serving the other store's artifacts. Never re-resolve artifact status yourself, and never branch on the store: read the locators the dispatcher returned in `artifactPaths`.
- For every store, treat native status JSON as authoritative over prompt inference or manually reconstructed state.
- When `blockedReasons` is non-empty, do not proceed to terminal, archive, or apply work. Return or report `blockedReasons` and stop. Explicit read-only verification may still diagnose accessible work without granting edit authority. When `nextRecommended` is `resolve-blockers`, always report `blockedReasons` and stop. When `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase — missing planning artifacts are the expected output of those phases, not genuine blockers.
- `notes` is a separate, always-present array of informational diagnostics. A non-empty `notes` NEVER blocks anything: never withhold apply, sync, archive, or a terminal route because a note is present, and never read a note as a blocker. Report notes next to the route you report, so a human sees a future authority need before reaching it. `blockedReasons` stays reserved for genuine blockers, so the gate above reads `blockedReasons` alone.
- `nextRecommended` is a bounded machine token for routing, not human prose. Route only by `nextRecommended` and dependency states. A genuine blocker's human-readable explanation belongs in `blockedReasons`; a non-blocking explanation belongs in `notes`.
- If the binary is unavailable or invalid, report that native status is unresolved. Do not fabricate native-shaped status, recompute readiness, or invoke continue as a fallback.

## Status Schema

Render the native projection as markdown or JSON. These fields document the exact frozen external `StatusV2Projection`, not permission to reconstruct native readiness:

```yaml
schemaName: gentle-ai.sdd-status
schemaVersion: 2
changeName: <change-name-or-null>
artifactStore: openspec | engram | hybrid | none
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
consent: <optional exact gentle-ai.sdd-integration.consent/v1 envelope>
phaseInstructions:
  apply: [<instruction strings>]
  verify: [<instruction strings>]
  archive: [<instruction strings>]
nextRecommended: propose | spec | design | tasks | apply | verify | archive | sdd-new | select-change | resolve-blockers
blockedReasons: []
notes: []
```

SDD status never reads review mode, offers RDD, or exposes review state. After completed implementation, proceed directly toward archive; verification is optional and standalone non-SDD review remains separate.

`phaseInstructions` is optional and appears only when instructions are requested. It carries execution-phase keys (`apply`, `verify`, `archive`); planning-phase instructions (`propose`, `spec`, `design`, `tasks`) are surfaced in dispatcher markdown. `consent` requires an OpenSpec-backed native status reporting `blocked(edit_authority_missing)` with a valid persisted marker; never reconstruct it. Empty path fields MUST be arrays, not null. `blockedReasons` and `notes` are always arrays as well, both `[]` rather than null when empty. `changeName` and `changeRoot` are nullable; all other non-optional sections are required in native output.

## Apply State

- `blocked`: Required apply artifacts are missing, task selection is ambiguous, or action context makes edits unsafe.
- `all_done`: Tasks artifact exists and every implementation task is checked `[x]`.
- `ready`: Tasks artifact exists, at least one implementation task remains unchecked, and edit scope is safe.

## Dependency States

- `proposal`, `specs`, `design`, and `tasks` report whether prerequisite artifacts are blocked, ready, or all done.
- `apply` is `ready` only when specs, design, and tasks are available and task progress is not all done.
- `verify` is an optional diagnostic and may inspect partial implementation. Its availability never certifies a report or task completion; missing artifacts limit conclusions.
- Completed implementation normally recommends `archive`; pending work normally recommends `apply`. Explicit archive can record incomplete work, but actual edit permissions and mechanical archive/spec-composition safety checks remain mandatory.
- Report presence is a locator/status fact, not a verdict. Missing, stale, malformed, or failed reports do not block archive or create remediation state. Preserve historical bytes and report actual unfinished tasks and findings without inventing PASS.
- No SDD phase offers or launches RDD, ordinary 4R, or Judgment Day. Delivery follows ordinary repository policy; optional diagnostics create no review or delivery authority.

## Action Context Guard

The orchestrator MUST carry `actionContext` into any phase launch.

- If manually reconstructed context cannot prove edit ownership or allowed edit roots, stop before editing.
- If `allowedEditRoots` is present, only edit files within those roots.
- If a command cannot prove a file is inside the authoritative workspace or allowed edit roots, stop and ask for clarification.

## Edit Authority Consent

Status inspection requires no launch, review, delivery, or archive preflight and never runs a phase.
Without a persisted marker, missing-root status omits consent and displays explicit preparation guidance.
Before continue, the host must check current human scope includes the change-local marker; a planning-only
marker grant does not grant source roots. Read-only/excluded-marker scope suppresses the mutating call.
Continue preserves valid marker bytes and reads back the no-replace winner; malformed, inaccessible,
failed publication/readback, or detected directory replacement emits no usable consent. No universal
atomic visibility is claimed for publisher copy fallbacks. Grant and grant replay consume the CURRENT
persisted marker and refuse absent/stale identity without initializing a marker or appending authority.


A change whose tasks.md work units target paths outside `allowedEditRoots` never reports apply ready. Native status reports `applyState: blocked` and `blockedReasons` carries a `blocked(edit_authority_missing)` reason naming each unauthorized edit root and the three exits: edit tasks.md so every work unit stays inside the authorized edit roots, grant this change edit authority for the named edit roots, or mark a read-only input with `(read-only)` on its line.

- Detection is conservative prose inspection: backticked path-like tokens inside markdown checkbox lines that resolve to a path outside the authorized roots. A different repository is named by its Git root; a same-repository target is narrowed to its containing edit root; a directory in no Git repository is named as itself. A backticked path immediately followed by `(read-only)` (case-insensitive) is a read-only input and not an edit target; the marker annotates only the path it follows, so an unmarked path on the same line still counts.
- Read-only status never prepares consent; after explicit authorized `sdd-continue` preparation, an OpenSpec-backed status reporting `blocked(edit_authority_missing)` carries the typed `gentle-ai.sdd-integration.consent/v1` envelope as the optional `consent` block: headline, reason, `value`, the missing roots as evidence, exactly two choices with answer tokens `granted` and `declined` (each with label, effect, and an exact invocation), and an off-path note.
- A later work unit's own unauthorized root does NOT block the current work unit. It is reported as a `note(future_edit_roots)` entry in `notes` naming that future root, so the current unit's `applyState` stays `ready` and `blockedReasons` stays empty until that work unit is reached.
- Answer flow: the orchestrator relays the COMPLETE envelope losslessly as a blocking prompt. Only on the human's explicit `granted` answer does the agent execute the envelope's named grant invocation, verbatim and exactly once, then re-enter through native status. The agent NEVER runs the grant unprompted and NEVER answers on the human's behalf.
- Decline stays blocked: the agent runs the envelope's decline invocation, nothing is persisted, the change stays `blocked(edit_authority_missing)`, and the reason names all three exits.

## Status Output

Every command that acts on a change MUST show status before launching an executor or performing archive work:

- Active change selection and schemaName.
- Artifact statuses and paths/topics used as context.
- Task progress and unchecked task list when tasks exist.
- Next recommended action.
- `blockedReasons` whenever it is non-empty, including any edit-root blockers.
- `notes` whenever it is non-empty, labeled as informational and explicitly not a blocker.

For an already archived change, terminal `all_done` dependencies mean no phase remains; they do not certify implementation or verification. Read the archive report and preserved task/report artifacts for actual outcomes.
