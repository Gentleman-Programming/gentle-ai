---
description: Guided SDD walkthrough using the current codebase
---

Read `~/.pi/agent/skills/sdd-onboard/SKILL.md` first, then follow it inline.

Pi MVP defaults:
- Artifact store mode: `openspec` unless the user asks for `none` or confirms Engram tools are available.
- Do not call Engram/MCP tools unless they are visible and verified in this Pi session.

Task:
Guide the user through a complete SDD cycle using their real codebase. Teach by doing: initialize, explore, propose, spec, design, tasks, apply, verify, and archive. Pause at decision points unless the user chooses automatic mode.

Return: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
