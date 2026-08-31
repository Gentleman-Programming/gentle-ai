# Apply Progress: issue-771-opencode-model-selector

## Work Unit

- Unit: unit-1-config-boundary
- Strategy: auto-chain, stacked-to-main
- Scope: effective JSONC config snapshot, provider/model extraction, LM Studio URL direct/fallback handling, assignment presence types, runtime-first catalog merge.

## Completed Tasks

- [x] 1.1 Config tests cover JSONC comments/trailing commas, precedence, missing default write target, and shared path selection.
- [x] 1.2 Config tests cover configured providers/models and LM Studio direct `url` precedence with `options.baseURL` fallback.
- [x] 1.3 Catalog merge tests cover runtime-authoritative duplicates and configured-only additions.
- [x] 2.1 `internal/opencode/config.go` adds resolver/parser snapshot, provider/model extraction, URL metadata, and assignment presence.
- [x] 2.2 `internal/opencode/models.go` and `internal/opencode/catalog.go` expose provider URL metadata and runtime-first merge behavior.
- [x] 3.1 Configure Models TUI test covers a JSONC configured custom provider/model plus a runtime provider in the same picker catalog.
- [x] 4.1 `internal/tui/model.go` and `internal/tui/screens/model_picker.go` merge configured providers into runtime catalog discovery without changing picker navigation handlers.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `internal/opencode/config_test.go` | Unit | N/A (new file) | ✅ `ResolveEffectiveConfig` tests failed to compile before implementation | ✅ Focused tests passed | ✅ JSONC, precedence, and missing config cases | ✅ gofmt |
| 1.2 | `internal/opencode/config_test.go` | Unit | N/A (new file) | ✅ Provider/model and URL tests failed to compile before implementation | ✅ Focused tests passed | ✅ direct URL and baseURL fallback cases | ✅ gofmt |
| 1.3 | `internal/opencode/catalog_test.go` | Unit | ✅ existing focused opencode tests passed before modification | ✅ `MergeConfiguredCatalog` tests failed to compile before implementation | ✅ Focused tests passed | ✅ duplicate runtime and configured-only cases | ✅ gofmt |
| 2.1 | `internal/opencode/config.go` | Unit | N/A (new file) | ✅ Covered by 1.1/1.2 RED tests | ✅ Focused tests passed | ✅ parsed providers and assignment presence cases | ✅ gofmt |
| 2.2 | `internal/opencode/catalog.go`, `internal/opencode/models.go` | Unit | ✅ existing focused opencode tests passed before modification | ✅ Covered by 1.3 RED tests | ✅ Focused tests passed | ✅ URL fill and runtime precedence cases | ✅ gofmt |

## Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/opencode -run 'Config|Catalog|URL' -count=1` → exit 0, `ok github.com/gentleman-programming/gentle-ai/v2/internal/opencode 0.603s` |
| Runtime harness command/scenario and exact result | N/A for this slice: no TUI, install/inject, or sync runtime boundary is wired in unit 1; LM Studio direct/fallback URL behavior is covered at the config-boundary unit layer. |
| Rollback boundary | Revert `internal/opencode/config.go`, `internal/opencode/config_test.go`, and the provider URL/catalog merge changes in `internal/opencode/catalog.go`, `internal/opencode/catalog_test.go`, and `internal/opencode/models.go`. |

## Additional Verification

- Safety net: `go test ./internal/opencode -run 'Test(DiscoverCatalog|ModelEffortLevels|ReviewPhases)' -count=1` → exit 0, `ok .../internal/opencode 0.153s`.
- Package check: `go test ./internal/opencode -count=1` → exit 0, `ok .../internal/opencode 1.156s`.
- Excluded file guard: `git diff --name-only -- internal/assets/opencode/plugins/review-result-artifacts.ts internal/assets/review_plugin_recovery_test.go` → no output.

## Changed-Line Estimate

- Estimated authored delta: about 456 additions and 5 checkbox-line edits, including SDD progress artifacts.
- Code/test delta before SDD artifact updates: 419 additions across `internal/opencode`.

## Work Unit 2: Selector Wiring

- Unit: unit-2-selector-wiring
- Strategy: auto-chain, stacked-to-main
- Scope: Configure Models only. Install/inject, sync preservation, unrelated issue scopes, and review plugin/recovery files remain outside this slice.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 3.1 | `internal/tui/model_test.go` | TUI integration via `Model.Update()` | ✅ `go test ./internal/tui/... -run 'ModelPicker|Configure' -count=1` → exit 0 before edits | ✅ New Configure Models JSONC custom-provider test failed with only `runtime-ai` visible | ✅ `go test ./internal/tui -run TestConfigureOpenCodeModelsShowsJSONCCustomProviderWithRuntimeProviders -count=1` → exit 0 | ✅ Same test asserts both custom configured provider and independent runtime provider each expose one selectable tool-call model | ✅ gofmt |
| 4.1 | `internal/tui/model.go`, `internal/tui/screens/model_picker.go` | TUI integration via runtime discovery message | ✅ Existing focused TUI/model-picker tests passed before production edits | ✅ Covered by 3.1 RED test before wiring | ✅ `go test ./internal/tui/... -run 'ModelPicker|Configure' -count=1` → exit 0 | ✅ `go test ./internal/tui/... -count=1` → exit 0 preserves broader picker/navigation semantics | ✅ gofmt |

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/tui/... -run 'ModelPicker|Configure' -count=1` → exit 0, `ok .../internal/tui 0.238s`, `ok .../internal/tui/screens 0.180s` |
| Runtime harness command/scenario and exact result | Configure Models JSONC custom provider scenario via direct Bubbletea `Model.Update()` path: enter `ScreenModelConfig` cursor 1, execute runtime catalog discovery command, then assert `runtime-ai` and `custom-cloud` providers remain selectable with one model each → exit 0 in `TestConfigureOpenCodeModelsShowsJSONCCustomProviderWithRuntimeProviders`. |
| Rollback boundary | Revert unit-2 changes in `internal/tui/model.go`, `internal/tui/model_test.go`, and `internal/tui/screens/model_picker.go`; leave unit-1 `internal/opencode/*` config boundary intact. |

### Additional Verification

- Package check: `go test ./internal/tui/... -count=1` → exit 0, `ok .../internal/tui 2.571s`, `ok .../internal/tui/screens 0.385s`.
- Combined focused check: `go test ./internal/opencode ./internal/tui/... -run 'Config|Catalog|URL|ModelPicker|Configure' -count=1` → exit 0, `ok .../internal/opencode 0.777s`, `ok .../internal/tui 0.334s`, `ok .../internal/tui/screens 0.363s`.
- Excluded file guard: `git diff --name-only -- internal/assets/opencode/plugins/review-result-artifacts.ts internal/assets/review_plugin_recovery_test.go` → no output.

### Changed-Line Estimate

- Estimated authored unit-2 code/test delta before SDD artifact updates: 108 changed lines (`99` additions, `9` deletions) across `internal/tui/model.go`, `internal/tui/model_test.go`, and `internal/tui/screens/model_picker.go`; SDD artifact edits add approximately 35 authored lines.

## Work Unit 3: Install and Sync Preservation

- Unit: unit-3-install-sync
- Relaunch context: previous unit-3 apply launch was interrupted by provider usage limit; this relaunch inspected and reused valid partial RED tests/production edits, then repaired incomplete sync helper placement and effective-path handling.
- Strategy: auto-chain, stacked-to-main
- Scope: install/inject writes an existing `opencode.jsonc`; sync preserves current and explicitly cleared OpenCode assignments against stale Gentle AI state; `internal/cli/run.go` is included only to align sync backup/verification path selection with the effective OpenCode settings file; excluded review plugin/recovery files remain untouched.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 3.2 | `internal/components/sdd/inject_test.go` | Integration via `Inject()` with temp OpenCode config | ✅ Partial RED existed on relaunch; first focused run failed because `opencode.json` was created beside existing `opencode.jsonc` | ✅ `TestInjectOpenCodeWritesExistingJSONCConfig` existed and failed against partial implementation | ✅ `go test ./internal/components/sdd -run 'TestInjectOpenCodeWritesExistingJSONCConfig' -count=1` → exit 0 | ✅ Asserted JSONC write target, no `opencode.json`, retained user field, and assigned SDD agent model | ✅ Extracted local effective settings-path helper and gofmt |
| 3.3 | `internal/cli/sync_test.go` | Sync component integration | ✅ Partial RED existed on relaunch; first focused run failed to compile because helper was inserted inside `RunSync` | ✅ Current-assignment and cleared-assignment tests existed and failed before repair | ✅ `go test ./internal/cli -run 'TestRunSyncPreserves(Current\\|Cleared)OpenCodeAssignmentOverStaleState' -count=1` → exit 0 | ✅ Covered present assignment retention and cleared assignment retention while still applying root-model fallback to new SDD agents | ✅ Moved restore helper outside `RunSync`, reused local effective-path selection, and gofmt |
| 3.4 | Git diff guard | Diff/runtime guard | ✅ Guarded files were unchanged at relaunch | ✅ Guard command included in verification evidence | ✅ `git diff --name-only -- internal/assets/opencode/plugins/review-result-artifacts.ts internal/assets/review_plugin_recovery_test.go` → no output | ➖ Single negative-control guard: protected paths must not appear in diff | ✅ No source refactor needed |
| 4.2 | `internal/components/sdd/inject.go` | Install/inject integration | ✅ Existing OpenCode inject regression subset passed after repair | ✅ Covered by 3.2 RED test | ✅ `go test ./internal/components/sdd -run 'TestInjectOpenCode(AndKilocodeLanguageContractOutputs\\|WritesCommandFiles\\|IsIdempotent\\|MultiModeIdempotent)' -count=1` → exit 0 | ✅ Existing JSONC target and fresh `opencode.json` default paths both covered | ✅ Kept lookup local to the target config directory so tests do not capture the developer's real OpenCode config; assignment presence comes from `internal/opencode/config.go`, so no legacy assignment-reader file change was needed |
| 4.3 | `internal/cli/sync.go`, `internal/cli/run.go` | Sync component integration | ✅ Existing focused CLI tests passed after repair | ✅ Covered by 3.3 RED tests | ✅ `go test ./internal/cli -run 'TestRunSyncPreserves(Current\\|Cleared)OpenCodeAssignmentOverStaleState' -count=1` → exit 0 | ✅ Present and cleared assignment branches both exercised | ✅ Shared sync backup-path logic with the effective local OpenCode settings target; `run.go` keeps the candidate path list consistent with `sync.go` for existing `opencode.jsonc` |
| 4.4 | package/test commands | Verification | ✅ Prior unit-1/unit-2 tests retained | ✅ N/A: verification task proves completed behaviors | ✅ Focused and affected commands recorded below; full `go test ./...` and `go vet ./...` are not claimed in apply evidence | ✅ Install/inject, sync-present, sync-cleared, Configure Models, LM Studio direct/fallback URL, and excluded-file guard are covered across unit evidence | ✅ `gofmt`, `git diff --check`, and affected `go vet` passed |

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command and exact result | `go test ./internal/components/sdd ./internal/cli -run 'TestInjectOpenCodeWritesExistingJSONCConfig\|TestRunSyncPreserves(Current\|Cleared)OpenCodeAssignmentOverStaleState' -count=1` → exit 0, `ok .../internal/components/sdd 1.775s`, `ok .../internal/cli 2.883s`. |
| Runtime harness command/scenario and exact result | Install/inject JSONC scenario via `Inject()` updates existing `opencode.jsonc`, preserves user field, and does not create `opencode.json`; sync-present/sync-cleared scenarios via `componentSyncStep.Run()` preserve current `sdd-apply` assignment and preserve an explicitly cleared assignment against stale persisted state. |
| Rollback boundary | Revert unit-3 changes in `internal/components/sdd/inject.go`, `internal/components/sdd/inject_test.go`, `internal/cli/sync.go`, `internal/cli/sync_test.go`, and the SDD task/progress artifact edits; leave unit-1 `internal/opencode/*` and unit-2 `internal/tui/*` behavior intact. |

### Additional Verification

- Focused behavior: `go test ./internal/components/sdd ./internal/cli -run 'TestInjectOpenCodeWritesExistingJSONCConfig|TestRunSyncPreserves(Current|Cleared)OpenCodeAssignmentOverStaleState' -count=1` → exit 0.
- OpenCode inject regression subset: `go test ./internal/components/sdd -run 'TestInjectOpenCode(AndKilocodeLanguageContractOutputs|WritesCommandFiles|IsIdempotent|MultiModeIdempotent)' -count=1` → exit 0.
- SDD package: `go test ./internal/components/sdd -count=1` → exit 0, `ok .../internal/components/sdd 210.264s`.
- OpenCode/TUI affected packages: `go test ./internal/opencode ./internal/tui/... -count=1` → exit 0, `ok .../internal/opencode 1.404s`, `ok .../internal/tui 2.718s`, `ok .../internal/tui/screens 0.183s`, `? .../internal/tui/styles [no test files]`.
- CLI new focused tests: `go test ./internal/cli -run 'TestRunSyncPreserves(Current|Cleared)OpenCodeAssignmentOverStaleState' -count=1` → exit 0, `ok .../internal/cli 2.092s`.
- Affected vet: `go vet ./internal/opencode ./internal/tui/... ./internal/components/sdd ./internal/cli` → exit 0, no output.
- Whitespace: `git diff --check` → exit 0, no output.
- Excluded file guard: `git diff --name-only -- internal/assets/opencode/plugins/review-result-artifacts.ts internal/assets/review_plugin_recovery_test.go` → no output.
- Broader pattern suite note: `go test ./internal/components/sdd ./internal/cli -run 'OpenCode|Assignment|Sync|Inject' -count=1` passed `internal/components/sdd` but failed in unrelated `internal/cli` tests on this macOS `/var` temp environment with pre-existing `acquire install state lock: review store lock could not be acquired: not a directory` failures.
- Full-suite note: apply evidence does not claim repository-wide test or vet success; those broader commands remain independent verify-phase responsibilities or require an environment without the local `/var` temp lock failure.

### Changed-Line Estimate

- Relaunch unit-3 authored code/test delta is approximately 281 changed lines across `internal/components/sdd/inject.go`, `internal/components/sdd/inject_test.go`, `internal/cli/run.go`, `internal/cli/sync.go`, and `internal/cli/sync_test.go`, before SDD artifact updates.
- Whole worktree implementation delta across units 1-3 is currently `476 insertions(+), 24 deletions(-)` in 11 code/test files before OpenSpec artifact diffs.

## Remaining Tasks

- [x] 3.2 Add failing install/inject test proving existing `opencode.jsonc` is updated instead of creating the wrong `opencode.json`.
- [x] 3.3 Add failing sync tests for current assignment preservation and explicitly cleared assignment preservation against stale state.
- [x] 3.4 Add excluded-file diff guard verifying protected files remain untouched.
- [x] 4.2 Update install/inject effective JSONC target handling using shared OpenCode assignment presence; no legacy assignment-reader file change was needed.
- [x] 4.3 Update sync overlay behavior for absent-only persisted assignments.
- [x] 4.4 Run focused and affected verification commands; repository-wide test/vet and the broader `internal/cli` pattern sweep are not claimed as passed in apply evidence.
