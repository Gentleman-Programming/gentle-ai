# Tasks: RDD Root Simplification — Wave 5 (Gate Cutover)

## Gate

HARD-GATED: Wave 5 chains after BOTH Wave 3 AND Wave 4 land on the tracker
branch (`feature/rdd-root-simplification`). `resolveGoverningAuthority`,
`CandidateIdentity` promotion, `ReceiptRef`, and capability admission are
Wave 3/4 deliverables absent at `d591f4cf`; no Wave 5 slice may start before
both merge. Verify both waves are on the tracker (sdd-attempt ledger or
`git log feature/rdd-root-simplification` for wave3/wave4 slice merges)
before opening Wave 5 PR0.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | S1 ~650, S2 ~350, S3 ~700, S4 ~900, S5 ~800, S6 ~500, S7 ~600 (total ~4500) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR0 → S1 → S2 → S3 → S4 → S5 → S6 → S7 |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |
| Per-slice PR budget (session override) | ≤1000 authored lines/slice |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | PR | Focused test | Harness | Rollback |
|---|---|---|---|---|---|
| PR0 | Land W5 SDD artifacts; confirm W3+W4 tracker gate | tracker base | N/A (docs) | N/A — SDD artifacts only | Revert `openspec/changes/rdd-root-simplification-wave5/**` |
| S1 | Characterization corpus (legacy funnel, invalidation verb, decline, pre-PR delta rows) + 35-cell matrix harness | PR0-base | `go test ./internal/reviewtransaction/... -run Characterization` | N/A — golden corpus, no runtime scenario yet | Revert characterization test files + harness generator |
| S2 | Kill switch consulted once before any authority read + per-gate disabled/double-eval goldens | S1-base | `go test ./internal/cli/... -run Disabled` | 5-gate disabled-fixture double-eval bench | Revert single-call ordering; restore two late reads |
| S3 | `NativeGateEvaluation` additive `Relation`/`Next`; `gateVerdict` totality; every denial names a next step | S2-base | `go test ./internal/reviewtransaction/... -run GateVerdict` | 5-gate deny-fixture bench | Revert additive fields + `gateVerdict`; composite literals stay keyed |
| S4 | `projectLegacyAuthority`; legacy evaluated through algebra; receipt precedence; byte-identity | S3-base | `go test ./internal/reviewtransaction/... -run ProjectLegacyAuthority` | 5-gate byte-hash-before/after bench | Revert `legacy_projection.go`; `resolveGoverningAuthority` legacy cell reverts to byte-identical branch |
| S5 | Pre-PR chain composition deletion; pinned explained divergences | S4-base | `go test ./internal/reviewtransaction/... ./internal/cli/... -run PrePRComposition` | black-box denial-names-next-step bench journey | Revert `compact_chain.go` deletion from git history |
| S6 | Decline downgrade to ordinary unmanaged; read-only parser retained | S5-base | `go test ./internal/reviewtransaction/... -run CandidateDecline` | declined-candidate bench journey | Revert `candidate_decline.go` resolver/writer deletion |
| S7 | Invalidation verb deletion, `StateInvalidated` parse-only (LANDS LAST — only destructive step) | S6-base | `go test ./internal/reviewtransaction/... -run Invalidation` | full 35-cell matrix golden re-run | Restore `compact_approved_invalidation.go` from git history |

## Gate Regression Test Index (#2222/#2239 supersession evidence)

One named test per gate × {disabled, deny, allow} branch (15 tests, S2–S4),
plus switch-off double-eval byte-equivalence (5 tests, S2) and pre-PR
composition-specific corroboration (S5):

- Disabled (S2, #2222): `TestPostApplyGate_Disabled_ReportsUnmanagedBeforeAuthorityRead`, `TestPreCommitGate_Disabled_ReportsUnmanagedBeforeAuthorityRead`, `TestPrePushGate_Disabled_ReportsUnmanagedBeforeAuthorityRead`, `TestPrePRGate_Disabled_ReportsUnmanagedBeforeAuthorityRead`, `TestReleaseGate_Disabled_ReportsUnmanagedBeforeAuthorityRead`
- Double-eval byte-equivalence (S2): `TestDisabledGateOutput_DoubleEval_ByteEquivalent_PostApply`, `TestDisabledGateOutput_DoubleEval_ByteEquivalent_PreCommit`, `TestDisabledGateOutput_DoubleEval_ByteEquivalent_PrePush`, `TestDisabledGateOutput_DoubleEval_ByteEquivalent_PrePR`, `TestDisabledGateOutput_DoubleEval_ByteEquivalent_Release`
- Deny (S3): `TestPostApplyGate_Deny_ChangedRelationCarriesNextStep`, `TestPreCommitGate_Deny_ChangedRelationCarriesNextStep`, `TestPrePushGate_Deny_ChangedRelationCarriesNextStep`, `TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition`, `TestReleaseGate_Deny_ChangedRelationCarriesNextStep`
- Allow (S4): `TestPostApplyGate_Allow_ExactReceiptGovernsDelivery`, `TestPreCommitGate_Allow_ExactReceiptGovernsDelivery`, `TestPrePushGate_Allow_ExactReceiptGovernsDelivery`, `TestPrePRGate_Allow_ExactReceiptGovernsDelivery`, `TestReleaseGate_Allow_ExactReceiptGovernsDelivery`
- #2239 (S5): `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved`

## Phase 1 (PR0): SDD Artifacts

- [ ] 1.1 Land `openspec/changes/rdd-root-simplification-wave5/{proposal,specs,design,tasks}.md` (already written).
- [ ] 1.2 Confirm Gate: verify Wave 3 AND Wave 4 have landed on `feature/rdd-root-simplification` before opening any Wave 5 slice PR.
- [ ] 1.3 Archive Wave 4 (`openspec/changes/rdd-root-simplification-wave4/**` → `openspec/specs/`) when its turn comes, mirroring prior wave pattern.

## Phase 2 (S1): Characterization Corpus + Gate-Boundary Matrix Harness (zero behavior change)

- [ ] 2.1 RED: `TestLegacyFunnelCharacterization_RunFacadeLegacyValidateNegotiated` — Wave-1 golden covering-array pattern (`-update`) pinning `runFacadeLegacyValidateNegotiated`'s observable contract (currently zero test references).
- [ ] 2.2 RED: `TestInvalidationVerbCharacterization_InvalidateApprovedCompactAuthority` — pins `review_facade.go:1371`'s writer-lock + rewrite + `os.Remove` behavior before deletion.
- [ ] 2.3 RED: `TestPrePRChainCompositionRemovalDelta` — DELTA rows layered onto existing `compact_chain_test.go`'s 25 test funcs, isolating exactly the rows S5 will delete.
- [ ] 2.4 RED: `TestCandidateDeclineCharacterization_ResolveCandidateDeclineForGate` — characterizes `ResolveCandidateDeclineForGate` + `RecordCandidateDecline` writer before S6 removal (spec requires characterization to precede `candidate_decline.go` removal, not just `compact_chain.go`).
- [ ] 2.5 GREEN: 2.1–2.4 pass against current (pre-cutover) code — zero behavior change.
- [ ] 2.6 Build `testdata/gate-boundary-matrix.golden` generator: 5 gates × 7 relations = 35 rows `{gate, relation, verdict, next_step, explained, reason}`, generated from the algebra (harness only — not yet wired to production gate output; wiring lands incrementally in S2–S7, full run in S6/S7).
- [ ] 2.7 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update`; refusal-resolution notes (none pending this slice).

## Phase 3 (S2): Kill Switch Consulted Once + Per-Gate Disabled Goldens

- [ ] 3.1 RED: `TestKillSwitchOrdering_SingleCallBeforeAuthorityRead` — one `reviewDeliveryDisposition(ctx, root, false)` call immediately after flag/contract resolution, before `discoverCompactFacadeGateReview` or any authority read; fails against current two-late-reads shape (`review_facade.go:2905`, `:2967`).
- [ ] 3.2 RED per-gate disabled branch (5 named tests, #2222 evidence — see index above): kill switch off + ambiguous/corrupted authority-store fixture ⇒ ordinary unmanaged delivery, `reason_code: reviews_disabled`, no discovery kind, underlying authority error never surfaces.
- [ ] 3.3 RED switch-off byte-equivalence via same-fixture double-eval (5 named tests, see index above): evaluate the same fixture twice while switch is OFF, assert byte-identical serialized `NativeGateEvaluation` output across both evaluations (idempotence proof, zero mutation on repeat).
- [ ] 3.4 Implement single-call kill-switch ordering; remove the two late reads (`review_facade.go:2905`, `:2967`); wire `emitDisabledUnmanagedDelivery`.
- [ ] 3.5 GREEN: 3.1–3.3 pass (11 named tests this slice).
- [ ] 3.6 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update` for removed late-read call sites; refusal-resolution notes (none pending).

## Phase 4 (S3): NativeGateEvaluation Additive Relation/Next + Executable Next Step Per Denial

- [ ] 4.1 RED: `TestGateVerdict_TotalFunction_35Cells` — table-driven totality test over `gateVerdict(gate, relation)`; every one of the 5×7=35 pairings resolves, no unhandled case.
- [ ] 4.2 RED per-gate deny branch (5 named tests — see index above): `changed` relation ⇒ denial carries a typed transition; `unknown` relation ⇒ stop + reason_code (never a bare denial).
- [ ] 4.3 Add `Relation CandidateRelation` and `Next *GateNextStep{Transition, ReasonCode}` fields to `NativeGateEvaluation` (`gate.go:109`); verify all 47 non-test + test composite literals stay keyed (additive-only, compile-clean).
- [ ] 4.4 Implement `gateVerdict(gate GateKind, relation CandidateRelation) (GateResult, GateNextStep)` total function.
- [ ] 4.5 GREEN: 4.1, 4.2 pass (6 named tests this slice).
- [ ] 4.6 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update` for `gateVerdict`/`GateNextStep` exports; refusal-resolution notes (none pending).

## Phase 5 (S4): projectLegacyAuthority + Legacy Evaluated Through Algebra + Byte-Identity

- [ ] 5.1 RED: `TestProjectLegacyAuthority_Purity` — `projectLegacyAuthority(chain, artifacts)` is a pure read-only function of on-disk bytes; asserts zero writes/locks.
- [ ] 5.2 RED per-gate allow branch (5 named tests — see index above): covers both v3-present and legacy-only-present-via-projection cases reaching the same `exact`-relation allow.
- [ ] 5.3 RED: `TestLegacyAuthorityAlone_DeniesNewLineageCandidate` — unconditional receipt precedence; legacy-only authority never authorizes a new-lineage candidate.
- [ ] 5.4 RED: `TestLegacyReceiptBytes_ByteIdenticalAcrossAllFiveGates` — hash `review-state.json` + `review-receipt.json` before/after a full validate at each of the 5 gates; zero diff.
- [ ] 5.5 RED (regression, Migration item 4): `TestInFlightCorrection_PreCutoverFinalizes_ReceiptValidatesViaNewPath` — correction opened pre-cutover finalizes under the prior lifecycle; its receipt then validates through the new read-only path.
- [ ] 5.6 Create `internal/reviewtransaction/legacy_projection.go`: `projectLegacyAuthority(chain ValidatedChain, artifacts facadeArtifacts) (CandidateIdentity, ReceiptRef, error)`; delete `runFacadeLegacyValidateNegotiated` re-entry from the funnel.
- [ ] 5.7 Wire `resolveGoverningAuthority`'s "new absent, legacy present" cell to `projectLegacyAuthority` + `relateCandidates` (replacing the byte-identical legacy path).
- [ ] 5.8 GREEN: 5.1–5.5 pass (8 named tests this slice).
- [ ] 5.9 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update` for `legacy_projection.go` exports (remove `runFacadeLegacyValidateNegotiated` baseline entry if now unreachable); refusal-resolution notes (none pending).

## Phase 6 (S5): Pre-PR Chain Composition Deletion

- [ ] 6.1 RED: `TestPrePRComposition_ZeroCallers` — AST/call-graph guard: no gate calls `EvaluateCompactPrePRChain` to authorize delivery.
- [ ] 6.2 RED: `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved` — #2239 corroborating test: composition function no longer exists in the call graph, proven by call-absence.
- [ ] 6.3 RED: `TestPrePRDivergence_CompatibleBaseAdvanceExplained` and `TestPrePRDivergence_ChangedExplained` — pre-PR's `compatible_base_advance` and `changed` cells pinned as named, explained divergences (`compact_gate.go:91-102` boundary-proof reason), never silent differences.
- [ ] 6.4 Delete `compact_chain.go` (`EvaluateCompactPrePRChain`, `compactPrePRChainProof`, helpers); delete the DELTA-marked rows from 2.3's characterization corpus whose behavior is intentionally gone, keep surviving rows.
- [ ] 6.5 GREEN: 6.1–6.3 pass (4 named tests this slice; 4.2's `TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition` now exercises the real deleted-composition path).
- [ ] 6.6 Run the full 35-cell gate-boundary-matrix golden (2.6's harness, now wired through S2–S5): zero unexplained divergences, two explained divergence cells for pre-PR.
- [ ] 6.7 Bench journey: black-box "denial names a runnable next step" journey per `rdd-defect-workflow` (`bench/journeys_wave5.go`).
- [ ] 6.8 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update` for deleted `compact_chain.go` symbols; refusal-resolution notes (none pending).

## Phase 7 (S6): Decline Downgrade to Ordinary Unmanaged

- [ ] 7.1 RED: `TestCandidateDecline_ZeroCallers` — AST/call-graph guard: no code path constructs delivery authorization from a decline record.
- [ ] 7.2 RED: `TestCandidateDecline_UnmanagedDelivery_ByteIdenticalToDisabled` — declined candidate reaches ordinary unmanaged delivery, output byte-identical to the kill-switch-off golden (3.3's fixtures), no receipt-like record created or read as authority.
- [ ] 7.3 Delete `ResolveCandidateDeclineForGate`, funnel branch (`review_facade.go:2941-2945`), `emitCandidateDeclinedUnmanagedDelivery`, `RecordCandidateDecline` writer (only non-test caller: `review_facade.go:1606`); keep `parseCandidateDeclineAuthorization` read-only.
- [ ] 7.4 GREEN: 7.1, 7.2 pass (2 named tests this slice).
- [ ] 7.5 Bench journey: declined candidate reaches ordinary unmanaged delivery (`bench/journeys_wave5.go`, extends 6.7).
- [ ] 7.6 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update` for removed decline-resolver/writer symbols (parser stays baselined read-only); refusal-resolution notes (none pending).

## Phase 8 (S7): Invalidation Verb Deletion (lands LAST — only destructive authority step)

- [ ] 8.1 RED: `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates` — a receipt that would have been `os.Remove`'d under the pre-cutover writer stays present on disk post-cutover; the gate denies with a derived mismatch relation instead.
- [ ] 8.2 RED: `TestPreCutoverInvalidatedRecordsStayReadable` — `StateInvalidated`/`InvalidationEvidence` parse without rewrite.
- [ ] 8.3 RED: `TestNoGateWritesAuthority_CallAbsenceGuard` — AST/guard test: no gate code path calls `acquireStoreLock`, `writeAtomic`, or `os.Remove` (proven by call-absence, not a passing green path, per success criterion 1).
- [ ] 8.4 Delete `compact_approved_invalidation.go` (`InvalidateApprovedCompactAuthority`, `CompactApprovedInvalidationRequest`, `invalidateApproved`, `compactInvalidationTarget*`, `compactInvalidationDenialBound`) and the `review invalidate` compact branch; `invalidated` becomes derived: `relation ∈ {changed, unrelated} ⇒ GateInvalidated`. Legacy-v1 `review invalidate` operator branch retains its write (Wave 7 deletes it).
- [ ] 8.5 Update `internal/sddstatus/runtime_ledger_self_remediation_test.go`: drop the invalidation-verb caller (its only test caller).
- [ ] 8.6 GREEN: 8.1–8.3 pass (3 named tests this slice).
- [ ] 8.7 Re-run the full 35-cell gate-boundary-matrix golden (6.6) post-invalidation-derivation: zero unexplained divergences.
- [ ] 8.8 `ReceiptPath()` reader audit (audit-gated ratification): sweep in-repo + bundled Pi assets for readers depending on file-absence as the invalidation signal; migrate findings to `review validate`; add an rc release-notes line about receipt-file persistence under derived invalidation.
- [ ] 8.9 Close #2222/#2239 as superseded: cross-reference the 15 named per-gate tests (S2 disabled + S3 deny + S4 allow) plus S5's `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved` as supersession evidence in the PR description / issue comments.
- [ ] 8.10 Full verification: `go test ./... -count=1`; bench module; `scripts/deadcode-ratchet.sh --update` for deleted `compact_approved_invalidation.go` symbols; refusal-resolution notes (all four ratified assumptions + the audit-gated item confirmed resolved, none pending).
