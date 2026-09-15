---
description: Archive a completed SDD change — syncs specs and closes the cycle
---

If the native `sdd-archive` sub-agent is available, delegate this command to it.
Otherwise, read the skill file at `~/.claude/skills/sdd-archive/SKILL.md` FIRST, then follow its instructions exactly inline.

CONTEXT:
- Working directory: Detect agent-side before proceeding by running `git rev-parse --show-toplevel` with the Bash tool; if that fails, run `pwd` with the Bash tool.
- Current project: Derive agent-side from the detected working directory basename. Do not use slash-command shell interpolation for this value.
- Artifact store mode: engram

TASK:
Archive the active SDD change. Read available diagnostics as history, not an archive certificate. Then:

STATUS GATE:
Read `~/.claude/skills/_shared/sdd-status-contract.md` and produce structured status before acting. If `$ARGUMENTS` is missing or ambiguous, ask the user to choose and STOP. Do not guess. Use refreshed native SDD status and preserve actual edit permissions. Completed implementation normally recommends archive; an explicit archive request may close unfinished work without a verification certificate. Missing, stale, malformed, or failed optional reports and unfinished tasks do not gate archive. Preserve their bytes and report unresolved work honestly. SDD never offers or launches RDD. If status reports `workspace-planning`, STOP and explain that workspace archive is not supported in this slice. Carry `contextFiles`, task progress, dependency states, and `actionContext` into the native sub-agent prompt when delegating.

ENGRAM PERSISTENCE (artifact store mode: engram):
CRITICAL: mem_search returns 300-char PREVIEWS, not full content. Call mem_get_observation(id) for every found artifact; missing reports do not gate archive.
STEP A — SEARCH (get IDs only):
  mem_search(query: "sdd/{change-name}/proposal", project: "{project}") → save proposal_id
  mem_search(query: "sdd/{change-name}/spec", project: "{project}") → save spec_id
  mem_search(query: "sdd/{change-name}/design", project: "{project}") → save design_id
  mem_search(query: "sdd/{change-name}/tasks", project: "{project}") → save tasks_id
  mem_search(query: "sdd/{change-name}/verify-report", project: "{project}") → save verify_id
STEP B — RETRIEVE FULL CONTENT (mandatory):
  mem_get_observation(id: proposal_id) → full proposal
  mem_get_observation(id: spec_id) → full spec
  mem_get_observation(id: design_id) → full design
  mem_get_observation(id: tasks_id) → full tasks
  if verify_id exists: mem_get_observation(id: verify_id) → full optional verification report
Record all observation IDs in the archive report for traceability.
Treat `verify-report` and `apply-progress` as intermediate snapshots: the archive report records the state at close per the skill's Final-State Authority section, and explicit final-state facts in your launch prompt outrank stale snapshot claims.

Archival is a MECHANICAL filesystem operation: copy/move artifacts with `cp -R`/`mv`/`git mv` via the shell only — NEVER via model Read/Write, which can truncate bytes silently while reporting success. After every copy/move, run `diff -r` (source vs. destination, archive-report additive-only) and include its verbatim output in the result; an empty diff is the only passing evidence, and a skipped/missing `diff -r` FAILS the phase. If shell access is unavailable, STOP and report `blocked` — do NOT fall back to Read/Write copying.
Save:
  mem_save(title: "sdd/{change-name}/archive-report", topic_key: "sdd/{change-name}/archive-report", type: "architecture", project: "{project}", capture_prompt: false, content: "{archive report with observation IDs}")
  Set capture_prompt: false when the Engram tool schema supports it; if an older schema rejects or does not expose the field, omit it rather than failing.

Then:
1. Sync delta specs into main specs (source of truth)
2. Move the change folder to archive with date prefix
3. Verify the archive is complete

Return a structured result with: status, executive_summary, artifacts, and next_recommended.
