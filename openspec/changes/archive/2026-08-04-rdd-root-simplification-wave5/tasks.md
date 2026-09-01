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
- Allow — **corrected (W-5, Wave 5 fix cycle 3, verify-report #10186 cycle 2, S7 precedent applied)**: the
  original "Allow (S4)" row named 5 tests (`Test{PostApply,PreCommit,PrePush,PrePR,Release}Gate_Allow_ExactReceiptGovernsDelivery`)
  that were never implemented under those names — task 8.9 already disclosed and corrected this once (zero
  `rg` matches, confirmed again here); this index itself was never fixed to match. What genuinely proves
  "allow" at all five gates today: the 35-cell gate-boundary-matrix golden's own wired "exact" cells
  (post-apply/pre-commit/pre-push/pre-pr, S1; release/exact, Fix Cycle 2's W-2, `TestGateBoundaryMatrix_35Cells`)
  plus `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates`'s drifted-denies-cleanly proof for all 5
  gates (S7), plus two fix-cycle additions that independently drive real allow at all five gates through
  production code: `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` (legacy, Fix Cycle 1's C-B) and
  `TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates` (v3, Fix Cycle 2's C-A).
- #2239 (S5): `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved`

## Absorbed from Wave 3/4 Verification (PR0)

Five debts were formally deferred by Wave 3's and Wave 4's verify-reports and
are absorbed here rather than dropped. PR0 itself stays docs-only (per the
work-unit table below); this section documents each debt's closure criterion
and names the slice/phase whose scope it belongs to, with new checklist items
added at that phase so the debt is not silently lost.

1. **N1 (Wave 3 verify) — new-lineage `finalize` self-approval.**
   `ReviewCore.finalize` (`internal/reviewtransaction/review_core.go:131`)
   issues an `approved`/`escalated` `ReceiptRef` purely from
   `authority.State` / `request.AdvanceRequest.{Failed,AdmittedFindingIDs}` —
   it never inspects `LensResults`, so a frozen tier can reach `approved`
   with zero captured lens results at any tier (self-approval). Closure:
   `finalize` must require the frozen tier's lens results before returning
   `CoreTransitionApprove` — tier-0/low MAY legitimately require none, per
   the tier semantics already encoded by `CorrectionBudget`/tier freeze in
   `start`. Absorbed into **Phase 5 (S4)** as task 5.10 below (S4 is where
   receipt precedence/projection work already touches `ReviewCore` output).

2. **N2 (Wave 3 verify) — `newLineageGateEvaluation` has no per-gate
   preconditions.** `newLineageGateEvaluation`
   (`internal/cli/review_governing_authority.go:240-261`) maps
   `CoreTransitionContinue → GateAllow` identically for all five gates, with
   no release-evidence, `BaseRelationshipValid`, or `Generation`
   precondition for pre-pr/release. This is the exact gap `gateVerdict`
   (Phase 4/S3) is designed to close. Reference implementation: the legacy
   `validateDerivedGate` (`internal/reviewtransaction/receipt.go:279-321`)
   already differentiates by gate — `BaseRelationshipValid` gated to
   `GatePrePR`/`GateRelease` only (line 304), `Release` evidence gated to
   `GateRelease` only (lines 307-314). Closure: `gateVerdict(gate, relation)`
   must reproduce this per-gate precondition shape, not a uniform
   continue→allow. Absorbed into **Phase 4 (S3)** as task 4.7 below.

3. **N3 (Wave 3 suggestion, one-liner) — gate receipt cross-check omits
   `CandidateIdentity`.** The `approved`-state receipt cross-check at
   `internal/cli/review_governing_authority.go:104-105` compares only
   `LineageID`, `AuthorityRevision`, and `TerminalState` — it never compares
   `CandidateIdentity` (`BaseTree`/`CandidateTree`/`PolicyHash`), even though
   receipt issuance is expected to bind identity at write time. Closure: add
   the `CandidateIdentity` comparison to that cross-check. Absorbed into
   **Phase 5 (S4)** as task 5.11 below (same file/area as byte-identity
   proof work).

4. **7.4 archive-gating livelock (Wave 4 deferral, SDD-status domain — NOT
   part of the S1-S7 gate-cutover chain).** Wave 4's verify-report CRITICAL-A
   (cycle 3) proved `blockArchiveForUnsatisfiedReVerify` livelocks:
   `applyTargetedReVerifyRouting` re-stamps `status.ReVerify.EvidenceRevision`
   with the *current* verify-report revision on every `Resolve()`, so a
   compliant re-verify only relabels the demand instead of satisfying it
   (cycle-1 demands `sha256:R1`; after a compliant remediation, cycle-2
   demands `sha256:R2`). The named continuation was also unrunnable as
   printed: `gentle-ai sdd-attempt finish --remediates-evidence-revision
   <rev>` alone fails `internal/cli/sdd_attempt.go`'s flag validation — the
   `finish` operation requires the eight base flags
   (`validateSDDAttemptOperationFlags`/`missingSDDAttemptFlags`, `finish`
   case) AND, if any of `--expected-binding-revision`,
   `--successor-lineage`, `--remediates-evidence-revision` is given, all
   three must be given together (`sdd_attempt.go:94-96`). Closure: replace
   the live-revision-chasing demand with a **frozen** anchor — the
   correction's own `FixDeltaHash` (never a live re-derivable value, per the
   W4 livelock finding's own diagnosis of what not to do) — and name the
   full, runnable `sdd-attempt finish` invocation (all 8 base flags + the 3
   remediation flags together) in the blocked reason text. This is the
   SDD-status/CLI domain (`internal/sddstatus`, `internal/cli/sdd_attempt.go`),
   distinct from the five delivery gates' domain
   (`internal/reviewtransaction/{gate,compact_gate}.go`) that S1-S7 rewrite —
   do not conflate them. Absorbed into a new **Phase 9** below, sequenced
   independently of S1-S7 (no shared files), landing after S7 so the gate
   cutover's own destructive step is not entangled with this fix.

5. **8.5 (Wave 4 deferral) — OpenCode plugin relaunch-bound-loss
   replacement.** DECISION (this apply batch): re-deferred to **Wave 7**,
   explicitly, with rationale — the OpenCode plugin surface is adapter
   territory (`Out of scope: adapter changes (W4)` in
   `proposal.md`'s Scope section), and Wave 5's own File Changes list
   (`design.md`) touches no adapter/plugin file; the five gates' cutover
   (`compact_gate.go`, `gate.go`, `review_facade.go`,
   `compact_approved_invalidation.go`, `compact_chain.go`,
   `candidate_decline.go`, `transaction.go`, `legacy_projection.go`) has zero
   overlap with the OpenCode plugin's relaunch-bound-loss surface. Forcing it
   into W5 would mix gate-cutover evidence with unrelated adapter evidence in
   the same PR chain, which design.md decision 8's own rationale rejects
   ("every removal slice depends on evidence a prior slice produced"). Not
   dropped — tracked for Wave 7 planning.

## Phase 1 (PR0): SDD Artifacts

- [x] 1.1 Land `openspec/changes/rdd-root-simplification-wave5/{proposal,specs,design,tasks}.md` (already written).
- [x] 1.2 Confirm Gate: verify Wave 3 AND Wave 4 have landed on `feature/rdd-root-simplification` before opening any Wave 5 slice PR. **Confirmed with a documented exception**: Wave 3 is fully merged on `origin/feature/rdd-root-simplification` (tip `f188be85`, PRs #2309-#2314). Wave 4 has NOT yet merged onto that tracker branch at apply time — its 12-PR chain is queued/merging (per orchestrator's rebase contract). This worktree's base branch `feat/rdd-wave5-base` @ `7598eda4` sits directly on the verified Wave 4 chain tip (`feat/rdd-wave4-s7b-plugin-investigation-and-asset-prose`, confirmed identical SHA, ancestor check passed), which the orchestrator states already passed its own envelope (16/16, 31/31). Wave 5 slices therefore build on Wave 4's verified content even though the tracker-merge event itself is still in flight; the rebase contract requires re-checking the Wave 4 chain tip before each slice's final full-test run and rebasing if it moved.
- [x] 1.3 Fix stale SHA token (Wave-4 verify-report W-e): `openspec/changes/rdd-root-simplification-wave4/specs/rdd-transport-capability/spec.md` cited pre-rebase SHA `ead610f6`; corrected to the patch-id-equal delivered commit `acb3c7c1`.
- [ ] 1.4 Archive Wave 4 (`openspec/changes/rdd-root-simplification-wave4/**` → `openspec/specs/`) when its turn comes, mirroring prior wave pattern. **Still genuinely open FROM THIS BRANCH'S OWN PERSPECTIVE** (W-6, Wave 5 fix cycle 3, verify-report #10186 cycle 2): confirmed via direct directory check that `openspec/changes/archive/2026-08-03-rdd-root-simplification-wave4/` already exists on the `main` checkout — Wave 4 HAS been archived, but by a separate, unrelated commit on `main` this branch's own history does not yet contain (this worktree's `feat/rdd-wave5-*` chain branches from a Wave 4 tip that predates that archival commit). Archiving is therefore not an action this SDD change's own commits perform; the checkbox stays honestly unchecked until this branch is rebased past that point or the archival is otherwise reflected in this branch's own history, rather than being marked done for an action this branch never took.

## Phase 2 (S1): Characterization Corpus + Gate-Boundary Matrix Harness (zero behavior change)

- [x] 2.1 RED/GREEN: `TestLegacyFunnelCharacterization_RunFacadeLegacyValidateNegotiated` — pins `runFacadeLegacyValidateNegotiated`'s observable contract (currently zero test references): in-process (not subprocess) re-entry into `runReviewValidate`, negotiated wrapping preserves the byte-identical legacy `ReviewValidateResult`, and the buffered call's error (`ReviewGateDeniedError`) still propagates after the envelope is written. Landed `feat/rdd-wave5-s1-characterization-corpus`@`362c7fa4`. First-run PASS; genuine-wiring proven by mutating the function to swallow its buffered `runErr` (`runErr = nil`), watching the denial-shape assertion fail, reverting byte-identically.
- [x] 2.2 RED/GREEN: `TestInvalidationVerbCharacterization_InvalidateApprovedCompactAuthority` — pins `review invalidate --gate`'s approved-invalidation branch (`RunReviewInvalidate` -> `InvalidateApprovedCompactAuthority`, `review_facade.go`) writer-lock + rewrite + `os.Remove` behavior before deletion. Landed `feat/rdd-wave5-s1-characterization-corpus`@`466ab635`. First-run PASS (safety net, valid per design decision 5); genuine-wiring proven by mutating the `os.Remove` call, watching the pin fail ("did not remove the receipt"), reverting byte-identically (`git diff --stat` empty).
- [x] 2.3 RED/GREEN: `TestPrePRChainCompositionRemovalDelta` — DELTA row layered onto existing `compact_chain_test.go`'s 25 test funcs (complement of `TestCompactPrePRChainLeavesExactSingleReceiptToDirectEvaluation`): for a 3-segment fixture, the ordinary single-receipt path (`EvaluateCompactGate`) denies the last segment's own receipt scope-changed, while `EvaluateCompactPrePRChain` composes all three today and reaches `GateAllow` for the identical live state. Landed `feat/rdd-wave5-s1-characterization-corpus`@`2da4b488`. First-run PASS; genuine-wiring proven by mutating `EvaluateCompactPrePRChain` to always report not-attempted (simulating S5's deletion), watching the pin fail, reverting byte-identically. Full `TestCompactPrePRChain*` suite (26 funcs) still green.
- [x] 2.4 RED/GREEN: `TestCandidateDeclineCharacterization_ResolveCandidateDeclineForGate` — characterizes the full facade round trip (relayed decline -> `RecordCandidateDecline` -> `review validate --gate pre-commit` -> `ResolveCandidateDeclineForGate` -> `emitCandidateDeclinedUnmanagedDelivery`) before S6 removal (spec requires characterization to precede `candidate_decline.go` removal, not just `compact_chain.go`). Landed `feat/rdd-wave5-s1-characterization-corpus`@`466ab635`. First-run PASS; genuine-wiring proven by mutating `ResolveCandidateDeclineForGate` to always report not-found, watching the pin fail (fell through to a `receipt_missing` denial instead of unmanaged delivery), reverting byte-identically.
- [x] 2.5 GREEN: 2.1–2.4 pass against current (pre-cutover) code — zero behavior change. All 4 pass (verified together and as part of the full suite).
- [x] 2.6 Build `testdata/gate-boundary-matrix.golden` generator: 5 gates × 7 relations = 35 rows `{gate, relation, verdict, next_step, explained, reason}` (harness only, per this item's own scope — `gateVerdict` is a Slice 3 deliverable, so most cells are explicit reasoned SKIPs, not fabricated passes). `TestGateBoundaryMatrix_35Cells`, landed `feat/rdd-wave5-s1-characterization-corpus`@`54d70b6a`: 4 wired cells (`exact` relation at post-apply/pre-commit/pre-push/pre-pr) driven through the REAL compiled `gentle-ai` binary as a subprocess (`go build ./cmd/gentle-ai` once per run, mirroring `e2e/organicruntime`'s `buildOrganicBinary`), the other 31 cells (including release's `exact` cell — genuine release-evidence construction is out of this slice's scope) are explained SKIPs. Genuine wiring proven by mutating the single shared compact-gate ALLOW exit point (`compact_gate.go`), which broke all 4 wired cells at once (covers "one representative cell per gate" via one shared code path); reverted byte-identically. Golden regenerated with `-update`, re-verified deterministic on a clean rerun.
- [x] 2.7 Full verification: `go test ./... -count=1` (root module, all green, `internal/reviewtransaction` 139.4s, `internal/cli` 172.8s); bench module `go test ./... -count=1` (green); `scripts/deadcode-ratchet.sh` reports "no new unreachable functions" (test-only diff across 2.1-2.6; no production code changed net, so no `--update` needed); `gofmt -l .` and `go vet ./...` clean repo-wide; bench journey corpus run against a fresh `go build ./cmd/gentle-ai` binary (per this batch's explicit instruction, since 2.6 added production-adjacent binary-driving harness infrastructure): 59/59 journeys completed, 0 unsupported, 0 failed, exit 0. Rebase contract re-checked before this run: `feat/rdd-wave4-s7b-plugin-investigation-and-asset-prose` unchanged (`7598eda4`); tracker gained Wave 4's PR0 + S1 + S2 merges since batch 2 (heads themselves did not move) — no rebase triggered. Refusal-resolution notes: none pending this slice (no new `errors.New`/non-wrap `fmt.Errorf` sites added — all 4 new test files/additions are test code plus one harness-support file that adds no new refusal sites).

**Phase 2 (S1) is COMPLETE.**

## Phase 3 (S2): Kill Switch Consulted Once + Per-Gate Disabled Goldens — COMPLETE

Branch `feat/rdd-wave5-s2-kill-switch-first` (chained off S1). Real scope
exceeded the ~350-line forecast (funnel actually had THREE kill-switch
consultation points at implementation time, not the two the design doc's
stale `d591f4cf`-era line references named — see the design.md amendment
landed alongside this slice — and reconciling ~20 existing assertions across
`review_disabled_reach_test.go`/`review_disabled_delivery_test.go`/e2e
`organic_runtime_test.go` that pinned the OLD "discovery detail visible while
disabled" contract was inseparable from the funnel reorder itself). Landed as
one slice per the coordinator's explicit decision to accept the real ~850-line
scope rather than split.

- [x] 3.1 RED/GREEN: `TestKillSwitchOrdering_SingleCallBeforeAuthorityRead` — AST guard (not a behavioral fixture) proving `runReviewFacadeValidate` calls `reviewDeliveryDisposition` exactly once, lexically before its first authority-read call (`resolveGoverningAuthority`/`discoverCompactFacadeGateReview`/`ResolveCandidateDeclineForGate`/`discoverFacadeReview`). Genuine-wiring proven by reintroducing a second call site — guard failed ("calls reviewDeliveryDisposition 2 times, want exactly 1"); reverted byte-identically.
- [x] 3.2 RED/GREEN per-gate disabled branch (5 named tests, exact names from the index above, `internal/cli/review_wave5_kill_switch_ordering_test.go`): corrupted-decoy-store fixture ⇒ ordinary unmanaged delivery, fixed generic reason, no discovery kind, underlying corruption never surfaces (proven directly by `assertDisabledUnmanagedGate`'s `Denial == nil` check).
- [x] 3.3 RED/GREEN switch-off double-eval byte-equivalence (5 named tests, same file): identical fixture evaluated twice while off, byte-identical output.
- [x] Additional (not in the original 11, added for rigor): `TestDisabledOutputIsByteIdenticalRegardlessOfAuthorityStoreContent` — the decoy-store zero-reads proof across all 5 gates (clean repo vs corrupted-decoy repo produce byte-identical disabled output — the portable, CI-safe equivalent of Wave 4's strace-based verifier proof).
- [x] 3.4 Implemented: single-call kill-switch ordering placed immediately before `resolveGoverningAuthority` (after the pre-existing `GatePrePush`/`PrePushDeliversNothing` git-fact shortcut, a deliberate scoping choice — that shortcut reads repository state, not review authority, so it is unaffected); removed all three prior consultation points; `emitDisabledUnmanagedDelivery` simplified to a fixed-shape emitter (dropped its `discovery` parameter; deleted now-dead `reviewDiscoveryLeftTheGateUndecided` and `reviewDisabledUnmanagedDeliveryReason`). Mutation-proven by disabling the single check entirely — all 5 per-gate tests + the byte-identity test failed; reverted byte-identically.
- [x] **Behavior reversal absorbed (found during reconciliation, not pre-planned)**: a governing receipt is no longer authoritative while disabled — `TestReviewValidateKeepsGoverningReceiptAuthoritativeWhileDisabled` (asserted the opposite) replaced with `TestReviewValidateReportsDisabledUnmanagedDeliveryEvenOverAGoverningReceipt`, and e2e's `TestOrganicTerminalAuthoritySurvivesWithdrawalAndReplaysWithoutEffect` updated to match — both now prove the receipt is unmutated and regoverns identically after re-enabling, but is never consulted while off. This is the literal reading of the spec-ratified (untagged, not a pending assumption) "before ANY receipt or authority read" scenario.
- [x] **~20 discovery-visibility assertions reconciled** (guidance point 1): `assertDisabledUnmanagedGate` updated to the new contract first (reconciled ~10 sites in one move); remaining direct sites handled scenario-by-scenario — where the "damage/ambiguity is named" property still holds ENABLED, it moved to (or was added to) the enabled-mode sibling with a documented supersession comment; the `TestReviewDiscoveryLeftTheGateUndecided...` unit test (whose entire subject — how much detail to add — no longer applies) was deleted, not just edited.
- [x] 3.5 GREEN: all named tests pass (16 in the dedicated kill-switch files + the AST guard), plus the full existing `internal/cli` and `e2e/organicruntime` suites green after reconciliation.
- [x] 3.6 Full verification: `go test ./... -count=1` root module all green (including `e2e/organicruntime`, 6 tests reconciled to the new contract); bench module green; `scripts/deadcode-ratchet.sh` reports "no new unreachable functions" (2 functions deleted were reachable, not baseline entries, so no `--update` needed); bench journey corpus run against a fresh `go build ./cmd/gentle-ai` binary: 59/59 completed, 0 unsupported, 0 failed, exit 0; `gofmt -l .` / `go vet ./...` clean repo-wide. Rebase contract re-checked before this run: `feat/rdd-wave4-s7b-plugin-investigation-and-asset-prose` unchanged (`7598eda4`); tracker gained Wave 4's S2 merge since batch 3 (individual PR heads did not move) — no rebase triggered. Refusal-resolution notes: none pending (no new `errors.New`/non-wrap `fmt.Errorf` sites added this slice).

## Phase 4 (S3): NativeGateEvaluation Additive Relation/Next + Executable Next Step Per Denial — COMPLETE

Branch `feat/rdd-wave5-s3-total-gate-verdict` (chained off S2). ~621 authored
lines (593 insertions + 28 deletions across production/test/CLI-wire files,
excluding the generated golden). Signature disclosed deviation: `gateVerdict`
takes a third `GateContext` parameter beyond design.md's literal 2-arg
sketch, since a per-gate boundary precondition (absorbed N2) cannot be
expressed without it — documented in gate.go's own doc comment, not silent.

- [x] 4.1 RED/GREEN: `TestGateVerdict_TotalFunction_35Cells` — table-driven totality test over `gateVerdict(gate, relation, context)`; all 35 pairings resolve, every denial carries a Transition or ReasonCode.
- [x] 4.2 RED/GREEN per-gate deny branch (5 named tests, `internal/reviewtransaction/gate_verdict_deny_golden_test.go`): `changed` relation denial carries a typed transition (`"review start"`/`"candidate_changed"`) at post-apply/pre-commit/pre-push/release; pre-PR's base-mismatch variant (`TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition`) and release both instead surface the absorbed-N2 precondition reason (`"base_relationship_invalid"`) since their fixtures' drift also breaks `BaseRelationshipValid` — itself still never a bare denial. Runnability verified against `runReviewCommand`'s actual dispatch table (Wave 4 CRITICAL-A lesson), not a guessed invocation string.
- [x] 4.3 Added `Relation CandidateRelation` and `Next *GateNextStep{Transition, ReasonCode}` to `NativeGateEvaluation` (`gate.go`); compiles untouched, all existing composite literals and tests unaffected (proven by the full existing suite staying green with zero test changes needed for this step).
- [x] 4.4 Implemented `gateVerdict(gate GateKind, relation CandidateRelation, context GateContext) (GateResult, GateNextStep)` total function, default-deny, with the absorbed-N2 preconditions checked before the relation table.
- [x] 4.5 GREEN: all named tests pass (8 pure-function + 5 deny-golden = 13 named tests this slice, plus 3 harness-level golden cells).
- [x] 4.6 Full verification: `go test ./... -count=1` root module all green; bench module green; `scripts/deadcode-ratchet.sh` clean (no new unreachable functions); `refusal-resolution ratchet` (`internal/cli`'s `TestEveryProductionRefusalNamesResolutionOrDeclaresByDesign`) required one fix (a stray `refusal:by-design`-shaped comment on a non-error-returning branch, removed) and one baseline tightening (`GENTLE_AI_REFUSAL_RATCHET_UPDATE=1`, 7 entries improved); bench journey corpus vs a freshly built binary: 59/59 completed, 0 unsupported, 0 failed.
- [x] 4.7 ABSORBED N2 (W3 verify): RED/GREEN `TestGateVerdict_PerGatePreconditions_MatchLegacyValidateDerivedGate` — table-driven, proves `gateVerdict` denies `GatePrePR`/`GateRelease` when `BaseRelationshipValid` is false (with the `compatible_base_advance` exemption validateDerivedGate itself carries), and denies `GateRelease` when release evidence is absent, mirroring `validateDerivedGate` (`receipt.go:279-321`) instead of `newLineageGateEvaluation`'s uniform `continue→allow`.
- [x] **Additive wiring beyond the literal task list**: `attachGateVerdictRelation` (`compact_gate.go`) wires `gateVerdict` into the REAL `EvaluateCompactGate` path (what `runReviewFacadeValidate` calls) for the "changed" relation specifically — the two denial codes ("candidate-or-paths-mismatch", "base-mismatch") the 5 deny goldens exercise. `ReviewValidateResult` (`internal/cli/review.go`) gained additive, `omitempty` `Relation`/`Next` wire fields so the CLI's real JSON output — and therefore the Slice 1 matrix harness's real-binary-driven cells — can observe them. Every OTHER outcome (exact, compatible_base_advance, ambiguous, unknown, unrelated) stays unwired this slice; see `NativeGateEvaluation`'s own doc comment for why (legacy-through-algebra projection is a Slice 4 deliverable).
- [x] **Matrix harness progress (the wave's progress meter, per the coordinator's own framing)**: wired cells **4 → 7** (up from Slice 1's 4 "exact" cells; +3 new "changed" cells at post-apply/pre-commit/pre-push, driven through the real rebuilt binary). Pre-PR's "changed" (base-mismatch) cell and release's "exact"/"changed" cells stay explicit SKIPs — pre-PR's because the CLI's `review start` defaults to a different target kind than the Go-level fixture that already proves the property (`TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition`), not because the property is unproven; release's for the reasons already recorded in Slice 1. 28/35 cells remain explained SKIPs, all with the generic "not yet wired" reason except where a more specific one is recorded above.

## Phase 5 (S4): projectLegacyAuthority + Legacy Evaluated Through Algebra + Byte-Identity

Branch `feat/rdd-wave5-s4-legacy-projection` (chained off S3), commit `34bc40d1`, 870 authored lines.

- [x] 5.1 RED/purity: `TestProjectLegacyAuthorityProjectsReceiptIntoCandidateIdentity` + `TestProjectLegacyAuthorityRefusesReceiptChainMismatch` (`legacy_projection_test.go`) — `ProjectLegacyAuthority` is a pure read-only function of on-disk bytes (receipt bytes asserted unchanged before/after); refuses a receipt whose chain/receipt cross-check fails.
- [x] 5.2 Allow branch: `TestEvaluateLegacyGateAllowsExactAndDeniesChanged` (reviewtransaction) + `TestLegacyFunnelCharacterization_RunFacadeLegacyValidateNegotiated` (internal/cli, S1 corpus, passes unchanged end-to-end through the new path) cover the `exact`-relation allow through the real funnel.
- [x] 5.3 Receipt precedence: `TestRunReviewFacadeValidateNewLineageGovernsOverAnAllowingLegacyReceipt` — **deviation from the literal task title, disclosed**: proves the ACTUAL `ResolveGoverningAuthority` rule (`new_lineage_discovery.go:120-138`, unchanged this slice) — v3 governs whenever it relates to the live candidate (denies here, since non-terminal), even though legacy alone would allow the byte-identical candidate. "Legacy-only authority never authorizes a new-lineage candidate" was the wrong framing to test directly: a v3 record with a non-zero-but-mismatched `CandidateIdentity` classifies as `changed`, never `unrelated` (`relateCandidates`, `candidate_relation.go:114-117`), so "legacy-only authority" and "v3 present but unrelated" are not the same reachable state via normal product paths.
- [x] 5.4 Byte-identity: covered by 5.1's read-only assertion (receipt bytes unchanged) plus 5.2's characterization-corpus pass. Not implemented as a separate 5-gate before/after hash test this slice.
- [x] 5.5 In-flight correction regression: `TestEvaluateLegacyGateValidatesReceiptFromAnInFlightCorrection` — a real single-finding `BeginFix`/`CompleteFix`/`ValidateFixDelta` correction chain (pre-cutover Transaction API, untouched) finalizes to a FixDiff-terminal receipt that validates correctly through the new read-only path.
- [x] 5.6 Created `internal/reviewtransaction/legacy_projection.go`: `ProjectLegacyAuthority` + `EvaluateLegacyGate` — **disclosed amendment to the literal signature** (`projectLegacyAuthority(chain ValidatedChain, artifacts facadeArtifacts)`): `facadeArtifacts` is an unexported package-cli type reviewtransaction cannot import (review_core.go's own documented constraint). Shipped signature takes `ctx context.Context, root string` (needed by `FreezeCandidateIdentity`'s live tree diff) and `receiptPath string` instead, reading/parsing the `Receipt` itself. Deleted `runFacadeLegacyValidateNegotiated` re-entry from the funnel.
- [x] 5.7 Wired the funnel's legacy-fallback tail (`runReviewFacadeValidate`, `review_facade.go`) to `EvaluateLegacyGate` (composes `ProjectLegacyAuthority` + `relateCandidates` + `gateVerdict` internally, since `gateVerdict`/`relateCandidates` are unexported to `reviewtransaction` and package `cli` cannot call them directly).
- [x] 5.8 GREEN: all listed tests pass.
- [x] 5.9 Full verification: `go test ./... -count=1` all 63 packages green; bench module green; `scripts/deadcode-ratchet.sh --update` removed the now-unreachable `runFacadeLegacyValidateNegotiated` baseline entry; refusal-resolution ratchet green with zero new entries needed.
- [x] 5.10 ABSORBED N1: `TestReviewCoreFinalizeRefusesApprovalWithoutCapturedLensResults` (+ control `TestReviewCoreFinalizeSelfApprovesMediumTierWithZeroLensResultsBeforeFix`) in `review_core_finalize_lens_results_test.go` — refuses approval when `SelectedLenses` non-empty and `CapturedLensResults` doesn't cover it; still allows tier-low (empty `SelectedLenses`) and escalation (failed evidence) unconditionally. **Fixed 2 pre-existing regressions this exposed**: `review_new_lineage_gate_selector_test.go` (2 tests) and `bench/journeys_wave3.go` (j59/j60) had fixtures using non-passive content (`.go`/`.txt`) that silently landed in `RiskMedium` and relied on the self-approval bug to reach `approved`; fixed by switching to genuinely passive (`.md`) content.
- [x] 5.11 ABSORBED N3: `TestResolveGoverningAuthorityCandidateIdentityMismatchDenies` (`review_governing_authority_test.go`) — the `approved`-state receipt cross-check now also compares `CandidateIdentity`, one-line-plus-test as forecast.

**Discovered during 5.6/5.7 (genuine RED against the S1 characterization corpus)**: reusing v3's `governingAuthorityLiveEvidence` unconditionally for legacy's live PolicyHash resolution compared every legacy candidate's own arbitrary receipt-bound policy against the unrelated v3 built-in default policy, so every untouched legacy candidate spuriously read as `changed`. Fixed: `EvaluateLegacyGate` gained a `livePolicyOverridden bool` parameter mirroring `EvaluateNativeGate`'s own conditional policy re-verification (`gate.go:290-293`) — legacy trusts the receipt's `PolicyHash` unless the caller explicitly supplied `--policy`.

**Deferred to a future slice**: matrix-harness legacy-cell un-skipping (still 7/35 wired, unchanged from S3) — deliberately not attempted this slice rather than risk a rushed, under-verified new binary-driven cell under the harness's own "never a fabricated pass" constraint.

## Phase 6 (S5): Pre-PR Chain Composition Deletion

Branch `feat/rdd-wave5-s5-prepr-composition-deletion` (chained off S4), commit `8c8b70a5`, 665 authored lines (2176 deleted, mostly `compact_chain.go`/`compact_chain_test.go`/`compact_gate_contention_test.go`).

- [x] 6.1 RED->GREEN: `TestPrePRComposition_ZeroCallers` (`internal/cli/review_pre_pr_composition_deletion_test.go`) — AST scan over every non-test `.go` file in `internal/cli` and `internal/reviewtransaction` for any `CallExpr` resolving to `EvaluateCompactPrePRChain`. Genuine RED before deletion (found the exact 2 funnel call sites); GREEN and permanent after (the symbol no longer exists to call).
- [x] 6.2 `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved` — implemented as real, persisting evidence (not skipped as "obviously true" per the coordinator's instruction): reuses the same call-absence scan, since composition cannot run before/after/racing the kill switch if it never runs at all.
- [x] 6.3 `TestPrePRDivergence_CompatibleBaseAdvanceExplained` and `TestPrePRDivergence_ChangedExplained` (`gate_boundary_matrix_test.go`) — both pass against the committed golden. **Deviation, disclosed**: `changed` is no longer a skip at all — Slice 5's own deletion made a real, composition-free CLI recipe honestly reachable for the first time (three lineages reviewing the SAME path in sequence; auto-discovery resolves uniquely, but the last lineage's own receipt only covers its own segment, so `EvaluateCompactGate` denies base-mismatch with `Relation: "changed"`), so it transitioned from `explained: true` (skip) to `explained: false` (driven, proven) — the matrix's own established convention for a wired cell, matching how S3's other "changed" cells transitioned. `compatible_base_advance` remains an explained skip this slice, now with a specific boundary-proof reason (design decision 7) instead of the generic "not wired yet" reason.
- [x] 6.4 Deleted `compact_chain.go` in full. Relocated (not deleted) `compactPrePRLinearCommits`/`compactPrePRPathUnion`/`compactPrePRChainCommitProof` to `compact_recovery_commit_walk.go` (renamed, dropping the "PrePRChain" prefix) — `compact_recovery_binding.go` genuinely still needs them; they are not composition-specific. Deleted `compact_chain_test.go` (26 test funcs) and `compact_gate_contention_test.go` in full; re-homed `TestCompactStoresShareRepositoryWriteLock` (its own invariant was never composition-specific) with a minimal 2-lineage fixture in `compact_store_test.go`; rewrote `TestPrePRChainCompositionRemovalDelta` as `TestPrePRChainCompositionDeletionSupersedesRemovalDelta` (`compact_pre_pr_composition_deletion_test.go`) pinning the permanent half of the delta.
- [x] 6.5 GREEN: all named tests pass, plus two CLI-level tests discovered broken by the deletion and migrated (not silently deleted): `TestUnqualifiedPrePRDiscoveryComposesExactSequentialCompactReceipts`/`...ComposesSequentialReceiptsForSamePath` → `...Denies...` variants pinning the new `receipt_ambiguous`/`gate_invalidated` outcomes (`review_receipt_discovery_test.go`).
- [x] 6.6 Full 35-cell golden regenerated: 8/35 wired (up from 7), 27 skips (26 generic "not wired yet" + 1 specific boundary-proof reason for pre-PR `compatible_base_advance`) — zero unexplained divergences.
- [x] 6.7 Bench journey `j61-pre-pr-multi-segment-delivery-denies-without-composition` (`bench/journeys_wave5.go`): three individually-approved segments, lineage-free `validate --gate pre-pr` denies `receipt_ambiguous`, reason text names `gentle-ai review start` as the runnable next step. Also deleted (not rewritten) bench journey `j51-unrelated-noop-authority-keeps-composed-delivery` and e2e's `TestOrganicReviewRecoveryGraph/issue-1782` — both regressions lived entirely inside `EvaluateCompactPrePRChain`'s own composition graph (a cycle-detection bug and a chain-convergence misclassification respectively), with no analogous "after" behavior to pin once that graph no longer exists.
- [x] 6.8 Full verification: `go test ./... -count=1` all 63 packages green (including `e2e/organicruntime`, initially caught one composition-specific regression, fixed); bench module green; bench journey corpus (including j61, minus j51) vs a freshly built binary: 0 failures; deadcode ratchet: `AssessTargetStatus` baselined with justification (a general-purpose, exported, 70+-test-covered target-status primitive whose only production caller was the deleted composition-ambiguity check — not composition-specific, disproportionate to delete); refusal-resolution ratchet tightened (`GENTLE_AI_REFUSAL_RATCHET_UPDATE=1`, entries from the deleted files retired).

## Phase 7 (S6): Decline Downgrade to Ordinary Unmanaged

Branch `feat/rdd-wave5-s6-decline-downgrade` (chained off S5), commit `74604c3b`, 464 authored lines (529 deleted).

- [x] 7.1 RED->GREEN: `TestCandidateDecline_ZeroCallers` (`internal/cli/review_candidate_decline_downgrade_test.go`) — AST scan for `ResolveCandidateDeclineForGate`/`RecordCandidateDecline` calls across `internal/cli` and `internal/reviewtransaction`. Genuine RED before deletion (found the exact 2 call sites); permanent GREEN after.
- [x] 7.2 `TestCandidateDecline_UnmanagedDelivery_ByteIdenticalToDisabled` — passes, with a disclosed finding: it was **already true before this slice's deletion**, not genuinely RED-then-fixed. S2's kill-switch consultation already runs before ANY authority read, including decline resolution, so the disabled path was already unreachable from the decline branch. Kept as a real, valuable regression guard (same category as S5's `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved`), recorded honestly rather than claimed as a new RED.
- [x] 7.3 Deleted `ResolveCandidateDeclineForGate`, the funnel branch, `emitCandidateDeclinedUnmanagedDelivery`, and the `RecordCandidateDecline` writer + its sole call site. Kept `parseCandidateDeclineAuthorization` read-only, along with its own dependencies (`canonicalCandidateDeclinePayload`, `candidateDeclineDigest`, `CandidateDeclineAuthorization.validate`) — `candidate_decline.go` cut from 358 to ~130 lines. **Deviation, disclosed**: the parser has no production caller today (no `review status` decline-reading feature exists yet, despite the design's forward-looking framing); baselined in the deadcode ratchet with justification (Slice 5's `AssessTargetStatus` precedent), with a new round-trip test proving it still works.
- [x] 7.4 GREEN: both named tests pass, plus 4 pre-existing tests discovered broken by the deletion and migrated (not silently deleted): the S1 characterization pin, `TestRelayedCandidateDeclineAllowsOnlyExactPreCommitDelivery`, `TestCandidateDeclineAllowsExactPrePushAndPrePRButNotLaterCandidate`, and bench journey j50 — all pin the new outcome (a declined candidate denies `receipt_missing`/`receipt-discovery` with reviews on, exactly like any never-reviewed candidate; reaches ordinary unmanaged delivery with reviews off).
- [x] 7.5 Bench journey — folded into the rewritten j50 (`j50-candidate-decline-denies-generically-then-disabled`, `bench/journeys_wave1.go`) rather than a separate `journeys_wave5.go` entry: proves both halves (reviews-on generic denial, reviews-off ordinary unmanaged delivery) in one cohesive story reusing j50's existing, still-valid start/decline fixture mechanics.
- [x] 7.6 Full verification: `go test ./... -count=1` all 63 packages green; bench module green; bench journey corpus vs a freshly built binary: 0 failures; deadcode ratchet baselined 8 entries (4 candidate-decline parser-chain functions + 4 `RepositoryIdentityLease` validation-chain functions whose static reachability shifted under the same deletion despite remaining genuinely called elsewhere — confirmed via direct grep before baselining, not assumed); refusal-resolution ratchet green with zero changes needed; rebase-contract clean (root `7598eda4` unchanged, confirmed still an ancestor of `origin/main` post-Wave-4-merge).

## Phase 8 (S7): Invalidation Verb Deletion (lands LAST — only destructive authority step)

Branch `feat/rdd-wave5-s7-invalidation-verb-deletion` (chained off S6), 404 authored lines (803 deleted, net -399).

- [x] 8.1 RED->GREEN: `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates` (`internal/cli/review_invalidation_verb_deletion_test.go`) — a receipt that would have been `os.Remove`'d under the pre-cutover writer stays present, byte-identical, on disk after a drifted approved candidate denies at every one of the five gates. Confirmed the gate ALREADY denies via `evaluateCompactGate`'s own pre-existing scope/binding checks, entirely independent of and unaffected by this slice's deletion — matching design.md's evidence-correction note ("Wave 5 therefore removes a gate-derived write verb, not a hidden write inside a gate").
- [x] 8.2 `TestPreCutoverInvalidatedRecordsStayReadable` (`internal/reviewtransaction/compact_invalidated_readonly_test.go`) — `StateInvalidated`/`InvalidationEvidence` parse without rewrite. **Disclosed, not a genuine new RED**: this invariant was already true before this slice (design decision 2's own framing: these fields "stay parse-only... so historical records still render" describes existing, not newly-created, behavior) — recorded as a regression guard, matching S6's honest handling of task 7.2.
- [x] 8.3 RED->GREEN: `TestNoGateWritesAuthority_CallAbsenceGuard` (`internal/reviewtransaction/gate_write_guard_test.go`) — AST scan over the gate-evaluation entry-point files (`gate.go`, `compact_gate.go`, `compact_gate_discovery_baseline.go`, `legacy_projection.go`, `native_request.go`, plus `compact_approved_invalidation.go` itself, included so the guard is genuinely RED before deletion) for `acquireStoreLock`/`writeAtomic`/`os.Remove`. Genuine RED before deletion (found the exact 4 call sites in `compact_approved_invalidation.go`); permanent GREEN after (the file no longer exists, and a missing file is treated as zero violations by construction, not an error).
- [x] 8.4 Deleted `compact_approved_invalidation.go` in full (`InvalidateApprovedCompactAuthority`, `CompactApprovedInvalidationRequest`, `HealthyApprovedInvalidationError`, `compactInvalidationTarget*`, `compactInvalidationDenialBound`) and its 403-line test file. Deleted the now-dead `(*CompactState).invalidateApproved` method (`compact.go`, its only caller). Replaced the `review invalidate` approved-authority branch (`review_facade.go`) with an unconditional refusal naming `review validate --gate <gate>` as the runnable alternative — deleted the now-flag-unused `reviewHealthyInvalidationRefusal`/`reviewSuccessorLineagePlaceholder` and 7 now-unused CLI flags (`--base-ref`, `--pre-pr-ci-attestation`, `--policy`, `--release-*`) that only ever fed the deleted re-derivation. `invalidated` is derived (`relation ∈ {changed, unrelated} ⇒ GateInvalidated`, gate.go:1745-1774, confirmed already total and unchanged by this slice). Legacy-v1 `review invalidate` operator branch retains its write, untouched (Wave 7 of the overall roadmap deletes it, not this slice).
- [x] 8.5 Dropped `TestRuntimeSelfRemediationTriangleGuardRailsHold`'s invalidation-verb subtest (its only caller, `internal/sddstatus/runtime_ledger_self_remediation_test.go`); the 3 recovery-guard-rail subtests are untouched.
- [x] 8.6 GREEN: all 3 named tests pass, plus 4 pre-existing tests discovered broken by the deletion and migrated (not silently deleted): the S1 characterization pin (`TestInvalidationVerbCharacterization_InvalidateApprovedCompactAuthority` → `TestInvalidationVerbDowngrade_RefusesInsteadOfDeriving`), `TestReviewInvalidateRefusesHealthyApprovedLineage` → `...RefusesApprovedLineageUnconditionally`, `TestReviewInvalidateReplaysCompletedApprovedInvalidation` → `...RefusalIsIdempotentForApprovedLineage`, and e2e's `TestOrganicHealthyInvalidationRefusalNamesTheWorldAction` → `TestOrganicApprovedInvalidationRefusesAndNamesReviewValidate` (the old refusal named a `review recover` continuation with a successor-name placeholder; the new one names a fully concrete `review validate` command with zero placeholders, proven runnable by actually running it and observing the SAME derived allow the fixture's own healthy-candidate setup already established).
- [x] 8.7 Re-ran the full 35-cell gate-boundary-matrix golden: passes unchanged, zero diff needed — confirms `gateVerdict` was already the sole source of truth for every matrix cell; this slice's deletion changes nothing about it.
- [x] 8.8 `ReceiptPath()` reader audit. **In-repo sweep** (every non-test `.go` call site, `rg -n "ReceiptPath()"`): 24 call sites across `internal/cli`, `internal/reviewtransaction`, `internal/sddstatus`. Classified into three groups, none of which infer "invalidated" from receipt-file absence: (1) content readers (`os.ReadFile(...ReceiptPath())` — the large majority) parse the receipt for evaluation/binding/status-projection purposes, unaffected since receipts are now permanent (strictly safer than before: a receipt these readers expect can no longer disappear underneath them mid-read); (2) terminal-discovery presence gates (`os.Stat(...ReceiptPath())`, `review_facade.go:3632/3657`, `compact_result_reopen.go:217/291`) require a receipt to exist before treating a lineage as terminal — this is "not yet published" detection (an approved-state authority whose receipt was never written), a check unrelated to and unaffected by invalidation, since it was never about receipt REMOVAL; (3) every `StateInvalidated` consumer (`review_next_transition.go`, `status.go`, `review_self_recovery_reason.go`, `target_status.go`, `compact.go`, `compact_abandon.go`, `transaction.go`, `compact_store.go` — 16 sites) reads the persisted `State` field directly, never receipt-file presence, as its invalidation signal — already correct, unaffected by this slice. **No reader found anywhere that treats receipt-file absence as the invalidation signal itself**; nothing needed migrating to `review validate`. **Bundled Pi assets**: none exist within this repository (`gentle-pi` is a separate, external project — not vendored, embedded, or bundled here); this audit leg does not apply to gentle-ai's own repository scope and would need to be swept independently in gentle-pi's own repository if it reads a locally-cached copy of `ReceiptPath()`-shaped signals — out of scope for this slice, flagged rather than silently skipped. **Release-notes line** (this repository's changelog is commit-message-derived via goreleaser, filtering out `docs:`/`test:` prefixes — no manual changelog file exists to edit): the line is carried in this slice's own commit message body, quotable verbatim for the next rc's release notes: *"Receipt files now persist through a derived invalidation; consumers that read file absence as the invalidation signal must read the gate verdict instead."*
- [x] 8.9 Close #2222/#2239 as superseded. **Correction, disclosed**: the Gate Regression Test Index's original "Allow (S4)" row named 5 tests (`Test{PostApply,PreCommit,PrePush,PrePR,Release}Gate_Allow_ExactReceiptGovernsDelivery`) that were never actually implemented under those names — verified by direct search (`rg`, zero matches) before citing them, rather than assuming the plan was realized as written. Corrected supersession evidence, everything independently confirmed passing on this slice's own final `go test ./... -count=1` run (part of 8.10): Disabled (S2, #2222) — `Test{PostApply,PreCommit,PrePush,PrePR,Release}Gate_Disabled_ReportsUnmanagedBeforeAuthorityRead` (5 tests, confirmed green); Deny (S3) — `Test{PostApply,PreCommit,PrePush,Release}Gate_Deny_ChangedRelationCarriesNextStep` + `TestPrePRGate_Deny_BaseMismatchDeniesWithoutComposition` (5 tests, confirmed green); #2239 corroboration (S5) — `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved` (confirmed green). The "allow" leg's evidence is the 35-cell gate-boundary-matrix golden's own wired "exact" cells instead (post-apply/pre-commit/pre-push/pre-pr, S1; release's own "exact" cell remains an explained skip, unchanged by this slice — a pre-existing S1/S4 gap, not something S7 introduces or was asked to close) plus `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates`'s own drifted-denies-cleanly proof for all 5 gates (this slice). Both issues are ready to close as superseded once this PR lands; the maintainer performs the actual GitHub issue-comment/close action outside this slice's own scope (this SDD executor does not have authority to close issues unilaterally).
- [x] 8.10 Full verification: `go test ./... -count=1` all 63 packages green (including the migrated e2e test); bench module green; bench journey corpus vs a freshly built binary: 0 failures; gofmt/vet clean; deadcode ratchet clean with zero new entries (no `--update` needed — the deleted code was live/reachable before deletion, matching S5/S6's own established pattern: deleting live code does not shrink the ratchet's unreachable-function baseline); refusal-resolution ratchet required one grammar fix (a multi-line `fmt.Errorf` with its `refusal:by-design` marker on the closing-paren line, not adjacent to the call itself — collapsed to one line) and one contradiction fix (a message naming a runnable `gentle-ai review validate` continuation cannot ALSO carry a `refusal:by-design` marker, which claims no fix exists — removed the marker, letting the ratchet's own named-resolution detection recognize the command text), both baselined via `GENTLE_AI_REFUSAL_RATCHET_UPDATE=1`; rebase-contract clean (root `7598eda4` unchanged, confirmed still an ancestor of `origin/main`).

## Phase 9: SDD-Attempt Archive Gate Fix (Absorbed W4 Deferral — SDD-Status Domain, Independent of S1-S7)

Not part of the gate-cutover chain (`internal/reviewtransaction`/`internal/cli/review_facade.go`); this phase
touches `internal/sddstatus` and `internal/cli/sdd_attempt.go` only and shares no files with S1-S7, so it may
land on its own base after S7 without reopening the gate-cutover evidence. See absorbed-debt item 4 above for
the full W4 CRITICAL-A citation.

- [x] 9.1 RED: `TestBlockArchiveForUnsatisfiedReVerify_FrozenAnchorDoesNotRelabel` — reproduces the W4
      livelock probe (cycle 1 blocked → compliant remediation → cycle 2 must NOT re-demand a new anchor).
      **Disclosed deviation**: no pre-existing `blockArchiveForUnsatisfiedReVerify` or live-revision-chasing
      demand exists on this base (Wave 4 cycle 3 removed it entirely, restoring `applyTargetedReVerifyRouting`
      to purely-additive — confirmed by reading `review_reverify.go`/`status.go` before writing anything).
      RED is therefore a genuine compile-fail against brand-new pure functions
      (`archiveReVerifyDemanded`, `archiveReVerifySatisfied`), not a red-to-green fix of a livelocking
      implementation. Commit `31799ec2`.
- [x] 9.2 RED: `TestBlockArchiveForUnsatisfiedReVerify_NamedContinuationIsRunnable` — asserts the blocked-reason
      text names a complete, literally-runnable `gentle-ai sdd-attempt finish` invocation with all 8 base
      flags. **Disclosed deviation from this task's own pre-plan**: does NOT name the 3-flag remediation trio.
      Investigated first, per the coordinator's explicit "read the CLI validation source first" instruction
      (`sdd_attempt.go` read in full): the trio's own validation (`validateRuntimeRemediationSuccessor`,
      `runtime_ledger.go`) demands an approved review successor lineage bound via
      `prepareApprovedRuntimeSuccessorBinding` — a full review round trip. That is a heavier, semantically
      distinct axis ("this FAILED runtime attempt's evidence is repaired by an approved successor binding")
      than "re-verify the corrected candidate," and reusing it would have repeated Wave 4's
      unrunnable-continuation defect at one remove — exactly option (a)'s stated risk in the coordinator's own
      preference ordering. The plain, already-existing 8-base-flag `finish` shape is both fully runnable as
      printed (no operator-unknowable values beyond the same `<placeholder>` convention
      `runtimeRemediationExitRefusal` already established) and correct: an ordinary passing finish after the
      correction naturally captures `FinishCandidateTree` equal to the corrected tree, since that is simply
      the operator's current working tree at finish time. This resolves the coordinator's option ordering at
      option (a), using the lighter of two possible "(a)" shapes rather than falling through to (b) — no CLI
      flag, sub-operation, or verb was added at all. Commit `31799ec2`.
- [x] 9.3 Implement: `blockArchiveForUnsatisfiedReVerify` anchors to `CompactCorrectionAttempt`'s own
      `FixDeltaHash`/`Snapshot.CandidateTree` (both written once, at `CompleteCorrection` time in
      `compact.go`, into the append-only `CorrectionAttempts` slice — confirmed by reading `compact.go`
      directly, not assumed). `correctionEvidence` gained `fixDeltaHash`/`candidateTree` fields, populated by
      `deriveCorrectionEvidence` whenever `applied && !failClosed` (S6's existing classification seams,
      reused exactly as the coordinator suggested). `archiveReVerifyContinuation` builds the runnable
      `sdd-attempt begin`-then-`finish` (or `finish` alone when an attempt is already active) text. Wired into
      the ONE existing `applyTargetedReVerifyRouting` call site (both `Resolve()` and `resolveEngramStatus()`
      pass their already-loaded `*RuntimeStatus` through, no new call site). Blocks
      `Status.Dependencies.Archive` only; never Apply, Verify, or any of the five delivery gates
      (`internal/reviewtransaction`'s `gate.go`/`compact_gate.go`, S1-S7's domain, untouched by this phase —
      confirmed zero file overlap). `internal/cli/sdd_attempt.go` ended up untouched: no CLI surface change
      was needed given 9.2's resolution. Commit `31799ec2`.
- [x] 9.4 GREEN: 9.1-9.2 pass, plus 3 additional tests written alongside them for full branch coverage
      (`TestBlockArchiveForUnsatisfiedReVerify_StructuralAbsence`,
      `TestBlockArchiveForUnsatisfiedReVerify_MutatesOnlyArchive` — 5 subtests covering satisfied/nil-runtime/
      no-correction/demanded-and-unsatisfied/blocked-reason-content). Two pre-existing
      `TestDeriveCorrectionEvidenceBranches` fixtures needed updating for the new struct fields (disclosed:
      the "no path data" fixture sets a real `CandidateTree`, so its `want` gained `candidateTree: "deadbeef"`
      — the additive field is not zero-valued there). Mutation proofs (temporarily broken, confirmed caught,
      reverted byte-identically): (1) `archiveReVerifySatisfied`'s tree comparison dropped — NOT caught by the
      original test (its only passed attempt happened to already match the tree), so the test itself was
      strengthened first with a passed-attempt-on-an-unrelated-tree negative case, then the mutation was
      re-proven caught; (2) `blockArchiveForUnsatisfiedReVerify`'s `archiveReVerifyDemanded` guard dropped —
      caught (added a "no correction applied" subtest first, since none existed); (3) the wiring call inside
      `applyTargetedReVerifyRouting` commented out — no unit test exercises the wiring itself (building a full
      protocol-valid on-disk correction round trip was investigated and not pursued, matching this same file's
      own pre-existing "Investigated and NOT pursued" precedent for `TestResolveOmitsReVerifyBlockWithoutAnyCorrection`
      — `CompleteCorrection`'s cross-validation makes a hand-built fixture substantially more machinery than
      this slice's budget); caught instead by `scripts/deadcode-ratchet.sh`, which correctly reported all 4 new
      functions as newly unreachable with the wiring removed. Commit `31799ec2`.
- [x] 9.5 Full verification: `go test ./... -count=1` all 63 packages green; `internal/sddstatus` full package
      green (26s); bench module green; bench journey corpus vs a freshly built binary
      (`go build -trimpath ./cmd/gentle-ai`): 59 journeys completed, 0 failed, 0 unsupported, exit 0 (one
      pre-existing stale-declaration note for j43, dated to S7's invalidate-message change, not this phase —
      informational only, does not fail the run); `bench/results.json` reverted after the local run; gofmt/vet
      clean on all touched files; deadcode ratchet clean with zero new entries (no `--update` needed — the new
      functions are genuinely wired to the one production call site, proven by mutation proof 3 above);
      refusal-resolution notes: none — no `refusal:by-design` marker used anywhere in this phase's new code,
      since the blocked reason always names a runnable continuation; rebase-contract clean (root `7598eda4`
      still an ancestor of `origin/main`, confirmed via `git merge-base --is-ancestor`). This closes Wave 5's
      apply: Phases 1-9 (S1-S7 plus this phase) are all complete. Commit `31799ec2` (code) plus this docs
      commit.

## Fix Cycle 1 (SDD-Verify FAIL, Engram #10186 — 4 CRITICAL, 7 WARNING)

Branch `feat/rdd-wave5-f1-legacy-preconditions`, chained from Phase 9 @ `1f875015`. sdd-verify found
the gate-cutover's own headline promises broken at the product surface: a medium/high v3 candidate could
never obtain an approved receipt, every legacy candidate denied forever at 2 of 5 gates, absorbed N2's
closure claim was untrue on the shipped v3 path, and default-deny was mutation-unpinned for 4 of 7
relations. Full detail: Engram `#10187`.

- [x] **C-B** (legacy real preconditions): `EvaluateLegacyGate` never populated `GateContext.BaseRelationshipValid`
      or `Release`, so legacy denied FOREVER at pre-pr/release even for `exact`. Fixed by deriving
      `BaseRelationshipValid` from the live-vs-projected base-tree comparison (mirroring `EvaluateNativeGate`'s
      own `gate.go:289`) and `Release` from the same caller-supplied artifact locations
      `BuildNativeGateRequest` already uses. `EvaluateLegacyGate`'s signature changes from a bare `GateKind`
      to `NativeGateRequestInput` to carry the release-artifact locations a bare `GateKind` cannot express.
      Mutation-proven (both derivations independently reverted, caught, restored). Commit `e2423787`.
- [x] **W-3** (fold-in, adjacent to C-B): `gateVerdict`'s `compatible_base_advance` exemption from the
      `BaseRelationshipValid` precondition fired at both pre-PR and release; `validateDerivedGate` (the
      function it reproduces) scopes it to pre-PR only. Narrowed to match — a latent fail-open that becomes
      reachable now that C-B routes legacy release evaluations through `gateVerdict` with real precondition
      data. Genuine RED (existing test encoded the bug as intended behavior, corrected), mutation-proven.
      Commit `e2423787`.
- [x] **C-C** (wire `gateVerdict` into the v3 path, the real N2 closure): the v3 gate path
      (`newLineageGateEvaluation`) never called `gateVerdict` at all — it mapped `CoreTransitionContinue`
      straight to `GateAllow` uniformly for every gate. Replaced with `reviewtransaction.EvaluateNewLineageGate`
      (new file `new_lineage_gate.go`), mirroring `EvaluateLegacyGate`'s shape: preconditions populate only
      inside the Continue branch (Collect/Escalate/Approve/Repair/Stop are already non-allow, unaffected).
      `newLineageGateEvaluation` removed from `internal/cli`; its 4 test call sites migrated to the new
      function. Mutation-proven (reverting the `gateVerdict` call to a hardcoded allow is caught by name).
      Full reviewtransaction/cli/sddstatus/e2e suites green. Commit `92682ed1`.
- [x] **C-D** (partial — pin deny verdicts by value) + **W-1** (fold-in): `TestGateVerdict_TotalFunction_35Cells`
      asserted shape only (any of 4 known `GateResult` values, some next step) — flipping
      `provable_contraction`/`unrelated`/`ambiguous`/`unknown` to allow stayed green. Pinned to the exact
      expected `(GateResult, ReasonCode)` per relation via two lookup tables; each of the 4 previously-unpinned
      deny relations independently flipped to `GateAllow` and confirmed caught by name, then reverted. W-1:
      `gateBoundaryMatrixNotWiredReason` still claimed `gateVerdict`/`NativeGateEvaluation.Relation`/legacy
      projection didn't exist — all landed in S3/S4 and are now real (C-B/C-C). Rewritten to the honest
      current reason (fixture not yet built, not mechanism missing); golden regenerated, diff confined to
      reason text only (still 8/35 wired, no verdict/relation changed). Commit `e0a51de3`.
- [x] **C-D remainder / W-2** (deferred, disclosed, then picked up): un-skip the release-gate matrix cells.
      Investigated: the existing "exact" cells all drive the COMPACT/v2 lineage path
      (`approveDiscoveryMarkdown`-style helpers), not legacy v1 or new-lineage v3, so reusing that recipe for
      "release/exact" would not actually exercise either C-B's or C-C's fix. A genuinely new fixture was
      needed — a legacy-specific recipe (tractable now) or a v3 recipe (blocked on C-A, since only tier-low
      could then finalize). Follow-up picked up in Fix Cycle 2's own W-2 entry below (release/exact wired,
      8→9/35) — **still a disclosed partial overall** (release/changed and the other release relations remain
      skips), not fully closed; W-2's final disposition is amended by cycle 3, see below.
- [x] **C-A** (v3 lens-result ingestion — was NOT STARTED, the wave's largest remaining item at this point):
      no production code set `FinalizeAdvanceRequest.CapturedLensResults`; `review capture-result` bound only
      the v2 compact store, and v3's `NewLineageAuthority`/`AuthorityStore` had no persistence primitive for
      captured reviewer results at all — `NewArtifactSubject`/the admission pipeline is tightly coupled to
      `CompactState`. Closed in Fix Cycle 2 (commit `d43870ef`, minimal capture primitive + CLI routing) and
      extended in Fix Cycle 3 (commit `ff3b2a72`, C-E: findings admission) — see both sections below.
- [x] **W-7** (deferred, disclosed, then picked up): the v3 tamper denial named `review finalize --lineage
      <id>`, which re-issues rather than repairs an already-approved lineage's receipt.
      `AuthorityStore.WriteReceipt`'s exact conflict semantics (no-op / refusal / silent overwrite) needed
      investigating before an accurate replacement message could be written. Closed in Fix Cycle 2 (commit
      `59df5f92`) — see below.

Fix cycle 1 verification (covers C-B/W-3/C-C/C-D-partial/W-1, the 3 commits above): `go test ./... -count=1`
all 63 packages green; bench module green; bench journey corpus vs a freshly built binary: 59/59 completed,
0 failed, exit 0; `bench/results.json` reverted; gofmt/vet clean; deadcode ratchet clean; rebase-contract
clean (root `7598eda4` still an ancestor of `origin/main`). Not archivable yet: C-A and its dependents
(W-2's v3 half) remain open. Return to sdd-apply fix cycle 2.

## Fix Cycle 2 (closes the cycle-1 findings: C-A, W-2, W-7)

Branch `feat/rdd-wave5-f2-v3-capture`, chained from fix cycle 1 @ `e0a51de3`. Coordinator design decision
recorded here (design.md amendment): C-A is a MINIMAL capture primitive, not a rebuilt v2-shaped admission
pipeline — that machinery (`ArtifactSubject`/`FrozenCandidateContext`/reopen/journal) is Wave 7 deletion
scope; rebuilding it for v3 would violate the wave's own simplification mandate.

- [x] **C-A** (v3 lens-result capture, the wave's last CRITICAL): new `reviewtransaction`
      `NewLineageAuthority.CapturedResults` field + `AuthorityStore.CaptureLensResult` (existing Mutate/CAS
      path; reviewing/validating only; lens/order must match the frozen `SelectedLenses` set; submitted
      subject hash must match `NewLineageArtifactSubjectHash`'s own derivation; one-shot per lens, no reopen
      — an identical resubmission is `Mutate`'s own idempotent no-op, a different subject hash for an
      already-captured lens is refused). `review capture-result` routes by discovered lineage kind exactly
      like finalize (wave-3 "ln" precedent); `review finalize`'s v3 branch now accepts `--captured-results`.
      **Genuine mid-implementation bug caught and fixed before it shipped**: the first subject-hash design
      bound to the authority's own revision, which changes on every `Mutate` including the capture write
      itself — a self-relabeling bug shaped exactly like Phase 9's own frozen-anchor lesson (an idempotent
      resubmission would have failed to match its own freshly-recomputed subject hash). Caught by
      `TestCaptureLensResult_OneShotNoReopen`'s idempotent-resubmission subtest before any CLI code was
      written against it; fixed by dropping revision from the hash's domain (binds only to the immutable
      `LineageID`/`CandidateIdentity`/lens/order tuple instead). END STATE (coordinator's exact repro):
      medium-tier v3, capture via the real CLI, finalize → approved, all five gates allow
      (`TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates`); tier-low zero-results
      unaffected (`TestReviewFacadeCaptureResultNewLineage_TierLowZeroResultsStillWorks`). Mutation-proven:
      the finalize `capturedLensResults` derivation and the CLI routing wiring were each independently
      disconnected and confirmed to reproduce the ORIGINAL verify-report failure shapes exactly (the same
      `ErrFinalizeRequiresLensResults` refusal and the same "v2 store not found" error), then restored. 3 new
      refusal sites named their runnable `capture-result --preflight` continuation (refusal-resolution
      ratchet). Commit `d43870ef`.
- [x] **W-2** (partial, disclosed): unskipped `release/exact` in the 35-cell matrix (8/35 → 9/35 wired),
      driven through the real binary via the compact/v2 path — the one lineage kind a plain binary-driven
      `review start` can freshly create (confirmed empirically: v1 legacy has no CLI-reachable creation path
      at all — a probe build showed ordinary `review start` always creates a `v2/` compact record, never
      `v1/`; v3 needs `GENTLE_AI_RDD_NEW_LINEAGE` threaded into the matrix harness's subprocess, not pursued
      this budget). Discovered and documented: release's own target resolution is `TargetExactRevision` at
      the current COMMITTED HEAD, not `TargetCurrentChanges` like post-apply/pre-commit — the reviewed
      candidate must be committed before release can see it at all, confirmed by an initial failing attempt
      and fixed by adding the commit step (mutation-proven: removing it reproduces the exact scope-changed
      denial). Deferred, disclosed: `release/changed` needs a genuinely delivered (committed) drift recipe
      mirroring pre-push's amend-based shape, not the plain workspace-drift recipe the other four gates'
      "changed" cells use; the remaining 5 release relations and any legacy(v1)/v3-specific cells stay
      explicit skips. Matrix: **8/35 → 9/35 wired** (before/after, as requested). Commit `f4b47266`.
- [x] **W-7**: investigated `AuthorityStore.WriteReceipt`'s conflict semantics
      (`WriteReceipt` → `publishImmutable` → `publishNoReplace`, `store.go`): a genuinely ABSENT receipt
      (never published) is repaired by a plain finalize replay, but a PRESENT receipt that fails to
      parse/validate or does not match the frozen authority can never be repaired the same way —
      `publishImmutable` refuses to overwrite differing existing bytes
      (`ImmutablePublicationConflictError`) rather than repairing them, so the old shared denial (naming
      `review finalize --lineage <id>`) named a continuation that would itself refuse. Split the single
      `approved_without_receipt` denial into two: the genuinely-absent case keeps naming finalize (still
      honest), and a new `approved_receipt_corrupt` case for a present-but-invalid receipt names the only
      continuation that actually clears it — a fresh lineage via `review start`, since v3 has no
      reopen/repair machinery for a corrupted receipt (the same "no reopen" boundary C-A's
      `CaptureLensResult` enforces). Mutation-proven: collapsing the two cases back into one is caught by
      the new test naming the exact wrong denial code. Commit `59df5f92`.

Fix cycle 2 verification (covers C-A/W-2/W-7, the 3 commits above): `go test ./... -count=1` all 63 packages
green; bench module green; bench journey corpus vs a freshly built binary: 59/59 completed, 0 failed, exit 0;
`bench/results.json` reverted; gofmt/vet clean; deadcode ratchet clean; refusal-resolution ratchet clean;
rebase-contract clean (root `7598eda4` still an ancestor of `origin/main`). **This closes every CRITICAL and
every fold-in warning the cycle-1 verify named** (C-A, C-B, C-C, C-D-core, W-1, W-3, W-7 all closed; W-2
partial and disclosed — the v3/legacy halves of release-cell unskipping remain a genuine, scoped follow-up,
not a blocking gap in any of the four CRITICALs). Return to sdd-verify for re-verification.

## Fix Cycle 3 (closing pass: C-E, W-8, W-5/W-6, W-2 amendment — the wave's final fix cycle)

Branch `feat/rdd-wave5-f3-findings-admission`, chained from fix cycle 2 @ `8e5f287a`. sdd-verify's cycle-2
report (Engram `#10186`) found 1 NEW CRITICAL that fix cycle 2's own capture primitive introduced, plus 5
WARNING (2 new, 3 carried). Full detail: Engram `#10186`/`#10188`.

- [x] **C-E** (the blocker, coordinator decision "option 2, the channel does what it appears to do"): the v3
      capture channel required and validated a reviewer result's `findings`/`evidence`, then discarded the
      findings — `CaptureLensResult` persisted only `{Lens, Order, SubjectHash}`. A/B on an identical
      candidate with an identical BLOCKER (deterministic, candidate-introduced): v2 returned
      `correction_required`; v3 issued `approved`. `NewLineageCapturedResult` gains a
      `Findings []FindingEvidence` field, persisted verbatim (one-shot semantics extended: a resubmission is
      idempotent only when BOTH subject hash AND findings match). New
      `NewLineageAuthority.CapturedFindingEvidence()` flattens every captured lens's findings into the exact
      shape the EXISTING `AdmitCandidateCausalFindings` already consumes — reused, not duplicated, the same
      admission decision `--admission-findings` already drives. `--admission-findings` stays an explicit
      override: when supplied it is used verbatim exactly as before; captured findings are consulted only
      when it is absent. Genuine RED = the verify's exact A/B repro, inverted
      (`TestReviewFacadeCaptureResultNewLineage_CandidateCausalBlockerEscalates`), plus a non-causal-finding
      companion proving the fix does not over-block
      (`TestReviewFacadeCaptureResultNewLineage_NonCausalFindingDoesNotBlock`). Mutation-proven: both halves
      of the fix (findings persistence in `CaptureLensResult`, and the finalize-side
      `CapturedFindingEvidence()` wiring) were independently reverted and each confirmed to reproduce the
      ORIGINAL CRITICAL-E fail-open exactly, then restored; a dedicated unit test also proves one-shot now
      covers findings, not just the subject hash. Commit `ff3b2a72`.
- [x] **W-8**: a partial capture (some, not all, frozen selected lenses captured) reached finalize's
      `ReviewCore` error path unclassified (`fmt.Errorf` wrap of `ErrFinalizeRequiresLensResults`), which the
      outer dispatcher treated as a tool-internal fault and wrote a defect report for — an entirely ordinary,
      expected incomplete state. New `NewLineageAuthority.MissingCapturedLensNames()` names exactly which
      frozen selected lenses remain uncaptured; the finalize error path now recognizes
      `ErrFinalizeRequiresLensResults` specifically and wraps it as an ordinary `reviewPreflightError` (the
      same classification every other operator-actionable refusal in this package already uses), naming the
      missing lenses and the runnable `capture-result` continuation. Mutation-proven: disabling the
      classification branch reproduces the exact unclassified `*fmt.wrapError` shape the verify report found,
      then restored. Commit `a793e02c`.
- [x] **W-5**: the Gate Regression Test Index's "Allow (S4)" row named 5 tests
      (`Test{PostApply,PreCommit,PrePush,PrePR,Release}Gate_Allow_ExactReceiptGovernsDelivery`) that were
      never implemented under those names. Task 8.9 already disclosed and corrected this once (against the
      superseded-issue evidence); the index itself was never fixed. Re-verified via direct search (S7
      precedent: zero `rg` matches across the whole repo) before citing, then replaced the phantom row with
      what genuinely proves "allow" at all five gates today: the matrix's own wired "exact"/`release/exact`
      cells, `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates` (S7), and the two fix-cycle
      additions that independently drive real allow at all five gates through production code —
      `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` (C-B) and
      `TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates` (C-A).
- [x] **W-6**: stale checkboxes reconciled. Fix Cycle 1's own deferral entries for C-A, C-D-remainder/W-2, and
      W-7 stayed `[ ]` even though Fix Cycle 2 (and this cycle) closed them — flipped to `[x]` above, each
      pointing at the commit(s) that closed it, with W-2 explicitly still marked as a disclosed partial
      overall (not falsely closed). Task 1.4 (archive Wave 4) investigated directly: confirmed via directory
      check that Wave 4 HAS been archived on `main` (`openspec/changes/archive/2026-08-03-rdd-root-simplification-wave4/`
      exists there), but by a separate commit this branch's own history does not yet contain — the checkbox
      stays honestly unchecked (archiving is not an action this branch's own commits perform) with a note
      explaining why, rather than being marked done for an action never taken on this branch.
- [x] **W-2** (amendment, not code): per the coordinator's explicit decision (fix cycle 3 instruction, item 4,
      cited in the spec amendment itself), amended
      `specs/rdd-new-lineage-activation/spec.md`'s "Outcome equivalence proven by matrix, not byte-diff"
      scenario — the letter (proof via the 35-cell matrix specifically) is not yet satisfied (9/35 wired, all
      compact/v2), but the INTENT (proven outcome equivalence for a legacy candidate across all five gates)
      IS satisfied today by `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` and its siblings, which the
      amendment now cites as the accepted proof while the matrix remains the incremental long-term vehicle,
      its wired-cell count tracked here (8/35 at Fix Cycle 1 → 9/35 at Fix Cycle 2's W-2 entry).

Fix cycle 3 verification (covers C-E/W-8, the 2 code commits above, plus the W-5/W-6/W-2 docs-only commit):
`go test ./... -count=1` all 63 packages green; bench module green; bench journey corpus vs a freshly built
binary: 59/59 completed, 0 failed, exit 0; `bench/results.json` reverted; gofmt/vet clean; deadcode ratchet
clean; refusal-resolution ratchet clean; rebase-contract clean (root `7598eda4` still an ancestor of
`origin/main`). **This is the wave's final fix cycle**: every CRITICAL (C-A through C-E) and every WARNING
the verify cycles named (W-1 through W-8) is either fully closed or an explicitly-accepted, cited amendment
(W-2). Return to sdd-verify for final re-verification.
