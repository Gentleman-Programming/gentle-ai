# Feature: communitytool version-aware reconcile (#984)

## Goal
`gentle-ai install` / `gentle-ai sync` re-run the CodeGraph npm install when the
installed CLI is older than the target contract, instead of only advising.
`--force-community-tools` forces a fresh install/reconcile bypassing the
reconcile-satisfied shortcut. CLI only (TUI surfacing is a follow-up slice).

## Owner-ratified decisions (2026-09-26)
1. Upgrade trigger: **automatic** during install/sync.
2. Trigger comparison: installed version < `codeGraphUpstreamVersion` (existing
   in-repo contract constant). Deterministic, offline, reuses the existing
   advisory path. NOT npm-latest (network-dependent, flaky decision input).
   Upgrade command stays `@latest` (existing `PackageName`).
3. Scope: CLI slice only. No TUI changes.

## Acceptance scenarios (from issue #984 + decisions)
- Installed CodeGraph < contract → install/sync re-runs the npm install
  (`@latest`) and then reconciles agent wiring. No manual action needed.
- Installed == or > contract → no reinstall (today's skip behavior preserved).
- `--force-community-tools` → skips the reconcile-satisfied shortcut and
  re-runs the community-tool install path even when satisfied.
- Contract older → dropped-blind-targets advisory path unchanged.
- Binary not installed → current first-install path unchanged.

## Constraints
- 400-line budget. Strict TDD: failing test per scenario first.
- Files allowed: internal/components/communitytool/{tool.go,codegraph_version.go}
  + tests; internal/cli/run.go (flag wiring) + its test. Nothing else.
- No new contracts/enums beyond what the scenarios require.

## Race status (2026-09-26)
- Rivals: aleka #1059 (last commit 2026-07-12, stale, CI incomplete),
  #2056 (2026-07-30, stale). Non-maintainer. Proceed-permitted per
  collab-with-alan pre-apply rule; adjudication comment posted on #984.
- No maintainer PR closes #984 as of 2026-09-26 (re-verify before push).

## Tasks
- [ ] T1: reconcile upgrade path (installed < contract → re-run npm install)
- [ ] T2: `--force-community-tools` flag wiring (install + sync)
- [ ] T3: regression tests for no-op when current + first-install unchanged
- [ ] T4: verification (build + package tests + fake-HOME run) + work-unit commit
- [ ] T5: PR body per template + push + CI watch

## Evidence log
- (append commit SHAs and verification output per task)
- Worker round 1 (R1+R3): TDD RED "runner was never called" → GREEN; 11 tests pass. Paused honestly on R2: flag registration lives in internal/cli/install.go|sync.go (surfaces expanded).
- Worker round 2 (R2): flag via explicit parameter (no globals); Install keeps 3-arg legacy signature (TUI unchanged); parser+help+bypass tests RED→GREEN; 18 named tests pass.
- Worker round 3 (sync parity): sync step "sync:community-tool:codegraph-reconcile" runs BEFORE guidance/pi steps; RunSync threads SyncFlags.ForceCommunityTools; RunSyncWithSelection public entry passes false (TUI unchanged).
- Anti-slop trim (2026-09-26, commit 04479c4ae): owner challenged the diff size; audit against the AI-slop discipline removed the EnsureCodeGraphUpToDate alias + its 4 tests (invented contract re-covering existing assertions), TestInstallForceFalseHonoursR1Invariants (all 3 cases already covered), TestSyncHelpAdvertises..., TestSyncCurrentVersionReconcileStepIsNoOp; merged the two sync-step tests into one. −343/+41. Kept TestInstallHelpAdvertisesForceCommunityTools (matches the file's existing help-test convention, e.g. TestInstallChannelHelpMentionsNightly).
- Pre-existing failures (verified via git stash on clean base): 16 TestPiCodeGraph* environmental (harness not scoping ~/.gentle-shell/agent/agents/) + 3 CLI integration tests. Not caused by this change.
- Diff size: 941+/158- across 16 files (~1100 lines) — over the 400-line budget; size:exception vs chained-split decision pending owner.
