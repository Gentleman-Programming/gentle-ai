# Tasks: Fix persona/artifact language contract

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 700-1000, with possible overflow if golden fixtures churn |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1: RED tests and helpers → PR 2: SDD assets, OpenCode/Kilocode overlays, delegation forwarding → PR 3: comment-writer/persona option support and goldens |
| Delivery strategy | single PR approved by user |
| Chain strategy | not used for this apply |

Decision needed before apply: No
Chained PRs recommended: Yes, but user approved one PR
Chain strategy: single PR
400-line budget risk: High

## TDD Evidence Requirements

Strict TDD is active. Use `go test ./...` as the required full test runner, with targeted package tests during development.

- RED evidence: each test task records the failing test command, failing test names, and the old leak or missing contract that caused failure before implementation edits.
- GREEN evidence: each implementation task records the targeted passing command that proves the smallest behavior now works.
- TRIANGULATE evidence: add or confirm independent coverage across more than one agent/path so the fix is not tailored only to OpenCode or one phrase.
- REFACTOR evidence: after green behavior, run `gofmt` on changed Go files, remove duplicated test helper noise, update only necessary goldens, and re-run affected tests.

## 1. Infrastructure

### 1.1 Discover the persona option and supported asset matrix

- [x] Verify whether `gentleman-neutral-artifacts` already exists in `internal/model/types.go`, `internal/cli/validate.go`, `internal/tui/screens/persona.go`, `internal/components/persona/inject.go`, `internal/assets/`, `internal/catalog/agents.go`, and existing install/sync tests.
- [x] Record the discovered state in apply notes. Initial planning scan found only a mention in `context.md`, so apply should assume missing until re-verified.
- [x] If missing, execute the conditional implementation and verification tasks for model enum, CLI, TUI, install, and sync support below.

### 1.2 RED: Add all-agent SDD asset language-contract guards

- [x] Add tests in `internal/assets/assets_test.go` or `internal/assets/language_contract_test.go` that enumerate every supported SDD orchestrator asset: Claude, OpenCode, Kilocode via OpenCode, Kimi, Codex, Gemini, Qwen, Cursor, Windsurf, Antigravity, Kiro, generic fallback, OpenClaw, Pi, Trae, and any newly discovered supported agent-specific asset.
- [x] Assert persona-agnostic SDD assets include the three-domain contract: direct conversation follows persona, technical artifacts default to English, comments follow target context language.
- [x] Assert persona-agnostic SDD assets reject known leaks: `elegí`, `Respondé`, and `¿Querés ajustar algo o continuamos?`.
- [x] Keep explicit allowlists for Gentleman direct-conversation persona/output-style assets.
- [x] RED evidence: run targeted asset tests and capture expected failures before editing assets.

### 1.3 RED: Add root and embedded `comment-writer` consistency tests

- [x] Add tests that read `skills/comment-writer/SKILL.md` and `internal/assets/skills/comment-writer/SKILL.md`.
- [x] Assert both sources require target-context language, explicit user override precedence, neutral/professional Spanish by default, and no forced Rioplatense/voseo default for all Spanish comments.
- [x] RED evidence: run the targeted package test that reads both files and capture the current root/embedded drift.

### 1.4 RED: Add OpenCode/Kilocode overlay and shared prompt tests

- [x] Add or extend tests in `internal/components/sdd/` to inspect OpenCode single-mode and multi-mode outputs from `internal/assets/opencode/sdd-overlay-single.json`, `internal/assets/opencode/sdd-overlay-multi.json`, inlined placeholder behavior, and written shared prompt files.
- [x] Include Kilocode as a named case, not only implied OpenCode coverage.
- [x] Assert generated overlays and referenced shared prompt files carry the artifact/comment contract and omit the known leak terms.
- [x] RED evidence: run targeted `internal/components/sdd` tests and capture failures from stale overlay/shared prompt wording.

### 1.5 RED: Add install and sync propagation tests

- [x] Add fresh install tests for SDD plus skills outputs in `internal/components/sdd/`, `internal/components/skills/`, and/or `internal/cli/` so installed prompt and skill files contain the updated contract.
- [x] Add stale sync tests in `internal/cli/sync_test.go` that pre-seed old SDD/comment-writer content, run sync, and assert stale Rioplatense defaults are not regenerated.
- [x] Include neutral persona + SDD + skills combinations for OpenCode and Kilocode.
- [x] RED evidence: capture failing fresh install and stale sync cases before implementation.

### 1.6 RED: Add delegation and subagent contract-forwarding tests

- [x] Add tests that inspect orchestrator delegation/subagent prompt surfaces in `internal/assets/*/sdd-orchestrator.md`, `internal/assets/skills/sdd-*/SKILL.md`, and any shared phase prompt assets under `internal/assets/skills/_shared/`.
- [x] Assert delegated phase prompts forward technical-artifact English defaults, neutral/professional Spanish artifact defaults, and context-reactive comment defaults.
- [x] Cover native subagents, dynamic subagents, inline phase contexts, OpenCode shared prompt files, and overlay placeholder references where they exist.
- [x] RED evidence: capture failing delegation-forwarding assertions before asset edits.

### 1.7 RED: If missing, add `gentleman-neutral-artifacts` support tests

- [x] Add failing tests for enum and validation support in `internal/model/types.go` and `internal/cli/validate.go`.
- [x] Add failing TUI option/label tests in `internal/tui/screens/persona.go`, `internal/tui/screens/review.go`, and existing `internal/tui/model_test.go` coverage.
- [x] Add failing persona injection/install/sync tests in `internal/components/persona/inject_test.go`, `internal/cli/sync_test.go`, and `internal/components/golden_test.go`.
- [x] RED evidence: capture failures proving the option is not yet supported.

## 2. Implementation

### 2.1 GREEN: Normalize all SDD orchestrator assets

- [x] Update `internal/assets/opencode/sdd-orchestrator.md`, `internal/assets/claude/sdd-orchestrator.md`, `internal/assets/kimi/sdd-orchestrator.md`, `internal/assets/codex/sdd-orchestrator.md`, `internal/assets/gemini/sdd-orchestrator.md`, `internal/assets/qwen/sdd-orchestrator.md`, `internal/assets/cursor/sdd-orchestrator.md`, `internal/assets/windsurf/sdd-orchestrator.md`, `internal/assets/antigravity/sdd-orchestrator.md`, `internal/assets/kiro/sdd-orchestrator.md`, and `internal/assets/generic/sdd-orchestrator.md`.
- [x] Replace persona-agnostic voseo examples with neutral/professional or language-neutral wording.
- [x] Add clear artifact/comment language-boundary wording without weakening Gentleman direct conversation.
- [x] GREEN evidence: targeted all-agent asset language-contract tests pass.

### 2.2 GREEN: Fix OpenCode/Kilocode migration, overlays, and shared prompts

- [x] Update preserved prompt migration text in `internal/components/sdd/inject.go` if it can retain or regenerate stale OpenCode language.
- [x] Update `internal/assets/opencode/sdd-overlay-single.json` and `internal/assets/opencode/sdd-overlay-multi.json` only where overlay prompts or placeholders need explicit forwarding.
- [x] Update `internal/assets/skills/sdd-*/SKILL.md` and `internal/assets/skills/_shared/*` only if shared prompt tests prove executor prompt files need the contract there.
- [x] GREEN evidence: OpenCode and Kilocode overlay/shared prompt tests pass.

### 2.3 GREEN: Align root and embedded `comment-writer`

- [x] Update `skills/comment-writer/SKILL.md` and `internal/assets/skills/comment-writer/SKILL.md` so both require target-context language, explicit user override precedence, and neutral/professional Spanish by default.
- [x] Remove or reframe any wording that forces Rioplatense Spanish for every Spanish comment.
- [x] GREEN evidence: root/embedded consistency tests pass.

### 2.4 GREEN: Preserve persona boundaries

- [x] Inspect `internal/assets/*/persona-gentleman.md`, `internal/assets/generic/persona-neutral.md`, `internal/assets/claude/output-style-gentleman.md`, and `internal/assets/kimi/output-style-gentleman.md`.
- [x] Add artifact-boundary wording only if needed, while preserving Gentleman direct-conversation Rioplatense teaching voice.
- [x] GREEN evidence: persona allowlist tests pass and no persona-agnostic artifact tests regress.

### 2.5 GREEN: Forward the contract through delegation/subagent prompts

- [x] Update the relevant orchestrator/delegation prompt text in `internal/assets/*/sdd-orchestrator.md`, `internal/assets/skills/sdd-*/SKILL.md`, and `internal/assets/skills/_shared/*` so delegated phase executors receive the artifact/comment contract.
- [x] Ensure direct conversation persona rules are not forwarded as artifact language defaults.
- [x] GREEN evidence: delegation-forwarding tests pass across native, dynamic, inline, and OpenCode shared prompt paths.

### 2.6 GREEN: If missing, implement `gentleman-neutral-artifacts` model, CLI, and TUI support

- [x] Add the persona ID in `internal/model/types.go`.
- [x] Accept and validate the value in `internal/cli/validate.go`.
- [x] Add TUI selection and review labels/descriptions in `internal/tui/screens/persona.go` and `internal/tui/screens/review.go`.
- [x] Update `internal/tui/model.go` preset/component behavior only if the new persona changes component selection.
- [x] GREEN evidence: enum, CLI validation, and TUI tests for the new option pass.

### 2.7 GREEN: If missing, implement `gentleman-neutral-artifacts` install and sync support

- [x] Add the required persona asset source(s) under `internal/assets/` using existing persona asset conventions.
- [x] Route the new persona in `internal/components/persona/inject.go` and cover it in `internal/components/persona/inject_test.go`.
- [x] Update sync state handling in `internal/cli/sync.go` and stale/fresh coverage in `internal/cli/sync_test.go` if the new value must round-trip through state.
- [x] Update `internal/components/golden_test.go` and affected `testdata/golden/` files only after behavior tests pass.
- [x] GREEN evidence: install and sync tests for the new option pass.

### 2.8 GREEN: Update affected goldens after behavior is proven

- [x] Regenerate or manually update only affected golden fixtures under `testdata/golden/` after targeted behavior tests are green.
- [x] Review golden diffs for contract wording and absence of known leaks.
- [x] GREEN evidence: golden tests pass without masking failed behavior assertions.

### 2.9 REFACTOR: Keep the implementation reviewable

- [x] Deduplicate repeated banned-term and required-contract assertions into small helpers in the relevant test package.
- [x] Keep tests with the behavior they verify rather than splitting by file type.
- [x] Run `gofmt` on changed Go files.
- [x] REFACTOR evidence: targeted package tests still pass after cleanup.

## 3. Testing/Verification

### 3.1 TRIANGULATE all-agent matrix coverage

- [x] Confirm coverage includes OpenCode, Kilocode, at least one non-OpenCode dedicated agent, and the generic fallback in independent assertions.
- [x] Confirm OpenClaw, Pi, and Trae fallback/markdown-section behavior is tested so Claude-only wording is not accidentally reused where generic wording is expected.
- [x] TRIANGULATE evidence: targeted all-agent matrix tests pass and fail if any supported SDD asset is removed from the matrix.

### 3.2 TRIANGULATE comment language behavior

- [x] Confirm `comment-writer` tests cover Spanish context, English context, mixed context with target-message language, explicit user override, and explicit regional Spanish signal.
- [x] TRIANGULATE evidence: tests prove neutral/professional Spanish is the default for Spanish comments without forbidding explicit regional examples.

### 3.3 TRIANGULATE install/sync and overlay regeneration

- [x] Confirm fresh install and stale sync cases inspect generated files, not only embedded sources.
- [x] Confirm OpenCode/Kilocode tests inspect merged JSON overlays and referenced shared prompt files.
- [x] TRIANGULATE evidence: targeted install/sync and overlay tests pass for stale and fresh paths.

### 3.4 Run targeted verification

- [x] Run `go test ./internal/assets/...`.
- [x] Run `go test ./internal/components/sdd/...`.
- [x] Run `go test ./internal/components/skills/...`.
- [x] Run `go test ./internal/components/persona/...` if persona assets/options changed.
- [x] Run `go test ./internal/cli/...` if install/sync or CLI validation changed.
- [x] Run `go test ./internal/tui/...` if TUI persona option support changed.

### 3.5 Run full required verification

- [x] Run `go test ./...`.
- [x] Run `go vet ./...`.
- [x] Compare the final implementation against every scenario in `openspec/changes/fix-persona-artifact-language-contract/spec.md`.

### 3.6 Apply workload gate, split, and rollback boundaries

- [x] Before `sdd-apply`, check expected diff size against the user budget of 1000 changed lines and the repo CI/review 400-line risk.
- [x] If expected changed lines exceed 400, pause for a delivery decision: chained PRs vs accepted `size:exception`.
- [x] Use rollback boundaries by work unit: RED tests/helpers, SDD asset/overlay/delegation edits, `comment-writer`/persona support, and golden regeneration.
- [x] If changed lines approach or exceed 1000, stop and split before continuing.
