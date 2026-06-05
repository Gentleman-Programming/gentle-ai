# Apply Progress: Claude/OpenCode Orchestrator Parity

## Status

All tasks complete. Ready for `sdd-verify`.

## Completed Tasks

- [x] 1.1 Reviewed proposal/design as implementation checklist.
- [x] 1.2 Used OpenCode orchestrator as semantic reference only.
- [x] 2.1 Added Claude `### Chain Strategy` guidance with `stacked-to-main` and `feature-branch-chain`.
- [x] 2.2 Added `chain_strategy` propagation with `delivery_strategy` to `sdd-tasks`.
- [x] 2.3 Added `chain_strategy` propagation with `delivery_strategy` to `sdd-apply`.
- [x] 2.4 Reworded Claude delegation semantics around Claude Code's native Agent/Task mechanism and no OpenCode background-agent persistence.
- [x] 3.1 Added static assertions for canonical strategy names and propagation wording.
- [x] 3.2 Added negative assertions against OpenCode plugin/background persistence semantics.
- [x] 3.3 Refreshed direct Claude golden files through the repo `-update` path.
- [x] 3.4 Reran focused Claude golden tests without `-update`.
- [x] 4.1 Ran focused Claude asset validation tests.
- [x] 4.2 Ran focused affected asset/component tests.
- [x] 4.3 Verified diff remains within low-risk single-PR scope.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | N/A | Process | ✅ `go test ./internal/assets -run 'TestClaudeEmbeddedAssetLayout\|TestSDDOrchestratorAssetsScopedToDedicatedAgent'` | ➖ No production behavior; checklist review before editing | ✅ Artifacts reviewed | ➖ Structural/process task | ➖ None needed |
| 1.2 | N/A | Process | ✅ Same safety net | ➖ No production behavior; reference review only | ✅ OpenCode asset inspected without modifying it | ➖ Structural/process task | ➖ None needed |
| 2.1-2.4, 3.1-3.2 | `internal/assets/assets_test.go` | Unit/static asset | ✅ `go test ./internal/assets -run 'TestClaudeEmbeddedAssetLayout\|TestSDDOrchestratorAssetsScopedToDedicatedAgent'` | ✅ `TestClaudeSDDOrchestratorChainStrategy` failed before asset change on missing `### Chain Strategy` | ✅ `go test ./internal/assets -run 'TestClaudeSDDOrchestratorChainStrategy'` | ✅ Required positive and negative assertions cover labels, propagation, and Claude-native delegation wording | ✅ Wording kept scoped to Claude asset; tests still pass |
| 3.3-3.4 | `internal/components/golden_test.go` + Claude golden fixtures | Golden | ✅ Existing golden tests exercised the injection path | ✅ `go test ./internal/components -run 'TestGoldenSDD_Claude\|TestGoldenCombined_Claude'` failed before update due expected fixture drift | ✅ `go test ./internal/components -run 'TestGoldenSDD_Claude\|TestGoldenCombined_Claude' -update`, then pass without `-update` | ✅ Standalone and combined Claude injection paths both updated | ✅ No harness changes needed |
| 4.1-4.3 | `internal/assets/assets_test.go`, `internal/components/golden_test.go` | Focused package | ✅ Prior focused tests green after implementation | ✅ Covered by static/golden RED steps above | ✅ Focused asset and component commands passed | ✅ Both direct and injection paths verified | ✅ Diff stat within 70 tracked changed lines |

## Test Summary

- **Total tests written**: 1 static asset test (`TestClaudeSDDOrchestratorChainStrategy`).
- **Total tests passing**: Focused asset and component suites passed.
- **Layers used**: Unit/static asset and golden injection tests.
- **Approval tests**: Existing Claude golden tests used to approve rendered-output changes.
- **Pure functions created**: 0 — this is static asset/golden text work.

## Commands Run

1. `go test ./internal/assets -run 'TestClaudeEmbeddedAssetLayout|TestSDDOrchestratorAssetsScopedToDedicatedAgent'`
2. `go test ./internal/assets -run 'TestClaudeSDDOrchestratorChainStrategy'` — expected RED before asset update.
3. `go test ./internal/assets -run 'TestClaudeSDDOrchestratorChainStrategy'`
4. `go test ./internal/components -run 'TestGoldenSDD_Claude|TestGoldenCombined_Claude'` — expected RED before golden update.
5. `go test ./internal/components -run 'TestGoldenSDD_Claude|TestGoldenCombined_Claude' -update`
6. `go test ./internal/components -run 'TestGoldenSDD_Claude|TestGoldenCombined_Claude'`
7. `go test ./internal/assets -run 'TestClaudeEmbeddedAssetLayout|TestClaudeSDDOrchestratorChainStrategy'`
8. `go test ./internal/assets ./internal/components -run 'TestClaude|TestGoldenSDD_Claude|TestGoldenCombined_Claude|TestSDDOrchestratorAssetsScopedToDedicatedAgent'`
9. `git diff --stat`

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `internal/assets/claude/sdd-orchestrator.md` | Modified | Added Chain Strategy guidance, propagated `chain_strategy`, and clarified Claude-native delegation semantics. |
| `internal/assets/assets_test.go` | Modified | Added static validation for required Claude strategy/propagation/delegation wording. |
| `testdata/golden/sdd-claude-claudemd.golden` | Modified | Refreshed direct Claude SDD golden output. |
| `testdata/golden/combined-claude-claudemd.golden` | Modified | Refreshed combined Claude golden output. |
| `openspec/changes/claude-opencode-orchestrator-parity/tasks.md` | Modified | Marked all apply tasks complete. |
| `openspec/changes/claude-opencode-orchestrator-parity/apply-progress.md` | Created | Saved cumulative apply progress and TDD evidence. |

## Deviations from Design

None — implementation matches design. OpenCode assets and runtime injection code were not modified.

## Issues Found

None.

## Workload / PR Boundary

- Mode: single PR
- Current work unit: Claude orchestrator parity + direct validation refresh
- Boundary: Claude asset wording, direct static assertions, direct Claude golden fixtures, OpenSpec task/progress artifacts
- Estimated review budget impact: Tracked production/test/golden diff is 70 insertions and 9 deletions, below the 130-240 line forecast and well under the 400-line budget.
