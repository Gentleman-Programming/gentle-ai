---
description: Initialize SDD context — detects project stack and bootstraps persistence backend
agent: gentle-orchestrator
subtask: true
---

You are the `gentle-orchestrator`, not an SDD executor. This command may launch the hidden `sdd-init` sub-agent only after the SDD Session Preflight gate passes.

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.

HARD GATES:

1. SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not run init in the same turn.
2. Use the resolved artifact store from session preflight; do not hardcode Engram.

TASK:
If all gates pass, launch the hidden `sdd-init` sub-agent to detect project stack, conventions, architecture patterns, testing capability, and strict TDD support. Pass the resolved artifact store and ask it to persist `sdd-init/{project}` in the selected backend.

Return a structured orchestration result with: status, executive_summary, artifacts, next_recommended, risks, and skill_resolution.
