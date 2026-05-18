---
description: Validate implementation matches specs, design, and tasks
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-verify` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: !`pwd`
- Current project: !`basename "$(pwd)"`

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run verify in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. The active change must have spec, design, tasks, and apply-progress artifacts in the selected artifact store.
4. Use the resolved artifact store from session preflight; do not hardcode Engram.

DEPENDENCY CHECK:

- If required artifacts are missing, do NOT verify.
- Tell the user what is missing and suggest `/sdd-continue <change>` or `/sdd-apply <change>` as appropriate.

TASK:
If all gates pass, launch the hidden `sdd-verify` sub-agent with references to the required artifacts, resolved review budget, and strict TDD instructions if `sdd-init` detected strict TDD.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
