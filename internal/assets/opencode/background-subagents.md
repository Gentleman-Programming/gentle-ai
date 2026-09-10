<!-- gentle-ai:opencode-background-subagents -->
### OpenCode Background Subagent Policy

Once this addendum is present and the Task capability is active, the parent MUST request `background: true` for eligible independent, non-mutating work when it has concrete non-overlapping work to continue and does not need that result before its next action.

Eligible work includes:

- repository exploration/mapping.
- external/web research is eligible only when independent and non-blocking; provider or network failure means no silent retries or duplicate launches.
- independent planning inputs.
- read-only audits/reviews outside formal RDD.
- independent long-running test/build/lint/check-only verification, but only when it does not mutate shared source, generated artifacts, repository state, or files needed by concurrent work. If that isolation cannot be established, run it in the foreground.

### Joining background work

Independent tasks whose results are needed only at a later join may launch concurrently. At the parent level, keep no more than 2 background tasks active concurrently. The parent must join only through completion notifications: never poll, sleep, run status checks, or proactively read results. A background failure still emits a completion/failure notification. Never silently retry; any retry requires explicit reclassification and must not duplicate active work. Do not duplicate launches or overlap files or topics.

### Foreground invariants

Keep foreground for immediate next-step dependencies, user decisions or interaction, dependent SDD phases and gatekeepers, SDD apply or any writer, archive or closure, lifecycle gates, and formal RDD/4R reviewers, refuters, validators, or Judgment Day actors whenever their governing contract requires foreground. Any same-worktree or shared mutation is foreground. Never run parallel writers in one worktree.

### Capability limits

Background work is process-local and non-durable; restarting the parent process loses all background jobs. Do not claim durable scheduling, recovery, isolation, or runtime activity. If `background` is absent from the Task schema, or the capability is disabled or unknown, omit it and use the safe foreground fallback.
<!-- /gentle-ai:opencode-background-subagents -->
