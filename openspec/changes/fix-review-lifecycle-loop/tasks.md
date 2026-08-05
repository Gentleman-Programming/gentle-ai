# Tasks: Fix Review Lifecycle Loop

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 180–300 authored; 2–4 generated golden |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | One focused PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Correct compact-v2 receipt/gate | One focused PR | `go test ./internal/reviewtransaction` | fixture-backed gate evaluation | compact code/tests |
| 2 | Align CLI, rules, and asset | Same PR | `go test ./internal/cli ./internal/components/sdd` | rendered rules/asset assertions | CLI, renderer, golden, asset/tests |

## Phase 1: Compact-v2 RED → GREEN

- [x] 1.1 **RED** — In `internal/reviewtransaction/compact_store_test.go`, persist/reload a manual historical-v2 state and receipt fixture; assert regenerated receipt equality and unchanged persisted bytes. Covers **Compact-v2 persisted receipt compatibility**.
- [x] 1.2 **GREEN** — In `internal/reviewtransaction/compact.go`, always bind the final tree to `CurrentSnapshot` and preserve the frozen initial/genesis path digest. Remove the candidate legacy predicate and keep schema-v2 fields/bytes unchanged.
- [x] 1.3 **REFACTOR/verify** — Run `go test ./internal/reviewtransaction`; retain a small named legacy predicate and gofmt without altering authority or schema.

## Phase 2: Final-scope gate RED → GREEN

- [x] 2.1 **RED** — In `internal/reviewtransaction/compact_gate_test.go`, prove pre-push and pre-PR allow an exact final tree whose live paths are a subset of genesis after a full revert, and deny a new path.
- [x] 2.2 **GREEN** — Make compact delivery validation require the exact final tree and treat genesis paths as the maximum authorized envelope; do not bind delivery to correction-delta paths.
- [x] 2.3 **REFACTOR/verify** — Run `go test ./internal/reviewtransaction`; preserve the existing fail-closed mismatch path.

## Phase 3: Maintainer-only guidance RED → GREEN

- [x] 3.1 **RED** — In `internal/cli/review_test.go`, table-test `scope-changed` JSON/action is `explicit-maintainer-action`, non-success, and never `create-new-lineage`; in `internal/components/sdd/triggerrules_test.go`, require maintainer wording and forbid automatic lineage/review/budget text.
- [x] 3.2 **GREEN** — Update `internal/cli/review.go` action mapping and `internal/components/sdd/triggerrules.go` receipt table; regenerate `internal/components/testdata/golden/trigger-rules-default.golden` through `RenderTriggerRules`, never hand-edit it.
- [x] 3.3 **REFACTOR/verify** — Run `go test ./internal/cli ./internal/components/sdd` and confirm existing `allow`, `invalidated`, and `escalated` wording remains covered.

## Phase 4: Archive contract and review boundary

- [x] 4.1 **RED** — In `internal/components/sdd/bounded_review_contract_test.go`, assert `internal/assets/skills/_shared/review-ledger-contract.md` lists only post-apply, pre-commit, pre-push, pre-pr, release; assert archive still requires structured status, `reviewGate.result: allow`, and an approved receipt.
- [x] 4.2 **GREEN** — Narrow that shared asset’s generic gate list; preserve archive readiness checks and add no `archive` CLI gate.
- [ ] 4.3 **Final verification/commit** — Run focused tests, `go test ./...`, `go vet ./...`; review `git diff --stat` before commit. Stop/re-plan before 800 changed lines; commit the one work unit as `fix(review): preserve compact final scope` (tests/golden/assets included). Roll back by reverting this commit; v2 data remains readable.
