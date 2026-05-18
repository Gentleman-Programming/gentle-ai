---
description: Explore and investigate an idea or feature — reads codebase and compares approaches
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-explore` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: !`pwd`
- Current project: !`basename "$(pwd)"`
- Topic to explore: $ARGUMENTS

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run explore in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. Use the resolved artifact store from session preflight; do not hardcode Engram.

TASK:
If all gates pass, launch the hidden `sdd-explore` sub-agent to investigate "$ARGUMENTS". This is exploration only: no file edits and no implementation.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
