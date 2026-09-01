# Archive Report: RDD Root Simplification — Wave 5 (Gate Cutover)

**Change**: `rdd-root-simplification-wave5`
**Archived**: 2026-08-04 → `openspec/changes/archive/2026-08-04-rdd-root-simplification-wave5/`
**Store**: hybrid (filesystem + Engram)
**Status**: CLOSED — verified `pass_with_warnings`, archived clean.

## What Shipped

Gate cutover converting the Wave 3 additive branch into the single, unconditional read-only evaluation path for all five delivery gates (`post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`) across every lineage kind (legacy v1, compact v2, new v3):

- Gate-derived invalidation write (`InvalidateApprovedCompactAuthority`) deleted; `invalidated` is now fully derived from the relation algebra, never a new stored write.
- Pre-PR receipt-graph composition (`EvaluateCompactPrePRChain`, `compact_chain.go`) deleted; pre-PR denies on the same relation-derived path as the other four gates.
- Candidate-decline delivery authorization (`ResolveCandidateDeclineForGate`, `candidate_decline.go` resolver/writer) deleted; decline downgrades permanently to ordinary unmanaged delivery.
- `NativeGateEvaluation` gained additive `Relation`/`Next` fields; `gateVerdict(gate, relation, context)` is a total function proven mutation-pinned across all 35 relation × gate cells.
- Legacy lineages evaluate through the shared algebra via read-only `projectLegacyAuthority`/`EvaluateLegacyGate` — zero in-place rewrite, byte-identical stored bytes proven before/after.
- New-lineage (v3) reviewer findings now genuinely bind: `NewLineageAuthority.CapturedFindingEvidence()` feeds the existing `AdmitCandidateCausalFindings`, closing the fail-open where a captured BLOCKER was silently discarded.
- 35-cell gate boundary matrix golden extends Wave 1's differential matrix from relations to gate verdicts (9/35 wired by close, zero unexplained divergences; pre-PR's `compatible_base_advance`/`changed` cells pinned as named, explained divergences).

**Delivery chain**: PR chain #2370–#2383 (S1–S7 + Phase 9 + three fix cycles), boundary/tracker PR #2390, landed on `main` at commit `e599c679` (confirmed: HEAD = main WITH Wave 5 merged, per orchestrator launch-prompt final-state fact — this outranks any earlier in-flight/worktree state recorded in intermediate snapshots).

## Verification Summary — 3 Cycles, 4→1→0 Critical Convergence

| Cycle | Verdict | Criticals | Requirements | Scenarios |
|---|---|---|---|---|
| 1 | FAIL | 4 (C-A, C-B, C-C, C-D) | 12/17 | 21/26 |
| 2 | FAIL | 1 (C-E, new) | 16/17 | 25/26 |
| 3 (final) | **PASS WITH WARNINGS** | **0** | **17/17** | **26/26** |

- **Cycle 1** (candidate `1f875015`): C-A (v3 medium/high candidates permanently undeliverable — no lens-result ingestion path), C-B (legacy denied forever at pre-pr/release — missing `BaseRelationshipValid`/`Release` derivation), C-C (absorbed N2 closure claim untrue at the product surface — v3 gate path never called `gateVerdict`), C-D (default-deny mutation-unpinned for 4 of 7 relations). Fix cycle 1 closed C-B, C-C, C-D-core, W-1, W-3 with mutation proof.
- **Cycle 2** (candidate `8e5f287a`): all four cycle-1 criticals re-proven closed by independent A/B repro against a freshly built binary. One NEW critical, C-E — fix cycle 2's own v3 capture primitive validated reviewer findings then silently discarded them, so a candidate-causal BLOCKER that v2 blocked (`correction_required`) issued an `approved` receipt on v3. Fix cycle 3 closed C-E via `NewLineageCapturedResult.Findings` + `CapturedFindingEvidence()` reuse of the existing admission function.
- **Cycle 3, final** (candidate `63c0583a`): C-E closed and mutation-proven (stripping the admission-wiring half reproduces the exact original fail-open and fails the named test by name). W-8 closed (partial capture now an ordinary `reviewPreflightError`, continuation executed to completion by the verifier). W-2 amendment accepted (letter/intent split: matrix is 9/35 wired, all compact/v2; intent — legacy outcome equivalence at all five gates — proven by `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` and siblings, independently read and confirmed by the verifier, not just trusted by name). W-4 closed as a side effect (one-shot conflict now CLI-reachable). W-5/W-6 stale-index and stale-checkbox cleanup verified honest.

**A/B parity proof (C-A through C-E)**: every critical was closed by the verifier independently re-running its own original repro against a freshly rebuilt binary at the new candidate — never by trusting the fix-cycle's own claim. C-E's closure additionally carries the verifier's own mutation strip (removing the admission-wiring branch reproduces the exact pre-fix fail-open and fails `TestReviewFacadeCaptureResultNewLineage_CandidateCausalBlockerEscalates` by name), the strongest evidence class available.

## Carry-Forwards (Non-Blocking, Recorded for Future Waves)

These do not block this archive — verify-report cycle 3 explicitly rules none of them CRITICAL — but are recorded here so they are not silently lost.

### W-9 / W-10 / W-11 — v3 admission-strictness divergences: MANDATORY pre-v3-activation conditions for Wave 7

Newly raised in cycle 3's adversarial pass over `8e5f287a..63c0583a`. All three are v2-vs-v3 divergences on the *new, activation-gated* lineage (behind `GENTLE_AI_RDD_NEW_LINEAGE`, default off), each traced to `AdmitCandidateCausalFindings`' documented, spec-cited routing that the coordinator's own design decision directed the fix cycle to reuse rather than replace:

- **W-9** (fail-open): `causal_disposition: unknown` on a severe finding approves on v3, escalates on v2. Contradicts the root orchestration contract's "unknown escalates."
- **W-10** (fail-open, **sharpest and cheapest to fix**): the v3 capture path accepts a `BLOCKER` with no `evidence_class`/`causal_disposition` — which `review schema reviewer`'s own published schema forbids and which v2 refuses at capture time — and then approves it. Confined to code fix cycle 3 touched (`newLineageCapturedFindings` + the v3 capture validator); closed by mirroring v2's existing capture-time refusal (suggestion S-7).
- **W-11** (fail-closed): a `WARNING`-severity finding with a candidate-causal disposition escalates on v3 but stays non-blocking (`validating`) on v2 — `AdmitCandidateCausalFindings` never consults severity, so v3 over-blocks against the root contract's "WARNING/SUGGESTION remain info."

**Disposition**: recorded as **MANDATORY conditions to resolve before v3 activation ships** (i.e., before `GENTLE_AI_RDD_NEW_LINEAGE` is removed/defaulted-on in Wave 7), per the verify-report's own closing recommendation ("I would take it before v3 activation ships, not before this wave archives"). W-10 is explicitly the sharpest and cheapest of the three and should be taken first. None blocks this archive: v3 stays behind its activation switch, the shipped default v2 path is unaffected and correct on all three, and none is a regression relative to cycle 2 (where all v3 findings were dropped, not selectively admitted).

### 8.5 — OpenCode plugin relaunch-bound-loss replacement: re-deferred to Wave 7

Originally a Wave 4 deferral. Wave 5's own coordinator decision (tasks.md, "Absorbed from Wave 3/4 Verification") re-deferred it explicitly to Wave 7 on scope grounds: the OpenCode plugin surface is adapter territory (`Out of scope: adapter changes (W4)` per proposal.md), and Wave 5's File Changes list touches zero adapter/plugin files — the five gates' cutover has zero overlap with the plugin's relaunch-bound-loss surface. Forcing it into Wave 5 would have mixed gate-cutover evidence with unrelated adapter evidence in the same PR chain, which design.md decision 8's own rationale rejects. Not dropped — tracked for Wave 7 planning.

### #2222 / #2239 supersession — CLOSED with named per-gate tests

Both issues are closed as superseded, not merged first, per the ratified condition ("close as superseded once this ordering is proven by a named per-gate regression test," maintainer 2026-08-02). Evidence shipped and independently confirmed by the verifier on the final candidate's own `go test ./... -count=1` run:

- **#2222** (disabled short-circuit): `Test{PostApply,PreCommit,PrePush,PrePR,Release}Gate_Disabled_ReportsUnmanagedBeforeAuthorityRead` (5 named tests, S2) — kill switch consulted exactly once, before any authority read, mutation-proven in both directions (stripping the check fails 4 tests; adding a second call site fails the AST guard by exact count).
- **#2239** (kill switch before pre-PR composition): `TestPrePRGate_KillSwitchBeforeComposition_TriviallyPreserved` (S5) — trivially preserved since pre-PR chain composition no longer exists to race against.
- The "allow" leg (originally a phantom test-name row in the Gate Regression Test Index, corrected in fix cycle 3's W-5) is proven instead by the 35-cell matrix's own wired "exact" cells plus `TestReceiptFilePersistsAfterDerivedInvalidation_AllFiveGates` (S7) plus `TestEvaluateLegacyGateAllowsExactAtAllFiveGates` (legacy, C-B) and `TestReviewFacadeCaptureResultNewLineage_MediumTierFinalizeAllowsAllFiveGates` (v3, C-A).

The maintainer performs the actual GitHub issue-comment/close action outside this SDD change's own scope.

### Task 1.4 — Wave 4 archival checkbox: honestly unchecked, not a Wave 5 blocker

`tasks.md` task 1.4 ("Archive Wave 4") remains `[ ]` in the archived copy, by design, not oversight. Confirmed directly (fix cycle 3, W-6): Wave 4 **has** been archived on `main` (`openspec/changes/archive/2026-08-03-rdd-root-simplification-wave4/`), but by a separate commit this branch's own history never contained — this worktree's `feat/rdd-wave5-*` chain branched from a Wave 4 tip that predates that archival commit. The checkbox correctly stays unchecked for an action this SDD change's own commits never performed, rather than being falsely marked done. Per this archive's own launch-prompt final-state fact, Wave 4 is already archived on `main` independently of this change, so no follow-up action is required from Wave 5's own scope.

## Spec Merge Inventory (openspec/specs/, hybrid mode)

| Domain | Action | Details |
|---|---|---|
| `rdd-receipt-only-gates` | **Created** (new capability) | 7 requirements, 12 scenarios — full spec copied verbatim, no prior main spec existed |
| `rdd-delivery-exception-removal` | **Created** (new capability) | 4 requirements, 4 scenarios — full spec copied verbatim, no prior main spec existed |
| `rdd-review-core-transitions` | **Updated** (ADDED) | +1 requirement (`validate` Is The Single Governing Path For Legacy Lineages, 2 scenarios) appended; all pre-existing requirements preserved unchanged |
| `rdd-candidate-relation-algebra` | **Updated** (ADDED) | +2 requirements (Gate Boundary Descriptor Is A First-Class Algebra Input; Verdict Is A Total Function Of Relation × Gate, 3 scenarios total) appended; all pre-existing requirements preserved unchanged |
| `rdd-new-lineage-activation` | **Updated** (MODIFIED, replace-in-place) | 3 requirements replaced by name-matched delta: "Coexistence Precedence Matrix (Amendment C)" → "Unconditional Receipt Precedence (Amendment C Generalized)"; "Additive Gate Branch, Switch-Off Byte-Equivalence, Not a Cutover" → "Cutover Replaces The Additive Gate Branch" (carries the W-2 amendment block verbatim); "Rollback Disables New Starts Only" → "Rollback Restores The Additive Branch, Never Invalidation Writes". Non-Wave-5 requirements ("Distinct Env Switch...", "Kill-Switch-Off Is Structurally Unfailable...") preserved unchanged. |

All 5 delta specs merged from `openspec/changes/rdd-root-simplification-wave5/specs/**` (on-disk, post-ratification/post-amendment versions — RATIFIED/AUDIT-GATED markers and the W-2 amendment block, which supersede the earlier "Assumption, pending maintainer confirmation" tags recorded in the Engram spec artifact).

## Per-File Table (source | archived)

| File | Source lines | Archived lines | Method |
|---|---|---|---|
| `proposal.md` | 97 | 97 | Full Read → Write |
| `design.md` | 136 | 136 | Full Read → Write |
| `tasks.md` | 543 | 543 (verified: line 543 = final line in both) | Full Read (3 chunks, offset-verified) → Write |
| `verify-report.md` | 691 | 691 (verified: line 691 = final line in both) | Full Read (2 chunks) → Write |
| `specs/rdd-receipt-only-gates/spec.md` | 108 | 108 | Full Read → Write |
| `specs/rdd-delivery-exception-removal/spec.md` | 47 | 47 | Full Read → Write |
| `specs/rdd-review-core-transitions/spec.md` | 21 | 21 | Full Read → Write |
| `specs/rdd-candidate-relation-algebra/spec.md` | 23 | 23 | Full Read → Write |
| `specs/rdd-new-lineage-activation/spec.md` | 75 | 75 | Full Read → Write |

Every file copied via full Read then full Write — no summarization, rephrasing, or reconstruction. `tasks.md` and `verify-report.md` end-of-file line numbers were independently re-read post-write to confirm parity (543 and 691 respectively, matching source).

## Task Completion Gate

All implementation tasks across Phases 1–9 and all three fix cycles are checked `[x]`, with one deliberate, explained exception: task 1.4 (see "Carry-Forwards" above) — not a Wave 5 implementation task, an external archival-tracking checkbox for a different change that was independently satisfied outside this branch's own history. No reconciliation was needed or performed; this is the persisted state as written by `sdd-apply`.

## Engram Observation IDs (traceability)

- Proposal: `#10139`
- Delta specs: `#10143`
- Design (corrective re-run): `#10144`
- Tasks (final state, 13 revisions): `#10160`
- Verify-report (cycle 3 final, 3 revisions, cycles 1–2 preserved as appendices): `#10186`
- This archive report: see topic key `sdd/rdd-root-simplification-wave5/archive-report`

## Source Directory

`openspec/changes/rdd-root-simplification-wave5/` — all 9 files copied byte-complete to this archive folder. Source directory removal is a shell-side operation outside this agent's toolset; the orchestrator must delete `openspec/changes/rdd-root-simplification-wave5/` to complete the move.
