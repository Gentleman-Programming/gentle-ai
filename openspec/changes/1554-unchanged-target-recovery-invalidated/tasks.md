# Tasks: Relax CLI unchanged-target recovery gate for `--disposition invalidated`

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~60-90 (1 conjunct in `review_facade.go`; new test file with ~4-5 subtests) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | RED tests + gate conjunct in `internal/cli/review_facade.go` (~553-555) | PR 1 | `go test ./internal/cli/... -run TestUnchangedTargetRecovery` | Real temp Git repo via `initReviewCLIRepo` + `RunReviewInvalidate`/`RunReviewRecover` (no mocks) | Revert the single conjunct and delete the new test file |

## Phase 1: RED — Failing Tests First

- [x] 1.1 Create `internal/cli/review_recovery_unchanged_target_test.go`. Add shared fixture helper building a `StateInvalidated` predecessor with `TargetBaseDiff` (or overlay) initial snapshot via `initReviewCLIRepo` + `RunReviewInvalidate`, then a recovery attempt producing an identical target `Identity` (same `--base-ref`/candidate).
- [x] 1.2 `TestUnchangedTargetRecovery_InvalidatedAdmitted`: `RunReviewRecover(--disposition invalidated)` on the fixture. Assert it currently fails with `"recovery scope has not changed"` (proves RED — spec Scenario "Unchanged-target invalidated recovery succeeds").
- [x] 1.3 `TestUnchangedTargetRecovery_EscalatedStillBlocked`: same fixture, `--disposition escalated` with a fully valid `--maintainer-authorization`. Assert error is exactly `"recovery scope has not changed"` AND no successor record is persisted (load predecessor store again, confirm state unchanged) — spec Scenario "Unchanged-target escalated still rejected at CLI level".
- [x] 1.4 `TestUnchangedTargetRecovery_ScopeChangedStillBlocked`: same fixture, `--disposition scope_changed`. Assert error is exactly `"recovery scope has not changed"` — spec Scenario "Unchanged-target scope_changed still rejected at CLI level".
- [x] 1.5 `TestUnchangedTargetRecovery_BaseMismatchStillBlocked`: same fixture, `--disposition invalidated` with a `--base-ref` resolving to a different base tree. Assert error is exactly `"recovery base-ref does not match predecessor base"` — spec Scenario "Base-tree mismatch still rejected for invalidated".
- [x] 1.6 `TestUnchangedTargetRecovery_ChangedTargetInvalidatedStillSucceeds`: same predecessor kind, but successor snapshot has a genuinely different `Identity`, `--disposition invalidated`. Assert `RunReviewRecover` returns nil (no-regression happy path, unaffected by the conjunct).
- [x] 1.7 Run `go test ./internal/cli/... -run TestUnchangedTargetRecovery -v`. Confirm 1.2 fails on `"recovery scope has not changed"` and 1.3/1.4/1.5/1.6 already pass pre-change (they document current behavior that must NOT regress).

## Phase 2: GREEN — Minimal Production Change

- [x] 2.1 In `internal/cli/review_facade.go` (~553-555), change the gate condition to add `&& reviewtransaction.RecoveryDisposition(*disposition) != reviewtransaction.RecoveryInvalidated`, matching the `RecoveryDisposition(*disposition)` conversion precedent at line 512. Do not touch lines 518-519 (`--committed-only`/`--base-ref` matching check) or line 550-552 (base-tree guard).
- [x] 2.2 Run `go test ./internal/cli/... -run TestUnchangedTargetRecovery -v`. Confirm 1.2 now passes (admitted, delegated to `validateCompactRecoveryEdge`'s `RecoveryInvalidated` case) and 1.3/1.4/1.5/1.6 still pass unchanged.

## Phase 3: Regression Safety Net

- [x] 3.1 Confirm `TestEscalatedRecoveryRequiresChangedTarget` (existing package-level test locking `errCompactRecoveryTargetUnchanged`) still passes unmodified — proves the #1419/#1429 invariant is untouched.
- [x] 3.2 Grep the diff for any edit outside the single conjunct on the gate line and outside the new test file; if found, revert and re-scope.

## Phase 4: Verification

- [x] 4.1 Run `go test ./internal/cli/...` (full package) and `go test ./internal/reviewtransaction/...` — no regressions.
- [x] 4.2 Run `go test ./...` (full suite).
- [x] 4.3 Run `go vet ./...`.
- [x] 4.4 Run `git diff --stat` and confirm total changed lines stay well under the 400-line review budget.
- [x] 4.5 Confirm no `git push` or GitHub interaction occurred (hard constraint from proposal/spec).
