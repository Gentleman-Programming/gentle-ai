# Tasks: Level-Neutral Persona Parity

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 450-650 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 assets/contract → PR 2 injection/output styles → PR 3 sync fallback/e2e |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Neutral contract assets and asset tests | PR 1 | Base: main/tracker; no behavior wiring. |
| 2 | Claude/Kimi/OpenCode/Kilocode injection behavior | PR 2 | Depends on PR 1; includes RED/GREEN tests. |
| 3 | Sync fallback semantics and final verification | PR 3 | Depends on PR 2; includes sync tests and golden/e2e updates. |

## Phase 1: RED - Contract and Asset Tests

- [x] 1.1 Add failing asset tests in `internal/assets/language_contract_test.go` for generic/hermes neutral parity, interaction discipline, artifact language, and banned regional voice.
- [x] 1.2 Add failing embed coverage in `internal/assets/assets_test.go` for `claude/output-style-neutral.md` and `kimi/output-style-neutral.md`.
- [x] 1.3 Replace neutral expectations in `internal/components/persona/inject_test.go`: Claude must write Neutral style/settings, Kimi style must be non-empty, OpenCode/Kilocode sync cleanup must remove only `agent.gentleman`.
- [x] 1.4 Update `internal/cli/sync_test.go` fallback cases to expect neutral for missing/invalid/unreadable state and preserve explicit Gentleman/neutral/custom selections.

## Phase 2: GREEN - Assets

- [x] 2.1 Update `internal/assets/generic/persona-neutral.md` with Gentleman-equivalent mentor rules, short replies, one-question stop, no-menu default, verification-first, and artifact-language boundary.
- [x] 2.2 Update `internal/assets/hermes/persona-neutral.md` with the same contract while preserving Hermes identity, skill, and memory mechanics.
- [x] 2.3 Create `internal/assets/claude/output-style-neutral.md` with `name: Neutral` and neutral mentor/output-style rules.
- [x] 2.4 Create `internal/assets/kimi/output-style-neutral.md` with meaningful non-empty neutral output-style content.

## Phase 3: GREEN - Injection and Sync

- [x] 3.1 Update `internal/components/persona/inject.go` to write Claude `neutral.md`, set `outputStyle: "Neutral"`, remove stale Gentleman managed artifacts, preserve user styles, and remain idempotent.
- [x] 3.2 Update Kimi Jinja module injection to write neutral output style from `kimi/output-style-neutral.md`; reject empty/placeholder neutral content by construction or guard.
- [x] 3.3 Update OpenCode/Kilocode sync-managed neutral cleanup to remove only `agent.gentleman` while preserving sibling `agent` entries and malformed JSON tolerance.
- [x] 3.4 Update `internal/cli/sync.go` `applyResolvedPersona` comments and fallback logic so missing/invalid/unreadable persisted persona resolves to `model.PersonaNeutral`.

## Phase 4: REFACTOR and Verification

- [x] 4.1 Refactor duplicated test assertions into small helpers without weakening failure messages.
- [x] 4.2 Update golden/e2e fixtures only where affected by neutral asset or sync output changes.
- [x] 4.3 Run `go test ./...`, `go vet ./...`, and `RUN_FULL_E2E=1 e2e/docker-test.sh` only if sync/install behavior proves platform-sensitive.
