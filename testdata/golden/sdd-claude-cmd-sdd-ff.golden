---
description: Fast-forward all SDD planning phases — proposal through tasks
---

If the native `sdd-orchestrator` agent is available, delegate this command to it.
Otherwise, follow the SDD orchestrator workflow inline using the instructions already installed in `~/.claude/CLAUDE.md`.

WORKFLOW:
Run these sub-agents in sequence:
1. `sdd-propose` — create the proposal
2. `sdd-spec` — write specifications
3. `sdd-design` — create technical design
4. `sdd-tasks` — break down into implementation tasks

Present a combined summary after ALL phases complete (not between each one).

CONTEXT:
- Working directory: !`echo -n "$(pwd)"`
- Current project: !`echo -n "$(basename $(pwd))"`
- Change name: $ARGUMENTS
- Artifact store mode: engram

ENGRAM NOTE:
Sub-agents handle persistence automatically. Each phase saves its artifact to engram with topic_key "sdd/$ARGUMENTS/{type}" where type is: proposal, spec, design, tasks.

Read the orchestrator instructions to coordinate this workflow. Do NOT execute phase work inline when a native sub-agent is available.
