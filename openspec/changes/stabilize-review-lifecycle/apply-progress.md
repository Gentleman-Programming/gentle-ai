# Apply Progress: Stabilize Review Lifecycle

## PR1 Status

- Delivery strategy: chained PRs, feature-branch-chain.
- Work unit: PR1, dormant lifecycle contracts only.
- Completed tasks: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3.
- Explicit boundary: no `GateArchive`, `archive-sdd`, remediation route, status routing, or guidance activation was added.

## Files Changed

| File | Change |
|---|---|
| `internal/reviewtransaction/compact.go` | v3-compatible final-scope receipt identity and dormant verify lifecycle states/events. |
| `internal/reviewtransaction/compact_gate.go` | Read-only `CompactGateReader`; no archive gate. |
| `internal/reviewtransaction/compact_store.go` | CAS-bound dormant verify failure, remediation, and escalation transitions. |
| `internal/reviewtransaction/receipt.go` | Structured scope denial and maintainer action fields. |
| `internal/reviewtransaction/compact_gate_test.go` | Reader purity contract. |
| `internal/reviewtransaction/compact_store_test.go` | CAS lifecycle, stale competitor, and frozen-budget contract. |
| `internal/reviewtransaction/receipt_test.go` | Final-scope agreement, incomplete identity, and scope-change action contract. |
| `internal/reviewtransaction/snapshot_test.go` | Sorted snapshot-path and final-scope identity contract. |
| `internal/sddstatus/review_binding.go` | Pure legacy binding parse/bytes/revision helpers. |
| `internal/sddstatus/verification.go` | Pure, content-bound remediation evidence helper. |
| `internal/sddstatus/review_binding_test.go` | Legacy byte and CAS compatibility contract. |
| `internal/sddstatus/remediation_test.go` | Exact remediation evidence contract. |
| `internal/cli/review_facade.go` | Pure dormant-boundary probe; no route registration. |
| `internal/cli/review_facade_test.go` | Dormant adapter contract. |
| `internal/cli/review_invalidate_test.go` | Invalidation help excludes PR2 routes. |
| `internal/cli/review_lifecycle_integration_test.go` | Existing review help surface excludes PR2 routes. |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `receipt_test.go`, `compact_gate_test.go`, `compact_store_test.go` | Unit | Baseline: `go test ./internal/reviewtransaction ./internal/sddstatus ./internal/cli` exit 0 | `FinalScope`/reader symbols undefined (compile failure) | Same command exit 0; 1,105/1,105 focused tests passed | Exact, changed candidate, incomplete identity, reader purity | `gofmt`; focused suite exit 0 |
| 1.2 | `snapshot_test.go`, `compact_gate_test.go`, `receipt_test.go` | Unit | Baseline: 1,105/1,105 focused tests passed | Original scope-action contract referenced undefined `FinalScope` comparison | Same command exit 0; 1,105/1,105 passed | Sorted paths, exact identity, divergent scope, no lineage mutation | `gofmt`; focused suite exit 0 |
| 1.3 | `compact_store_test.go`, `remediation_test.go`, `review_invalidate_test.go`, `review_lifecycle_integration_test.go` | Unit | Baseline: 1,105/1,105 focused tests passed | Lifecycle CAS methods/states undefined (compile failure) | Same command exit 0; 1,105/1,105 passed | Failure, authorization, completion, stale competitor, restart/surface compatibility | `gofmt`; focused suite exit 0 |
| 1.4 | `review_binding_test.go`, `review_facade_test.go`, `review_invalidate_test.go`, `review_lifecycle_integration_test.go` | Unit | Baseline: 1,105/1,105 focused tests passed | Binding/facade helper symbols undefined (compile failure) | Same command exit 0; 1,105/1,105 passed | Exact/stale revisions; legacy bytes; each existing command surface excludes PR2 routes | `gofmt`; focused suite exit 0 |
| 2.1 | `receipt_test.go`, `snapshot_test.go`, `compact_gate_test.go` | Unit | Baseline: 1,105/1,105 focused tests passed | Same 1.1/1.2 contract RED failures | Same command exit 0; 1,105/1,105 passed | Receipt/snapshot/gate identity variants | `gofmt`; focused suite exit 0 |
| 2.2 | `compact_store_test.go` | Unit | Baseline: 1,105/1,105 focused tests passed | Same 1.3 lifecycle-method RED failure | Same command exit 0; 1,105/1,105 passed | CAS/replay/stale competitor and immutable budget | `gofmt`; focused suite exit 0 |
| 2.3 | `review_binding_test.go`, `remediation_test.go`, `review_facade_test.go` | Unit | Baseline: 1,105/1,105 focused tests passed | Binding/evidence/adapter helper symbols undefined (compile failure) | Same command exit 0; 1,105/1,105 passed | Legacy bytes, exact/stale CAS, generated evidence, inactive adapters | `gofmt`; focused suite exit 0 |

## Strict-TDD Test Summary

- Focused command: `go test ./internal/reviewtransaction ./internal/sddstatus ./internal/cli -count=1` — exit 0; 1,105 total / 1,105 passing tests.
- Broader command: `go test ./...` — exit 0; 5,076/5,076 passing test events across the repository.
- Layers: Unit 1,105; Integration 0; E2E 0. The added lifecycle-surface test is a unit-level CLI contract and does not execute PR2 routes.
- Approval tests: 3 corrective contract tests added after the original RED/GREEN cycle (`snapshot_test.go`, `review_invalidate_test.go`, `review_lifecycle_integration_test.go`); they capture existing dormant behavior and required no production change.
- Pure functions created: 5 (`FinalScopeForSnapshot`, `CompareFinalScope`, `ParseReviewBinding`, `CompareReviewBindingRevision`, `BuildRemediationEvidence`); `CompactGateReader.Read` and dormant facade probe are pure adapters.

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command | `go test ./internal/reviewtransaction ./internal/sddstatus ./internal/cli -count=1` — exit 0; 1,105/1,105 tests passed. |
| Broader Go verification | `go test ./...` — exit 0; 5,076/5,076 passing test events. |
| Runtime harness | N/A — PR1 deliberately exposes only pure dormant contracts and does not introduce an activated runtime boundary. |
| Rollback boundary | Revert the changed `internal/reviewtransaction`, `internal/sddstatus`, and `internal/cli` files together; no production route, persisted migration, or guidance activation was introduced. |

## Changed-Line Count

- PR1 authored implementation/test changes: 433 lines (427 additions, 6 deletions), including the new dormant lifecycle integration contract.
- Pre-existing planning artifacts excluded from that count: `openspec/changes/stabilize-review-lifecycle/` existed before apply; this progress file and task-checkbox updates are tracked separately as OpenSpec delivery metadata.
- PR1 remains below the 3,000-line slice stop threshold.

## Remaining PR2 Tasks

- [ ] 3.1 RED activation contract for `GateArchive`, native archive/remediation dispatch.
- [ ] 3.2 RED archive coordinator/projection and relocation failure cases.
- [ ] 3.3 RED one-lineage Git/bare-remote lifecycle flow.
- [ ] 4.1 Activate facade/CLI archive and remediation only in PR2.
- [ ] 4.2 Add archive coordinator, projection, bound archive gate, and status routing.
- [ ] 4.3 Add lifecycle guidance/assets and embedding tests.
- [ ] 4.4 Final PR2 verification, E2E, bounded review, and commit.

## PR1 Delivery Boundary

- [ ] Parent-owned ordinary bounded review and commit remain pending as task 2.4. They were not started or performed by this apply executor.

## Apply Result

| Field | Value |
|---|---|
| status | partial |
| artifacts | `tasks.md`; `apply-progress.md`; 16 exact implementation/test files listed above |
| next_recommended | Parent-owned PR1 delivery boundary: ordinary bounded review and commit after apply evidence acceptance; PR2 remains blocked. |
| risks | 433 authored changed lines remain below 3,000 but exceed the generic 400-line review guideline; follow the configured feature-branch chain. No PR2 route/status/guidance activation. |
| skill_resolution | paths-injected — `sdd-apply`, `go-testing`, `gentle-ai-chained-pr`, `work-unit-commits` |
