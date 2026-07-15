# Design: Stabilize Review Lifecycle

## Technical Approach

Extend compact receipt/state and SDD binding via an archive coordinator; gates stay read-only. Authority, identity, hooks, risk, telemetry, and provider cleanup stay excluded.

## Architecture Decisions

| Option | Tradeoff | Decision |
|---|---|---|
| General authority | Broad | Reject; retain `CompactStore`/`ReviewBinding`. |
| Archive in status | Hidden mutation | Reject; `sddarchive.Service` mutates. |
| Gate recovery | Validation mutation | Reject; explicit CAS only. |
| One PR | Over budget | Two-slice feature chain. |

## Contracts and Flow

`CompactReceipt` v3 adds `FinalScope{CorrectionBaseTree,CandidateTree,PathsDigest,IntendedUntrackedProof,LedgerIDs,FixDeltaHash,EvidenceHash}`. `GateDenial` returns identities/diffs, `scope-changed`, and `MaintainerAction`.

`CompactState` adds `verify_failed`, `verify_remediating`, event-bearing CAS operations `RecordVerifyFailure`, `AuthorizeVerifyRemediation`, `CompleteVerifyRemediation`, and `EscalateValidatorFailure`:

    approved → verify_failed → verify_remediating → validating → approved
                         validator/exhaustion failure ↘ escalated

Lineage, genesis, policy, findings, lines, and budget are immutable. Replay is idempotent; competitors fail. Legacy empties never rewrite bytes.

Archive ordering is atomic and fail-closed:

    CLI capture → sync specs → refresh projection → GateArchive → prepare CAS
        → same-device rename → byte/mode verification → final CAS

`reconcile` retries/finalizes only an exact destination. EXDEV, ambiguity, reappearance, or drift fails without deletion.

## Dormant Activation Boundary

PR1 adds only schemas, states/events/CAS methods, pure helpers, and compatibility coverage. Production routes MUST NOT import or activate `GateArchive`, `archive-sdd`, remediation, status changes, or guidance. Existing CLI/gate/status behavior and receipt/binding bytes remain identical.

PR2 atomically activates native acceptance, facade/CLI, coordinator/projection, binding/archive routing, status, guidance/assets, and E2E.

## Testing and Threat Matrix

| Boundary | Applicability | Planned RED safe/failure contract |
|---|---|---|
| Documentation-like paths | N/A — classification unchanged | None. |
| Git repository selection | Applicable | Relative/absolute selected roots allow; foreign/symlink roots deny. |
| Commit state | Applicable | Exact stage allows; `commit -a` drift and empty index deny. |
| Push state | Applicable | Exact tracking/first-push/refspec allows; ambiguity denies. |
| PR commands | N/A — pre-PR reads refs only | No command execution. |

## Traceable Forecast

| PR | File / non-overlapping operation | Lines |
|---|---|---:|
| PR1 | `internal/reviewtransaction/compact.go` — receipt/state/events | 180–220 |
| PR1 | `internal/reviewtransaction/compact_store.go` — dormant CAS | 180–220 |
| PR1 | `internal/reviewtransaction/compact_gate.go` — reader/final-scope diagnostics | 90–110 |
| PR2 | `internal/reviewtransaction/compact_gate.go` — archive evaluation/recheck | 40–50 |
| PR1 | `internal/reviewtransaction/receipt.go` — v3 validation/denials | 35–40 |
| PR2 | `internal/reviewtransaction/receipt.go` — archive kind | 10–15 |
| PR2 | `internal/reviewtransaction/native_request.go` — archive acceptance | 25–35 |
| PR1 | `internal/reviewtransaction/snapshot.go` — scope diff | 35–45 |
| PR1 | `internal/cli/review_facade.go` — pure adapters | 55–65 |
| PR2 | `internal/cli/review_facade.go` — remediation/archive dispatch | 105–125 |
| PR2 | `internal/cli/review.go` — archive actions | 30–40 |
| PR1 | `internal/sddstatus/review_binding.go` — v2 schema/Load/CAS | 90–110 |
| PR2 | `internal/sddstatus/review_binding.go` — projection/root routing | 40–50 |
| PR2 | `internal/sddarchive/archive.go` — coordinator | 200–240 |
| PR2 | `internal/sddarchive/projection.go` — sync admission | 120–150 |
| PR2 | `internal/sddstatus/review_gate.go` — bound archive gate | 60–75 |
| PR2 | `internal/sddstatus/status.go` — state/action routing | 25–35 |
| PR1 | `internal/sddstatus/verification.go` — evidence helper | 40–50 |
| PR1 | `internal/reviewtransaction/compact_store_test.go` — CAS/restart | 240–280 |
| PR1 | `internal/reviewtransaction/compact_gate_test.go` — purity/final scope | 110–130 |
| PR2 | `internal/reviewtransaction/compact_gate_test.go` — archive TOCTOU | 40–50 |
| PR1 | `internal/reviewtransaction/receipt_test.go` — compatibility | 50–60 |
| PR1 | `internal/cli/review_facade_test.go` — adapter/unchanged routes | 70–80 |
| PR2 | `internal/cli/review_facade_test.go` — activated commands | 110–130 |
| PR1 | `internal/cli/review_invalidate_test.go` — dormant transitions | 50–60 |
| PR1 | `internal/sddstatus/review_binding_test.go` — v2 bytes/CAS | 90–105 |
| PR2 | `internal/sddstatus/review_binding_test.go` — archive projection/root | 60–75 |
| PR2 | `internal/sddarchive/archive_test.go` — crash/CAS | 280–330 |
| PR2 | `internal/sddarchive/projection_test.go` — sync admission | 150–180 |
| PR2 | `internal/sddstatus/bounded_review_test.go` — status routing | 110–130 |
| PR1 | `internal/sddstatus/remediation_test.go` — evidence/state | 70–90 |
| PR1 | `internal/cli/review_lifecycle_integration_test.go` — legacy fixture | 100–120 |
| PR2 | `internal/cli/review_lifecycle_integration_test.go` — Git flow | 250–300 |
| PR2 | `internal/assets/skills/_shared/review-ledger-contract.md` — guidance | 15–20 |
| PR2 | `internal/assets/skills/_shared/sdd-status-contract.md` — guidance | 20–30 |
| PR2 | `internal/assets/skills/sdd-archive/SKILL.md` — ordering | 30–40 |
| PR2 | `internal/components/sdd/bounded_review_contract_test.go` — assertions | 25–35 |
| PR2 | `internal/components/sdd/review_foundations_test.go` — parity | 15–20 |
| PR2 | `internal/assets/assets_test.go` — embedding | 20–30 |

PR1: **1,485–1,785**; PR2: **1,780–2,185**; total: **3,265–3,970** authored, non-overlapping, sub-3,000 lines.

## Chain Acceptance and Rollback

Draft tracker `feat/stabilize-review-lifecycle` targets `main`; `feat/stabilize-review-lifecycle-01-contracts` targets `feat/stabilize-review-lifecycle`; `feat/stabilize-review-lifecycle-02-activate` targets `feat/stabilize-review-lifecycle-01-contracts`. Merge tracker after both.

PR1 acceptance: tests pass; fixtures prove legacy byte and route/help/status compatibility; assertions prove activation unreachable. Rollback is PR1-only: no migration or production-written state exists.

PR2 acceptance: Go tests/vet and bare-remote E2E prove correction→restart→remediation→verify→archive→stage→commit→push→pre-PR with one lineage/budget. Rollback restores dormant PR1, disables activation, retains evidence, and fails closed. Revert PR2 first.
