# Proposal: Claude/OpenCode Orchestrator Parity

## Intent

Close the confirmed parity gap where Claude Code SDD orchestration lacks OpenCode’s explicit Chain Strategy contract and uses async/delegation wording that can imply OpenCode plugin-backed persistence.

## Scope

### In Scope
- Update the Claude Code SDD orchestrator asset to include Chain Strategy semantics aligned with OpenCode.
- Forward `chain_strategy` alongside `delivery_strategy` into `sdd-tasks` and `sdd-apply` guidance.
- Adjust Claude-native async/delegation wording so it stays accurate for Claude Code.
- Refresh Claude golden outputs and focused tests affected by asset text changes.

### Out of Scope
- Broad rollout to other agents/subagents.
- OpenCode plugin changes or OpenCode asset behavior changes.
- Implementation beyond this first controlled Claude asset fix.

## Capabilities

### New Capabilities
- `sdd-orchestrator-assets`: Defines expected behavior for embedded SDD orchestrator guidance assets across supported agent targets.

### Modified Capabilities
- None.

## Approach

Apply the exploration recommendation: patch only `internal/assets/claude/sdd-orchestrator.md`, using OpenCode’s Chain Strategy section as the behavioral reference while avoiding OpenCode-specific plugin assumptions. Then update Claude-related golden fixtures and any static assertions that depend on the generated Claude text. Keep this as a single small PR/work unit.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/assets/claude/sdd-orchestrator.md` | Modified | Add chain strategy guidance and Claude-native async wording. |
| `internal/assets/opencode/sdd-orchestrator.md` | Reference | Baseline for Chain Strategy semantics; no change intended. |
| `internal/components/sdd/inject.go` | Reference | Claude injection path reads the embedded Claude asset. |
| `internal/components/golden_test.go` | Modified | Validate refreshed Claude injected output. |
| `internal/assets/assets_test.go` | Possible Modified | Adjust static text expectations if affected. |
| `testdata/golden/sdd-claude-claudemd.golden` | Modified | Refresh standalone Claude SDD output. |
| `testdata/golden/combined-claude-claudemd.golden` | Modified | Refresh combined Claude output. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| OpenCode-specific async assumptions leak into Claude wording | Medium | Write Claude-native wording; use OpenCode only as Chain Strategy reference. |
| Golden fixtures drift from asset changes | Medium | Refresh both Claude goldens and run focused tests. |
| Scope creep into other agents | Low | Limit file changes to Claude asset and direct test fixtures. |

## Rollback Plan

Revert the Claude orchestrator asset change plus refreshed Claude golden/test expectation updates in one work-unit revert.

## Dependencies

- Existing OpenCode orchestrator Chain Strategy wording as reference.
- Existing golden-test pipeline for Claude injected output.

## Success Criteria

- [ ] Claude SDD guidance documents `stacked-to-main` and `feature-branch-chain` selection.
- [ ] Claude SDD guidance passes `chain_strategy` to `sdd-tasks` and `sdd-apply`.
- [ ] Claude async wording avoids OpenCode plugin-backed persistence claims.
- [ ] Focused Claude golden/static tests pass.
