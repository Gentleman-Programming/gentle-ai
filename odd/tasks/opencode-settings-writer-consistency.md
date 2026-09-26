# OpenCode settings writer consistency

## Objective
Ensure install and sync write OpenCode component settings only to the effective JSON/JSONC configuration selected by the existing resolver, without changing a distinct user-owned settings file. Prepare new, independently scoped PRs rather than continuing closed #5000–#5003.

## Problem and why
Issue #4471's `__managed_by` rejection is handled by merged #5004. A distinct JSON/JSONC writer inconsistency remains: component writers may use `adapter.SettingsPath(homeDir)` while routing/model assignment use the effective selected settings path. This can split managed configuration across files or change the user's separate JSON.

## Scope and constraints
- Authorized: user requested new PRs after closing #5000–#5003; no push/merge until applicable checks and issue-first policy are satisfied.
- Start from merged `main` at `3a19dbf8961bea9771b4e867cf3872e47ebe2699` in an isolated clean worktree. Preserve the uncommitted experimental authority-sidecar candidate in its separate worktree; do not transplant it without evidence.
- Prefer passing the existing selected path to writers over introducing a new persistent authority sidecar/state. No new state or TUI behavior without evidence that path plumbing alone cannot solve the defect.
- Preserve global and workspace scope, JSONC comments, symlink safety, file modes, snapshot/rollback and user-owned data. Test-first RED on clean main for each reachable writer. Generated technical artifacts in English.
- Forecast 500–800 authored lines across integration/tests, subject to measured diffs. Delivery strategy: `single-pr` with a user-accepted size exception. Rationale: one root invariant across all reachable settings writers needs integrated isolation and rollback tests; report measured final size without code-golf. Applying protected `size:exception` label still requires its exact direct instruction and target-specific verification before PR delivery.

## Tasks
- [x] A1 [delegated]: Reproduce and fix the Theme OpenCode writer bypass on clean main, aligning install/sync path declarations, backup and tests; observe RED/GREEN. Route: delegated, multi-file behavior/tests. Commit identity to be recorded below; focused and isolated full CLI checks passed.
- [ ] A2 [delegated]: Reproduce and fix remaining reachable settings writers (Persona settings, permissions, Context7, Engram), align backup, reporting and rollback, retaining non-OpenCode behavior. Verify focused and full applicable checks. Route: delegated, multi-file behavior/tests. Commit pending.
- [ ] A3 [inline/delegated verification]: Independently verify actual incremental diffs and native review when available; satisfy issue-first policy, open new scoped PR(s), wait for green CI and merge under ordinary repository policy. Route: state inline, tests/review delegated. Commit not applicable.

## Acceptance and checks
On a dual-file fixture with selected JSONC and a decoy user JSON, install and sync change only selected JSONC; on workspace scope, only workspace settings change. Backup/rollback restore exact original bytes and any affected mode; all component writers reached by those flows honor selection. Focused Go tests, deadcode ratchet, gofmt, full applicable CI and native review evidence are recorded. No old contributor PR is reopened.

## Progress
- #5004 merged as `3a19dbf`; #5000–#5003 closed by explicit user decision, with English closure explanations. Original #4471 scope is complete, separate from this new defect.
- Independent local authority-sidecar experiment in `4471-pr-5000-repair` remains uncommitted, partial: Engram path tests pass, full Engram package has four unclassified Pi provisioning failures, and Theme/Persona output-style remain unverified. Treat as reference only, not delivery evidence.
- New branch `fix/opencode-settings-writer-consistency` began clean at origin/main. CodeGraph exploration was unavailable beyond init; scoped source mapping used.
- A1 implementation: Theme now receives the effective settings path on install/sync, aligns backup/reporting, preserves existing `0600` mode and rejects `0000`, with dual-file, workspace, idempotence and post-write rollback tests. RED was observed on the original wrong-file write and mode broadening; focused Theme/CLI tests, deadcode ratchet, gofmt and diff check passed. Isolated `go test -timeout 20m ./internal/cli -count=1` passed in 690.799s with `PI_CODING_AGENT_DIR` unset and original Go caches retained. Ambient full CLI had failed on a dangling live prompt symlink; that environmental failure remains disclosed. No PR or merge yet.
- The user subsequently chose one PR with a size exception for this feature instead of a chain. Next: independently verify A1 and close its work-unit commit, then implement A2 before preparing the single PR. No push, PR or merge yet.
