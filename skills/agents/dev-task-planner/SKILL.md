---
name: dev-task-planner
description: "Trigger: dev task planner, dev-task-planner, task planner, plan tasks. Decomposes specs and design into small, actionable tasks."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill to transform Specs and Design into executable work units (Tasks).

## Hard Rules
- Tasks must be small and executable. Do not output broad tasks like "implement payments".
- Each task must specify: ID, repository, dependencies, goal, expected files, acceptance criteria, and verification methods.

## Decision Gates
| Need | Action |
|------|--------|
| Task touches backend and frontend | Split into separate backend and frontend tasks |
| Task involves complex DB changes | Route to `database-specialist` |

## Execution Steps
1. Ingest Specs and Design.
2. Break down the work into logical, small commits/tasks.
3. Define dependencies between tasks to form an execution order.
4. Output the structured tasks for the implementers.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/spec` and `sdd/{change-name}/design` (both required).
- Writes: `sdd/{change-name}/tasks` — same topic_key `sdd-tasks` uses. When a task's `repository` field differs across tasks (multi-repo change), each task must still declare its own `repository` so implementers know which `apply-progress/{repo-slug}` to write.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: a list of tasks, each with `id`, `repository`, `depends_on`, `goal`, `expected_files`, `acceptance_criteria`, and `verification`.
