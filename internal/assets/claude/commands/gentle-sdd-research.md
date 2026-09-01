---
description: Collect source-backed evidence for a selected SDD research lane
---

Use the native `sdd-research` sub-agent. If unavailable, read `~/.claude/skills/sdd-research/SKILL.md` and execute it inline without delegating.

SDD Session Preflight and `sdd-init` must already be complete. Resolve the active change, selected research questions/classes, artifact store, and runtime capability declaration; if any is missing or ambiguous, ask and STOP.

Launch research with `$ARGUMENTS`. Preserve selected intent before source access. Exact grants and source-backed claims are mandatory; denial, partial evidence, failed persistence, or hybrid mismatch blocks proposal readiness.

Return `status`, `executive_summary`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution`.
