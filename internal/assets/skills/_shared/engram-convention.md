# Engram Convention (reference documentation)

Critical Engram calls (`mem_search`, `mem_save`, `mem_get_observation`) belong in the calling skill's own contract. This reference explains ODD task recovery; do not use a search preview as the full task document.

## ODD task naming and recovery

For a substantial authorized feature, the local task document is `odd/tasks/{feature-name}.md` and its project-scoped Engram mirror has topic key `odd/{feature-name}/tasks`. The mirror contains the full document and repository-relative locator, not a summary. Use a stable filename-safe feature name; never overwrite another feature. Both writes must be read back because they are not atomic.

To resume, obtain session context, search the current project and feature for the task topic, retrieve its full observation with `mem_get_observation`, then read the local task file. Reconcile before continuing. Preserve both versions of irreconcilable edits and ask only about the actual conflict. If Engram is unavailable, keep local progress and mark the mirror pending; resynchronize when available. Never treat a stale mirror as authority over observed code or tests.

Memory lifecycle rule (when Engram exposes lifecycle metadata/tooling):
- At session start or before architecture-sensitive work, call `mem_review` with action `list` for the current project when the tool is available.
- If `mem_review` is unavailable, do not fail the task. Continue with normal `mem_context`/`mem_search`, and still apply lifecycle metadata from any returned observations when present.
- `active` memories may be used normally.
- `needs_review` memories are stale context, not trusted facts.
- Surface `needs_review` context and verify it against current evidence before relying on it.
- Do NOT call `mem_review` with action `mark_reviewed` automatically. Only call `mark_reviewed` after explicit user confirmation or through a dedicated memory maintenance command.

## Writing and updating

Use the resolved project name for project-scoped memory. A `mem_save` with the same `topic_key`, project and scope is an upsert: it overwrites previous content, not an audit trail. Read the current observation before updating an evolving topic and preserve unrelated progress. Use `mem_update` when the exact observation ID is available. For the automated task-document mirror, set `capture_prompt: false` when supported; for normal human-directed memory saves, retain the normal capture behavior. If the tool does not support that field, omit it rather than failing. Never store secrets or user data in a task mirror.

After each verified task transition, update the local document and its full Engram mirror, then read both back. Record actual checks and work-unit evidence, including failures and pending mirrors; do not claim a successful mirror when its write failed.
