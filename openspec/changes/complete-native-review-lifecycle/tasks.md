# Tasks: Complete Native Review Lifecycle

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 450–600 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | One approved `size:exception` PR; no chained slices |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Frozen-ledger native lifecycle and shared contract | One `size:exception` PR | `go test ./internal/reviewtransaction ./internal/cli ./internal/components/sdd` | Fixture-driven `review-step`: complete-fix → validate-fix → `review-resume` | Revert the nine listed native, asset, and test files together; v1 records remain readable |

## Phase 1: RED — Native Boundaries

- [x] 1.1 In `internal/reviewtransaction/{transaction,store}_test.go`, add failing cases accepting canonical immutable-genesis path subsets and rejecting added, renamed, deleted, and intended-untracked out-of-scope correction paths before append, with no state or budget mutation.
- [x] 1.2 In the same tests, add failing cases for exact frozen-ID binding, missing/stale/failed original-criteria or regression evidence, and successor tampering.
- [x] 1.3 Add failing follow-up cases proving late observations remain non-blocking and never mutate findings, outcomes, scope, ledger IDs, or counters; an original-criterion regression escalates without another correction.

## Phase 2: GREEN — Native Lifecycle

- [x] 2.1 Add the canonical subset helper in `internal/reviewtransaction/snapshot.go`; enforce it before mutation in `transaction.go` and during semantic successor validation in `store.go`.
- [x] 2.2 Extend `ScopedValidationResult` in `transaction.go` with SHA-256-bound original-criteria and correction-regression results plus follow-ups; derive approval only from both checks and preserve one correction budget.
- [x] 2.3 Update `internal/cli/review.go` and `internal/cli/review_test.go` so targeted validation appends its authoritative v1 CAS operation and remains resumable; add no command, schema, receipt, gate, or bundle surface.

## Phase 3: RED/GREEN — Shared Contract Parity

- [x] 3.1 In `internal/components/sdd/{bounded_review_contract_test,review_ledger_contract_test}.go`, add catalog-driven RED parity assertions for every `catalog.AllAgents()` asset: required immutable scope, frozen ledger, targeted validation, and non-blocking follow-ups; forbid stale broad-review language.
- [x] 3.2 Update only `internal/assets/skills/_shared/review-ledger-contract.md` to satisfy parity with concise shared lifecycle wording; leave renderers and per-agent assets unchanged.

## Phase 4: Refactor and Verify

- [x] 4.1 Refactor only duplicated validation/path assertions while preserving v1 CAS, receipts, gates, resume, and bundles; run `gofmt` on changed Go files.
- [x] 4.2 Remove redundant tests, unused compatibility branches, and stale broad post-fix review language from every shipped agent asset; keep only assertions that protect scope, the frozen ledger, targeted verification, and all-agent parity.
- [x] 4.3 Run focused evidence: `go test ./internal/reviewtransaction ./internal/cli ./internal/components/sdd`; record original acceptance and correction-regression results.
- [x] 4.4 Run full verification: `go test ./...` and `go vet ./...`; compare every spec scenario and task, escalating contradictions without reopening review.
- [x] 4.5 Add the missing final-verification contradiction test proving escalation without another review or correction budget.
- [x] 4.6 Add one transaction test proving multiple atomic work units still consume one correction budget; complete the HYBRID apply-progress evidence.
