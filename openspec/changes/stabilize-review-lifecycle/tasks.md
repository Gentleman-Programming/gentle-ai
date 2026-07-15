# Tasks: Stabilize Review Lifecycle

## Review Workload Forecast

| PR | Changed lines / risk | Branch → target | Work unit |
|---|---|---|---|
| Tracker | 0 / Low | `feat/stabilize-review-lifecycle` → `main` (draft) | no implementation |
| PR1 | 1,485–1,785 / High | `feat/stabilize-review-lifecycle-01-contracts` → tracker | dormant contracts |
| PR2 | 1,780–2,185 / High | `feat/stabilize-review-lifecycle-02-activate` → PR1 | atomic activation |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

Stop and re-plan if either authored slice exceeds 3,000 lines. Retarget/rebase polluted child diffs.

| Unit | Focused test / runtime | Rollback |
|---|---|---|
| PR1 commit: `feat(review): add dormant lifecycle contracts` | `go test ./internal/reviewtransaction ./internal/sddstatus ./internal/cli`; N/A—unreachable | Revert PR1; no production state |
| PR2 commit: `feat(review): activate archive lifecycle` | `go test ./... && go vet ./...`; `RUN_FULL_E2E=1 e2e/docker-test.sh` | Revert PR2 first; dormant PR1 fails closed |

## Phase 1: PR1 — RED, dormant only

- [x] 1.1 RED `compact_gate_test.go`, `receipt_test.go`, `compact_store_test.go`: v3 `FinalScope` agreement; unknown/incomplete denial; reader purity and zero lineage/budget. **Ledger R1–R2**.
- [x] 1.2 RED `snapshot_test.go`, `compact_gate_test.go`, `receipt_test.go`: exact snapshot/final-scope identities, sorted path delta, read-only gate evaluation, `scope-changed`, maintainer action, and no automatic lineage. **Ledger R4**.
- [x] 1.3 RED `compact_store_test.go`, `remediation_test.go`, `review_invalidate_test.go`, `review_lifecycle_integration_test.go`: same-lineage CAS/replay/restart contracts, stale competitor, immutable budget, escalation/remediation, and dormant CLI-surface compatibility. **Ledger R3**.
- [x] 1.4 RED `review_binding_test.go`, `review_facade_test.go`: legacy bytes, routes/help/status identical; archive/remediation unreachable.

## Phase 2: PR1 — GREEN/REFACTOR

- [x] 2.1 GREEN `compact.go`, `receipt.go`, `snapshot.go`, `compact_gate.go`: `FinalScope`, construction/validation, `GateDenial`, scope helpers, `CompactGateReader`; no production `GateArchive`. **R1,R2,R4**.
- [x] 2.2 GREEN `compact.go`, `compact_store.go`: states/events and CAS `RecordVerifyFailure`, `AuthorizeVerifyRemediation`, `CompleteVerifyRemediation`, `EscalateValidatorFailure`; legacy empties do not rewrite. **R3**.
- [x] 2.3 GREEN `review_binding.go`, `verification.go`, `review_facade.go`: dormant v2 Load/CAS, evidence helper, pure adapters; refactor/gofmt and rerun PR1 tests.
- [ ] 2.4 PR1 delivery boundary (parent-owned, not apply): ordinary bounded review and commit after apply evidence is accepted. Do not activate PR2 behavior.

## Phase 3: PR2 — RED, activation contract

- [ ] 3.1 RED `compact_gate_test.go`, `review_facade_test.go`: locked `GateArchive`, native expected-revision archive/remediation dispatch, no new lineage/budget. **R2,R5**.
- [ ] 3.2 RED `archive_test.go`, `projection_test.go`, binding/status tests: capture→prepare→rename→verify→CAS/reconcile; EXDEV, ambiguity, reappearance, drift deny without deletion. **R5**.
- [ ] 3.3 RED Git/bare-remote `review_lifecycle_integration_test.go`: one correction→restart→remediation→verify→archive→stage→commit→push→pre-PR lineage/budget; roots, stage, push threat cases. **R5**.

## Phase 4: PR2 — GREEN/REFACTOR

- [ ] 4.1 GREEN activate only now: `compact_gate.go`, `receipt.go`, `native_request.go`, `review_facade.go`, `review.go` (`archive-sdd capture|relocate|reconcile`; remediation modes). **R2–R5**.
- [ ] 4.2 GREEN `sddarchive/{archive,projection}.go`, `review_binding.go`, `review_gate.go`, `status.go`: same-device manifest/projection/root/status routing. **R5**.
- [ ] 4.3 GREEN assets `review-ledger-contract.md`, `sdd-status-contract.md`, `sdd-archive/SKILL.md`; embedding/component tests: supported flow and scope action only. **Assets R1–R2**.
- [ ] 4.4 Refactor/gofmt; final `go test ./...`, `go vet ./...`, E2E, bounded review, commit. Reuse receipt at commit/push/PR gates; no new review.
