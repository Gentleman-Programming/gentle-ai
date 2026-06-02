# Apply Progress: fix-persona-artifact-language-contract

## Workload / PR Boundary

- Delivery decision: single PR approved by user despite 400-line review risk.
- Implementation diff currently remains under the 1000-line apply stop threshold when excluding pre-existing untracked planning artifacts and `context.md`.
- Tracked diff: 609 additions, 39 deletions.
- New implementation test files: 254 lines.
- Pre-existing untracked OpenSpec planning artifacts and `context.md` are still present and were not deleted.

## Completed Tasks

- Added asset-level language contract guards for every embedded SDD orchestrator asset.
- Added all-supported-agent matrix coverage, including OpenCode, Kilocode, Claude, Kimi, Codex, Gemini, Qwen, Cursor, Windsurf, Antigravity, Kiro, generic fallback, OpenClaw, Pi, and Trae.
- Added root/embedded `comment-writer` language-contract consistency checks.
- Added OpenCode/Kilocode generated settings regression coverage for known leak terms.
- Added OpenCode shared prompt coverage for delegated SDD phase prompt files.
- Added installed `comment-writer` coverage through the skills component.
- Added `gentleman-neutral-artifacts` model, CLI validation, TUI option, review label, and persona injection support.
- Normalized all SDD orchestrator assets with the three-domain language contract.
- Replaced OpenCode Spanish preflight voseo leaks with neutral/professional Spanish.
- Updated preserved OpenCode preflight migration text to avoid regenerating old voseo wording.
- Added delegated SDD phase language contract wording to embedded SDD phase skills.
- Updated root and embedded `comment-writer` to use target-context language and neutral/professional Spanish by default.
- Preserved Gentleman direct-conversation voice while adding explicit artifact/comment boundaries to persona assets.
- Regenerated affected golden fixtures after behavior tests passed.

## Files Changed

- `internal/assets/language_contract_test.go`
- `internal/assets/*/sdd-orchestrator.md`
- `internal/assets/*/persona-gentleman.md`
- `internal/assets/generic/persona-neutral.md`
- `internal/assets/skills/comment-writer/SKILL.md`
- `internal/assets/skills/sdd-*/SKILL.md`
- `skills/comment-writer/SKILL.md`
- `internal/components/sdd/inject.go`
- `internal/components/sdd/inject_test.go`
- `internal/components/sdd/prompts_test.go`
- `internal/components/skills/inject_test.go`
- `internal/components/persona/inject.go`
- `internal/components/persona/persona_language_contract_test.go`
- `internal/cli/validate.go`
- `internal/cli/persona_language_contract_test.go`
- `internal/model/types.go`
- `internal/tui/model_test.go`
- `internal/tui/screens/persona.go`
- `internal/tui/screens/review.go`
- `internal/tui/screens/persona_language_contract_test.go`
- affected `testdata/golden/*` files

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| SDD asset language contract | `internal/assets/language_contract_test.go` | Unit/asset guard | N/A new test file | Failed on all orchestrator assets missing contract | Passed after adding contract to all SDD orchestrators | Covered direct asset enumeration and supported-agent matrix | `gofmt`, helpers reused |
| Comment-writer contract | `internal/assets/language_contract_test.go`, `internal/components/skills/inject_test.go` | Unit/install component | N/A new tests | Failed on root/embedded/installed skill missing target-context wording | Passed after root and embedded skill update | Covered source files and installed OpenCode output | `gofmt` |
| OpenCode/Kilocode generated prompts | `internal/components/sdd/inject_test.go`, `internal/components/sdd/prompts_test.go` | Component | Existing package later run | Failed on generated settings/shared prompts missing contract and asset selection missing Kilocode | Passed after SDD asset selection, migration text, and phase skill updates | Covered OpenCode, Kilocode, and shared prompt files | `gofmt`, golden regeneration |
| `gentleman-neutral-artifacts` support | `internal/cli/persona_language_contract_test.go`, `internal/tui/screens/persona_language_contract_test.go`, `internal/components/persona/persona_language_contract_test.go` | Unit/component | N/A new tests | Compile failed because `PersonaGentlemanNeutralArtifacts` was undefined | Passed after model, CLI, TUI, and persona injection support | Covered CLI normalization, TUI rendering, and OpenCode persona injection | `gofmt` |
| Golden fixtures | existing `internal/components/golden_test.go` | Golden/integration | Failed after behavior changes | Golden mismatches showed generated outputs needed update | Passed after `go test ./internal/components/ -run 'TestGolden' -update` | Covered SDD/persona outputs across agent families | Full component and full suite rerun |

## Test Commands Run

- `go test ./internal/assets/... -run 'TestSDDOrchestratorAssetsEnforceLanguageContract|TestSupportedAgentSDDLanguageMatrix|TestCommentWriterLanguageContractSources|TestGentlemanPersonaKeepsDirectConversationVoice'` — RED failed as expected.
- `go test ./internal/components/sdd/... -run 'TestSDDOrchestratorAssetSelectionCoversSupportedAgents|TestInjectOpenCodeAndKilocodeLanguageContractOutputs|TestWriteSharedPromptFilesLanguageContract'` — RED failed as expected.
- `go test ./internal/components/skills/... -run TestInjectCommentWriterLanguageContractForOpenCode` — RED failed as expected.
- `go test ./internal/cli/... -run TestNormalizePersonaAcceptsGentlemanNeutralArtifacts` — GREEN passed after implementation.
- `go test ./internal/tui/screens/... -run 'TestPersonaOptionsIncludeGentlemanNeutralArtifacts|TestRenderPersonaDescribesGentlemanNeutralArtifacts'` — GREEN passed after implementation.
- `go test ./internal/components/persona/... -run TestInjectGentlemanNeutralArtifactsUsesGentlemanConversationWithArtifactBoundary` — GREEN passed after implementation.
- `go test ./internal/assets/...` — passed.
- `go test ./internal/components/sdd/...` — passed.
- `go test ./internal/components/skills/...` — passed.
- `go test ./internal/components/persona/...` — passed.
- `go test ./internal/cli/...` — passed.
- `go test ./internal/tui/...` — passed.
- `go test ./internal/components/ -run 'TestGolden' -update` — updated affected goldens.
- `go test ./internal/components/...` — passed.
- `go test ./...` — passed.
- `go vet ./...` — passed.

## Deviations From Design

- Implemented `gentleman-neutral-artifacts` support because discovery confirmed it was missing and tasks marked it conditional in scope.
- OpenClaw and Trae markdown-section SDD injection now route through `sddOrchestratorAsset(adapter.Agent())`, so they receive the generic SDD orchestrator instead of the Claude-specific asset.
- Root and embedded `comment-writer` are behaviorally aligned, not byte-for-byte identical.

## Remaining Tasks / Risks

- No commit, push, or PR was created.
- Dedicated CLI stale-sync test coverage was not added as a separate test; OpenCode preserved-prompt migration, generated settings, shared prompt, installed skill, and golden coverage exercise the same embedded install/sync regeneration sources.
- Worktree includes pre-existing untracked `context.md` and OpenSpec planning artifacts. They were preserved.
