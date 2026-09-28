# Persistence Contract for Organic Work

## Small and substantial work

Small, understood work needs no durable task artifact. For substantial authorized implementation, create `odd/tasks/{feature-name}.md` before the first source write and mirror the full current document under project-scoped Engram topic `odd/{feature-name}/tasks`. The document records objective, scope, constraints, actionable task IDs, acceptance checks, progress and next step. Do not create a planning artifact for read-only work. The local document and memory mirror are complementary recovery copies, not competing approval authorities.

Read back both writes. If Engram is unavailable, preserve the local document and explicitly mark the mirror pending without blocking independent safe work. If a local write is unsafe, preserve existing state and report the limitation. On resume, retrieve the full observation (search results are truncated), read the document and reconcile them before modifying either. Preserve conflicting versions until the human resolves a genuine conflict. Preserve historical user data; do not erase or replace it to make recovery appear clean.

## Delegated work

The parent selects relevant context and gives the worker its authorized edit surfaces, task document locator, TDD mode and source, exact test runner and verification commands. The worker reads the task document before editing, stays inside those surfaces, and reports actual checks. Significant verified project discoveries can be saved with `mem_save` under the resolved project. The parent reconciles the task document and full mirror after each observed transition. A task checkbox is not review authority.

Skill paths are supplied in `## Skills to load before work`; workers read those exact `SKILL.md` files before task-specific work. See `skill-resolver.md` for matching and reporting.

## Safe writes and response order

Engram upserts replace existing topic content. Read, merge and write the complete task document rather than replacing earlier completed work with one batch. Set `capture_prompt: false` for an automated task mirror when the tool supports it; normal human-directed saves use the memory provider's default behavior. Never claim a write succeeded without readback. After persisting, finish with a complete text handoff; a memory tool result is not a reply. A worker does not close the parent's session.

Optional research findings stay in the conversation unless the user or authorized task contract requests persistence; disclose failed writes honestly. Native RDD review remains separate from task persistence, and delivery follows ordinary repository policy.
