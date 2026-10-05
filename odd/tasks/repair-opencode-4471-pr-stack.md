# Repair OpenCode #4471 PR stack

## Objective
Deliver a reviewable, green sequence for PRs #5004 → #5000 → #5001 → #5002 → #5003, fixing the `__managed_by` provider rejection without altering user-owned OpenCode settings.

## Problem and why
OpenCode forwards unknown agent fields to the provider, so managed agents carrying `__managed_by` fail on strict `opencode-go`. The existing PRs are blocked by deadcode, failing integration tests, cumulative review-size gates, missing labels, and an absent uninstall diff.

## Scope and constraints
- Authorized: repair the named PR branches and merge each in order after required checks pass. Do not bypass protection or alter unrelated local changes.
- Use an isolated worktree. Preserve JSONC comments, user-owned agents, symlink refusal, rollback and settings authority.
- Delivery strategy: stacked-to-main, already chosen by the existing five-PR chain; each PR must become a coherent incremental unit. Forecast: roughly 1,600 authored lines across the chain; count actual incremental diffs before delivery. One honest slicing pass, no code-golf to meet the 400-line gate.
- Test first when a deterministic regression is runnable; document observed RED/GREEN, or explain environmental limitations.

## Tasks
- [ ] T1 [delegated]: Make #5004 marker cleanup reachable as a coherent first behavior with integration tests; resolve deadcode and validate focused and required tests. Route: delegated, multi-file behavior/tests. Commit: pending. Review: pending.
- [ ] T2 [delegated]: Repair #5000 authority slice, production reachability and safe pinning; keep incremental diff and CI within policy. Route: delegated, multi-file behavior/tests. Commit: pending. Review: pending.
- [ ] T3 [delegated]: Repair #5001 sync migration, upgrade compatibility, rollback and preflight tests. Route: delegated, multi-file behavior/tests. Commit: pending. Review: pending.
- [ ] T4 [delegated]: Repair #5002 install/TUI preflight and test expectations without regressing upgrades. Route: delegated, multi-file behavior/tests. Commit: pending. Review: pending.
- [ ] T5 [delegated]: Restore #5003 uninstall lifecycle and TOCTOU tests; verify actual incremental diff. Route: delegated, multi-file behavior/tests. Commit: pending. Review: pending.
- [ ] T6 [inline]: Validate target policy, labels and size, update each PR safely, wait for green required CI, merge sequentially and verify default branch. Route: inline remote state/delivery. Commit: not applicable.

## Acceptance and checks
Each delivered PR has one real behavior with tests, only its incremental diff, at most 400 changed lines or a separately authorized policy exception, and green required CI. The final merged main removes legacy marker from eligible agents, never writes it back, preserves unrelated settings/JSONC, rejects invalid authority before mutation, and removes its sidecar only after successful uninstall state updates. No merge proceeds on failing or unknown required checks.

## Progress
- Evidence: initial PR heads #5004 d652462a, #5000 e1c402c3, #5001 6b662650, #5002 97bf7d74, #5003 96d8810. All were OPEN/BLOCKED with `Unit Tests` failure. #5003 head deleted its uninstall implementation and tests.
- Working branch: `fix/4471-pr-stack-repair`, isolated from the unrelated dirty checkout.
- T1 local candidate: production sync cleanup runs after guidance consumes legacy markers; new integration tests cover eligible/user-owned agents, symlinks, idempotency and post-write exact-byte rollback. Focused upgrade/migration tests, filemerge and deadcode ratchet pass after gofmt; full CLI suite timed out at 480 seconds without result. Work-unit commit `0dc8cc60cd40b175b371769d8e5495107191ae8a` (not yet published or fully checked). T1 remains open pending CI.
- Review workload: current #5004 diff is 288 lines; candidate adds about 243 changed source/test lines and this task document (~560 cumulative). The user explicitly authorized the protected `size:exception` label for #5004 only to preserve these five PR IDs; actor permission is admin. This does not waive CI. No other PR has an exception authorization. Next: rerun focused checks after gofmt, commit the T1 work unit, update #5004 and await required CI before considering a merge.
