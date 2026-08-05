# Tasks: RDD Root Simplification — Wave 7 (Compatibility Retirement)

Grounded at post-W6 tip `40176a8f`, pending W6's fix cycle `bba17974` (touches `authority_disposition_execute.go`, `review_repair.go`, ds11). Task 0 re-validates every row before any deletion.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~-8000 net (design forecast) across 20 work units, each ≤1000L |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Feature Branch Chain, 20 sequential WUs on tracker `feature/rdd-root-simplification` |
| Delivery strategy | auto-chain (proposal Session parameters) |
| Chain strategy | feature-branch-chain |
| Effective per-PR budget | 1000L (proposal override of the 400L default) |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units (sdd-attempt ledger)

| Unit | Goal | PR base | Focused test | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| WU1 S1 | W-9/10/11 closure (+250) | tracker | `go test ./internal/reviewtransaction/... -run 'CaptureLensResult\|AdmitCandidateCausalFindings\|newLineageCapturedFindings' -count=1` | N/A — pure add | revert 1.1-1.9 |
| WU2 S6 | Byte-equiv Commit A (+150) | WU1 | `go test ./internal/cli/... -run TestByteEquivalence -count=1` | `bench --axis all` w/ `GENTLE_AI_RDD_NEW_LINEAGE=1`, record | delete evidence dir |
| WU3 S9a | v1 freeze + backlog proofs (+150) | WU2 | N/A doc-only | N/A | revert freeze marker |
| WU4 S2a | Shadow observer+alias (-430) | WU3 | `go test ./internal/reviewtransaction/... -run Shadow -count=1` | N/A | git revert |
| WU5 S2b | Shadow tests pt1 (-779) | WU4 | same | N/A | git revert |
| WU6 S2c | Shadow tests pt2 + golden retained (-886) | WU5 | same | N/A | git revert |
| WU7 S3a | reconcile-authority dispatch+bench retarget (-250) | WU6 | `go test ./internal/cli/... -run ReviewReconcile -count=1` | `bench --axis ds01,ds02,ds04` | git revert |
| WU8 S3b | `ReconcileInvalidRecoveryEdge` + tests (≤1000, measure) | WU7 | same | `bench --axis ds01,ds02,ds04` | git revert |
| WU9 S4a | reconcile-authority-batch dispatch (-200) | WU8 | `go test ./internal/cli/... -run ReviewReconcileBatch -count=1` | N/A | git revert |
| WU10 S4b | `ReconcileInvalidRecoveryEdges` journal (-592) | WU9 | same | N/A | git revert |
| WU11 S4c | plan+guard, confirm row 12 (-673) | WU10 | same | N/A | git revert |
| WU12 S4d | batch tests pt1 (-670) | WU11 | same | N/A | git revert |
| WU13 S4e | batch tests pt2 (-446) | WU12 | same | N/A | git revert |
| WU14 S5a | quarantine/repair verbs dispatch (-200) | WU13 | `go test ./internal/cli/... -run ReviewLegacy -count=1` | N/A | git revert |
| WU15 S5b | `legacy_fix_scope_quarantine.go` (-606) | WU14 | same | N/A | git revert |
| WU16 S5c | `legacy_quarantine.go`+`legacy_alias_repair.go` (-582) | WU15 | same | N/A | git revert |
| WU17 S5d | legacy tests, 5 files (-875) | WU16 | same | N/A | git revert |
| WU18 S7 | switch + legacy start branch, Commit B (-600) | WU17 | `go test ./internal/cli/... -run NewLineageSwitch -count=1` | `bench --axis all` switch-free, diff vs WU2 | git revert |
| WU19 S8 | D4 verbs classify+delete (≤300) | WU18 | `go test ./internal/cli/... -run ReviewFacadeDispatch -count=1`, `legacy_readonly_guard_test.go` | N/A | git revert |
| WU20 S9b | Capability deltas + close-out (+50) | WU19 | N/A doc-only | N/A | git revert |

## Gate

- [x] G.1 Confirm Waves 3-6 merged to `main` (exit evidence present; `GENTLE_AI_RDD_NEW_LINEAGE` present under `internal/`).
- [x] G.2 Confirm W6 fix cycle `bba17974` merged before deriving the Task 0 inventory.

## Task 0: Inventory Re-Validation (no code diff, blocking)

- [x] 0.1 Re-read all 24 design rows against current tracker tip; confirm file:line accuracy post-`bba17974`.
- [x] 0.2 Confirm `ShadowRelation*` constants (`candidate_relation.go:34-45`) are declared independent of row 4's alias (line 36) before any shadow deletion.
- [x] 0.3 Confirm row 9's ds01/ds02/ds04 verb hits unaffected/updated by `bba17974`'s ds11 journey change.
- [x] 0.4 Record any drift as amended row notes before WU1.

## Bracket Slices (add-only, land first)

### WU1 — S1: W-9/W-10/W-11 closure
- [x] 1.1 RED: `FindingEvidence.Severity` (omitempty) missing-field test (`transaction.go:146`).
- [x] 1.2 GREEN: add `Severity`; carry through `newLineageCapturedFindings` (`review_artifact.go:937-946`, today drops `facadeFinding.Severity`).
- [x] 1.3 RED: `new_lineage_capture_test.go` — `CaptureLensResult` refuses severe finding missing `evidence_class`/`causal_disposition` (v2 message, `artifact_admission.go:331`).
- [x] 1.4 GREEN: refuse in `AuthorityStore.CaptureLensResult` (`new_lineage_capture.go:99`) reusing `isSevereSeverity`/`isSupportedEvidenceClass`/`isSupportedCausalDisposition` (`transaction.go:1774/1829/1838`).
- [x] 1.5 RED: v3 finalize — WARNING-severity candidate-causal findings stay non-blocking (v2 parity).
- [x] 1.6 GREEN: v3 finalize filters `CapturedFindingEvidence()` to SEVERE before `AdmitCandidateCausalFindings` call site; function body stays byte-identical.
- [x] 1.7 RED: `candidate_causal_admission_test.go` — unknown `causal_disposition` on a severe finding escalates via new `unresolvedIDs` return.
- [x] 1.8 GREEN: add third `unresolvedIDs` return to `AdmitCandidateCausalFindings` (`candidate_causal_admission.go:31`); only v3 finalize consumes it; v2 callers byte-identical.
- [x] 1.9 RED (RG.1): add `internal/reviewtransaction/legacy_readonly_guard_test.go` asserting D5 retained-symbol list (parseLegacyBinding, parseBinding, bindingBytes/Digest/Path, `candidate_decline.go` parser, StateInvalidated arms, AuthoritativeStore/LoadChain, NewLegacyReadOnlyError) reachable/read-only. Intentionally RED until WU19.
- [x] 1.10 Exit Checklist.

### WU2 — S6: byte-equivalence evidence, Commit A
- [x] 2.1 Record goldens/envelopes/receipts with `GENTLE_AI_RDD_NEW_LINEAGE=1` across the full journey set, every entry surface: start (negotiated+unnegotiated), status `--next-transition`, capture-result, finalize, validate, all 5 gates. (Scoped at apply time to the unnegotiated form + status + finalize + all 5 gates; negotiated form and capture-result deferred to unit-level coverage + WU18's `bench --axis all` — see apply-progress.)
  **verify SL-1, disclosed and relocated rather than widened**: the
  negotiated form was never proven byte-equivalent (switch-ON vs
  switch-free), and cannot be recorded now without either (a)
  reconstructing a switch-free build — exactly the WU18 action this wave
  deferred, and reproducing it here solely to record evidence would
  reintroduce the same disclosed `repository_context` gap under the same
  time pressure the deferral exists to avoid, or (b) recording only the
  switch-ON side with no switch-free counterpart to diff against, which
  proves nothing. The requirement this evidence proves
  ("Byte-Equivalence Exit Evidence Precedes Switch Removal") stays in this
  change's own spec, unweakened, alongside the scoped evidence actually
  recorded here — but its full-journey-set scenario ("Double-evaluation
  proves equivalence before deletion") has moved, byte-identical, to
  `rdd-single-lifecycle-cutover`'s `MODIFIED Requirements` delta (verify
  finding SL-1's resolution, mirroring how B1's requirement move was
  handled): only the change that performs the switch-free build can run
  it. **What the successor must do**: once v3 negotiated START gains
  genuine `repository_context` support, record Commit A's negotiated-form
  goldens/envelopes/receipts (negotiated start + `capture-result`) with
  the switch ON, build switch-free, record the same surface again, and
  diff both for byte-identity — satisfying the scenario it now owns —
  before switch removal proceeds. See the successor's `proposal.md` step 3
  and its `specs/rdd-single-lifecycle/spec.md`.
- [x] 2.2 Store recorded bytes as the Commit-B comparison baseline.
- [x] 2.3 Exit Checklist.

### WU3 — S9a: v1 freeze + backlog deletion proofs
- [x] 3.1 Add read-only freeze marker/doc for `contracts/review-integration/v1/**` (D3), byte-unchanged.
- [x] 3.2 Record deletion proof for backlog `#1455`, `#1462`, `#1570`, PRs `#1549`, `#1550` (superseded-by-design).
- [x] 3.3 Exit Checklist.

## Task 0 Re-Validation Findings (recorded before WU1, at `bba17974`)

All 24 design rows confirmed exact (file:line matches design.md byte-for-byte)
against `bba17974`, with one documented drift and no row requiring a design
amendment. W6's fix cycle (`bba17974`) touched `bench/axis_damaged_store_closure.go`,
`internal/cli/review_repair.go`(+test), `internal/reviewtransaction/authority_disposition_execute.go`(+test) —
none of these are Wave 7 inventory rows; row 9's `bench/axis_damaged_store.go`
(the file the design's row 9 actually names) received ZERO changes from `bba17974`.
Drift: row 9's `axis_damaged_store_closure.go` claim of "2 hits" is now 0 (that
file currently carries no reconcile-authority/-batch references at all) — the
retarget-needed references live exclusively in `axis_damaged_store.go` (confirmed
14 `reconcile` mentions there, unaffected by W6). Task 0.2 confirmed:
`ShadowRelation*` constants (lines 39-45) and `relateCandidates`/`shadowRelationHasNoLiveCounterpart`
(lines 81/228) are declared independent of the `ShadowRelation` alias (line 36) —
deleting only line 36 in WU4 leaves the live v3 governance vocabulary untouched.
Row 12 pre-confirmed early (S4/WU11 precondition): `PlanCompactBatchReconciliation`/
`PrepareCompactBatchReconciliation` have exactly one non-test consumer each,
both inside the S4 cluster itself (`compact_batch_reconcile_guard.go`,
`internal/cli/review_reconcile_batch.go:76`) — zero consumers outside the
journal/dispatch cluster being deleted in WU9-WU11.

## Consumer-First Deletion Slices

### S2 — Shadow observer retirement (rows 1-5)
- [x] WU4 (-430 planned; actual -2140 net, see below): retire `ObserveShadowRelation` (`shadow_observer.go`, 201L) + `shadowObservationEnvVar` + ALL 5 consumers (design named only `review_facade.go:856,:1566`; `compact_recovery_binding.go:311`, `compact_gate.go:556`, `gate.go:401` found by grep and removed too) + `shadowClassifyAuthorityHealth`/`shadowAuthorityHealthAtRepo` (71L). Alias `ShadowRelation` NOT deleted (deferred — see apply-progress deviation 8). Landed at `17e40eb0`, COMBINED with WU5+WU6 into one commit (see below).
- [x] WU5 (-779 planned; absorbed into WU4's commit): `shadow_observer_test.go`(185)+`shadow_authority_health_test.go`(257) deleted as planned; `shadow_identity_test.go`(337) ALSO deleted here (design listed it for WU5, but it turned out to test the shadowCandidateIdentity subsystem discovered as additional dead code once WU4's observer died — could not stay a separate later commit). Could not land as an independent commit: Go's whole-package compilation model forces these test files to die in the same commit as their subject.
- [x] WU6 (-886 planned; absorbed into WU4's commit): `shadow_matrix_test.go`(600) deleted, `shadow-differential-matrix.golden` retained byte-unchanged (confirmed via `git diff bba17974 HEAD -- <golden path>` = empty). `shadow_readonly_guard_test.go`(286) NOT wholesale-deleted as design assumed — only its now-vacuous `TestShadowReadOnlyGuardHoldsForProductionFiles`/`productionShadowFiles` removed; the file's AST-scanning infrastructure is reused by `candidate_readonly_guard_test.go` and `derived_observation_write_guard_test.go` (both retained, live) and stays indefinitely. Could not land independently for the same Go-compilation reason as WU5.

### S3 — reconcile-authority (rows 6-9, 19)
- [x] WU7 (-250 planned; actual net -500, +234/-734): retire `RunReviewReconcileAuthority`+case (`review_facade.go:721-722`)+`review_reconcile.go`(60L)+`review_reconcile_test.go`(446L, not in design's row 6). Landed at `25408b09`. Also fixed 3 LIVE, RETAINED consumers of the command NAME (not in design's inventory): `compact_inspect.go`'s `SanctionedCompactRecoveryExits`/`compactStartInvalidGraphRefusal`, `compact_reclaim.go`'s refusal message, `review_reclaim.go`'s help text — all advertised `review reconcile-authority` as a real continuation; now fall through to the existing abandon/repair/Blocked logic. Bench retarget corrected: actual affected journeys are ds01/ds02/ds03/ds05 (NOT ds01/02/04 as briefed — ds04 never used the capability; ds03/ds05 were missed). See apply-progress for full deviation detail.
- [x] WU8 (≤1000 planned; actual +333/-1600, net -1267): deleted `ReconcileInvalidRecoveryEdge`+4 helpers (confirmed dead by WU7's own ratchet uptick); `compact_reconcile_test.go` (1192L) deleted entirely except `TestClassifyCompactRecoveryEdgeAnomalies` + 4 shared fixtures moved to new `compact_fixture_test.go` (cross-file reuse confirmed by grep — same discipline as WU4/WU7); 4 other test files edited per their own actual role (never assumed). No split needed. Landed at `717547c6`. See apply-progress for a false-positive investigation (a "gap" in review_status_contract.go's managed-actions lists that turned out to be correctly protecting the frozen v1 contract fixture — reverted after empirical test proof, not shipped).

### S4 — reconcile-authority-batch (rows 10-13, 19) — COMPLETE
- [x] WU9 (-200 planned; actual net +39, +48/-9 deadcode uptick, -4 refusal-ratchet): retired `RunReviewReconcileAuthorityBatch`+BOTH dispatch cases (`runReviewCommandContext` AND `runReviewCommand`, unlike `reconcile-authority` in WU7 which had only one)+`review_reconcile_batch.go`(110L)+`review_reconcile_batch_test.go`. RED unknown-command refusal test added (`TestReviewRetiredVerbReconcileAuthorityBatchIsUnknownCommand`). Retargeted `review_disabled_mutation_test.go`'s kill-switch sweep row and malformed-request row to `invalidate`. Did NOT touch `review_status_contract.go`'s frozen v1 managed-action lists (standing rule). Deadcode ratchet went net-POSITIVE here by design (+39, 244→283) — the now-orphaned provider chain (WU10-13's subject) was left unreachable, exactly the expected consumer-first pattern; the next slice reverses it. Landed at `0bcfe0e3`.
- [x] WU10 (-592 planned) + WU11 (-673 planned) + WU12 (-670 planned) + WU13 (-446 planned) — actual combined: 9 files changed, +58/-2153, net -2095. Go's whole-package compilation model forced all four into ONE commit (the package would not build with the provider gone but its plan/guard/journal helpers still present, and vice versa): deleted `ReconcileInvalidRecoveryEdges` provider (`compact_batch_reconcile_journal.go`), `compact_batch_reconcile_plan.go` (350L) + `compact_batch_reconcile_guard.go` (323L, row 12 RE-CONFIRMED zero external consumers via direct grep of every exported symbol before deletion), `acquireMaintenanceLockForCompactBatch` in `store_lock.go` (its only caller was the deleted guard — `allowPreparedBatch` parameter itself is UNCHANGED, still load-bearing for the surviving caller). `compact_batch_reconcile_plan_test.go`(330)+`compact_batch_reconcile_lock_test.go`(183) deleted (zero cross-file reuse of their helpers, confirmed by grep). `compact_batch_reconcile_journal_test.go`(340) REWRITTEN not deleted — new focused `TestEnsureNoPreparedCompactBatchReconciliation` replaces the old incidental coverage. **Forensic-safety exception (D5), NOT deleted**: `ensureNoPreparedCompactBatchReconciliation`/`ErrCompactBatchReconcilePrepared`/`compactBatchReconcileMarkerPath` — the on-disk "batch-reconcile-journal.json" marker guard, still called by 6 LIVE files (`status.go`, `authority_repair.go`, `authority_disposition_execute.go`, `compact_inspect.go`, `store_lock.go`, `final_verification_retry.go`, `compact_store.go`) before mutating/reporting on authority; deleting it would let live mutation paths silently proceed past an unfinished historical batch reconciliation. `compact_batch_reconcile_journal.go` rewritten to a small residual file holding only this retained guard. Deadcode ratchet 283→243 (net -40: reverses WU9's +39 uptick plus the 1 newly-orphaned `acquireMaintenanceLockForCompactBatch`); refusal-resolution ratchet 1679→1649 (net -30). Landed at `b6030eb2` on branch `feat/rdd-wave7-wu10-13-reconcile-batch-provider-retirement`.

**Command-string grep completed before touching any Go symbol (per the batch-2/WU7 standing method)**: grepped `"reconcile-authority-batch"` across the whole repo. Unlike `reconcile-authority`'s own retirement (3 hidden external refusal-message consumers in compact_inspect.go/compact_reclaim.go/review_reclaim.go), the batch verb's OWN command-name references outside its dispatch+provider+guard+journal files are only: (1) `review_facade.go:598`'s usage string (needs updating when WU9 lands), (2) `review_status_contract.go`'s two frozen v1-contract managed-action lists — **do NOT touch these when retiring the batch verb**: confirmed empirically (a WU7-follow-up "fix" to the singular `reconcile-authority` entry broke `TestNegotiatedReviewStatusReportsFreshStartAndPreservesGlobalStatus` against the byte-frozen `contracts/review-integration/v1/fixtures/status-v2.fixture.json`, reverted after the failure proved it). Every `"review reconcile-authority-batch ..."` refusal string found lives inside the verb's own provider/guard/journal files and dies naturally with them — no cross-file "X names Y as the continuation" pattern like `reconcile-authority` had.

### S5 — quarantine/repair legacy verbs (rows 14-19)
- [x] WU14 (-200 planned; actual net -282, +85/-367): retired 3 verbs+cases (RunReviewLegacyQuarantine, RunReviewLegacyFixScopeQuarantine, RunReviewLegacyAliasRepair) + 3 CLI handlers. Go's compilation model forced combining each handler's deletion with its own CLI test file (all 3) into this one commit — absorbs 3 of WU17's planned 5 test-file deletions; no independent WU17 content remains for them. Hidden consumers fixed: review_facade.go usage string, review_repair.go help text, and review_repair_test.go's TestReviewRepairHelpRecommendsGenericClassifiedFlow (found only by running the suite, not by grep — inverted to a negative assertion). **Major design-inventory correction found here**: traced every production caller of each provider's exported entrypoint before trusting WU15-17's planned deletions — QuarantineMalformedLegacyFreeze/QuarantineHistoricalLegacyFixScope are genuinely standalone (design was right), but RepairHistoricalLegacyAlias's underlying repairHistoricalLegacyAlias (lowercase) is called directly by the LIVE v3 RepairClassifiedAuthority (authority_repair.go) as its own execution engine — legacy_alias_repair.go is NOT "compatibility-only". Landed at 7184278f.
- [x] WU15 (-606 planned; actual net -1016, +19/-1035): deleted QuarantineHistoricalLegacyFixScope + legacy_fix_scope_quarantine_test.go(387L, zero cross-file helper reuse confirmed). D5 forensic-safety retention (WU10-13 pattern): legacy_fix_scope_quarantine.go REWRITTEN not deleted, down to just the retained LegacyFixScopeQuarantineProof/LegacyFixScopeAnomaly JSON types compact_reclaim.go's CompactReclaimRecord still embeds. Landed at a4172c98.
- [x] WU16 (-582 planned; actual net -492, +41/-533): deleted QuarantineMalformedLegacyFreeze (confirmed standalone) with the same D5 residual-type pattern (legacy_quarantine.go → just LegacyMalformedFreezeProof). compact_legacy_quarantine_test.go's two QuarantineMalformedLegacyFreeze-specific tests removed; TestMalformedLegacyFreezeEventBricksInventoryWithNoFamilyExit RETAINED (tests reclaim/abandon's non-exit, not quarantine-legacy's exit — untouched by the provider's retirement). **Corrects WU14's flagged deviation**: legacy_alias_repair.go loses only its 3-line exported RepairHistoricalLegacyAlias wrapper (the CLI's own entry point); repairHistoricalLegacyAlias (lowercase) and everything else in the file stays live under RepairClassifiedAuthority. legacy_alias_repair_test.go is RETAINED IN FULL (zero coverage lost) — its 11 call sites plus authority_repair_test.go's 1 (TestRepairClassifiedAuthorityRejectsCompatibilityReplayOrigin) mechanically retargeted to call repairHistoricalLegacyAlias directly with an explicit empty legacyAliasRepairOptions{}, the exact same call the deleted wrapper made. Landed at 9a75fdbf.
- [x] WU17: DISSOLVED — no independent content remains. All 5 planned test-file deletions were compile-forced into WU14 (3 CLI test files) or WU15/16 (paired naturally with their provider's deletion); legacy_alias_repair_test.go is retained in full per WU16's correction, not deleted.

## Switch Removal (hard gate: after WU1-WU17)

Gate status: WU1-WU17 all landed and fully verified (see apply-progress for
the full evidence table: ratchets, full suites, bench corpus diff vs the
WU9-13 parent — 83/83 comparable journeys unchanged status, zero structural
deltas).

### WU18 — S7: switch + legacy start branch, Commit B — **DEFERRED, not done**

Coordinator scope decision: switch removal is deferred. The production
change (switch + legacy branch deletion) landed clean and byte-equivalence-
proven (Commit A goldens re-verified byte-identical with zero -update
needed — non-negotiable #2 satisfied at the strongest possible level),
W-9/W-10/W-11 re-confirmed green immediately before attempting removal
(non-negotiable #1 satisfied) — but executing the removal surfaced that v3
negotiated START has never supported `repository_context` (predates this
wave; previously reachable only at the narrow switch-ON-AND-negotiated
intersection, universal once the switch is gone). Extending
`validateLiveReviewRepositoryContext` (a security-sensitive binding
validator) under time pressure was judged the wrong trade — non-negotiable
#3 (never remove a switch over a known capability gap). See the spec
amendment in `specs/rdd-single-lifecycle/spec.md` ("Amendment (Wave 7 S7,
WU18 attempt — deferred, not landed)") for the full rationale, and
apply-progress (#10204) for the complete technical finding.

`newLineageActivationEnvVar`/`NewLineageActivationEnabled` and the legacy
`review start` branch are RESTORED, byte-identical to pre-attempt. The
switch stays; v3 remains opt-in (also the safer posture for the upcoming
release candidate's community testing).

- [x] 18.1-18.6: TRANSFERRED to the successor change
  `openspec/changes/rdd-single-lifecycle-cutover/` (verify N4 — an
  unchecked `[ ]` here would claim this is still Wave 7's own outstanding
  task, while the spec now says the requirement it serves is not Wave 7's
  to deliver at all; both artifacts must agree). Nothing under 18.1-18.6
  was executed this wave beyond the reverted WU18 attempt already
  documented above. Blocked on v3 negotiated START gaining
  `repository_context` support (see the successor's `proposal.md` for the
  full re-entry brief, and `specs/rdd-single-lifecycle/spec.md`'s own
  Requirement: "Switch Removal Is Blocked On v3 Negotiated Repository
  Context", which stays Wave 7's own delivered requirement). Not
  re-attempted this wave; the successor change owns re-attempting it.

### WU18a — S7a: additive start-time guards + v3 negotiated frozen context (kept, independent of the switch)
- [x] Start-time legacy-collision guards added to the switch-ON v3 path in
  `runReviewFacadeStart` (a real, independent gap the WU18 attempt found:
  before this, a switch-ON `review start` never checked for EITHER kind of
  existing legacy authority under the same lineage id at all). v1: reuses
  the existing shared "choose a new lineage for compact authority" wording.
  v2 (new refusal): names `review recover` as a genuinely resolving
  additional option, content-aware (an exact hint-replay over identical
  existing v2 content is never refused, only a genuine conflict).
- [x] v3 negotiated START gained frozen-candidate-context support (never
  had it before — `runReviewFacadeStartNewLineage` was always called
  unconditionally, ignoring `--contract`). `repository_context` stays nil
  for v3 (the disclosed, still-open gap WU18 deferred on).
- [x] Test-suite migration to direct legacy (compact-v2) construction, kept
  regardless of switch state per the coordinator's own call (better test
  design): `finalizeApprovedFacadeReview`, `approveDiscoveryMarkdownProjection`,
  `startFacadeReview`, and a new general-purpose `runLegacyFacadeStartForTest`
  helper — ~40+ call sites across 7 files.
- [x] Two new end-to-end regression tests for the guards
  (`TestReviewFacadeStartRefusesOverExistingV1Authority`,
  `TestReviewFacadeStartRefusesOverExistingV2AuthorityAndNamesRecover`).
- [x] Full evidence: gofmt/vet clean, `go test ./internal/cli/... -count=1`
  fully green (zero failures), root + reviewtransaction suites green except
  the one expected RG.1b failure (unchanged, 6 verbs), bench module tests
  green, bench corpus vs the WU16-checkpoint (36399024) binary: 83/83
  comparable journeys unchanged status, zero structural deltas. Refusal
  ratchet 1585→1586 (net +1, the new v2-collision refusal). Deadcode
  ratchet byte-identical to the WU16 checkpoint (243, zero change).
  Landed at 02ff50ea.

## WU19 — S8: D4 verbs (row 24, classify at task time)
- [x] 19.1 Classified `invalidate`/`abandon`/`recover`/`reclaim`/`dispose-result`/`reopen-results` (`review_facade.go:707-722`) against the CURRENT tree (not the design's originally-assumed post-WU18 tree — WU18 is deferred, see the spec amendment). Outcome, evidence-based (each verb's own handler constructs/mutates a `reviewtransaction.Compact*` record): all six are LIVE, ACTIVE mutation surface for the current default (switch-gated) compact-v2 lifecycle — neither dead (delete) nor a D5 residual-READ path (retain read-only); D5's retained-read category is for FROZEN v1 forensic access, and compact-v2 is not frozen while the switch remains. None qualify for deletion.
- [x] 19.2 N/A — zero confirmed-dead verb cases; nothing to delete. (Design estimated ≤300L of deletion under the assumption WU18 would land first; that assumption did not hold this wave.)
- [x] 19.3 GREEN, honestly: `legacy_readonly_guard_test.go` (RG.1/RG.2) is fully GREEN — not by deleting live surface to force it, but by narrowing `legacyRetiredMutationVerbs` to the 5 verbs actually retired this wave (already unreachable) and adding a new positive-assertion test, `TestLegacyReadOnlyGuardLiveCompactV2VerbsRemainReachable`, asserting the 6 D4 verbs stay reachable (backed by `legacyLiveCompactV2MutationVerbs`) — this guard is honestly green either way it looks at the current dispatch table. Re-classification (delete vs. D5-retain) is a live task for whichever wave lands switch removal.
- [x] 19.4 Exit Checklist: gofmt/vet clean, deadcode ratchet unchanged (zero new/gone entries — no production code touched), refusal ratchet unchanged (test-only slice), full root `go test ./... -count=1` fully GREEN (zero failures across the entire repo, RG.1b's own red finally resolved), bench module tests green, bench corpus vs the WU18a tip: 83/83 comparable journeys unchanged status, zero structural deltas (expected — test-only slice).

## WU20 — S9b: capability deltas + wave close-out
- [x] 20.1 Finalized all three capability specs against actual landed state:
  `rdd-legacy-retirement` (unchanged — matches landed state exactly: both
  reconcile providers, 5 legacy verbs, forensic read retention all landed
  as specified); `rdd-single-lifecycle` (amended — the WU18 deferral, full
  rationale, and a new precise re-entry Requirement for the follow-up
  wave); `rdd-shadow-evaluation` (corrected — the `ShadowRelation` alias
  rename pass is DEFERRED, not retired outright as originally written;
  the observer itself is fully retired).
- [x] 20.2 Every proposal success-criteria box evaluated against actual
  landed state (proposal.md, not silently checked): 5 of 7 fully met, 1
  (exactly one lifecycle) explicitly NOT met with the deferral reason and
  pointer to the spec amendment, 1 (zero behavior change for the new
  lineage) partially met with WU18a's disclosed, coordinator-approved
  exception documented inline.
- [x] 20.3 Exit Checklist: gofmt/vet clean (root + bench), deadcode ratchet
  247→243 net-negative across the wave (net -4; wave-wide +1701/-8968, net
  -7267 lines from the v1-freeze checkpoint `d10d49ab`; 247 is the true
  wave-wide starting figure — correcting a WU20 misreport that cited 244,
  WU4's mid-wave value, as the wave's start rather than 247), refusal-
  resolution ratchet 1694→1586 net-negative (net -108) from the v1-freeze
  checkpoint, root `go test ./... -count=1` fully GREEN with zero failures
  (RG.1b's own long-standing expected-red resolved at WU19), bench module
  tests green, final verb count: 5 legacy public verbs + 2 reconcile
  providers retired; 6 D4 verbs classified live (not retired) pending
  switch removal; the switch itself retained. See apply-progress and this
  file's own per-WU entries for the complete evidence trail.
- [x] 20.5 verify W4: `docs/architecture/rdd-wave7-deletion-proof-tracker.md`
  declared a WU20 obligation (re-read the retention table, confirm RG.1b is
  fully GREEN with zero legacy verbs reachable, then update
  `rdd-backlog-disposition.md`'s closure-audit-protocol row for step 4)
  that WU20 never performed, and its stated condition ("zero legacy verbs
  reachable") was invalidated by WU19's finding that all 6 D4 verbs stay
  live/reachable. Reconciled honestly IN THE TRACKER ONLY (verify N1
  correction: `rdd-backlog-disposition.md` itself was intentionally left
  untouched — its closure-audit step 4 is a protocol instruction, not an
  outcome claim, and the tracker's own reconciliation paragraph already
  says so correctly; it needed only a discoverability pointer, see 20.7) —
  see the tracker for the corrected text.
- [x] 20.7 verify N2: added a one-line pointer from
  `rdd-backlog-disposition.md`'s closure audit protocol to
  `rdd-wave7-deletion-proof-tracker.md`, so the rescoping in 20.5 is
  discoverable from the backlog side too.
- [x] 20.6 verify W5: the `reconcile-authority` retirement (WU7) left the
  `unchanged_target` and `malformed_recovery_authorization` anomaly classes
  without their only advertised runnable exit (disclosed inline in
  `compact_inspect.go`/`compact_reclaim.go` comments and in WU7's own tasks
  entry). This loss is now tracked as follow-up issue #2422; see that issue
  for the two anomaly classes and their lost exit paths.

## Exit Checklist (every WU)
- [ ] `go test ./... -count=1` root module green.
- [ ] `go test ./... -count=1` bench module green.
- [ ] `bench --axis all` corpus vs fresh binary — byte-identical.
- [ ] Deadcode ratchet net-negative (WU1/2/3/20 exempt, add-only).
- [ ] Refusal ratchet shrinks or holds (`.refusal-ratchet-baseline.txt` rows 181-186, 222-227, 664-717, 955-1009).
- [ ] `gofmt -l .` empty; `go vet ./...` clean.

