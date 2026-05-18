---
description: Archive a completed SDD change — syncs specs and closes the cycle
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-archive` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: !`pwd`
- Current project: !`basename "$(pwd)"`

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run archive in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. The active change must have proposal, spec, design, tasks, apply-progress, and verify-report artifacts in the selected artifact store.
4. Use the resolved artifact store from session preflight; do not hardcode Engram.

DEPENDENCY CHECK:

- If the verification report is missing or does not say the change is ready, do NOT archive.
- Tell the user what is missing and suggest `/sdd-verify <change>` or `/sdd-continue <change>`.

TASK:
If all gates pass, launch the hidden `sdd-archive` sub-agent with references to all required artifacts and the resolved artifact store.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
