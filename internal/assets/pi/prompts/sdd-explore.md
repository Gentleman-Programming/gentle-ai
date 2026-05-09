---
description: Explore and investigate an idea or feature without edits
argument-hint: "<topic>"
---

Read `~/.pi/agent/skills/sdd-explore/SKILL.md` first, then follow it inline.

Topic: $ARGUMENTS

Pi MVP defaults:
- Artifact store mode: `openspec` unless the user asks for `none` or confirms Engram tools are available.
- Do not call Engram/MCP tools unless they are visible and verified in this Pi session.

Task:
Explore `$ARGUMENTS` in this codebase. Read relevant files, identify affected areas, compare approaches, and recommend a path. Do not edit code.

Return: status, executive_summary, detailed_report, artifacts, next_recommended, risks, and skill_resolution.
