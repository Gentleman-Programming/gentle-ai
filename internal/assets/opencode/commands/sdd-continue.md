---
description: Continue the next SDD phase in the dependency chain
agent: gentle-orchestrator
---

Follow the SDD orchestrator workflow to continue the active change; this explicit command may prepare its change-local consent marker but grants no edit roots.

HARD GATE:
SDD Session Preflight must already be complete for this session. It must include execution mode, artifact store, chained PR strategy, and review budget. If missing, ask the exact orchestrator preflight prompt and STOP. Do not launch the next phase in the same turn.

WORKFLOW:

1. For every declared store, first inspect with native `sdd-status`. Before calling `gentle-ai sdd-continue [change] --cwd <repo>`, the host MUST confirm the current human scope covers the selected change-directory marker. Read-only or excluded-marker scope forbids this mutating call; native allowed roots do not grant human consent. Preparation grants no source roots or attempts. If native resolution fails, report it without a local dispatch fallback.
2. Produce or consume structured status before acting: schemaName, planningHome/changeRoot, artifactPaths/contextFiles, task progress, dependency states, next recommended action, blocked reasons, and actionContext.
3. Check which artifacts already exist for the active change (proposal, specs, design, tasks)
4. Follow native `nextRecommended`; do not reconstruct a dependency graph.
5. Launch the appropriate sub-agent(s) for the next phase only if authoritative status says the dependency is ready. Route only by `nextRecommended` and dependency states; never infer from free text. If `blockedReasons` is non-empty, do not proceed to apply, archive, or terminal work. If `nextRecommended` is `verify`, verification/remediation may run only to refresh evidence; if `nextRecommended` is `resolve-blockers`, report `blockedReasons` and stop; if `nextRecommended` is a planning token (`propose`, `spec`, `design`, or `tasks`), launch the corresponding planning phase.
6. Present the result and ask the user to proceed

CONTEXT:

- Working directory: before doing anything else, run `git rev-parse --show-toplevel 2>/dev/null || pwd` with your bash tool and use the returned path as the authoritative workspace. In OpenCode Desktop (Electron) the parse-time interpolation resolves to the app data directory, not the project.
- Current project: the `basename` of the detected workspace above.
- Change name: $ARGUMENTS
- Execution mode: ask/cache per orchestrator
- Artifact store mode: ask/cache per orchestrator; do not hardcode Engram
- Delivery strategy: ask/cache per orchestrator
- Review budget: ask/cache per orchestrator

ENGRAM NOTE:
To check which artifacts exist in engram/hybrid, search: mem_search(query: "sdd/$ARGUMENTS/", project: "{project}") to list all artifacts for this change.
Sub-agents handle persistence automatically using the selected artifact store.

Read the orchestrator instructions to coordinate this workflow. Do NOT execute phase work inline — delegate to sub-agents.

STATUS CONTRACT:

Use native v2 for every store; never substitute continue for read-only status or selected child startup. Carry native `actionContext` intersected with the current narrower human scope into any executor. A missing marker is prepared only by expressly authorized continuation; a present marker is reused, never overwritten, and grant/replay must match its current identity.
