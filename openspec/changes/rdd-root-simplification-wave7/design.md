# Design: RDD Root Simplification — Wave 7 (Compatibility Retirement)

Grounded against the POST-W6 tree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave6 @ 40176a8f` (read-only; pending-verify caveat applies). Every `file:line` below was read at design time, not inferred.

## Technical Approach

Consumer-first deletion in nine slices. Providers are only deleted after their last consumer is gone; the deadcode + refusal ratchets are the arbiter (W4-S7 precedent). Three add-only slices bracket the deletions: W-9/10/11 closure first (pre-activation conditions), byte-equivalence evidence before the switch removal, freeze/proof last.

## Deletion Inventory (24 rows, derived at 40176a8f)

| # | Symbol / unit | File:line | Consumers | Slice |
|---|---|---|---|---|
| 1 | `ObserveShadowRelation` | `internal/reviewtransaction/shadow_observer.go` (201) | `internal/cli/review_facade.go:856`, `:1566` | S2 |
| 2 | `shadowObservationEnvVar` = `GENTLE_AI_RDD_SHADOW` | `shadow_observer.go` | `shadow_observer_test.go`, `new_lineage_switch_identity_test.go` | S2 |
| 3 | `shadowClassifyAuthorityHealth`, `shadowAuthorityHealthAtRepo` | `shadow_authority_health.go` (71) | `shadow_observer.go` + own test only (all unexported) | S2 |
| 4 | `type ShadowRelation = CandidateRelation` (alias only) | `candidate_relation.go:36` | in-module; no external importer possible (D1) | S2 |
| 5 | Shadow tests: `shadow_observer_test.go` (185), `shadow_authority_health_test.go` (257), `shadow_identity_test.go` (337), `shadow_readonly_guard_test.go` (286), `shadow_matrix_test.go` (600) | — | regenerate the retained golden | S2 |
| 6 | `RunReviewReconcileAuthority` + dispatch `case "reconcile-authority"` | `review_facade.go:721-722`; `internal/cli/review_reconcile.go` (60) | dispatch only | S3 |
| 7 | `ReconcileInvalidRecoveryEdge` | `compact_reconcile.go:233` | `review_reconcile.go:46`; 5 test files | S3 |
| 8 | `compact_reconcile_test.go` (1192) — reconcile-only subset | — | — | S3 |
| 9 | ds01/ds02/ds04 verb assertions | `bench/axis_damaged_store.go` (14 verb hits), `axis_damaged_store_closure.go` (2) | RETARGET, never delete (D2) | S3 |
| 10 | `RunReviewReconcileAuthorityBatch`/`runReviewReconcileAuthorityBatch` + cases `:680-681`, `:723-724` | `internal/cli/review_reconcile_batch.go` (110) | dispatch only | S4 |
| 11 | `ReconcileInvalidRecoveryEdges` | `compact_batch_reconcile_journal.go:71` (592) | `review_reconcile_batch.go:96` | S4 |
| 12 | `compact_batch_reconcile_plan.go` (350), `compact_batch_reconcile_guard.go` (323) | — | journal only — CONFIRM at task time | S4 |
| 13 | Batch tests: `..._journal_test.go` (340), `..._plan_test.go` (330), `..._lock_test.go` (183), `cli/review_reconcile_batch_test.go` (263) | — | — | S4 |
| 14 | `RunReviewLegacyQuarantine` + case `:729-730` | `internal/cli/review_legacy_quarantine.go` (57) | dispatch only | S5 |
| 15 | `RunReviewLegacyFixScopeQuarantine` + case `:731-732` | `internal/cli/review_legacy_fix_scope_quarantine.go` (57) | dispatch only | S5 |
| 16 | `RunReviewLegacyAliasRepair` + case `:733-734` | `internal/cli/review_legacy_alias_repair.go` (57) | dispatch only | S5 |
| 17 | `legacy_quarantine.go` (289), `legacy_fix_scope_quarantine.go` (606), `legacy_alias_repair.go` (293) | — | rows 14-16 only | S5 |
| 18 | Legacy tests: `legacy_alias_repair_test.go` (311), `legacy_fix_scope_quarantine_test.go` (387), `cli/review_legacy_quarantine_test.go` (128), `cli/review_legacy_alias_repair_test.go` (27), `cli/review_legacy_fix_scope_quarantine_test.go` (22) | — | — | S5 |
| 19 | Refusal-ratchet rows 181-186, 222-227, 664-717, 955-1009 | `.refusal-ratchet-baseline.txt` | ratchet arbiter | S3-S5 |
| 20 | `newLineageActivationEnvVar` | `review_core.go:31` | `review_core.go:40` | S7 |
| 21 | `NewLineageActivationEnabled` | `review_core.go:39-41` | `review_facade.go:1625` | S7 |
| 22 | Legacy `review start` branch | `review_facade.go:1628`→ end of `runReviewFacadeStart` | — | S7 |
| 23 | Switch tests + harness: `new_lineage_switch_identity_test.go`, `review_new_lineage_switch{,_off_golden}_test.go`, `..._rollback_safety_test.go`, `..._kill_switch_test.go:81`, `bench/runner.go:86`, `bench/journeys_wave3.go:11` | — | — | S7 |
| 24 | D4 verbs `invalidate`/`abandon`/`recover`/`reclaim`/`dispose-result`/`reopen-results` + handlers | `review_facade.go:709-728` | legacy-authority mutation only | S8 |

**RETAINED, corrects D1**: `CandidateRelation` and every `ShadowRelation*` constant (`candidate_relation.go:34-45`, `relateCandidates:81`, `shadowRelationHasNoLiveCounterpart:228`) are the LIVE v3 governance vocabulary — consumed by `new_lineage_discovery.go` and `internal/cli/review_governing_authority.go:69`. D1's "delete the whole shadow surface" is only true of the observer; the relation type must survive. `shadow-differential-matrix.golden` is retained as W1 evidence data with its generating test deleted.

## Architecture Decisions

| # | Decision | Choice | Rejected alternative | Rationale |
|---|---|---|---|---|
| 1 | Inventory derivation | Enumerated now at 40176a8f (table above), re-confirmed per slice by the ratchets | Defer wholly to task time | The proposal's High risk was a stale pre-W6 inventory; enumerating now converts it to a checkable artifact. Ratchet re-confirmation keeps the task-time obligation. |
| 2 | Slice ordering | Consumer-first per cluster: CLI verb + dispatch case first, provider second, dead tests third, all in one slice | Provider-first, or verb+provider in separate PRs | A provider deleted before its dispatch case breaks the build; a dispatch case deleted in a separate PR leaves a compiling but unreachable provider that the deadcode ratchet then flags mid-chain. |
| 3 | W-9/10/11 closure | v3-only, add-only, in S1 before any deletion | Fold into the switch-removal slice | Deleting the switch makes v3 the sole lifecycle; shipping a known fail-open (W-10) as the only lifecycle is strictly worse than shipping it behind a switch. |
| 3a | W-10 (severe finding with no evidence_class/causal_disposition accepted) | Add `Severity` to `FindingEvidence` (`transaction.go:146`, `omitempty`), carry it through `newLineageCapturedFindings` (`review_artifact.go:937-946`, which today DROPS `facadeFinding.Severity`), and refuse in `AuthorityStore.CaptureLensResult` (`new_lineage_capture.go:99`) reusing the in-package `isSevereSeverity`/`isSupportedEvidenceClass`/`isSupportedCausalDisposition` (`transaction.go:1774/1829/1838`) with `artifact_admission.go:331`'s verbatim message | CLI-only pre-check | The severity drop at `review_artifact.go:940-942` is the structural root of BOTH W-10 and W-11; a CLI-only check leaves the provider fail-open and gives finalize no severity for W-11. |
| 3b | W-11 (WARNING + causal disposition over-blocks) | v3 finalize filters `CapturedFindingEvidence()` to severe findings BEFORE `AdmitCandidateCausalFindings`; that function stays byte-identical | Add a severity gate inside `AdmitCandidateCausalFindings` | It is shared with the shipped `--admission-findings` channel, whose `FindingEvidence` carries no severity — a gate there would make every override finding non-severe and fail-open completely. |
| 3c | W-9 (`unknown` disposition fails open) | Add a third return `unresolvedIDs` to `AdmitCandidateCausalFindings` (`candidate_causal_admission.go:31`) for `CausalUnknown` + zero value; only the v3 finalize path consumes it and escalates | Reroute `unknown` to admitted inside the shared function | v2's main path already escalates `unknown` (`transaction.go:1623`, `compact_causality_test.go:32`); only its `--admission-findings` override does not. An additive return leaves every v2 caller byte-identical while the lineage that becomes the only lineage matches the root contract. The v2 override divergence (S-8) is recorded as a post-W7 follow-up, not silently changed inside a deletion wave. |
| 4 | Switch-removal evidence | Two-commit byte-equivalence inside S6/S7: commit A records golden/envelope/receipt bytes for the full journey set with `GENTLE_AI_RDD_NEW_LINEAGE=1` at every entry surface (`start` negotiated + unnegotiated, `status --next-transition`, `capture-result`, `finalize`, `validate`, all five gates); commit B deletes the switch and re-runs with the var UNSET; gate = byte identity of the recorded artifacts | A build-tag or test-seam double-eval in one binary | A production seam is new surface the wave forbids; recorded artifact bytes are the actual invariant and a failure shows as a visible diff, not a silent pass. |
| 5 | Legacy read retention (D5) | KEEP: `sddstatus/legacy_binding_read.go` `parseLegacyBinding` (+ `parseBinding`/`bindingBytes`/`bindingDigest`/`bindingPath`), `candidate_decline.go`'s parser, every `StateInvalidated` parse arm (`status.go:335`, `target_status.go:503`, `compact.go:476`, `receipt.go:272`), `AuthoritativeStore`/`LoadChain` + `NewLegacyReadOnlyError` (`review_facade.go:1632-1635`), `contracts/review-integration/v1/**` (51 files: 27 fixtures + 24 schemas). Guard: new `legacy_readonly_guard_test.go` following the existing `candidate_readonly_guard_test.go`/`shadow_readonly_guard_test.go` pattern, asserting no legacy-mutation entry point is reachable and no parse path writes | Delete the read path with the mutation path | Historical authority must still parse; the forensic invariant outranks surface-count reduction. |
| 6 | `bind-sdd` / `RuntimeStatus.Binding` / remediation-successor CAS (#10166) | RE-DEFER to a dated successor change; record the consumer inventory as evidence | Design an `SDDReceiptRef`-based successor here | `bind-sdd` is live (`review_facade.go:678`, `:737`) and `SDDReceiptRef`'s two-field shape has no analogue for `ExpectedBindingRevision`/`SuccessorLineageID`. Building one is NEW surface, which the proposal scopes out ("zero new verbs, zero new-lineage behavior change"). A deletion wave must not smuggle in a redesign. |
| 7 | W4 task 8.5 (relaunch-bound loss) | CLOSE AS OUT-OF-SCOPE FOR W7 — not `wontfix-superseded`. Standing follow-up owned by the admission flow | Close as superseded, or build a provider-side replacement here | #10166 confirmed zero cross-references: the plugin's `GENTLE_AI_REVIEW_BINDING` capture-task bookkeeping is structurally unconnected to Go-side legacy mutation. W7 retires nothing in that file, so the "confirm a replacement first" precondition never triggers. Calling it superseded would falsely imply the bounded-retry property is now covered — it is not. |
| 8 | PR slicing | Nine slices, consumer-cluster cut, ≤1000 authored lines each; every slice strongly net-negative except add-only S1/S6/S9 | One deletion PR per verb family | Rows 11-13 and 17-18 each exceed 1000 lines alone and MUST subdivide provider-vs-test at task time. |

## Slice Plan and Line Forecast

| Slice | Content | Authored Δ (forecast) | Net |
|---|---|---|---|
| S1 | W-9/10/11 closure + RED tests | +250 | positive (add-only) |
| S2 | Shadow observer retirement (rows 1-5) | −1900 | strongly negative — subdivide: S2a observer+cli, S2b tests |
| S3 | `reconcile-authority` (rows 6-9, 19) + bench retarget | −1700 | subdivide: S3a consumers+bench, S3b provider+tests |
| S4 | `reconcile-authority-batch` (rows 10-13, 19) | −2100 | subdivide: S4a consumers, S4b journal, S4c plan/guard/tests |
| S5 | quarantine/repair verbs (rows 14-19) | −2200 | subdivide: S5a CLI, S5b providers, S5c tests |
| S6 | Byte-equivalence evidence record (commit A) | +150 | positive (add-only) |
| S7 | Switch + legacy start branch (rows 20-23, commit B) | −600 | strongly negative |
| S8 | D4 verbs (row 24), classified at task time | TBD at task time | negative |
| S9 | v1 freeze declaration, deletion proofs #1455/#1462/#1570 + PRs #1549/#1550, capability deltas | +200 | positive (docs) |

`Decision needed before apply: Yes` — S2-S5 each exceed the 1000-line budget and require the cached `auto-chain` strategy plus the recorded subdivision.
`Chained PRs recommended: Yes`
`400-line budget risk: High`

## Data Flow (post-S7)

    review start ──→ ReviewCore.Next ──→ AuthorityStore.Mutate ──→ v3/review-state.json
                                                │
    capture-result ──→ CaptureLensResult ───────┤ (severity now carried, W-10)
                                                │
    finalize ──→ severe-filter ──→ AdmitCandidateCausalFindings ──→ receipt
                                     (admitted | followUp | unresolved→escalate, W-9/W-11)

    legacy v1/v2 records ──→ read-only parsers ──→ status projection (never mutated)

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | W-9/10/11 refusals; `AdmitCandidateCausalFindings` three-bucket purity | RED first in `new_lineage_capture_test.go`, `candidate_causal_admission_test.go` |
| Unit | Legacy read-only invariant survives every deletion | New `legacy_readonly_guard_test.go` |
| Integration | Dispatch no longer routes retired verbs; refusal ratchet shrinks | `review_facade` dispatch tests + `.refusal-ratchet-baseline.txt` regeneration per slice |
| E2E | Byte-equivalence at every entry surface; ds01/ds02/ds04 shapes still asserted after retarget | `bench --axis all`, gate-boundary golden without `-update` |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no file-classification or execution boundary is added or removed | — | — |
| Git repository selection | Applicable — deleted verbs took `--cwd`; the retained read paths and gates still resolve the Git common dir | Deletion must not change `reviewAuthorityRoot` resolution for any retained surface | One test asserting common-dir resolution byte-identity pre/post each deletion slice |
| Commit state | Applicable — gates (`pre-commit`, `pre-push`, `pre-pr`, `release`) validate the same receipt across the switch removal | Gate evaluation is untouched; only the start branch changes | Gate-boundary golden PASS without `-update` in S7 |
| Push state | N/A — no push/ref resolution is added or removed | — | — |
| PR commands | Applicable — CLI verb dispatch (`review_facade.go:687-742`) loses cases | An unknown verb must return the existing `unknown review command %q`, never a panic or silent no-op | One test per retired verb asserting the exact unknown-command refusal |

## Migration / Rollout

No data migration. Historical bytes (receipts, journals, bundles, quarantine residue) are never touched. Each slice is independently `git revert`-able. S7 is the ordering hinge and lands only after S1-S6.

## Open Questions

- [ ] S4 row 12: confirm `compact_batch_reconcile_plan.go`/`_guard.go` have no consumer outside the journal.
- [ ] S8 row 24: which D4 verbs have a residual legacy-read role that survives as read-only.
- [ ] D3: the declared support horizon for a pinned gentle-pi consuming `contracts/review-integration/v1`.
