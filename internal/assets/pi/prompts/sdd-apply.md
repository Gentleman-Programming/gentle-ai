---
description: Implement SDD tasks following specs and design
argument-hint: "[change]"
---

Read `~/.pi/agent/skills/sdd-apply/SKILL.md` first, then follow it inline.

Change: $ARGUMENTS

Pi MVP defaults:
- Artifact store mode: `openspec` unless the user asks for `none` or confirms Engram tools are available.
- Do not call Engram/MCP tools unless they are visible and verified in this Pi session.

Before editing:
1. Ensure `/sdd-init` has run.
2. Read the active proposal/spec/design/tasks artifacts.
3. Check the review workload guard and delivery strategy.
4. Check Strict TDD status.

Task:
Implement the next incomplete task batch. If Strict TDD is active, write a failing test first, implement the minimum, then refactor. Update task/progress artifacts.

Return: status, executive_summary, detailed_report with files changed, artifacts, next_recommended, risks, and skill_resolution.
