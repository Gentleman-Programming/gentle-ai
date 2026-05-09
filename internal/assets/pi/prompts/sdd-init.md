---
description: Initialize SDD context — detects project stack and bootstraps persistence
---

Read `~/.pi/agent/skills/sdd-init/SKILL.md` first, then follow it inline.

Pi MVP defaults:
- Artifact store mode: `openspec` unless the user asks for `none` or confirms Engram tools are available.
- Do not call Engram/MCP tools unless they are visible and verified in this Pi session.

Task:
Initialize Spec-Driven Development in this project. Detect stack, testing, conventions, architecture patterns, Strict TDD status, and skill registry. Persist results according to the chosen mode.

Return: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
