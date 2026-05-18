---
description: Start a new SDD change — runs exploration then creates a proposal
agent: gentle-orchestrator
---

Follow the SDD orchestrator workflow for starting a new change named "$ARGUMENTS".

HARD GATE:
SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not launch exploration or proposal in the same turn.

WORKFLOW:

1. Launch sdd-explore sub-agent to investigate the codebase for this change
2. Present the exploration summary to the user
3. Launch sdd-propose sub-agent to create a proposal based on the exploration
4. Present the proposal summary and ask the user if they want to continue with specs and design

CONTEXT:

- Working directory: !`pwd`
- Current project: !`basename "$(pwd)"`
- Change name: $ARGUMENTS
- Execution mode: ask/cache per orchestrator
- Artifact store mode: ask/cache per orchestrator; do not hardcode Engram
- Delivery strategy: ask/cache per orchestrator
- Review budget: ask/cache per orchestrator

ENGRAM NOTE:
Sub-agents handle persistence automatically using the selected artifact store. In engram/hybrid, each phase saves with topic_key "sdd/$ARGUMENTS/{type}".

Read the orchestrator instructions to coordinate this workflow. Do NOT execute phase work inline — delegate to sub-agents.
