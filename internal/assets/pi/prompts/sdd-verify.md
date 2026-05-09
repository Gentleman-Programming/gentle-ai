---
description: Validate implementation against specs, design, and tasks
argument-hint: "[change]"
---

Read `~/.pi/agent/skills/sdd-verify/SKILL.md` first, then follow it inline.

Change: $ARGUMENTS

Pi MVP defaults:
- Artifact store mode: `openspec` unless the user asks for `none` or confirms Engram tools are available.
- Do not call Engram/MCP tools unless they are visible and verified in this Pi session.

Task:
Read the proposal/spec/design/tasks/apply-progress artifacts for the active change. Check completeness, correctness, design coherence, tests, build, and spec compliance. Run the focused validation commands that are safe and available.

Return a structured verification report with: status, executive_summary, detailed_report, artifacts, next_recommended, risks, and skill_resolution.
