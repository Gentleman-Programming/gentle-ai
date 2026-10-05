# Claude native RDD reviewer models

Objective: Let Claude Code TUI users configure six native RDD reviewer roles and have the isolated Claude reviewer transport use the saved model assignment.

Problem: The current custom picker and persisted Claude phase assignments cover SDD/JD, while native RDD launches a separate Claude process without a model argument. Agent frontmatter alone cannot affect it.

Scope: risk, readability, reliability, resilience, refuter, validator; Claude picker, existing assignment storage, Claude generation where appropriate, native Claude reviewer transport, focused tests. Keep SDD, OpenCode and Codex behavior untouched. Preserve presets and the existing missing/invalid-assignment fallback; do not alter isolation. No new preset policy.

Constraints: No unrelated worktree changes; no remote operations. CLI help locally advertises `--model <model>` and `--effort <level>`; model selection is in scope, and any effort wiring requires verified applicability. ODD TDD mode has no confirmed source (do not infer it from OpenSpec); ordinary Go functional checks apply with `go test`.

Delivery: ask-on-risk; initial authored-line forecast ~250–400, revisit before commit. Branch point: 21032461. Engram mirror `odd/claude-native-rdd-models/tasks` pending: memory tools unavailable in this session. Route: delegated direct for both tasks; mapping/preparation and multi-file writer triggers. Advisory ~400 changed lines per task is not a cap; preserve coherent tests and readable code.

- [x] T1: Add six configurable role entries to custom Claude picker and preserve persisted assignments and generated agent fallback. Acceptance: picker can save/reopen all six roles; missing/invalid roles retain established default behavior. Checks: focused picker, app/state, generator tests passed. Route: delegated multi-file writer. Evidence: focused `go test` on picker, app, and Claude generator passed; five existing `review-*` agents map to saved roles, while validator remains native-process-only. Commit: `aa96bec9`.
- [x] T2: Resolve saved role assignment at native RDD execution and pass model through isolated Claude process without changing isolation flags or other providers. Acceptance: all six roles select their configured model; absent/invalid assignments fall back as before; transport tests cover process arguments. Checks: focused reviewerprovider and CLI tests passed, including invalid model/effort fallback. Route: delegated multi-file writer. Evidence: local Claude CLI `--model` help; helper-subprocess argument test; no `--effort` passed. Commit: pending.

Progress: T1 implementation and focused checks observed; T2 implementation and focused checks observed, pending commit and final assessment. Local Claude CLI help confirms `--model` support. Full SDD suite failed at `TestRetirePiSystemPromptBlocksSafeguards` (0644 versus 0600; unchanged HEAD test/implementation, baseline execution not proven). Full CLI suite timed out after 300 seconds; focused CLI tests passed. Runtime boundary tested via helper subprocess arguments, not a live Claude review. Next: record separate work-unit commits, assess native review, report outstanding checks.
