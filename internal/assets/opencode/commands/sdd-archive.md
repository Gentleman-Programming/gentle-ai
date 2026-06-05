---
description: Archive a completed SDD change — syncs specs and closes the cycle
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-archive` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run archive in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. Resolve the active change using the status contract. If `$ARGUMENTS` is missing or ambiguous, ask the user to choose and STOP. Do not guess.
4. Produce structured status before acting. Use the resolved artifact store from session preflight; do not hardcode Engram.
5. The active change must have tasks and verify-report artifacts in the selected artifact store. Proposal/spec/design are expected for full spec-driven archive; if missing, report the exact missing artifacts and require an explicit user override before archiving.
6. actionContext must allow archive operations. If status reports `workspace-planning`, STOP and explain that workspace archive is not supported in this slice.
7. The persisted tasks artifact must reflect completion before the archive is considered successful. Internal todos do not count, and `sdd-apply` is responsible for marking completed tasks.

DEPENDENCY CHECK:

- If the verification report is missing or does not say the change is ready, do NOT archive.
- If tasks still contains unchecked implementation items (`- [ ]`), do NOT archive by default. Send the change back to `sdd-apply` to correct the persisted tasks artifact. Only allow archive-time mechanical reconciliation when apply-progress / verify-report prove every unchecked task is complete; record the reconciliation in the archive report.
- If verify-report contains CRITICAL issues, do NOT archive. There is no CRITICAL override.
- Tell the user what is missing and suggest `/sdd-verify <change>` or `/sdd-continue <change>`.

TASK:
If all gates pass, launch the hidden `sdd-archive` sub-agent with the structured status, references to all required artifacts, the resolved artifact store, and any explicit non-critical partial-archive or stale-checkbox reconciliation text. Tell it to enforce the task completion gate before syncing specs or moving the archive folder, and to treat checkbox fixes as exceptional reconciliation rather than normal archive work.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
