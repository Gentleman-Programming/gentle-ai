# Proposal: All-Agent SDD Orchestrator Parity

## Intent

Roll out controlled SDD orchestrator parity from the OpenCode/Claude reference assets to the remaining non-Claude orchestrator assets. The change removes chain-strategy drift and stale platform-inaccurate delegation wording without changing runtime asset generation.

## Scope

### In Scope
- Update only non-Claude SDD orchestrator guidance assets for chain strategy guidance and propagation.
- Preserve platform-native execution wording for each host, including solo-inline hosts.
- Update direct static tests and direct golden fixtures affected by those asset changes.

### Out of Scope
- Runtime/template refactor, generator centralization, or asset rendering architecture changes.
- Claude and OpenCode orchestrator asset behavior changes beyond reference comparison.
- Broader docs, unrelated agent assets, non-orchestrator prompts, or unrelated golden churn.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `sdd-orchestrator-assets`: Extend existing chain strategy parity, propagation, platform wording, and validation requirements from Claude/OpenCode coverage to remaining non-Claude orchestrator assets.

## Approach

Use targeted asset parity. Treat `internal/assets/opencode/sdd-orchestrator.md` and `internal/assets/claude/sdd-orchestrator.md` as semantic references, then patch only remaining non-Claude orchestrator assets. Keep each platform’s orchestration model intact instead of normalizing all wording to OpenCode delegation terms.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/assets/{codex,gemini,qwen,generic,kimi,kiro,windsurf,antigravity}/sdd-orchestrator.md` | Modified | Add/align chain strategy guidance, forwarding, and platform-accurate wording. |
| `internal/assets/assets_test.go` | Modified | Broaden static parity assertions to non-Claude orchestrator assets. |
| `internal/components/sdd/inject_test.go` | Modified | Extend direct platform wording assertions where needed. |
| `internal/components/golden_test.go` | Modified | Keep generated orchestrator fixture comparisons current. |
| `testdata/golden/sdd-*.golden` | Modified | Update only directly affected golden fixtures. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Over-normalizing platform wording | Medium | Assert platform-native terms for Kimi, Kiro, Windsurf, and Antigravity. |
| Generic asset causes wider fixture churn | Medium | Update only direct consumers and review golden diffs semantically. |
| Golden updates hide missed requirements | Medium | Strengthen static assertions before accepting fixture changes. |

## Rollback Plan

Revert the non-Claude orchestrator asset edits, associated direct test changes, and direct golden fixture updates. No runtime migration or generated state rollback is required.

## Dependencies

- Existing `sdd-orchestrator-assets` spec.
- Existing golden fixture generation/comparison tests.

## Success Criteria

- [ ] Remaining non-Claude orchestrator assets include chain strategy guidance and propagation where delivery planning is relevant.
- [ ] Platform-specific delegation/inline semantics remain accurate.
- [ ] Direct static tests and impacted golden fixtures pass without unrelated churn.
