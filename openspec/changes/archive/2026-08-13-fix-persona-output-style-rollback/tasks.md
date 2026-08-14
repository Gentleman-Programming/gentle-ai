# Tasks: Rollback-safe persona output-style transitions (#3163)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~230-260 (seam+type ~35, helper+plumbing ~20, tests ~170, docs ~10) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR (3 work-unit commits) |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units (single PR)

| Unit | Goal | Focused test command | Runtime harness | Rollback boundary |
|------|------|----------------------|-----------------|-------------------|
| 1 | Seam + typed error (inject.go, inject_test.go) | `go test ./internal/components/persona/...` | N/A — success paths byte-identical; CLI scenario proven in unit 3 | Revert inject.go only; old `os.Remove`/generic-error behavior restored |
| 2 | Warning plumbing (persona_rollback.go, run.go, sync.go) | `go test ./internal/cli/ -run 'TestHandleRolledBack|TestPersona'` | N/A — pure classification helper; observable CLI path proven in unit 3 | Revert run.go/sync.go edits + delete persona_rollback.go; exits 1 again |
| 3 | CLI e2e parity tests (persona_transition_test.go) | `go test ./internal/cli/ -run 'TestPersonaOutputStyleTransitionRollback'` | CLI e2e test is the harness (precedent: sync_review_retirement_test.go:106) | Delete test file only |

## Phase 1: Removal seam + typed error (RED → GREEN)

- [x] 1.1 RED: in `internal/components/persona/inject_test.go`, write failure-injection test — override `persona.RemoveFileFn` with failing func (count calls, `t.Cleanup` restore), run Gentleman→Neutral transition; assert `*persona.OutputStyleRemovalError` carrying path + unwrapped injected error (SEN-INJECTED-REMOVAL-FAILURE). Compile-fails until 1.2.
- [x] 1.2 GREEN: in `internal/components/persona/inject.go` add `var RemoveFileFn = os.Remove` (near osReadFile :590), route `removeFileAtomic` (:696-708) through it; add `OutputStyleRemovalError` (Path/Err, Error(), Unwrap()) + `MessageRolledBackOutputStyle` const; replace :427 with `&OutputStyleRemovalError{Path: retiredPath, Err: err}`. Grep `remove retired output style` first (verified: no test asserts old text).
- [x] 1.3 Guard: success paths unchanged — `TestPersonaSyncOutputStyleSwitchIsIdempotent` (sync_test.go:3988) and SEN-SUCCESS-TRANSITION still green.

## Phase 2: Warning plumbing (D2)

- [x] 2.1 RED: `internal/cli/persona_rollback_test.go` — build `pipeline.ExecutionResult` directly; helper true only for typed error + `Rollback.Success`; false on rollback-failed, generic error, nil error.
- [x] 2.2 GREEN: create `internal/cli/persona_rollback.go` — `handleRolledBackPersonaTransition(exec pipeline.ExecutionResult) bool`; prints `WARNING: <const>` to stderr (pattern run.go:188/209/1711).
- [x] 2.3 GREEN: `run.go` :222-225 — call helper before wrapping `Execution.Err`; if true `return result, nil` (skips post-apply verification); same at `sync.go` :1599-1602.

## Phase 3: CLI e2e parity (persona_transition_test.go)

- [x] 3.1 e2e install rollback: failing `RemoveFileFn` → rollback restores gentleman.md + settings.json byte-for-byte, neutral.md absent, user file untouched, hook called once, stderr contains constant, err nil (SEN-ROLLBACK-RESTORES, SEN-NO-PARTIAL-STATE, SEN-EXIT-ZERO-ON-ROLLBACK, SEN-WARNING-EXPLAINS-ROLLBACK, SEN-NO-RETRY-LOOP, SEN-USER-FILE-PRESERVED). Mirror sync_test.go:3988 setup + `UserHomeDirFn` overrides (sync_review_retirement_test.go:52-55).
- [x] 3.2 parity (SEN-INSTALL-SYNC-PARITY): loop install (`RunInstall`) + sync (`RunSyncWithSelection`) with identical injection; both restore, warn, exit 0.
- [x] 3.3 Guard: `TestPersonaSyncOutputStyleSwitchIsIdempotent` + `TestSyncRollbackRestoresRemovedRetiredReviewPlugins` still pass (SEN-IDEMPOTENT-SECOND-RUN).

## Phase 4: Verification + docs

- [x] 4.1 Run `go test ./...` and `go vet ./...` — all green. (64 packages ok; 3 pre-existing `codex` failures confirmed at baseline `2ef223bf` — TestEveryManifestDigestStaysByteStable/codex, TestNegotiatedConsentEnvelopeBindsTheDeclaredRuntimeIdentity/codex, TestDirectRouteStillRefusesADeclaredRuntime/codex; all untouched by this change. `go vet ./...` clean.)
- [x] 4.2 Grep `docs/` for hard-fail (exit 1) claims about removal failure; update if found (no CHANGELOG.md exists). — No matches found; no doc changes needed.
