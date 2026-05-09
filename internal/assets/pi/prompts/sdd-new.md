---
description: Start a new SDD change with exploration and proposal
argument-hint: "<change>"
---

Use the SDD orchestrator instructions installed in `~/.pi/agent/APPEND_SYSTEM.md`. Work inline; do not assume native sub-agents.

Change: $ARGUMENTS

First ask/cache if not already chosen:
- Execution mode: `interactive` (default) or `auto`.
- Artifact store: `openspec` (Pi default), `none`, or confirmed `engram`/`hybrid`.
- Delivery strategy: `ask-on-risk` (default), `auto-chain`, `single-pr`, or `exception-ok`.

Workflow:
1. Ensure `/sdd-init` has run.
2. Run exploration for `$ARGUMENTS` using the `sdd-explore` skill.
3. Create a proposal using the appropriate SDD skill/conventions.
4. In interactive mode, summarize and ask before continuing to spec/design.

Do not call Engram/MCP tools unless they are visible and verified in this Pi session.
