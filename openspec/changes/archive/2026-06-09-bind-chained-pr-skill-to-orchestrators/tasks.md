# Tasks: Bind chained-pr Skill to SDD Orchestrator Assets

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (hand-written) | ~100–140 lines (tests + 11 template inserts + cursor section) |
| Estimated changed lines (generated goldens) | ~60–80 lines across 12 golden files |
| Total estimate | ~160–220 lines |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Notes

Hand-written diff is ~100–140 lines: 2 test files (≈40–60 lines combined) + 10 template one-liners (≈10 lines) + cursor's new `### Chain Strategy` section (≈18 lines). Golden churn is purely generated, limited to one new line per golden + cursor's new block — qualifies as `size:exception` territory for the golden portion but is clearly mechanical churn. No chained PRs needed.

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | All changes (tests RED → templates GREEN → goldens) | Single PR | Sequential TDD: test, fail, fix, regenerate, verify |

---

## Phase 1: Test-First — RED (extend static assertions before templates are changed)

- [x] 1.1 **Extend `internal/assets/assets_test.go` — `TestClaudeSDDOrchestratorChainStrategy`**: add the binding substring `treat \`chained-pr\` (registry skill \`gentle-ai-chained-pr\`) as a required skill match` to the `required` slice. Verify: `go test ./internal/assets/ -run TestClaudeSDDOrchestratorChainStrategy` → FAIL (template not yet edited).

- [x] 1.2 **Extend `internal/assets/assets_test.go` — `TestNonClaudeSDDOrchestratorChainStrategyParity` parity `required` slice**: add the binding substring to the inner `required` list so it is checked for all 8 existing parity rows. Verify: `go test ./internal/assets/ -run TestNonClaudeSDDOrchestratorChainStrategyParity` → FAIL.

- [x] 1.3 **Extend `internal/assets/assets_test.go` — add `cursor` and `opencode` rows to `TestNonClaudeSDDOrchestratorChainStrategyParity`**: insert `{path: "cursor/sdd-orchestrator.md", propagationScope: "prompt"}` and `{path: "opencode/sdd-orchestrator.md", propagationScope: "prompt"}` (opencode's forwarding line at line 271 already reads "Pass it as `chain_strategy` to `sdd-tasks` and `sdd-apply` prompts alongside `delivery_strategy`" — `prompt` substring confirmed present). Verify: same run → FAIL (cursor missing section, both missing binding).

- [x] 1.4 **Extend `internal/components/sdd/inject_test.go` — `TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording`**: add binding substring `treat \`chained-pr\` (registry skill \`gentle-ai-chained-pr\`) as a required skill match` to the `required` list of each of the four host entries (kimi, kiro, windsurf, antigravity). Verify: `go test ./internal/components/sdd/ -run TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording` → FAIL.

---

## Phase 2: Template Edits — GREEN (make RED tests pass)

- [x] 2.1 **Edit 10 templates that already have `### Chain Strategy`** — files: `internal/assets/{antigravity,claude,codex,gemini,generic,kimi,kiro,opencode,qwen,windsurf}/sdd-orchestrator.md`. For each, locate the `chain_strategy` forwarding line (the line containing `Pass it as \`chain_strategy\` to \`sdd-tasks\` and \`sdd-apply\` prompts`) and insert the canonical binding sentence immediately after it as a new paragraph:

  ```
  When delivery planning yields chained PRs, treat `chained-pr` (registry skill `gentle-ai-chained-pr`) as a required skill match: resolve it by registry name through this template's existing Sub-Agent Launch Pattern and inject the resolved `SKILL.md` path into the `sdd-tasks` and `sdd-apply` phase prompts under `## Skills to load before work`, instructing those phases to read and follow it BEFORE planning or creating any PR. Do not hardcode the skill path; defer resolution to the launch pattern.
  ```

  Verify per-template: binding substring present, no forbidden persistence wording introduced. Verify all: `go test ./internal/assets/ -run TestClaudeSDDOrchestratorChainStrategy` → PASS; `go test ./internal/assets/ -run TestNonClaudeSDDOrchestratorChainStrategyParity` (for the 8 original rows + opencode) → PASS.

- [x] 2.2 **Add full `### Chain Strategy` section to `internal/assets/cursor/sdd-orchestrator.md`**: insert after the `### Delivery Strategy` section (currently ends at line 166) and before `### Dependency Graph` (currently line 168). The new section body matches the design Interfaces verbatim, with the canonical binding sentence in place of `<binding sentence>`. Verify: `go test ./internal/assets/ -run TestNonClaudeSDDOrchestratorChainStrategyParity` cursor row → PASS.

---

## Phase 3: Golden Regeneration

- [x] 3.1 **Regenerate 12 chain-strategy goldens**: run `go test ./... -update` (or the project's golden-update flag — check the test file for the exact `-update` flag name). The 12 affected files are: `testdata/golden/sdd-opencode-multi-settings.golden`, `sdd-codex-agentsmd.golden`, `sdd-codex-agentsmd-powerful.golden`, `sdd-codex-agentsmd-lowcost.golden`, `sdd-claude-claudemd.golden`, `combined-claude-claudemd.golden`, `sdd-windsurf-global-rules.golden`, `sdd-vscode-instructions.golden`, `sdd-kiro-instructions.golden`, `sdd-gemini-geminimd.golden`, `sdd-antigravity-rulesmd.golden`, `combined-windsurf-global-rules.golden`. Note: cursor does not yet have a chain-strategy golden; if the test suite generates one it will be new, not an update.

---

## Phase 4: Full Verification

- [x] 4.1 **Run full test suite**: `go test ./internal/...` → all green. Confirmed no regressions in `TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims`, `TestOrchestratorsRejectDelegationBypassLanguage`, and `TestOrchestratorsRequireNonSkippableGeneralDelegationTriggers`.

- [x] 4.2 **Inspect golden diff**: confirmed each of the 12 regenerated goldens shows exactly one new line (the binding sentence) in the Chain Strategy section; cursor's golden (`sdd-cursor-rules.golden`) shows the full new section. No unrelated line churn found.

- [x] 4.3 **Confirm open questions resolved**: (a) opencode `chain_strategy` forwarding line contains `prompt` — confirmed at `opencode/sdd-orchestrator.md:271`. (b) Cursor `chain_strategy` forwarding line contains `prompt` — true after task 2.2 inserts the canonical section body which contains "prompts". Both parity rows use `propagationScope: "prompt"`. (c) Binding sentence does NOT trip any forbidden list for solo-inline hosts (windsurf/kimi/kiro/antigravity) — verified with full suite green.
