---
description: Collect source-backed evidence for a selected SDD research lane
---

The command actor is the orchestrator. Use the native `sdd-research` sub-agent. If unavailable, read `~/.claude/skills/sdd-research/SKILL.md` and execute it inline without delegating.

The collector is an output-only evidence collector. It may return a complete `blocked`, `partial`, or `done` record but must not retain intent, mutate repository state, save Engram state, select an artifact store, or persist research/preproposal. Do not let it read local artifacts or call persistence tools.

SDD Session Preflight and `sdd-init` must already be complete. Resolve the active change, selected research questions/classes, preflight-selected artifact store, and runtime capability declaration; if any is missing or ambiguous, ask and STOP.

Launch research with `$ARGUMENTS`. Exact grants and source-backed claims are mandatory; denial, partial evidence, failed persistence, or hybrid mismatch blocks proposal readiness.

The orchestrator validates and persists the returned envelope through the preflight-selected store route. Do this only after the collector returns.

Return `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`.
