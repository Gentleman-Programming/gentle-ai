# Tasks: feat(pi): resolve and invoke installed model-routing contract (#3530)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~280-320 A+D (2 new + 3 modified, goldens excluded) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Resolver + client with bounded probe and versioned JSON | PR 1 | `go test ./internal/agents/pi -run TestResolve -count=1` | `go test ./internal/agents/pi -count=1` with fake `lookPath`/`readFile`/`Runner` + FS snapshot | Revert one commit: 2 new + 3 modified in `internal/agents/pi`; no state |

## Phase 1: RED — Failing Tests First (strict TDD)

- [x] 1.1 Failing tests in `internal/agents/pi/model_routing_test.go` for PATH: hit returns probed PATH bin; miss falls through to package candidates
- [x] 1.2 Failing tests for precedence: project `settings.json` > user, `PI_CODING_AGENT_DIR` > `~/.pi/agent`, `AgentDir` > env (inject `modelRoutingReadFile`)
- [x] 1.3 Failing tests for `packageRootForSource`: `npm:`/`git:`/`local:` each maps to `docs/packages.md` layout
- [x] 1.4 Failing tests for `ResolvePackageBin` reuse: 64 KiB, duplicate-key reject, `absent-bin`/`unsafe-bin`/`missing-bin-target`/`non-regular`/`non-executable`, symlink containment
- [x] 1.5 Failing tests for bounded probe: timeout kills child → `timeout`, 64 KiB `LimitReader+1`, no `npm`/`pi install`/network, no-write snapshot
- [x] 1.6 Failing tests for client contract: `Capabilities`/`Inspect`/`Validate`/`Apply` each writes one `{contract:"gentle-pi.model-routing/v1"}` to stdin, reads bounded JSON, preserves unknowns
- [x] 1.7 Failing tests for error taxonomy: invalid JSON → `invalid-json`, wrong contract → `unsupported-contract`, non-zero exits → `probe-failed`, distinct `missing`/`malformed`/`timeout`
- [x] 1.8 Failing tests for cancellation + re-stat: `context` cancel kills child via `exec.CommandContext`; re-stat before spawn

## Phase 2: GREEN — Resolver & Client Implementation

- [x] 2.1 Create `internal/agents/pi/model_routing_resolver.go` with `Target` enum, `ResolveModelRoutingExecutable`, `resolveOnPATH`, `selectedPackageSource`, `packageRootForSource`, bounded `capabilities` probe
- [x] 2.2 Add injectable `modelRoutingLookPath`/`modelRoutingReadFile` vars; wire re-stat before `Runner` spawn
- [x] 2.3 Create `internal/agents/pi/model_routing_client.go` with `ModelRoutingClient{Bin,Timeout,Runner}`, `do()`, 4 ops, `wireRequest` const, exit map, bounded I/O
- [x] 2.4 Keep `internal/agents/pi/model_routing.go` as leaf verifier; delegate to `ResolvePackageBin`
- [x] 2.5 Modify `internal/agents/pi/adapter.go` to add read-only `gentlePiPackageSource` via `AgentConfigPath`/`CodeGraphPaths`; never call `mergePiSettingsFile`

## Phase 3: Verification & No-Regression

- [x] 3.1 Make RED green: `go test ./internal/agents/pi -run TestResolve -count=1` passes for PATH/precedence/kinds/manifest/probe/contract/exit/cancel
- [x] 3.2 Run `go test ./internal/agents/pi -count=1 && go vet ./internal/agents/pi`; assert no-network and FS unchanged
- [x] 3.3 Verify no regression: `go test ./internal/agents/pi ./internal/components/communitytool -count=1` — `Adapter`/`ProvisionEngramMCP` unchanged
- [x] 3.4 Confirm ≤400 A+D via `git diff --stat v2.5.0-rc.2..HEAD -- internal/agents/pi/`; ready to unblock #3522

## Dependencies

- `proposal`→`spec`→`design`→`tasks` done; `apply` needs `tasks`; `verify` needs `apply`; no `internal/tui/screens` (PR2 #3522)

## Verification

- Focused: `go test ./internal/agents/pi -run TestResolve -count=1`
- Harness: `go test ./internal/agents/pi -count=1` with fake Runner/FS snapshot + temp PATH bin
- Rollback: `git revert <commit>` — 2 new files + 3 modified, no persisted state
