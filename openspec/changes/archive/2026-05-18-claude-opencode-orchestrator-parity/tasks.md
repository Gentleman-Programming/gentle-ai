# Tasks: Claude/OpenCode Orchestrator Parity

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 130-240 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Claude orchestrator parity + direct validation refresh | PR 1 | Single deliverable; asset, static checks, goldens, focused go tests |

## Phase 1: Foundation / Scope Lock

- [x] 1.1 Review `openspec/changes/claude-opencode-orchestrator-parity/{proposal.md,design.md}` and use the requirements as an implementation checklist before editing `internal/assets/claude/sdd-orchestrator.md`.
- [x] 1.2 Inspect `internal/assets/opencode/sdd-orchestrator.md` as semantic reference only; note reusable Chain Strategy phrasing without copying OpenCode plugin-persistence claims into Claude asset.

## Phase 2: Core Asset Implementation

- [x] 2.1 Update `internal/assets/claude/sdd-orchestrator.md` to add `### Chain Strategy` with canonical values `stacked-to-main` and `feature-branch-chain`.
- [x] 2.2 Update Claude delivery-planning guidance in the same file so `chain_strategy` is passed together with `delivery_strategy` to `sdd-tasks`.
- [x] 2.3 Update Claude apply-launch guidance in the same file so `chain_strategy` is passed together with `delivery_strategy` to `sdd-apply`.
- [x] 2.4 Replace/adjust Claude async-delegation wording in `internal/assets/claude/sdd-orchestrator.md` to be Claude-native and explicitly avoid OpenCode plugin-backed persistence guarantees.

## Phase 3: Static + Golden Validation

- [x] 3.1 Add/adjust assertions in `internal/assets/assets_test.go` (new or existing `TestClaudeSDDOrchestratorChainStrategy`) to enforce canonical strategy names and propagation wording.
- [x] 3.2 Add/adjust negative assertion in `internal/assets/assets_test.go` ensuring Claude asset text does not require OpenCode plugin/background persistence semantics.
- [x] 3.3 Refresh `testdata/golden/sdd-claude-claudemd.golden` and `testdata/golden/combined-claude-claudemd.golden` via `go test ./internal/components -run 'TestGoldenSDD_Claude|TestGoldenCombined_Claude' -update` and review diff.
- [x] 3.4 Re-run same focused golden tests without `-update` to verify stable pass.

## Phase 4: Focused Verification and Closeout

- [x] 4.1 Run `go test ./internal/assets -run 'TestClaudeEmbeddedAssetLayout|TestClaudeSDDOrchestratorChainStrategy'` and ensure all new/updated assertions pass.
- [x] 4.2 Run `go test ./internal/assets ./internal/components -run 'TestClaude|TestGoldenSDD_Claude|TestGoldenCombined_Claude|TestSDDOrchestratorAssetsScopedToDedicatedAgent'` and confirm no collateral regressions in direct scope.
- [x] 4.3 Verify `git diff --stat` remains within forecasted single-PR scope and that changed files stay limited to Claude asset + direct static/golden validation targets.
