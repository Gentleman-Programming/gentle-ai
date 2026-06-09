# Apply Progress: bind-chained-pr-skill-to-orchestrators

**Change**: bind-chained-pr-skill-to-orchestrators
**Mode**: Strict TDD
**Batch**: 1 of 1 (all tasks complete)

---

## TDD Cycle Evidence

| Task | RED (test written first) | GREEN (implementation passes) | REFACTOR |
|------|--------------------------|-------------------------------|----------|
| 1.1 | `TestClaudeSDDOrchestratorChainStrategy` extended with binding substring → FAIL confirmed | claude/sdd-orchestrator.md edited → PASS | N/A (no refactor needed) |
| 1.2 | `TestNonClaudeSDDOrchestratorChainStrategyParity` parity required slice extended → FAIL confirmed (8 rows) | All 8 templates edited → PASS | N/A |
| 1.3 | cursor + opencode rows added to parity test → FAIL confirmed (cursor missing section) | cursor section added, opencode already had binding → PASS | N/A |
| 1.4 | `TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording` required lists extended → FAIL confirmed (4 hosts) | All 4 templates already covered by 2.1 edits → PASS | N/A |
| 2.1 | (GREEN phase — templates edited after RED) | All 10 templates edited with canonical binding sentence → full assets test PASS | N/A |
| 2.2 | (GREEN phase — cursor section added after RED) | cursor/sdd-orchestrator.md gained full `### Chain Strategy` section with binding → cursor parity row PASS | N/A |
| 3.1 | (Golden regen) | `go test ./internal/components/ -update` regenerated 13 goldens (12 expected + new sdd-cursor-rules.golden) | N/A |
| 4.1 | (Full suite) | `go test ./internal/...` → all green, zero failures | N/A |
| 4.2 | (Golden diff) | Diff limited to binding sentence (once per chain-strategy golden) + cursor's new section. No unrelated churn. | N/A |
| 4.3 | (Open questions) | (a) opencode prompt confirmed; (b) cursor prompt confirmed after 2.2; (c) forbidden lists untripped | N/A |

---

## Completed Tasks

- [x] 1.1 Extend `TestClaudeSDDOrchestratorChainStrategy` with binding substring
- [x] 1.2 Extend `TestNonClaudeSDDOrchestratorChainStrategyParity` parity required slice with binding substring
- [x] 1.3 Add `cursor` and `opencode` rows to `TestNonClaudeSDDOrchestratorChainStrategyParity`
- [x] 1.4 Extend `TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording` with binding substring for all 4 hosts
- [x] 2.1 Edit 10 templates that already have `### Chain Strategy` (antigravity, claude, codex, gemini, generic, kimi, kiro, opencode, qwen, windsurf)
- [x] 2.2 Add full `### Chain Strategy` section to cursor/sdd-orchestrator.md
- [x] 3.1 Regenerate 13 chain-strategy goldens (12 updated + sdd-cursor-rules.golden new)
- [x] 4.1 Full test suite green (`go test ./internal/...`)
- [x] 4.2 Golden diff limited to binding sentence + cursor section
- [x] 4.3 All open questions resolved

---

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `internal/assets/assets_test.go` | Modified | Added binding substring to `TestClaudeSDDOrchestratorChainStrategy` required slice; added binding substring to parity loop required slice; added cursor + opencode rows to `TestNonClaudeSDDOrchestratorChainStrategyParity` |
| `internal/components/sdd/inject_test.go` | Modified | Added binding substring to `required` list of all 4 hosts in `TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording` |
| `internal/assets/claude/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/codex/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/gemini/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/generic/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/kimi/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/kiro/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/opencode/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/qwen/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/windsurf/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/antigravity/sdd-orchestrator.md` | Modified | Inserted canonical binding sentence after chain_strategy forwarding line |
| `internal/assets/cursor/sdd-orchestrator.md` | Modified | Added full `### Chain Strategy` section (with binding sentence) after `### Delivery Strategy` |
| `testdata/golden/sdd-claude-claudemd.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/combined-claude-claudemd.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-codex-agentsmd.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-codex-agentsmd-powerful.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-codex-agentsmd-lowcost.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-windsurf-global-rules.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/combined-windsurf-global-rules.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-vscode-instructions.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-kiro-instructions.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-gemini-geminimd.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-antigravity-rulesmd.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-opencode-multi-settings.golden` | Regenerated | +1 binding sentence in Chain Strategy |
| `testdata/golden/sdd-cursor-rules.golden` | Regenerated (new content) | Full `### Chain Strategy` section with binding sentence |

---

## Deviations from Design

None — implementation matches design exactly. The cursor template received the exact section body specified in `design.md` Interfaces section. All 11 templates share the byte-identical canonical binding sentence.

## Open Questions Resolved

1. **opencode `chain_strategy` line contains `prompt`**: Confirmed. Line 271 reads "Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`". `propagationScope: "prompt"` is correct.
2. **cursor `chain_strategy` forwarding line contains `prompt`**: True after task 2.2 — the new section body includes "Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`". `propagationScope: "prompt"` is correct.
3. **`forbidden` lists for solo-inline hosts**: The canonical binding sentence does NOT trip any forbidden list. "phase prompts" is not "custom sub-agent prompts" (windsurf), "inline phase context" does not appear in the binding (antigravity). Full suite green confirms this.
4. **Golden `-update` invocation**: `go test ./internal/components/ -update`. The flag is `-update` defined in `internal/components/golden_test.go:30`.

## Issues Found

None.

## Status

10/10 tasks complete. Ready for verify.

## Workload / PR Boundary

- Mode: single PR
- Current work unit: all tasks (single batch)
- Boundary: all template edits + test extensions + golden regeneration
- Estimated review budget impact: ~13 golden files + 11 template edits + 2 test files ≈ 200-250 lines total (within 400-line budget)
