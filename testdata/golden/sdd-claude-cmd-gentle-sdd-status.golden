---
description: Show structured SDD status for an active change
---

Show structured SDD status for an active change. This command is read-only: do not launch SDD executors and do not edit files.

HARD GATE:

SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not inspect status in the same turn.

CONTEXT:

- Working directory: Detect agent-side before proceeding by running `git rev-parse --show-toplevel` with the Bash tool; if that fails, run `pwd` with the Bash tool.
- Current project: Derive agent-side from the detected working directory basename. Do not use slash-command shell interpolation for this value.
- Change name: $ARGUMENTS

TASK:

1. If the `gentle-ai` binary is available, run `gentle-ai sdd-status [change] --cwd <repo> --json --instructions` and treat its JSON as authoritative — but only when the session artifact store is `openspec` or `hybrid`. When the session artifact store is `engram`, do NOT invoke the native dispatcher at all — it cannot see the change (it reads only `openspec/changes/`); resolve status entirely from Engram (`mem_search` + `mem_get_observation` on the change's topic keys) using the manual status schema in `~/.claude/skills/_shared/sdd-status-contract.md` (the same schema used when the binary is unavailable). The dispatcher is authoritative only for `openspec`/`hybrid`. If unavailable, read `~/.claude/skills/_shared/sdd-status-contract.md` and follow it.
2. Resolve the active change:
   - If `$ARGUMENTS` is provided, validate that exact change in the selected artifact store.
   - If omitted and exactly one active change exists, select it and say how it was selected.
   - If omitted or ambiguous with multiple active changes, ask the user to choose and STOP. Do not guess.
3. Inspect the selected artifact store from session preflight. Do not hardcode Engram.
4. Return structured status with:
   - Active change selection and schemaName.
   - planningHome, changeRoot, artifactPaths, and contextFiles.
   - Artifact statuses for proposal, specs, design, tasks, apply-progress, and verify-report.
   - Task progress: total, completed, pending, and allComplete.
   - Dependency states for proposal, specs, design, tasks, apply, verify, and archive.
   - Next recommended action.
   - actionContext mode, workspace root, and allowed edit roots.

READ-ONLY RULES:

- Do not create, update, or delete artifacts.
- Do not mark tasks complete.
- Do not launch apply, verify, archive, or continue.
- Do not infer routing from free text. Use `nextRecommended` and dependency states. If `blockedReasons` is non-empty, do not proceed to apply, archive, or terminal work. If `nextRecommended` is `verify`, verification/remediation may run only to refresh evidence; if `nextRecommended` is `resolve-blockers`, report `blockedReasons` and stop; if `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase.
- If status cannot be resolved safely, return `status: blocked` with the missing information.
