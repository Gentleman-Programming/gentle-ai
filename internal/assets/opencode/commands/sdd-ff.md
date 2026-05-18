---
description: Fast-forward all SDD planning phases — proposal through tasks
agent: gentle-orchestrator
---

Follow the SDD orchestrator workflow to fast-forward all planning phases for change "$ARGUMENTS".

HARD GATE:
SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not launch planning sub-agents in the same turn.

WORKFLOW:
Run these sub-agents in sequence:

1. sdd-propose — create the proposal
2. sdd-spec — write specifications
3. sdd-design — create technical design
4. sdd-tasks — break down into implementation tasks

Present a combined summary after ALL phases complete (not between each one).

CONTEXT:

- Working directory: !`pwd`
- Current project: !`basename "$(pwd)"`
- Change name: $ARGUMENTS
- Execution mode: ask/cache per orchestrator
- Artifact store mode: ask/cache per orchestrator; do not hardcode Engram
- Delivery strategy: ask/cache per orchestrator
- Review budget: ask/cache per orchestrator

ENGRAM NOTE:
Sub-agents handle persistence automatically using the selected artifact store. In engram/hybrid, each phase saves with topic_key "sdd/$ARGUMENTS/{type}" where type is: proposal, spec, design, tasks.

Read the orchestrator instructions to coordinate this workflow. Do NOT execute phase work inline — delegate to sub-agents.
