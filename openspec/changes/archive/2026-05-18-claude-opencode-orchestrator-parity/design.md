# Design: Claude/OpenCode Orchestrator Parity

## Technical Approach

Patch only the Claude SDD orchestrator guidance asset, using OpenCode’s existing `### Chain Strategy` and review-workload wording as the behavioral reference. The implementation must preserve Claude Code terminology: Claude guidance may say `delegate`/`task`, but must not imply OpenCode plugin-backed background persistence. No runtime injection code changes are required because `injectMarkdownSections` already reads `assets.MustRead("claude/sdd-orchestrator.md")`, and the Claude golden tests already exercise that path.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Scope boundary | Modify `internal/assets/claude/sdd-orchestrator.md` only, plus direct Claude golden/static tests | Change generic/OpenCode assets or injection code | Proposal explicitly calls for a controlled first fix; OpenCode is reference-only and `inject.go` already selects the Claude asset for Markdown-section injection. |
| Chain wording | Add a Claude-native `### Chain Strategy` section with canonical `stacked-to-main` and `feature-branch-chain` values | Copy OpenCode text verbatim | Semantics must match, but Claude should not inherit OpenCode-specific assumptions. |
| Propagation wording | Update `Delivery Strategy`, review guard, and apply launch guidance to pass `chain_strategy` beside `delivery_strategy` | Only document strategies without propagation | Spec requires downstream `sdd-tasks` and `sdd-apply` receive both fields when delivery planning is relevant. |
| Validation | Refresh Claude goldens via existing `-update` flow and add/adjust static assertions in `internal/assets/assets_test.go` | Rely on goldens only | Goldens catch full rendered drift; static tests make canonical strategy labels and anti-plugin wording explicit. |

## Data Flow

```text
internal/assets/claude/sdd-orchestrator.md
        │ embedded FS
        ▼
internal/components/sdd.injectMarkdownSections
        │ InjectMarkdownSection("sdd-orchestrator", content)
        ▼
~/.claude/CLAUDE.md
        │ golden tests
        ▼
testdata/golden/sdd-claude-claudemd.golden
testdata/golden/combined-claude-claudemd.golden
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/assets/claude/sdd-orchestrator.md` | Modify | Add Chain Strategy section, forward `chain_strategy` to tasks/apply guidance, revise async wording to Claude-native semantics. |
| `internal/assets/opencode/sdd-orchestrator.md` | Reference only | Baseline for chain semantics; no edit intended. |
| `internal/components/sdd/inject.go` | Reference only | Confirms Claude injection already reads the Claude asset; no edit intended. |
| `internal/assets/assets_test.go` | Modify | Add static checks for required Claude strategy labels, propagation wording, and absence of OpenCode plugin-backed persistence claims. |
| `internal/components/golden_test.go` | Reference / possible modify | Existing tests and `-update` path produce golden refresh; no harness change expected unless test names need narrowing. |
| `testdata/golden/sdd-claude-claudemd.golden` | Modify | Refresh standalone Claude SDD output. |
| `testdata/golden/combined-claude-claudemd.golden` | Modify | Refresh combined Claude output containing the same SDD section. |

## Interfaces / Contracts

No Go API changes. The text contract is:
- Claude guidance MUST name exactly `stacked-to-main` and `feature-branch-chain`.
- Claude guidance MUST instruct passing `chain_strategy` with `delivery_strategy` to `sdd-tasks` and `sdd-apply`.
- Claude guidance MUST describe Claude Code delegation without OpenCode plugin/background persistence guarantees.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Static asset | Canonical strategy names, propagation text, no OpenCode plugin persistence claims | `go test ./internal/assets -run 'TestClaudeEmbeddedAssetLayout|TestClaudeSDDOrchestratorChainStrategy'` |
| Golden | Claude standalone and combined injected output | `go test ./internal/components -run 'TestGoldenSDD_Claude|TestGoldenCombined_Claude' -update`, inspect diff, then rerun without `-update`. |
| Focused package | All directly affected component/assets tests | `go test ./internal/assets ./internal/components -run 'TestClaude|TestGoldenSDD_Claude|TestGoldenCombined_Claude|TestSDDOrchestratorAssetsScopedToDedicatedAgent'` |

## Migration / Rollout

No migration required. Ship as one single-PR work unit: asset text + direct static/golden verification only.

## Open Questions

None.
