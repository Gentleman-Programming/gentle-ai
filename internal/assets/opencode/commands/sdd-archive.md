---
description: Archive a completed SDD change — syncs specs and closes the cycle
agent: axiom-orchestrator
---

You are the `axiom-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-archive` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run archive in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. Resolve the active change using the status contract. If `$ARGUMENTS` is missing or ambiguous, ask the user to choose and STOP. Do not guess.
4. Produce structured status before acting. Use the resolved artifact store from session preflight; do not hardcode Engram. Use refreshed native SDD status and preserve actual edit permissions. Completed implementation normally recommends archive; an explicit archive request may close unfinished work without a verification certificate.
5. Read available artifacts at their selected-store references. Missing or failed optional reports and unfinished tasks do not gate archive; SDD never offers or launches RDD.
6. actionContext must allow archive operations. If status reports `workspace-planning`, STOP and explain that workspace archive is not supported in this slice.

Record actual task completion, missing artifacts, and unresolved findings. Preserve historical task/report bytes; do not reconcile checkboxes during archive.

TASK:
If all gates pass, launch the hidden `sdd-archive` sub-agent with the structured status, available artifacts and the resolved artifact store. Archive is a record of actual state, not a completion or verification verdict. For `openspec` or `hybrid` stores only, mandate mechanical filesystem copy/move (`cp -R`/`mv`/`git mv` via the shell) for every filesystem artifact — NEVER model Read/Write copying, which can truncate bytes silently — and require a `diff -r` readback (source vs. destination, archive-report additive-only) whose verbatim output appears in the result; an empty diff is the only passing evidence and a skipped `diff -r` FAILS the phase. For `engram`, do not perform filesystem synchronization or archive moves; retrieve the required Engram artifacts and persist the final archive report to `sdd/{change-name}/archive-report`. For `none`, do not perform filesystem operations or Engram persistence; return the closure summary only. Forward explicit final-state facts for any work completed after `apply-progress` or `verify-report` were persisted (fixed warnings, resolved blockers, updated counts); those artifacts are intermediate snapshots, and explicit final-state facts in the launch prompt outrank stale snapshot claims.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
