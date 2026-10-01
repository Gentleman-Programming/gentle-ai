# Windows Full Suite green

## Objective
Make the `Windows Full Suite` workflow (`.github/workflows/windows-full-suite.yml`) pass on `main`.

## Problem
Every run since at least `5bf23ad9` (2026-09-30) fails deterministically on the same tests
(evidence: run 36884533649 on `0d2a0719`; logs copied to `/tmp/wfs-logs/<shard>.log`).
Shards red: `rest`, `cli-n-o`, `cli-r-other`, `cli-p-q-s-z`.

## Why
A permanently red lane hides real Windows regressions.

## Scope / authorized
Fix the failing tests or the product code they reveal, so the suite is green on Windows,
without weakening the behavior the tests protect. Single PR (user decision).

## Constraints
- No local Windows runner: RED evidence is the CI log; GREEN on Windows needs a CI run.
- Locally: Linux tests must stay green and `GOOS=windows go vet` must compile the touched packages.
- Fix product code when the test reveals a real Windows bug; adjust the assertion only when the
  expectation is genuinely Unix-specific.

## Tasks
- [x] T1 Portable assertions / Windows behavior (rest + cli-r-other shards)
  - `TestInjectRoutingGenericModelVariantUpgrade` (internal/components/agentguidance, orchestrator_test.go:178 "upgrade changed file mode")
  - `TestInjectAntigravityRejectsUnclassifiablePluginAssetBeforeWrites` (internal/components/engram, inject_test.go:437)
  - `TestProvisionEngramMCPRefusesMalformedMCPConfigWithoutClobbering`, `TestProvisionEngramMCPRefusesNonObjectMCPServersWhenMigrating` (internal/agents/pi, mcp_config_test.go:249)
  - `TestRunSyncWorkspaceScopeUpdatesOpenCodeManagedComponentsWithoutGlobalMutation` (internal/cli, sync_scope_test.go:393)
- [ ] T2 PowerShell continuation quoting (cli-n-o shard)
  - `TestOpenCodeV2SDKWindowsPowerShellContinuation` (internal/cli, run_test.go:1295)
- [ ] T3 V2 SDK npm timeouts and 30m shard timeout (cli-p-q-s-z shard)
  - `TestV2SDKProvision*`, `TestV2SDKFreshMissingConfigCreatedOnlyAfterConsent` ("npm timed out after 2m0s")
  - `TestStatusRecoverTransitionExecutesAccountingOnlyRecoveryWithoutSelectors` (incompatible-focus/policy)
  - `panic: test timed out after 30m0s`
- [ ] T4 Open single PR, run Windows Full Suite on the branch, confirm green

## Routing
- T1–T3: delegated direct (gentle-ai-worker) — reading prepares writes across several packages.
- T4: inline (git/gh state).

## Delivery
Strategy: `single-pr` (user). Forecast ~150–300 authored lines.

## Checks
- Focused `go test` of touched packages on Linux; `GOOS=windows go vet` of touched packages.
- Final: Windows Full Suite CI run on the PR branch.

## Progress
- T1 done (delegated gentle-ai-worker; risk tier medium, test-only). Root causes: %q-quoted paths in errors, Windows Perm() 0o666, native separators in verify IDs. Checks: Linux go test of 3 pkgs + TestRunSync ok, GOOS=windows go vet ok, gofmt clean; Windows GREEN pending CI (T4).
- Branch `fix/windows-full-suite-green` from `origin/main` `0d2a0719`.
