## Exploration: claude-opencode-orchestrator-parity

### Current State
Claude and OpenCode orchestrators already share the same delegation thresholds and index-first skill registry contract. The confirmed parity gaps are in orchestration guidance wording: OpenCode includes an explicit **Chain Strategy** section (`stacked-to-main` / `feature-branch-chain`) and passes `chain_strategy` into `sdd-tasks` and `sdd-apply`, while Claude currently does not. Claude also states `delegate (async)` as default, but OpenCode’s persisted async behavior depends on its `background-agents.ts` plugin, which is OpenCode-specific and not part of Claude.

### Affected Areas
- `internal/assets/claude/sdd-orchestrator.md` — primary file for first controlled fix (wording + chain strategy parity).
- `internal/assets/opencode/sdd-orchestrator.md` — parity reference baseline (already has Chain Strategy + `chain_strategy` propagation).
- `internal/components/golden_test.go` — golden coverage for Claude output (`sdd-claude-claudemd.golden`, `combined-claude-claudemd.golden`).
- `testdata/golden/sdd-claude-claudemd.golden` — expected standalone Claude injection output.
- `testdata/golden/combined-claude-claudemd.golden` — expected combined CLAUDE.md after persona + SDD + engram injection.
- `internal/assets/assets_test.go` — static assertions around orchestrator scoping/content that may require expectation tweaks if wording changes.
- `internal/components/sdd/inject.go` — confirms Claude injection path sources `assets.MustRead("claude/sdd-orchestrator.md")`.

### Approaches
1. **Claude-only orchestrator patch (controlled first fix)** — update Claude orchestration text to (a) add explicit Chain Strategy section and `chain_strategy` forwarding rules aligned with OpenCode semantics, and (b) tighten async delegation wording so it does not imply OpenCode plugin-backed persistence.
   - Pros: Minimal blast radius, matches requested controlled scope, easy rollback.
   - Cons: Leaves other agent orchestrators unchanged for now.
   - Effort: Low.

2. **Cross-agent normalization now** — apply the same chain-strategy + async wording adjustments across all orchestrator assets.
   - Pros: Broad consistency in one pass.
   - Cons: Larger review surface, violates requested first controlled step, higher regression risk.
   - Effort: Medium/High.

### Recommendation
Proceed with **Approach 1**. Update only `internal/assets/claude/sdd-orchestrator.md`, then regenerate/refresh affected Claude goldens and run focused tests that validate injected output. This gives parity for the immediate orchestration gap while respecting the user’s staged rollout plan (other agents later).

### Risks
- Wording drift risk: copying OpenCode sections too literally could reintroduce OpenCode-specific assumptions (plugin-backed async persistence) into Claude docs.
- Golden churn risk: small text edits will require synchronized golden updates; missing one causes test failures.
- Scope creep risk: touching shared injector behavior instead of asset-only changes would exceed the controlled-fix boundary.

### Ready for Proposal
Yes — the scope is clear, file/test impact is confirmed, and a low-risk first slice is identified.
