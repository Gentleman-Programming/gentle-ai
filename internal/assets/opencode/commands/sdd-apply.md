---
description: Implement SDD tasks — writes code following specs and design
agent: gentle-orchestrator
subtask: true
---

You are an SDD sub-agent. Read the skill file at ~/.config/opencode/skills/sdd-apply/SKILL.md FIRST, then follow its instructions exactly.

The sdd-apply skill (v2.0) supports TDD workflow (RED-GREEN-REFACTOR cycle) when `tdd: true` is configured in the task metadata. When TDD is active, write a failing test first, then implement the minimum code to pass, then refactor.

CONTEXT:
- Working directory: !`echo -n "$(pwd)"`
- Current project: !`echo -n "$(basename $(pwd))"`
- Artifact store mode: engram

TASK:
Implement the remaining incomplete tasks for the active SDD change.

ENGRAM PERSISTENCE (artifact store mode: engram):
CRITICAL: mem_search returns 300-char PREVIEWS, not full content. You MUST call mem_get_observation(id) for EVERY artifact.
STEP A — SEARCH (get IDs only):
  mem_search(query: "sdd/{change-name}/spec", project: "{project}") → save spec_id
  mem_search(query: "sdd/{change-name}/design", project: "{project}") → save design_id
  mem_search(query: "sdd/{change-name}/tasks", project: "{project}") → save tasks_id
STEP A2 — CHECK PREVIOUS PROGRESS (before starting work):
  mem_search(query: "sdd/{change-name}/apply-progress", project: "{project}") → if found, save progress_id
  - Previous apply-progress (if exists): `mem_search(query: "sdd/{change-name}/apply-progress", project: "{project}")` → read and merge
STEP B — RETRIEVE FULL CONTENT (mandatory):
  mem_get_observation(id: spec_id) → full spec
  mem_get_observation(id: design_id) → full design
  mem_get_observation(id: tasks_id) → full tasks (keep tasks_id for updates)
  IF progress_id exists: mem_get_observation(id: progress_id) → read previous progress, skip completed tasks, MERGE when saving
Update tasks as you complete them:
  mem_update(id: {tasks-observation-id}, content: "{updated tasks with [x] marks}")
Save progress:
  mem_save(title: "sdd/{change-name}/apply-progress", topic_key: "sdd/{change-name}/apply-progress", type: "architecture", project: "{project}", capture_prompt: false, content: "{progress report}")
  Set capture_prompt: false when the Engram tool schema supports it; if an older schema rejects or does not expose the field, omit it rather than failing.

For each task:
1. Read the relevant spec scenarios (acceptance criteria)
2. Read the design decisions (technical approach)
3. Read existing code patterns in the project
4. Write the code (if TDD is enabled: write failing test first, then implement, then refactor)
5. Mark the task as complete [x]

Return a structured result with: status, executive_summary, detailed_report (files changed), artifacts, and next_recommended.
