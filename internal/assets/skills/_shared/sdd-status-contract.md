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

- When the session artifact store is `openspec` or `hybrid` and the `gentle-ai` binary is available, prefer `gentle-ai sdd-status [change] --cwd <repo> --json --instructions` for read-only status and `gentle-ai sdd-continue [change] --cwd <repo>` for dispatcher output. When the store is `engram`, do not invoke those OpenSpec dispatcher commands (see the next bullet).
- The native dispatcher reads only OpenSpec file artifacts and always emits `artifactStore: openspec`; it cannot observe Engram-backed changes. Treat dispatcher status as authoritative only when the selected artifact store is `openspec` or `hybrid`. When the selected store is `engram`, resolve artifact status from Engram (`mem_search` + `mem_get_observation` on the change topic keys) using the manual status schema below, and disregard any dispatcher `blocked`, `Active OpenSpec change not found`, or `nextRecommended: sdd-new` result for an Engram change that exists.
- Runtime-attempt authority is different from artifact dispatch: normal runtime-bearing OpenSpec and Engram continuations MUST bracket external execution with `gentle-ai sdd-attempt acquire|settle --cwd <repo> --change <change>`. Their bounded result contains only `proceed`, `blocked`, or `complete` plus an opaque continuation token when required, and MAY carry `settle_obligation` on a `proceed`. The Git-common-dir immutable chain remains the sole authority for ordinals, cumulative attempt/line budgets, runtime evidence, and atomic bound remediation. Full `status|begin|finish|reset` operations remain diagnostic/compatibility surfaces; their payload MUST NOT be embedded in the SDD v1 status document. Never create OpenSpec attempt-ledger files or Engram attempt-ledger topics.
- A phase actor launched BY a parent that already holds a `proceed`-state acquire for that exact work unit is a distinct call/process, not a fresh continuation: it MUST NOT `acquire` again blind. Colliding with its own parent's active attempt is not a genuine `blocked: active_attempt` (#2291). It authenticates as that SAME attempt by passing the parent's returned token on its own `acquire --token <token>` call: a token matching the ledger's live active attempt returns `proceed` with that same token and zero mutation, while a non-matching token gets the ordinary `blocked: active_attempt` naming the real active token.
- For `openspec` and `hybrid` stores, treat native status JSON as authoritative over prompt inference or manually reconstructed state.
- When `blockedReasons` is non-empty, do not proceed to terminal, archive, or apply work. Return or report `blockedReasons` and stop unless `nextRecommended` is `verify`, in which case verification may run only to remediate or refresh evidence for the blockers. When `nextRecommended` is `resolve-blockers`, always report `blockedReasons` and stop. When `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase — missing planning artifacts are the expected output of those phases, not genuine blockers.
- `nextRecommended` is a bounded machine token for routing, not human prose. Route only by `nextRecommended` and dependency states.
- Human-readable explanation belongs in `blockedReasons`, not `nextRecommended`.
- If the binary is unavailable, fall back to this prompt contract and the manual status schema below. Manual fallback status MUST stay shape-compatible with native `gentle-ai.sdd-status` JSON even when values are reconstructed manually.

## Status Schema

Return status as markdown with these fields, or as equivalent JSON when the host supports it. This is the exact frozen external `StatusV1Projection`, not the extensible internal aggregate:

```yaml
schemaName: gentle-ai.sdd-status
schemaVersion: 1
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
  reviewPolicy: [<absolute path>]
  reviewLedger: [<absolute path>]
  reviewReceipt: [<absolute path>]
  reviewBundle: [<absolute path to reviews/chain-bundle.json>]
  reviewContext: [<absolute path>]
  reviewState: [<absolute path to reviews/transaction.json>]
contextFiles:
  proposal: [<absolute readable files>]
  specs: [<absolute readable files>]
  design: [<absolute readable files>]
  tasks: [<absolute readable files>]
  applyProgress: [<absolute readable files>]
  verifyReport: [<absolute readable files>]
  reviewPolicy: [<absolute readable files>]
  reviewLedger: [<absolute readable files>]
  reviewReceipt: [<absolute readable files>]
  reviewBundle: [<absolute readable files>]
  reviewContext: [<absolute readable files>]
  reviewState: [<absolute readable files>]
artifacts:
  proposal: missing | done | partial
  specs: missing | done | partial
  design: missing | done | partial
  tasks: missing | done | partial
  applyProgress: missing | done | partial
  verifyReport: missing | done | partial
  reviewPolicy: missing | done | partial
  reviewLedger: missing | done | partial
  reviewReceipt: missing | done | partial
  reviewBundle: missing | done | partial
  reviewContext: missing | done | partial
  reviewState: missing | done | partial
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
  lineageId: ""
  generation: 0
  fixBatch: 0
  reason: ""
reviewGate:
  result: allow | scope-changed | invalidated | escalated
  reason: <deterministic explanation>
reviewTransaction: <optional exact gentle-ai.review-transaction/v1 object>
consent: <optional exact gentle-ai.sdd-integration.consent/v1 envelope>
phaseInstructions:
  apply: [<instruction strings>]
  verify: [<instruction strings>]
  remediate: [<instruction strings>]
  archive: [<instruction strings>]
nextRecommended: propose | spec | design | tasks | apply | verify | remediate | archive | sdd-new | select-change | resolve-blockers
blockedReasons: []
```

`phaseInstructions` is optional and appears only when instructions are requested. It carries execution-phase keys (`apply`, `verify`, `remediate`, `archive`); planning-phase instructions (`propose`, `spec`, `design`, `tasks`) are surfaced in dispatcher markdown. `reviewGate` is a structurally absent key — not merely omitted early — whenever the kill switch is off, or whenever it is on with no review ever discovered for this candidate; both proceed to archive under ordinary policy with no `reviewGate` to check. When present (a review was actually discovered), its result uses only the four listed values. `reviewTransaction` is omitted until the review owner supplies the exact `gentle-ai.review-transaction/v1` object; manual fallback MUST NOT reconstruct it. `consent` is structurally absent everywhere except an OpenSpec-backed native status that reports `blocked(edit_authority_missing)` (see Edit Authority Consent below); manual fallback MUST NOT reconstruct it. `reviewPolicy` is present in `artifactPaths` and `contextFiles`; its artifact-state entry is present only for Engram status and omitted otherwise. A hybrid session projects file-backed legacy status as `artifactStore: openspec`; `hybrid` is not an SDD v1 wire token. Empty path fields MUST be arrays, not null. `changeName` and `changeRoot` are nullable; all other non-optional sections should be present in fallback output so consumers can parse native and manual status the same way.

## Apply State

- `blocked`: Required apply artifacts are missing, task selection is ambiguous, or action context makes edits unsafe.
- `all_done`: Tasks artifact exists and every implementation task is checked `[x]`.
- `ready`: Tasks artifact exists, at least one implementation task remains unchecked, and edit scope is safe.

## Dependency States

- `proposal`, `specs`, `design`, and `tasks` report whether prerequisite artifacts are blocked, ready, or all done.
- `apply` is `ready` only when specs, design, and tasks are available and task progress is not all done.
- `verify` is `ready` only when every implementation task is complete and required planning/apply evidence is available. Review presence, absence, or non-allow state is informational: it never routes status to `review`, suppresses test/build execution, or blocks verification. Apply-progress and focused work-unit checks support implementation evidence but never replace the independent final SDD verification.
- Verify routing parses only the strict leading `gentle-ai.verify-result/v1` envelope. It compares measured requirement/scenario totals with actual specs and requires current test/build commands, zero passing exit codes, and output hashes. Human prose never controls readiness.
- Failed evidence may route to `remediate` only when an exact persisted transaction lineage/generation has remaining mode-specific fix budget and names the same failed evidence revision. Remediation completion requires concrete focused-test, runtime-harness (or justified N/A), and rollback evidence bound to that transaction; a bare envelope never passes.
- `archive` is `ready` only when tasks are complete and strict SDD verification passes. `reviewGate` and `reviewOffer` are informational review context only: their presence, absence, or result never authorizes, blocks, or governs archive or delivery. Scope change and new external evidence retain their explicit lifecycle/recovery semantics without becoming archive gates.
- OpenSpec review artifacts use `openspec/changes/{change-name}/reviews/{transaction,ledger,receipt,chain-bundle,gate-context}.json`. Engram uses exact topics `sdd/{change-name}/review/{transaction,ledger,receipt,chain-bundle,gate-context}`. The chain bundle is a portable non-authoritative recovery source and requires explicit validated import into the repository-derived store. Do not substitute prompt-only state when these native artifacts are available.
- The review-ledger sentence "failure escalates and never starts another reviewer, refuter, correction, or validator" names four REVIEW actors and bounds the review lifecycle only. It never forbade SDD's own verification from running again. A maintainer-authorized remediation that the native runtime settles against the exact failed evidence revision ADMITS one verification refresh, which is why status routes to `verify` after it; that refresh is not a second independent verification and not another correction cycle. The historical failed report is preserved and never erased, no PASS is fabricated, and archive stays blocked until a current passing report exists.
- When `sdd-attempt status` carries a `consent` block (`gentle-ai.sdd-integration.consent/v1`), the attempt budget is exhausted and the ledger is ASKING, not reporting. Treat it as a Lossless Blocking Prompt: relay the complete envelope once — headline, reason, value, every evidence line, both choice labels and their effects, and the off-path note — in the active conversation language, preserving the order, the selection mode, and the answer tokens `granted` and `declined` verbatim. Never translate the tokens, commands or revisions. The human decides: never answer on their behalf, and never run the grant unprompted. On `granted`, run the envelope's grant invocation verbatim, exactly once, then re-query status. On `declined`, stop with all state preserved. In a non-interactive runtime, emit the complete envelope and STOP. When the envelope reports attempts that never ran the work, say so plainly: those attempts are provider defects, not evidence about the candidate, and more than one of them means a retry is unlikely to converge.
- Before a runtime-bearing continuation, call compact `sdd-attempt acquire` with `<acquire-id>` and launch only for `state: proceed`; retain its opaque token and call compact `sdd-attempt settle` after the external run with a distinct `<settle-id>`. Reuse each operation's own request ID only for its idempotent replay. `blocked` or `complete` stops the launch, and settle's three states alone control whether another bounded acquire is allowed. When acquire returns `settle_obligation`, RELAY IT TO THE HUMAN VERBATIM BEFORE LAUNCHING THE WORK UNIT, and carry it into the settle. It names a demand the settle will already enforce, derived from the immutable attempt chain, so it is not advice and not a warning to weigh: an attempt is a bounded, spendable resource, and discovering the demand at settle costs the operator that attempt with no way to reclaim it. It is never a block — the token is real and the launch proceeds. Settle derives current binding and failed-evidence authority internally unless legacy status lacks that revision, in which case pass the exact failed revision with `--remediates-evidence-revision`; pass `--successor-lineage` only when review approved a distinct successor. Reset remains an explicit maintainer scope decision and never occurs automatically. A passed remediation attempt routes to fresh verification only when its remediated-evidence revision exactly equals the strict failed verify envelope and its atomically selected compact successor still validates live.
- Planning and apply phases never auto-launch ordinary 4R or Judgment Day. Only after independent SDD verification passes may status present the optional review offer. Pre-commit, pre-push, pre-PR, and release follow ordinary repository policy; review outcomes are informational and never create a delivery gate or a new review budget. Major or post-incident releases may request explicit extraordinary review without making it delivery authority.

## Action Context Guard

The orchestrator MUST carry `actionContext` into any phase launch.

- If manually reconstructed context cannot prove edit ownership or allowed edit roots, stop before editing.
- If `allowedEditRoots` is present, only edit files within those roots.
- If a command cannot prove a file is inside the authoritative workspace or allowed edit roots, stop and ask for clarification.

## Edit Authority Consent

A change whose tasks.md work units target paths outside `allowedEditRoots` never reports apply ready. Native status reports `applyState: blocked` and `blockedReasons` carries a `blocked(edit_authority_missing)` reason naming each unauthorized edit root and both exits: edit tasks.md so every work unit stays inside the authorized edit roots, or grant this change edit authority for the named edit roots.

- Detection is conservative prose inspection: backticked path-like tokens inside markdown checkbox lines that resolve to a path in a Git repository outside the authorized roots. A different repository is named by its Git root; a same-repository target is narrowed to its containing edit root. A context reference can raise a false consent question; the consequence is a question, never silent authority.
- An OpenSpec-backed native status that reports `blocked(edit_authority_missing)` also carries the typed `gentle-ai.sdd-integration.consent/v1` envelope as the optional `consent` block: headline, reason, `value`, the missing roots as evidence, exactly two choices with answer tokens `granted` and `declined` (each with label, effect, and an exact invocation), and an off-path note. The granted choice names the exact runnable `gentle-ai sdd-attempt grant` invocation, bound to this change instance; the declined choice's invocation is native status re-entry. An Engram-backed change keeps the honest block without an envelope; both exits still apply.
- Answer flow: the orchestrator relays the COMPLETE envelope losslessly as a blocking prompt, with the same discipline as the review consent relay. Preserve the choices, order, selection mode, exact allowed-answer domain, and answer tokens; translate labels only within the lossless rules; never summarize or reshape. The human answers in conversation. Only on the human's explicit `granted` answer does the agent execute the envelope's named grant invocation, verbatim and exactly once, then re-enter through native status. The agent NEVER runs the grant unprompted and NEVER answers on the human's behalf.
- Decline stays blocked: the agent runs the envelope's decline invocation, nothing is persisted, the change stays `blocked(edit_authority_missing)`, and the reason names both exits.
- The grant is per-change and audited (who, when, which roots). It lives in the change's runtime ledger, is bound to the change instance, and dies with archive; a recreated change with the same name never inherits it. After a covering grant, status reports `allowedEditRoots` as the planning root plus the granted roots, apply becomes ready, and the `consent` block disappears.

## Status Output

Every command that acts on a change MUST show status before launching an executor or performing archive work:

- Active change selection and how it was resolved.
- Artifact statuses and paths/topics used as context.
- Task progress and unchecked task list when tasks exist.
- Next recommended action.
- `blockedReasons` when `nextRecommended` is not `verify`, plus any edit-root blockers.
