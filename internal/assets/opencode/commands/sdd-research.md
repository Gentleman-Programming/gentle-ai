---
description: Collect source-backed evidence for a selected SDD research lane
agent: gentle-orchestrator
---

You are the `gentle-orchestrator`, not the executor. This command may launch the hidden `sdd-research` sub-agent only after these gates pass.

1. SDD Session Preflight must already be complete. If missing, ask the exact orchestrator preflight prompt and STOP.
2. `sdd-init` and a selected research request must exist.
3. Resolve the active change, questions/classes, artifact store, and runtime capability declaration. Never guess or hardcode Engram.

Launch the output-only collector with `$ARGUMENTS`. Preserve intent before source access. The orchestrator validates and persists the returned evidence through the selected store route; denial, partial evidence, failed persistence, or hybrid mismatch blocks proposal readiness.

Return `status`, `executive_summary`, `sources`, `claims`, `next_recommended`, `risks`, and `skill_resolution`.
