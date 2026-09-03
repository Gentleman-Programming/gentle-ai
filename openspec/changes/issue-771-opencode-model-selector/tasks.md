# Tasks: OpenCode Model Selector Effective Config

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 450-650 authored lines |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 config boundary → PR 2 selector wiring → PR 3 install/sync evidence |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Effective JSONC config snapshot and catalog merge | PR 1 | `go test ./internal/opencode -run 'Config|Catalog|URL'` | LM Studio direct/fallback URL scenarios | `internal/opencode/*` |
| 2 | Configure Models shows configured plus runtime providers | PR 2 | `go test ./internal/tui/... -run 'ModelPicker|Configure'` | Configure Models JSONC custom provider | `internal/tui/model.go`, `internal/tui/screens/model_picker.go` |
| 3 | Install/inject and sync preserve present/cleared assignments | PR 3 | `go test ./internal/components/sdd ./internal/cli -run 'OpenCode|Assignment|Sync|Inject'` | install/inject JSONC, sync-present, sync-cleared, excluded-file diff guard | `internal/components/sdd/*`, `internal/cli/sync.go` |

Auto-chain note: next autonomous slices are the three units above; apply MUST select `stacked-to-main` or `feature-branch-chain` before editing.

## Phase 1: RED Tests for Config Boundary

- [x] 1.1 Add failing tests in `internal/opencode/config_test.go` for `opencode.jsonc` comments/trailing commas, precedence, missing-config write target, and shared path selection.
- [x] 1.2 Add failing tests in `internal/opencode/config_test.go` for configured providers/models and LM Studio direct `url` over `options.baseURL`, plus fallback URL.
- [x] 1.3 Add failing catalog merge tests in `internal/opencode/catalog_test.go`: runtime providers remain authoritative; configured-only providers/models are added.

## Phase 2: Config Boundary Implementation

- [x] 2.1 Create `internal/opencode/config.go` with effective config resolver, JSONC parser, `ConfigSnapshot`, provider/model extraction, URL metadata, and `AssignmentPresence`.
- [x] 2.2 Modify `internal/opencode/models.go` and `internal/opencode/catalog.go` only enough to expose URL metadata and runtime-first merge behavior.

## Phase 3: RED Tests for Operator Flows

- [x] 3.1 Add failing Configure Models test proving a JSONC custom provider/model appears while runtime providers remain selectable.
- [x] 3.2 Add failing install/inject test proving existing `opencode.jsonc` is updated instead of creating the wrong `opencode.json`.
- [x] 3.3 Add failing sync tests for current assignment preservation and explicitly cleared assignment preservation against stale state.
- [x] 3.4 Add excluded-file diff guard verifying `internal/assets/opencode/plugins/review-result-artifacts.ts` and `internal/assets/review_plugin_recovery_test.go` remain untouched.

## Phase 4: Flow Implementation and Verification

- [x] 4.1 Wire `internal/tui/model.go` and `internal/tui/screens/model_picker.go` to use merged runtime+configured catalog without changing picker navigation.
- [x] 4.2 Update `internal/components/sdd/inject.go` to write the effective JSONC target using assignment presence from `internal/opencode/config.go`.
- [x] 4.3 Update `internal/cli/sync.go` so stale Gentle AI state overlays only absent assignments, never present or cleared OpenCode intent.
- [x] 4.4 Run focused and affected tests/vet checks; record RDD evidence for required operator flows without claiming full-suite verification.
