---
description: Implement SDD tasks — writes code following specs and design
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command is allowed to launch the hidden `sdd-apply` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run apply in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. Resolve the active change using the status contract. If `$ARGUMENTS` is missing or ambiguous, ask the user to choose and STOP. Do not guess.
4. Produce structured status before acting and use it to confirm the active change has spec, design, and tasks artifacts in the selected artifact store.
5. Review workload guard must have passed. If task forecast exceeds the session review budget or needs a chained-PR decision, ASK and STOP unless the preflight strategy already resolves it.
6. actionContext must allow implementation edits. If status reports `workspace-planning` with no allowed edit roots, STOP before launching apply.

DEPENDENCY CHECK:

- If spec, design, or tasks are missing, do NOT implement.
- Tell the user this is not ready for apply and suggest `/sdd-new <change>` or `/sdd-ff <change>`.

TASK:
If all gates pass, launch the hidden `sdd-apply` sub-agent with:

- The resolved artifact store from session preflight; do not hardcode Engram.
- The structured status: schemaName, planningHome/changeRoot, artifactPaths/contextFiles, task progress, applyState, dependency states, and actionContext.
- References to the spec, design, tasks, and any apply-progress artifacts.
- The resolved delivery/chained PR strategy and review budget.
- Strict TDD instructions if `sdd-init` detected strict TDD.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.

POST-APPLY REVIEW ROUTING:
After apply returns, rerun native SDD status. If `nextRecommended: review`, the parent orchestrator begins negotiated review routing with `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v1 --next-transition`. Read only `next_transition` and route only from the returned `next_transition`: for `execute`, invoke its exact operation and ordered argument tokens unchanged; for `collect`, satisfy only its exact named inputs and capture operations, then query STATUS again; for `stop`, stop without running a lifecycle operation. The parent never substitutes direct START, and the apply executor never launches review.

### Authority-First Terminal Procedure

Use only the compact facade; it appends and reads back native authority before materializing existing compatibility artifacts.

| Order | Operation | Required result | Terminal mirrors |
|---|---|---|---|
| 01 | `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v1 --next-transition` | one provider-owned `next_transition` returned | blocked |
| 02 | `provider-returned transition` | exact `execute` operation/arguments or `collect` inputs completed; `stop` halts | blocked |
| 03 | repeat 01–02 | exact returned `review.validate` allows the terminal gate | blocked |
| 04 | `reconcile-terminal-mirrors` | existing mirrors reconciled | allowed |

After ambiguous output, query STATUS again; native discovery reports the committed authority and its next transition without another budget. Malformed or ambiguous lineage remains invalid.

Reuse a valid receipt; later commit/push/PR/release events only validate it.
