---
description: Start a new SDD change — runs exploration then creates a proposal
---

Read `~/.claude/skills/_shared/sdd-orchestrator-workflow.md` FIRST, then treat it as the authoritative SDD workflow instructions for this command.
The Claude Code session model is controlled by Claude Code; Gentle AI only configures models for Agent tool calls to phase sub-agents.

WORKFLOW:

1. Launch `sdd-explore` to investigate the codebase for this change
2. Present the exploration summary to the user
3. Launch `sdd-propose` to create a proposal based on the exploration
4. Present the proposal summary and ask the user if they want to continue with specs and design

CONTEXT:

- Working directory: Detect agent-side before proceeding by running `git rev-parse --show-toplevel` with the Bash tool; if that fails, run `pwd` with the Bash tool.
- Current project: Derive agent-side from the detected working directory basename. Do not use slash-command shell interpolation for this value.
- Change name: $ARGUMENTS
- Execution mode: ask/cache per orchestrator
- Artifact store mode: ask/cache per orchestrator
- Delivery strategy: ask/cache per orchestrator

ENGRAM NOTE:
Sub-agents handle persistence automatically. Each phase saves its artifact to engram with topic_key "sdd/$ARGUMENTS/{type}".

Use the lazy workflow instructions to coordinate this workflow. Do NOT execute phase work inline when a native sub-agent is available.
