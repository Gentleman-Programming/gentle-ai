# Apply Progress: All-Agent SDD Orchestrator Parity

## Status

All tasks in `openspec/changes/all-agent-orchestrator-parity/tasks.md` are complete for the maintainer-approved single-PR `size:exception` path.

## Scope Boundary

- Changed only non-Claude SDD orchestrator assets listed in `design.md`.
- Did not modify Claude or OpenCode orchestrator assets.
- Did not modify runtime/template/generator architecture.
- Updated direct static tests, focused injection tests, and direct semantic golden fixtures. `combined-windsurf-global-rules.golden` was also refreshed because broad verification surfaced the same direct Windsurf asset change through the combined installer fixture.
- Did not stage or modify untracked `.pi/`.

## Completed Tasks

- [x] 1.1 Confirm no runtime/template refactor scope and capture implementation boundaries.
- [x] 1.2 Add table-driven static assertion matrix for canonical strategies and `chain_strategy` propagation.
- [x] 1.3 Add forbidden-phrase assertions for inaccurate OpenCode persistence/subagent semantics.
- [x] 2.1 Update Codex orchestrator chain strategy and propagation wording.
- [x] 2.2 Update Gemini orchestrator chain strategy and propagation wording.
- [x] 2.3 Update Qwen orchestrator chain strategy and propagation wording.
- [x] 2.4 Update Generic orchestrator chain strategy and propagation wording.
- [x] 3.1 Update Kimi with `/skill:sdd-*` + `multiagent:Task` strategy context.
- [x] 3.2 Update Kiro with Kiro phase/native subagent context wording.
- [x] 3.3 Update Windsurf with inline phase-context propagation.
- [x] 3.4 Update Antigravity with inline phase-context propagation.
- [x] 4.1 Extend focused injection wording checks.
- [x] 4.2 Run focused static and injection tests.
- [x] 4.3 Refresh directly impacted golden fixtures and rerun targeted golden tests.
- [x] 4.4 Run broad `go test ./...` and `go vet ./...`.
- [x] 5.1 Verify changed files remain within approved asset/test/golden scope.
- [x] 5.2 Document `size:exception`; final diff exceeds 400 changed lines.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `internal/assets/assets_test.go`, `internal/components/sdd/inject_test.go` | Static/Injection | ✅ `go test ./internal/assets -run 'TestClaudeSDDOrchestratorChainStrategy\|TestSDDOrchestratorAssetsScopedToDedicatedAgent'` and injection baseline passed | ✅ Boundary captured through new static/injection tests before asset edits | ✅ New tests passed after asset updates | ✅ Covered delegate-capable, Kimi, Kiro, Windsurf, Antigravity, and generic consumers | ✅ Scope checked via `git diff --stat` and status |
| 1.2 | `internal/assets/assets_test.go` | Static unit | ✅ baseline focused assets passed | ✅ `TestNonClaudeSDDOrchestratorChainStrategyParity` failed on missing `### Chain Strategy` and propagation text | ✅ `go test ./internal/assets -run 'TestNonClaudeSDDOrchestratorChainStrategyParity\|TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims'` passed | ✅ Table covers 8 non-Claude assets and platform-specific propagation scopes | ✅ Consolidated assertions into table-driven checks |
| 1.3 | `internal/assets/assets_test.go` | Static unit | ✅ baseline focused assets passed | ✅ `TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims` failed on missing native wording | ✅ Platform-native forbidden/required assertions passed | ✅ Covers Kimi/Kiro/Windsurf/Antigravity and multiple forbidden OpenCode persistence claims | ✅ Kept checks behavior/text-contract focused |
| 2.1-2.4 | `internal/assets/assets_test.go` | Static unit | ✅ RED tests already failing before production edits | ✅ Delegate-capable assets failed missing canonical chain section | ✅ Focused asset tests passed after Codex/Gemini/Qwen/Generic edits | ✅ Four host assets plus generated generic VS Code fixture exercised | ✅ Reused canonical wording from reference assets without runtime refactor |
| 3.1-3.4 | `internal/assets/assets_test.go`, `internal/components/sdd/inject_test.go` | Static/Injection | ✅ RED tests already failing before production edits | ✅ Platform-native generated prompt test failed on missing `### Chain Strategy` | ✅ Focused asset + injection tests passed after Kimi/Kiro/Windsurf/Antigravity edits | ✅ Separate required wording for Kimi custom-agent prompt, Kiro phase context, Windsurf/Antigravity inline context | ✅ Replaced inaccurate solo-inline `delegate to sdd-init sub-agent` wording |
| 4.1 | `internal/components/sdd/inject_test.go` | Injection | ✅ focused injection baseline passed | ✅ New injection test failed on all four platform-native hosts before asset edits | ✅ New injection test passed after asset edits | ✅ Exercises generated files for Kimi, Kiro, Windsurf, and Antigravity | ✅ Table-driven generated prompt assertions |
| 4.2 | Existing focused test suites | Static/Injection | ✅ N/A verification task | ✅ Focused tests failed before implementation during RED cycle | ✅ `go test ./internal/assets -run 'Test.*SDDOrchestrator.*'` and `go test ./internal/components/sdd -run 'TestInject(Kimi\|Qwen\|Gemini\|OpenClaw\|.*Windsurf\|.*Antigravity\|.*Kiro)'` passed | ✅ Covers both static asset contracts and generated platform prompts | ➖ Verification only |
| 4.3 | `testdata/golden/*` | Golden | ✅ Targeted golden tests run after focused tests | ✅ Targeted golden run required `-update` after asset edits | ✅ Targeted update and rerun passed; broad run identified and refreshed combined Windsurf fixture | ✅ Direct SDD goldens plus combined Windsurf fixture covered direct asset consumers | ➖ Golden refresh only |
| 4.4 | Full suite | Broad regression | ✅ N/A verification task | ✅ First `go test ./...` exposed stale combined Windsurf golden | ✅ Re-run `go test ./...` passed after targeted combined fixture update; `go vet ./...` passed | ✅ Broad suite exercised all packages | ➖ Verification only |
| 5.1-5.2 | Git status/diff | Scope/delivery | ✅ N/A artifact check | ✅ Diff inspection surfaced >400 changed lines and approved scope | ✅ Scope limited to approved non-Claude assets/tests/goldens plus OpenSpec task/progress artifacts | ✅ Checked changed file set and documented `size:exception` | ➖ Documentation only |

## Verification Commands

- ✅ `go test ./internal/assets -run 'TestClaudeSDDOrchestratorChainStrategy|TestSDDOrchestratorAssetsScopedToDedicatedAgent'`
- ✅ `go test ./internal/components/sdd -run 'TestInject(Kimi|Qwen|Gemini|OpenClaw|.*Windsurf|.*Antigravity|.*Kiro)'` (baseline before edits)
- ❌ RED: `go test ./internal/assets -run 'TestNonClaudeSDDOrchestratorChainStrategyParity|TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims'`
- ❌ RED: `go test ./internal/components/sdd -run 'TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording'`
- ✅ GREEN: `go test ./internal/assets -run 'TestNonClaudeSDDOrchestratorChainStrategyParity|TestPlatformNativeSDDOrchestratorsAvoidOpenCodePersistenceClaims'`
- ✅ GREEN: `go test ./internal/components/sdd -run 'TestInjectKimiKiroWindsurfAntigravityPreserveNativeChainStrategyWording'`
- ✅ `go test ./internal/assets -run 'Test.*SDDOrchestrator.*'`
- ✅ `go test ./internal/components/sdd -run 'TestInject(Kimi|Qwen|Gemini|OpenClaw|.*Windsurf|.*Antigravity|.*Kiro)'`
- ✅ `go test ./internal/components -run 'TestGoldenSDD_(Codex|Gemini|Windsurf|Kiro|Antigravity|Cursor|VSCode)' -update`
- ✅ `go test ./internal/components -run 'TestGoldenSDD_(Codex|Gemini|Windsurf|Kiro|Antigravity|Cursor|VSCode)'`
- ❌ `go test ./...` initially failed on stale `TestGoldenCombined_Windsurf`.
- ✅ `go test ./internal/components -run 'TestGoldenCombined_Windsurf' -update`
- ✅ `go test ./internal/components -run 'TestGoldenCombined_Windsurf'`
- ✅ `go test ./...`
- ✅ `go vet ./...`

## Files Changed

- `internal/assets/{codex,gemini,qwen,generic,kimi,kiro,windsurf,antigravity}/sdd-orchestrator.md`
- `internal/assets/assets_test.go`
- `internal/components/sdd/inject_test.go`
- `testdata/golden/sdd-{antigravity,codex,gemini,kiro,vscode,windsurf}-*.golden`
- `testdata/golden/combined-windsurf-global-rules.golden`
- `openspec/changes/all-agent-orchestrator-parity/tasks.md`
- `openspec/changes/all-agent-orchestrator-parity/apply-progress.md`

## Deviations

- `combined-windsurf-global-rules.golden` was refreshed in addition to the targeted `sdd-*` fixtures because `go test ./...` showed the same direct Windsurf orchestrator asset text flows into the combined Windsurf installer fixture.

## Risks

- Final review size remains above the 400-line budget; this is covered by the maintainer-approved `size:exception` in the launch prompt and tasks forecast.
