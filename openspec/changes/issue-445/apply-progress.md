# Apply Progress: issue-445 (uninstall Engram scope safety)

**Mode**: Strict TDD  
**Delivery**: single-PR-default; actual ~582 lines (mostly tests) → PR needs `size:exception`

## Completed Tasks

- [x] Phase 1: `EngramUninstallScopeNone` constant
- [x] Phase 2: Service fail-closed + zero ops for `none` + defer reset to `none`
- [x] Phase 3: TUI defaults/lifecycle/nav (`defaultEngramUninstallScope`, optionCount, toggle by options index)
- [x] Phase 4: Screens No cleanup / confirm / result truthful for `none`
- [x] Phase 5: CLI/app pass-through verified (no changes needed)
- [x] Phase 6: focused `go test` + `go vet` green

All 22 tasks in `tasks.md` are checked `[x]`.

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `internal/model/types.go` | Modified | Added `EngramUninstallScopeNone = "none"` |
| `internal/components/uninstall/service.go` | Modified | Fail-closed setter; zero Engram ops for `none`; defer reset to `none` |
| `internal/components/uninstall/service_test.go` | Modified | RED→GREEN service coverage for none/unknown/reset |
| `internal/tui/model.go` | Modified | Defaults matrix, refresh/reset, full-mode global, toggle/optionCount |
| `internal/tui/model_test.go` | Modified | Lifecycle/default/toggle/dispatch tests; updated full-mode nav expectations |
| `internal/tui/screens/uninstall.go` | Modified | No cleanup option first; show when Engram selected; truthful confirm/result |
| `internal/tui/screens/uninstall_result_test.go` | Modified | Options/confirm/result tests for `none` |
| `openspec/changes/issue-445/tasks.md` | Modified | All tasks marked `[x]` |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1–1.2 | `service_test.go` | Unit | ✅ packages green | ✅ Written | ✅ Passed | ➖ Single (constant) | ➖ None needed |
| 2.1–2.5 | `service_test.go` | Unit | ✅ pre-existing Engram tests | ✅ Written | ✅ Passed | ✅ none/unknown/project/global/reset | ✅ Clean switch |
| 3.1–3.8 | `model_test.go` | Unit (KeyMsg) | ✅ baseline then update | ✅ Written | ✅ Passed | ✅ matrix + discovery + toggle + dispatch | ✅ helper extracted |
| 4.1–4.3 | `uninstall_result_test.go` | Unit | ✅ existing screen tests | ✅ Written | ✅ Passed | ✅ options/confirm/result/hide | ➖ None needed |
| 5.1–5.2 | N/A | Verify | ✅ cli/app compile+test | ➖ No coercion path | ✅ Pass-through only | ➖ N/A | ➖ N/A |

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and result | `go test ./internal/model/ ./internal/components/uninstall/ ./internal/tui/ ./internal/tui/screens/ -count=1` → all ok |
| Full suite | `go test ./... -count=1` → exit 0 |
| Vet | `go vet ./...` → exit 0 |
| Runtime harness | N/A — unit/TUI KeyMsg only; no live uninstall harness |
| Rollback boundary | Revert the 7 implementation files above; runtime scope only, no migration |

## Deviations from Design

None — implementation matches design default matrix and fail-closed service contract.

## Pure functions / helpers

- `defaultEngramUninstallScope`
- exported `UninstallEngramScopeOptions`
