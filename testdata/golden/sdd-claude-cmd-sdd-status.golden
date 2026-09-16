---
description: Show structured SDD status for an active change
---

Show structured SDD status for an active change. This command is read-only: do not launch SDD executors and do not edit files.

Inspection needs no execution preflight, review, delivery, or archive authorization. It grants no write authority.

CONTEXT:

- Working directory: Detect agent-side before proceeding by running `git rev-parse --show-toplevel` with the Bash tool; if that fails, run `pwd` with the Bash tool.
- Current project: Derive agent-side from the detected working directory basename. Do not use slash-command shell interpolation for this value.
- Change name: $ARGUMENTS

TASK:

1. If the `gentle-ai` binary is available, run `gentle-ai sdd-status [change] --cwd <repo> --json --instructions` for every declared artifact store, including Engram. Consume native v2 unchanged as authoritative. If native resolution fails or is invalid, report it and stop; do not reconstruct status locally or call continue. If the binary is unavailable, read `~/.claude/skills/_shared/sdd-status-contract.md` for non-authoritative diagnostic guidance only. Do not fabricate native-shaped status, readiness, or mutation authority.
2. Use the active selection returned by native status. If `$ARGUMENTS` is provided, require that exact identity; if native status requires selection, ask the user to choose and STOP. Do not guess or select from local artifact inspection.
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
- Display `nextRecommended` and `blockedReasons` without executing any recommendation, including planning phases. Never run the preparation invocation just because status displays it. Do not infer routing from free text.
- If status cannot be resolved safely, return `status: blocked` with the missing information.
