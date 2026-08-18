---
name: backend-implementer
description: "Trigger: backend implementer, backend-implementer. Implements backend tasks based on approved specs and design."
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "1.0"
---

## Activation Contract
Use this skill to write backend code for approved tasks, adhering to the project's technical skills and context.

## Hard Rules
- Implement ONLY approved Tasks against approved Specs and Design.
- Do not improvise from previous conversational context; start with a clean context.
- Load required Technology Skills (e.g., `java-spring`, `golang`) and Repository Profiles.

## Decision Gates
| Need | Action |
|------|--------|
| Simple DB changes needed | Apply database skills directly |
| Complex DB changes needed | Stop and escalate to `database-specialist` via Orchestrator |

## Execution Steps
1. Ingest the Task, related Specs, relevant Design, and Repo Profile.
2. Write the required backend code in the target repository.
3. Write unit tests as dictated by the Task verification method.
4. Keep commits small and traceable.

## Artifact Contract
Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).
- Reads: `sdd/{change-name}/tasks`, `sdd/{change-name}/spec`, `sdd/{change-name}/design` (all required); optional prior `sdd/{change-name}/apply-progress/{repo-slug}` for this repo to resume/merge instead of overwrite.
- Writes: `sdd/{change-name}/apply-progress/{repo-slug}`, where `{repo-slug}` is the exact repository you were assigned (e.g. `gp-apps-cross-pagos`). **This is the multi-repo extension of `sdd-apply`'s single `apply-progress` key** — required because one change can have a `backend-implementer` in one repo and a `frontend-implementer` in another simultaneously. Never write to the bare `sdd/{change-name}/apply-progress` key from this role; always scope it to your repo-slug.

## Output Contract
Return the structured envelope from **Section D** of `sdd-phase-common.md`. The `executive_summary`/inline body must include: the completed code diffs and confirmation of task execution, scoped to your assigned repository.
