---
description: Fast-forward all SDD planning phases — proposal through tasks
---

Follow the SDD orchestrator workflow inline using the instructions already installed in `~/.claude/CLAUDE.md`.
The Claude Code session model is controlled by Claude Code; Gentle AI only configures models for Agent tool calls to phase sub-agents.

WORKFLOW:
Honor the cached execution mode from SDD Session Preflight.

Planning phases:

1. `sdd-propose` — create the proposal
2. `sdd-spec` — write specifications
3. `sdd-design` — create technical design
4. `sdd-tasks` — break down into implementation tasks

- In `interactive` mode: run only the next planning phase, present its summary and artifact path(s), ask whether to adjust or continue, then STOP. Do not launch the following phase until the user confirms.
- In `auto` mode: run all planning phases back-to-back and present a combined summary after all phases complete.

CONTEXT:

- Working directory: Detect agent-side before proceeding by running `git rev-parse --show-toplevel` with the Bash tool; if that fails, run `pwd` with the Bash tool.
- Current project: Derive agent-side from the detected working directory basename. Do not use slash-command shell interpolation for this value.
- Change name: $ARGUMENTS
- Execution mode: ask/cache per orchestrator
- Artifact store mode: ask/cache per orchestrator
- Delivery strategy: ask/cache per orchestrator

ENGRAM NOTE:
Sub-agents handle persistence automatically. Each phase saves its artifact to engram with topic_key "sdd/$ARGUMENTS/{type}" where type is: proposal, spec, design, tasks.

Read the orchestrator instructions to coordinate this workflow. Do NOT execute phase work inline when a native sub-agent is available.
