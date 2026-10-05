<!-- gentle-ai:opencode-background-subagents -->
### OpenCode Background Subagent Policy

Use OpenCode's Task tool with `background: true` only for independent, read-only exploration, audit, or review work where the parent can continue non-overlapping work.

At the parent level, allow no more than 2 concurrent background tasks. Completion notifications only: do not poll, sleep, run status checks, or proactively read for completion.

Use foreground tasks when the result is needed before the next action, for user decisions, SDD apply or other writers, dependent verify evidence, archive, formal RDD/4R lenses, refuters, fix validators, Judgment Day actors, or dependent phases.

Do not duplicate launches or work, and do not overlap files or topics. Never run parallel writers in one worktree.

Background jobs are process-local and non-durable. A restart loses them; make no recovery claim. If `background` is absent from the Task tool schema, or the capability is disabled or unknown, omit `background` and run the task in the foreground.
<!-- /gentle-ai:opencode-background-subagents -->
