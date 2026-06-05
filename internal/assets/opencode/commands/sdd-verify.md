---
description: Validate implementation matches specs, design, and tasks
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-verify` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run verify in the same turn.
2. `sdd-init` must already exist or be run after preflight, per the orchestrator init guard.
3. Resolve the active change using the status contract. If `$ARGUMENTS` is missing or ambiguous, ask the user to choose and STOP. Do not guess.
4. Produce structured status before acting. Use the resolved artifact store from session preflight; do not hardcode Engram.
5. The active change must have tasks and implementation evidence. Missing specs/design may be handled gracefully by the verify skill, but missing tasks means there is nothing to verify.
6. actionContext must be safe for verification. If status reports `workspace-planning`, STOP and explain that full workspace implementation verification is not supported in this slice.

DEPENDENCY CHECK:

- If tasks are missing, do NOT verify.
- Tell the user what is missing and suggest `/sdd-continue <change>` or `/sdd-apply <change>` as appropriate.

TASK:
If all gates pass, launch the hidden `sdd-verify` sub-agent with the structured status, references to available artifacts, resolved review budget, and strict TDD instructions if `sdd-init` detected strict TDD.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
