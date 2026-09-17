---
description: Show structured SDD status for an active change
agent: gentle-orchestrator
---

You are the `gentle-orchestrator`. This command is read-only: it never prepares consent markers, launches SDD executors, or edits files.

Inspection needs no execution preflight, review, delivery, or archive authorization. It grants no write authority.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace.
- Current project: the `basename` of the detected workspace above.
- Change name: $ARGUMENTS

TASK:

1. Run `gentle-ai sdd-status [change] --cwd <repo> --json --instructions` for every declared artifact store, including Engram. Consume native v2 unchanged; if unavailable or invalid, report the failure without inventing native-shaped status or calling continue.
2. Resolve the active change:
   - If `$ARGUMENTS` is provided, validate that exact change in the selected artifact store.
   - If omitted and exactly one active change exists, select it and say how it was selected.
   - If omitted or ambiguous with multiple active changes, ask the user to choose and STOP. Do not guess.
3. Inspect the declared artifact store and locators returned by native status. Do not hardcode Engram.
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
- Display `nextRecommended` and `blockedReasons` without executing any recommendation, including planning phases. Never run the preparation invocation just because status displays it.
- If status cannot be resolved safely, return `status: blocked` with the missing information.
