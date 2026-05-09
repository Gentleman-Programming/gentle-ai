---
description: Continue the next SDD phase in the dependency chain
argument-hint: "[change]"
---

Use the SDD orchestrator instructions installed in `~/.pi/agent/APPEND_SYSTEM.md`. Work inline; do not assume native sub-agents.

Change: $ARGUMENTS

Workflow:
1. Ensure `/sdd-init` has run.
2. Inspect existing artifacts for the active or named change.
3. Determine the next dependency-ready phase:
   proposal → [spec + design] → tasks → apply → verify → archive.
4. Read the matching phase skill from `~/.pi/agent/skills/` and execute it inline.
5. In interactive mode, summarize and ask before the next phase.

Do not call Engram/MCP tools unless they are visible and verified in this Pi session.
