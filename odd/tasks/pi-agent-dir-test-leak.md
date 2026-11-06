# Pi agent dir override leaks tests into the real user home

## Objective

Stop `go test` from reading, writing, or deleting files in the user's real Pi agent directory when `PI_CODING_AGENT_DIR` is exported (as Gentle Shell does), without changing production behavior.

## Problem

`internal/agents/pi/adapter.go` `AgentConfigPath(homeDir)` honors `PI_CODING_AGENT_DIR` regardless of `homeDir`. Tests pass `t.TempDir()` as home, but under a Gentle Shell session (`PI_CODING_AGENT_DIR=~/.gentle-shell/agent`) the resolved paths point at the real agent directory. Observed: `~/.gentle-shell/agent/APPEND_SYSTEM.md` became a dangling symlink into a `TestRetirePiSystemPromptBlocksRefusesDirectSymlink` temp dir. `TestRetirePiSystemPromptBlocksRemovesOwnedEmptyFile` can delete a real file.

Leaking tests (mapped): `internal/components/agentguidance/pi_cleanup_test.go` (5 tests), `internal/components/uninstall/service_test.go` (`TestBuildPlanSnapshotsPiManifestAndOwnedOverlay`, `TestExecutePlanRetiresStalePiSystemPromptBlocks`), `internal/components/engram/inject_test.go` (`piAdapter()` users).

## Why

Every other relocatable adapter (vscode, windsurf, kiro, trae) already honors its override only when `isRealUserHome(homeDir)`. `PI_CODING_AGENT_DIR` is the only relocation variable without that guard. Applying the same guard closes the whole class structurally instead of patching tests one by one.

## Scope

- Honor an absolute or cwd-relative `PI_CODING_AGENT_DIR` only when `homeDir` is the real user home; keep `~`/`~/...` forms (contained under `homeDir`) honored always.
- Adjust tests that intentionally exercise the override with a temp home (for example by setting `HOME` so the temp home is the real home).
- Regression test proving a temp-home call ignores an absolute override.

## Constraints

- Production behavior unchanged: production callers pass the real home.
- No Gentle Shell changes. Technical artifacts in English.
- Out of scope: the existing dangling symlink in the user's home (user decision), the review selector mismatch (tracked separately).

## Tasks

- [x] T1 Guard the override with `isRealUserHome`, with RED/GREEN tests, and adapt intentional override tests. Route: delegated (writer; 4+ files). RED observed on `TestAgentConfigPathIgnoresAbsoluteOverrideForNonRealHome`, then GREEN. Intentional override tests (`internal/agents/pi/adapter_test.go`, `internal/cli/run_integration_test.go`, `internal/components/communitytool/pi_codegraph_test.go`) now make their temp home the real home via `HOME`/`USERPROFILE`; the communitytool file was added to scope by the parent because the guard broke it.
- [x] T2 Verify no remaining test writes outside temp dirs with a sentinel `PI_CODING_AGENT_DIR` run of the affected packages. Route: inline (parent). Sentinel left with 0 entries.

## Acceptance criteria

- With `PI_CODING_AGENT_DIR=<sentinel>` exported, `go test ./...` leaves the sentinel directory untouched.
- Existing override tests still prove the override works for the real home.
- `go test ./...` passes.

## Checks

- `go test ./internal/agents/pi/... ./internal/components/...`, then `go test ./...`; `go vet ./...`.

## Delivery

- Strategy: `ask-on-risk`. Forecast: under 150 authored changed lines.
- RDD: on (global). Repository policy: approved issue required before PR.

## Progress

- Worktree: `../gentle-ai-worktrees/pi-agent-dir-test-leak`, branch `fix/pi-agent-dir-test-leak` from `origin/main` (d23f49010).

## Caller audit

- No `--home` flag exists. Every production caller resolves the home through `osUserHomeDir = os.UserHomeDir` (`internal/cli/run.go:72`), reassigned only in tests. The guard is a no-op in production.

## Verification evidence

- `S=$(mktemp -d); PI_CODING_AGENT_DIR=$S go test ./internal/agents/pi/... ./internal/components/uninstall/... ./internal/components/engram/... ./internal/components/communitytool/...`: all ok; sentinel has 0 entries (parent run).
- `go vet ./...`: clean.
- `go test ./...`: all packages pass except `TestInjectRoutingStaysContainedUnderHostileEnvironment` (`internal/components/agentguidance`), which fails identically on unmodified `origin/main` and under `env -i HOME PATH` on macOS; Linux CI on `main` passes. Pre-existing and unrelated (XDG_CONFIG_HOME/APPDATA containment). `Windows Full Suite` on `main` (d23f49010) also fails before this change.

## Next step

Deliver: approved issue, then pull request.
