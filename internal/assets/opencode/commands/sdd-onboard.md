---
description: Guided SDD walkthrough — onboard a user through the full SDD cycle using their real codebase
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-onboard` sub-agent only after the orchestration gates below pass.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not start onboarding in the same turn.
2. Use the resolved artifact store from session preflight; do not hardcode Engram.

TASK:
If all gates pass, launch the hidden `sdd-onboard` sub-agent to guide the user through a real SDD cycle. Keep user-facing pauses in interactive mode and enforce the review budget before apply.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
