## OpenCode Background Subagent Policy

The OpenCode orchestrator dispatches genuine, independent work to native `task(..., background: true)` so the orchestrator session stays free to coordinate, observe, and report. The native task primitive is the only path that uses the background channel; the policy below scopes exactly what is and is not safe to background.

### What is backgroundable

Background is reserved for **genuinely independent work** that meets every condition below. The orchestrator classifies before launching; a single missing condition is enough to keep the work foreground.

- The work has no decision-gated dependency on prior user choice in this session.
- The work has no read or write dependency on a sub-agent result that has not yet returned.
- The work is not formal review, bounded review, Judgment Day, or any RDD-shaped authority actor.
- The work does not run, gate, or block another agent's terminal action (commit, push, PR open, archive, terminal status, decision envelope).
- The work does not require a foreground, interactive, or blocking prompt of its own.

If any of these is false, the work runs foreground. There is no background option for review, RDD actors, dependency-bearing phases, decision-required forks, or terminal lifecycle actions.

### Foreground fallback

The orchestrator falls back to **foreground** in every case where background cannot be enabled safely. The fallback fires when:

- The installed OpenCode does not support the native task background channel or its effective environment was not set before the server/TUI started.
- An attached client cannot change the effective environment of an already-running server; background then requires a restart, and until that restart happens the work runs foreground.
- Capability detection reports the channel is unavailable, unknown, or denied, or `/experimental/capabilities` does not list `backgroundSubagents`.
- The slice touches a worktree or branch where another writer is active, or the orchestrator detects a parallel-writer hazard in the same worktree.
- An unmanaged daemon or `serve`/`attach`/Desktop topology is detected and Gentle AI does not own its lifecycle; background is reported but never auto-applied.

The fallback reports **foreground** as the active mode, never claims background is on when it is not, and surfaces actionable restart or upgrade guidance when the runtime cannot honor background.

### Safety and lifecycle

- Native task background jobs are **process-local, non-durable, and provide no filesystem or worktree isolation**. Gentle AI never relies on durability, never parallelizes writers inside one worktree, and never assumes one background job is safe alongside another writer in the same tree.
- The orchestrator imposes a bounded concurrency policy for background work and rejects duplicate dispatch. Two launches of the same `(phase, task-fingerprint)` pair are one launch; the second is a no-op.
- Completion is **notification-driven**. The orchestrator never polls, never sleeps to wait, and never re-derives state by re-running the background work. Native task completion is the only signal that advances dependent work.
- Restart cancels and loses active background job state. The orchestrator reports restart-loss semantics, never claims recovery, and never resumes an active background job after a restart.

### Policy surface

- The policy is an additive section of the OpenCode orchestrator, scoped by managed markers. The orchestrator is the canonical shared SDD contract; the addendum is OpenCode-only and never applied to non-OpenCode runtimes.
- Kilocode, which historically shared the OpenCode orchestrator asset path, does **not** inherit this policy. Kilocode receives the canonical shared orchestrator without the addendum.
- The policy is observational, not auto-enabling. Activation and precedence live in the Gentle AI CLI and environment controls and are out of scope for the policy section itself.
