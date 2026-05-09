---
description: Archive a completed SDD change
argument-hint: "[change]"
---

Read `~/.pi/agent/skills/sdd-archive/SKILL.md` first, then follow it inline.

Change: $ARGUMENTS

Pi MVP defaults:
- Artifact store mode: `openspec` unless the user asks for `none` or confirms Engram tools are available.
- Do not call Engram/MCP tools unless they are visible and verified in this Pi session.

Task:
Confirm the verification report shows the change is ready. Then archive the active SDD change according to the active artifact store, preserving traceability to proposal/spec/design/tasks/apply/verify artifacts.

Return: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
