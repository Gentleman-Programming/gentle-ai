# Apply Progress: Level-Neutral Persona Parity

## Status

success

## Completed Tasks

- [x] 1.1 Add failing asset tests in `internal/assets/language_contract_test.go` for generic/hermes neutral parity, interaction discipline, artifact language, and banned regional voice.
- [x] 1.2 Add failing embed coverage in `internal/assets/assets_test.go` for `claude/output-style-neutral.md` and `kimi/output-style-neutral.md`.
- [x] 1.3 Replace neutral expectations in `internal/components/persona/inject_test.go`: Claude writes Neutral style/settings, Kimi style is non-empty, OpenCode/Kilocode sync cleanup removes only `agent.gentleman`.
- [x] 1.4 Update `internal/cli/sync_test.go` fallback cases to expect neutral for missing/invalid state and preserve explicit selections.
- [x] 2.1 Update `internal/assets/generic/persona-neutral.md` with mentor parity, short replies, one-question stop, no-menu default, verification-first, and artifact-language boundary.
- [x] 2.2 Update `internal/assets/hermes/persona-neutral.md` with the same contract while preserving Hermes identity, skill, and memory mechanics.
- [x] 2.3 Create `internal/assets/claude/output-style-neutral.md` with `name: Neutral` and neutral mentor/output-style rules.
- [x] 2.4 Create `internal/assets/kimi/output-style-neutral.md` with meaningful non-empty neutral output-style content.
- [x] 3.1 Update `internal/components/persona/inject.go` to write Claude `neutral.md`, set `outputStyle: "Neutral"`, remove stale Gentleman managed artifacts, preserve other settings, and remain idempotent.
- [x] 3.2 Update Kimi Jinja module injection to write neutral output style from `kimi/output-style-neutral.md`.
- [x] 3.3 Update OpenCode/Kilocode sync-managed neutral cleanup to remove only `agent.gentleman` while preserving sibling `agent` entries and tolerating malformed JSON.
- [x] 3.4 Update `internal/cli/sync.go` `applyResolvedPersona` comments and fallback logic so missing/invalid/unreadable persisted persona resolves to `model.PersonaNeutral`.
- [x] 4.1 Refactor duplicated test assertions into small helpers without weakening failure messages.
- [x] 4.2 Update golden fixtures affected by neutral persona output.
- [x] 4.3 Run focused tests plus `go test ./...` and `go vet ./...`; Docker E2E was not run because behavior was covered by unit/golden tests and no platform-sensitive Docker path changed.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `internal/assets/language_contract_test.go` | Unit | ✅ focused package baseline passed | ✅ failing neutral asset contract tests | ✅ `go test ./internal/assets` passed | ✅ generic + Hermes assets, required + banned markers | ✅ shared table-style assertions |
| 1.2 | `internal/assets/assets_test.go` | Unit | ✅ focused package baseline passed | ✅ missing embed asset tests failed | ✅ `go test ./internal/assets` passed | ✅ Claude + Kimi assets | ➖ Structural embed coverage |
| 1.3 | `internal/components/persona/inject_test.go` | Unit | ✅ focused package baseline passed | ✅ Claude/Kimi/OpenCode/Kilocode tests failed | ✅ `go test ./internal/components/persona` passed | ✅ install + sync, stale + malformed settings, idempotency | ✅ reused existing adapter helpers and focused assertions |
| 1.4 | `internal/cli/sync_test.go` | Unit | ✅ focused package baseline passed | ✅ fallback tests failed expecting neutral | ✅ focused CLI tests passed | ✅ missing state, invalid state, dry-run, explicit selection preservation | ✅ comments and test names aligned with new contract |
| 2.1-2.4 | `internal/assets/*` | Unit/golden | ✅ RED tests above | ✅ asset tests failed before asset updates | ✅ assets and golden tests passed | ✅ generic/Hermes/Claude/Kimi surfaces | ✅ shared neutral output-style wording |
| 3.1-3.4 | `internal/components/persona/inject_test.go`, `internal/cli/sync_test.go` | Unit | ✅ RED tests above | ✅ behavior tests failed before implementation | ✅ focused packages passed | ✅ Claude, Kimi, OpenCode, Kilocode, sync fallback variants | ✅ small output-style overlay helper |
| 4.1-4.3 | `testdata/golden/*` and full suite | Golden/unit | ✅ `go test ./...` exposed golden drift | ✅ golden mismatch confirmed | ✅ `go test ./...` and `go vet ./...` passed | ✅ Claude + OpenCode neutral golden fixtures | ✅ no further refactor needed |

## Test Summary

- Focused baseline before changes: `go test ./internal/assets ./internal/components/persona ./internal/cli` passed.
- RED run after test edits: focused packages failed for missing neutral assets, old neutral output-style assumptions, OpenCode/Kilocode sync cleanup, and sync fallback expectations.
- Focused GREEN: `go test ./internal/assets ./internal/components/persona ./internal/cli` passed.
- Golden update: `go test ./internal/components/ -run 'TestGoldenPersona_(Claude|OpenCode)_Neutral' -update`, inspected diff, then reran without `-update`.
- Broad verification: `go test ./...` passed.
- Static verification: `go vet ./...` passed.

## Deviations from Design

None. Implementation matches the OpenSpec design. Claude neutral now owns the managed `Neutral` output style, so explicit non-managed `outputStyle` settings are replaced when persona neutral is applied, while unrelated settings are preserved.

## Issues Found

- Existing tests encoded old behavior that neutral removed or preserved output-style settings; those tests were updated to the new `Neutral` output-style contract.
- Full suite exposed expected neutral persona golden drift; fixtures were updated and rerun deterministically.
