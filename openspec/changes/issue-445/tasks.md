# Tasks: Profile-aware uninstall Engram safety (issue-445)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 220–350 (types + service + TUI model + screens + tests) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception (not needed if under budget) |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Fail-closed `none` scope end-to-end (service + TUI defaults + UI copy) | PR 1 | `go test ./internal/model/ ./internal/components/uninstall/ ./internal/tui/ ./internal/tui/screens/` | N/A — unit/TUI keymsg only; no live uninstall harness | Revert PR; runtime scope only, no migration |

## Phase 1: Foundation — scope constant

- [x] 1.1 RED: add failing compile/use of `EngramUninstallScopeNone` in `internal/components/uninstall/service_test.go` (or minimal type assertion test)
- [x] 1.2 GREEN: add `EngramUninstallScopeNone = "none"` in `internal/model/types.go` (req: Scope Values)

## Phase 2: Service — fail-closed planning (TDD)

- [x] 2.1 RED: `service_test.go` — scope `none` + Engram selected → zero Engram targets/ops (req: None Scope)
- [x] 2.2 RED: `service_test.go` — unknown/empty/invalid scope → not `global`; zero Engram ops or reject (req: Fail-Closed)
- [x] 2.3 RED: `service_test.go` — `project` → workspace `.engram/` only; `global` → existing integration cleanup (req: Project/Global opt-in)
- [x] 2.4 GREEN: `SetEngramUninstallScope` accept only none|project|global; unknown → none; plan ComponentEngram for `none` as 0 ops in `service.go`
- [x] 2.5 GREEN: prefer post-run/idle reset of service field to `none` (not global) in `PartialUninstallWithProfiles` defer path

## Phase 3: TUI model — defaults, lifecycle, nav (TDD)

- [x] 3.1 RED: `model_test.go` — `NewModel` / `withResetUninstallState` default `UninstallEngramScope` = `none` (req: Lifecycle)
- [x] 3.2 RED: partial components→profiles / `refreshUninstallProfiles` → `none`, never force Global (req: Profile-Aware Defaults)
- [x] 3.3 RED: full / FullRemove / CleanInstall + Engram selected → `global`; without Engram → `none` (req: Full Mode boundary)
- [x] 3.4 RED: profile discovery failure clears lists and does **not** set Global (req: Lifecycle)
- [x] 3.5 RED: space toggle cycles none/project/global via options index; `optionCount` matches profiles + `len(scopeOptions)` + chrome (req: No Cleanup visible/nav)
- [x] 3.6 RED: `startUninstall` passes model scope unchanged to `UninstallWithProfilesFn` (dispatch)
- [x] 3.7 GREEN: implement `defaultEngramUninstallScope` + entry matrix in `internal/tui/model.go` (`refreshUninstallProfiles`, mode entry, reset, full populate)
- [x] 3.8 GREEN: `toggleCurrentUninstallEngramScope` + `optionCount`/`shouldShowUninstallEngramScopeSelection` use `len(options)` (no hard-coded +2 / idx→project)

## Phase 4: Screens — options, confirm, result (TDD)

- [x] 4.1 RED: screen tests — when Engram scope UI shown, **No cleanup** first; Project row only if project data; Global always (req: No Cleanup)
- [x] 4.2 RED: confirm/result for scope `none` → “None” / no Global implication (req: Confirm/Result Truthful)
- [x] 4.3 GREEN: `uninstallEngramScopeOptions` + render/confirm/result copy in `internal/tui/screens/uninstall.go` (+ `uninstall_result_test.go` / new test file)

## Phase 5: CLI/app pass-through (only if needed)

- [x] 5.1 Verify `internal/cli/uninstall.go` / `internal/app/app.go` already pass scope type; wire only if compile/call sites reject `none`
- [x] 5.2 RED→GREEN: CLI/unit tests if any path still coerces non-project → global

## Phase 6: Package verification

- [x] 6.1 `go test ./internal/model/ ./internal/components/uninstall/ ./internal/tui/ ./internal/tui/screens/`
- [x] 6.2 `go vet` on touched packages; fix any drift vs design default matrix
