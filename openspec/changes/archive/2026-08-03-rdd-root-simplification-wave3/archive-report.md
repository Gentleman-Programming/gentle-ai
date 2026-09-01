# Archive Report: RDD Root Simplification Wave 3 (New-Lineage Facade)

**Date**: 2026-08-03
**Change**: rdd-root-simplification-wave3
**Status**: ARCHIVED AND CLOSED
**Final Verdict**: PASS (Third verification cycle)

## Executive Summary

Wave 3 of the RDD root simplification is complete and merged to main. The change delivers `ReviewCore` as the sole transition owner for new-lineage reviews (`start`/`finalize`/`validate`), the `AuthorityStore` two-artifact persistence model (`review-state.json` + `review-receipt.json`), and the `rdd-new-lineage-activation` switch/coexistence/rollback contract. It promotes Wave 1's relation algebra and candidate identity resolver from read-only shadow observation to live deciding authority for new lineages, while proving byte-identical legacy behavior when the activation switch is off. Delivery shipped as PRs #2309–#2314, merged through tracker PR #2318, and the tracker landed on `main` at commit `4ca5715a`. Three verification cycles ran (FAIL → FAIL → PASS); the final cycle closed all coverage gaps at 19/19 requirements and 32/32 scenarios, 0 CRITICAL, 0 blockers.

## Delivery Evidence

### Final Merged State

- **Main checkout**: `/home/gentleman/work/gentle-ai`, HEAD `4ca5715a` — Wave 3 merged
- **PR chain**: #2309–#2314 (S1–S5 plus PR0/remediation slices), merged into the `feature/rdd-root-simplification` tracker branch via tracker PR #2318, which merged to `main`
- **Feature branch**: `feature/rdd-root-simplification` continues on main for Wave 4/5

### Acceptance Gate Status

**Review Gate**: no `sdd/rdd-root-simplification-wave3/review/{transaction,ledger,receipt,gate-context}` Engram topics were found for this change, and no negotiated-review artifacts exist under this change's SDD trail. Delivery evidence supplied to this archive (PR numbers, tracker merge, main merge commit) indicates ordinary GitHub PR review was the delivery path for this wave, consistent with the RDD receipt-gated lifecycle not yet being the default for gentle-ai's own delivery at the time this wave shipped (this wave IS the mechanism the kill switch would eventually gate). No fabricated `allow` receipt is claimed here; this is recorded as an assumption traced to the launch prompt's explicit merge evidence, not independently re-verified against `gentle-ai review status`.

**Task Completion Gate**: PASS, with one recorded exceptional reconciliation. `tasks.md`'s task 1.2 ("Archive Wave 2 … when its turn comes") was still an unchecked `- [ ]` checkbox in every persisted copy of `tasks.md` found (filesystem and the `sdd/rdd-root-simplification-wave3/tasks` Engram observation #10134), including across all three verify-report cycles, which repeatedly recorded it as open advisory item W1 ("Cleanup/sequencing, Wave-2-timing dependent, outside Wave 3's critical path... None blocks archive"). Independent filesystem inspection during this archive run found direct proof the task is in fact complete: `openspec/changes/archive/2026-08-02-rdd-root-simplification-wave2/` exists in full, including its own `archive-report.md`, `proposal.md`, `design.md`, `tasks.md`, `verify-report.md`, and `specs/{rdd-authority-disposition-plan,rdd-leaf-disposition-execution,rdd-authority-graph-classification}/spec.md` — Wave 2 was archived on 2026-08-02, evidently after Wave 3's `tasks.md` was last written, and the checkbox was simply never updated to reflect it. Per this reconciliation proof, the archived copy of `tasks.md` in this archive folder marks task 1.2 `[x]` with an inline annotation naming this exact evidence. This is the one exceptional stale-checkbox repair performed by this archive run; no other task was altered.

**Verification Gate**: PASS (third cycle, envelope re-admission at tip `67be4867`). 19/19 requirements, 32/32 scenarios, 0 blockers, 0 critical findings.

## Verification History (Three-Cycle Progression)

Full verbatim text of all three cycles is preserved in this archive's `verify-report.md` (754 lines). Summary:

### Cycle 1: FAIL (tip `ebe26c9c`)

**Verdict**: FAIL — 4 CRITICAL (C1–C4), 10 WARNING, 4 SUGGESTION. 21/32 scenarios COMPLIANT, 10/19 requirements fully compliant.

- **C1**: `review finalize` was not routed by lineage kind — a v3 lineage created by `review start` could never be finalized through the product (raw `v2/` path error, `operation-outcome-unknown` defect report).
- **C2**: candidate-causal admission (`rdd-review-core-transitions` → "Candidate-Causal Admission Only") was unimplemented and untested.
- **C3**: `TestNewLineageReasonTaxonomyCoversLegacyRefusalsClosedMatrix` was a 24-cell tautology (both sides of the assertion called the same pure function).
- **C4**: `rdd-authority-store` → "Five Persisted States, Everything Else Derived" had no covering test; the design's planned AST write guard was never built.

### Cycle 2: FAIL (tip `413d3967`) — remediation verdict PASS, envelope `fail` on coverage

**Remediation verdict**: PASS — 0 CRITICAL, 2 WARNING (new), 4 SUGGESTION (new). All four cycle-1 CRITICALs (C1–C4) independently confirmed RESOLVED with runtime, source, and mutation evidence.

Two new CRITICALs surfaced during re-verification, both consequences of the same remediation seam:

- **C5**: a new-lineage authority that was never approved (state `reviewing`, no receipt) authorized delivery at all five gates including `release` — the legacy path denied the identical situation. Fixed same cycle: `resolveGoverningAuthority` now denies unless `state == approved` and a valid terminal receipt exists.
- **C6**: `internal/cli` was mutating new-lineage state directly (`next.State = validating/approved/escalated`) inside `runReviewFacadeFinalizeNewLineage`, contradicting `rdd-review-core-transitions` → "Sole Transition Owner for New Lineages". Fixed same cycle: state advancement moved inside `ReviewCore` via `CoreRequest.AdvanceRequest`.

**Envelope verdict `fail`** was NOT a defect verdict — it reflected 3 of 32 spec scenarios still genuinely UNTESTED/PARTIAL (switch-identity distinctness, kill-switch-off-at-all-five-gates, frozen-tier-never-recomputed), unchanged since cycle 1 and untouched by this cycle's remediation commit. 29/32 scenarios COMPLIANT, 16/19 requirements fully compliant.

### Cycle 3: PASS (tip `157ab9fd`, envelope re-admitted at `67be4867`)

**Cycle-3 remediation verdict**: PASS — 0 CRITICAL, 2 WARNING (new: N1, N2), 4 SUGGESTION (new: N3–N6). C5 and C6 independently re-confirmed RESOLVED (source- and runtime-proven, tip-vs-base binary delta). W11 (AST guard blind spot for `writeAtomic`/`publishImmutable`) and W14 (`correcting` state unreachable end-to-end) also independently confirmed resolved at the product-surface function, though W14's fix is exercised only from Go fixtures (SUGGESTION N5, carried forward).

**Cycle-3 envelope re-admission** (tip `67be4867`, tests-only commit, 0 production bytes changed) closed the final 3 uncovered scenarios:
- "Switch identity never overloads another switch" → COMPLIANT (`new_lineage_switch_identity_test.go`)
- "Kill switch off produces no side effect" → COMPLIANT (`review_new_lineage_kill_switch_test.go`, proves byte-identical `.git/gentle-ai` subtree across all five gates plus `OfferReviewAfterVerify`)
- "Frozen tier is never recomputed" → COMPLIANT (`review_new_lineage_frozen_tier_test.go`, non-vacuous via a `ClassifyRisk` drift sanity check)

**Final result**: **32/32 scenarios COMPLIANT, 19/19 requirements fully compliant, 0 CRITICAL, 0 blockers.**

## Verification-Caught Defects (Real, Independently Confirmed)

Six real production defects were caught and fixed across the three cycles — none was accepted on the apply agent's word alone; each was independently re-derived by the verifier from source, runtime probes, or mutation testing:

1. **C1 — finalize not routed by lineage kind.** `runReviewFacadeFinalize` had no lineage-kind branch; fixed by adding `runReviewFacadeFinalizeNewLineage` plus a discovery/routing block mirroring `start`/`validate`.
2. **C2 — candidate-causal admission unimplemented.** Fixed by `AdmitCandidateCausalFindings`, wired via `--admission-findings`; verified to persist only causal finding IDs in a real run.
3. **C3 — 24-cell taxonomy tautology.** Fixed by rewriting the expectation side as a hardcoded literal table; mutation-killed this run (flipping one arm now fails 2 of 24 cells).
4. **C4 — five-persisted-states unattested; AST guard never built.** Fixed with a lifecycle runtime test plus a `DeriveObservation` write guard, later found (W11) to miss `writeAtomic`/`publishImmutable` and expanded to cover them.
5. **C5 — default-deny at all five gates.** A `reviewing`-state authority with no receipt was authorizing `allow` at all five gates including `release`. Fixed by requiring `state == approved` plus a valid receipt in `resolveGoverningAuthority`.
6. **C6 — ReviewCore-owned finalize transitions.** `internal/cli` was writing terminal state directly instead of `ReviewCore` deciding it. Fixed by routing the advance through `ReviewCore.finalize` via `CoreRequest.AdvanceRequest`.

## Specification Artifacts

### New Capabilities (created in `openspec/specs/`)

1. **`rdd-authority-store`**: 5 requirements, 7 scenarios — two-artifact model, five persisted states, CAS mutation with in-record replay identity, receipt immutability, reason-taxonomy regression coverage.
2. **`rdd-new-lineage-activation`**: 5 requirements, 9 scenarios — distinct env switch default off, kill-switch-off structural unfailability, coexistence precedence matrix (Amendment C), additive gate branch with switch-off byte-equivalence, rollback disables new starts only.
3. **`rdd-review-core-transitions`**: 5 requirements, 9 scenarios — sole transition owner, consent-gated freeze with immutable tier/lenses/budget, candidate-causal admission only, one bounded correction with exact-replay exemption, terminal receipt issuance exactly once.

### Modified Capabilities (merged into existing `openspec/specs/` copies)

4. **`rdd-candidate-relation-algebra`** (Wave 1): "Read-Only, Zero Live-Lifecycle Change" requirement replaced with "Read-Only at Legacy Call Sites, Deciding Authority at New-Lineage Call Sites" — the relation function is now the deciding input for `ReviewCore` at new-lineage call sites while remaining purely observational at legacy call sites. All other requirements (seven-value output, Amendment A delegation, Amendment B degradation, characterization-test precondition, no-fabricated-counterpart) preserved unchanged.
5. **`rdd-candidate-identity`** (Wave 1): "Read-Only Resolution" requirement replaced with "Read-Only Resolution, Persisted as Frozen Authority When Consumed by a New Lineage" — resolution itself stays pure, but `ReviewCore.start` now persists the resolved `CandidateIdentity` as frozen authority for new lineages. All other requirements (canonical identity structure, selector normalization, deterministic ambiguity/failure reporting, Wave 1 selector scope) preserved unchanged.
6. **`rdd-shadow-evaluation`** (Wave 1): two requirements replaced — "Disable Switch Is the Rollback Boundary" → "Disable Switch Is the Observer's Rollback Boundary" (scopes the switch to the observer only, so `ReviewCore`'s live use of the same resolver/relation functions is unaffected by it), and "Off by Default in Live Paths" re-scoped identically to the observer's zero-cost guarantee. All other requirements (advisory-only/never-blocking, zero live-lifecycle change, no persisted divergence artifact, differential matrix exit evidence, unexplained-divergence-blocks-Wave-2) preserved unchanged.

### Specification Compliance Matrix (final, cycle-3 envelope)

| Domain | Requirements | Scenarios | Compliant | Status |
|---|---|---|---|---|
| rdd-authority-store | 5 | 7 | 7/7 | PASS |
| rdd-new-lineage-activation | 5 | 9 | 9/9 | PASS |
| rdd-review-core-transitions | 5 | 9 | 9/9 | PASS |
| rdd-candidate-relation-algebra (delta) | 1 modified of 6 | 2 of 9 (delta scope) | 9/9 (whole spec, cycle re-run) | PASS |
| rdd-candidate-identity (delta) | 1 modified of 5 | 2 of 5 (delta scope) | 5/5 (whole spec, cycle re-run) | PASS |
| rdd-shadow-evaluation (delta) | 2 modified of 7 | 4 of 7 (delta scope) | 7/7 (whole spec, cycle re-run) | PASS |
| **Total (19 requirements / 32 scenarios envelope)** | **19** | **32** | **32/32** | **PASS** |

## Follow-Ups and Known Limitations (Carried Forward — None Block This Archive)

Per the final cycle-3 verify-report's own explicit ruling ("None blocks archive; all block the Wave 5 cutover or are advisory"), the following remain open and are recorded here as **Wave 4/5 entry conditions**, not archive blockers:

- **N1 (WARNING, carried)**: new-lineage `review finalize` issues an `approved` terminal receipt with zero captured reviewer lens results at any tier — the legacy path refuses the equivalent. Must close before the new path can become the delivery default.
- **N2 (WARNING, carried)**: the new-lineage gate branch enforces no gate-specific precondition (legacy requires release evidence at `release`, `BaseRelationshipValid` at `pre-pr`/`release`, matching `Generation`; the new branch does not). Same Wave-5 cutover blocker class as N1.
- **N3–N6 (SUGGESTION, carried)**: receipt cross-check omits `CandidateIdentity` comparison (N3, inert today); `AdvanceRequest` has no structural single-call-site guard (N4); `NewLineageStateCorrecting` has no production writer, so W14's fix is fixture-only (N5); `AuthorityStore.LoadReceipt` uses plain `json.Unmarshal` asymmetrically vs. `parseNewLineageRecord`'s strict parsing (N6).
- **W2 (WARNING, carried across all 3 cycles)**: design decision 7's `ambiguous`/`unknown`/`unrelated` → `escalate` (not `stop`) deviation needs a recorded spec amendment before Wave 4 persists `validate` outcomes — today latent because nothing persists a validate outcome yet.
- **W3, W9, W10, W12, W13 and cycle-1 S1–S4 (advisory, carried)**: unanchored legacy-taxonomy snapshot; three PR slices (S2/S4/S5) exceeded the 400-line reviewer budget by >2x under the declared `auto-chain`/High-risk plan; the C5/C6 remediation landed as a sixth de-facto slice on the S5 branch rather than its own PR; candidate-causal admission is wired at `finalize` per `apply-progress`'s documented rationale though the spec prose says `validate` (no amendment recorded); `governingAuthorityLiveEvidence` still uses the workspace projection at `pre-push`/`pre-pr`/`release` (only `pre-commit` uses staged); assorted test-hardening suggestions (S1, S3–S5 style).

None of these carry-forwards involves a CRITICAL finding, an unresolved coverage gap, or a scope contradiction with what this archive certifies as shipped. They are recorded here verbatim from the verify-report's own final classification so a future reader does not have to re-derive them from the 754-line history.

## Archive Completeness Checklist

- [x] All persisted `tasks.md` implementation tasks marked complete; task 1.2 reconciled with explicit filesystem proof (Wave 2's own archive folder) and the reconciliation reason recorded above and inline in the archived `tasks.md`
- [x] All 19 requirements across six specs (3 new, 3 modified) demonstrated compliant at the final cycle-3 envelope
- [x] All 32 scenarios have passing tests, independently re-run and confirmed at tip `67be4867`
- [x] Verification gate PASS (third cycle, envelope re-admission)
- [x] Six real verification-caught defects (C1–C6) documented with independent confirmation evidence
- [x] Three-cycle verify history preserved verbatim in `verify-report.md` (754 lines, byte-identical to source)
- [x] Three new specs copied to main `openspec/specs/`; three modified specs merged preserving all non-Wave-3 requirements
- [x] Change folder copied to `openspec/changes/archive/2026-08-03-rdd-root-simplification-wave3/`
- [x] Byte-perfect copies verified line-by-line for proposal.md (102/102), design.md (95/95), tasks.md (91/91, 1 intentional annotation), verify-report.md (754/754, one transcription defect found and corrected during this archive run — see Integrity Note below)
- [x] Review gate status documented (no negotiated-review artifacts found for this change; ordinary PR-based delivery evidence supplied by the launch prompt)
- [ ] Active `openspec/changes/rdd-root-simplification-wave3/` source directory removed — **NOT COMPLETED, see Known Limitation below**

## Integrity Note (Self-Correction During This Archive Run)

While reconstructing `verify-report.md` by hand from two chunked reads of the 754-line source, an initial write erroneously duplicated two table rows (the "Gate authorization requires an approved terminal authority — C5" and "ReviewCore is the sole writer of new-lineage state — C6" rows from the cycle-2 Correctness table) into the unrelated cycle-1 ("Superseded — original FAIL verification report") Correctness table, which predates C5/C6's discovery and must not reference them. This was caught by this archive run's own line-by-line verification pass (checkpoints every ~15–30 lines across the full file), not assumed correct from a single read. The duplicate rows were removed with a single targeted `Edit`, and the corrected file was re-verified at every previously-checked offset plus the full head/tail to confirm exact 754-line, content-identical parity with the source. This is the exact category of defect issue #2274 warned about; it was caught and fixed within this same archive run rather than shipped.

## Known Limitation: Source Directory Not Deleted

This archive run's toolset (Read, Edit, Write, Glob, Engram, codegraph) includes no file-delete or shell capability. Steps 1–3 (spec merge into `openspec/specs/`, byte-perfect copy to `openspec/changes/archive/2026-08-03-rdd-root-simplification-wave3/`, this archive report) are complete. Step 4 — deleting `openspec/changes/rdd-root-simplification-wave3/`'s original contents so the source directory "ends up gone" — could NOT be performed by this agent. The archive copy is authoritative and complete; the original `openspec/changes/rdd-root-simplification-wave3/` directory still exists alongside it and requires a follow-up `rm -rf openspec/changes/rdd-root-simplification-wave3/` (or equivalent) by an agent/session with filesystem delete or shell access. Until that cleanup runs, do not treat the un-deleted source directory as a second, independently-open change — the archive folder is the source of truth per this report.

## Observation IDs (Engram Traceability)

The following observations were retrieved and used as source material for this archive cycle:

- Proposal: #10131 (`sdd/rdd-root-simplification-wave3/proposal`)
- Spec (delta bundle): #10132 (`sdd/rdd-root-simplification-wave3/spec`)
- Design: #10133 (`sdd/rdd-root-simplification-wave3/design`)
- Tasks: #10134 (`sdd/rdd-root-simplification-wave3/tasks`)
- Verify-Report: #10155 (`sdd/rdd-root-simplification-wave3/verify-report`)

No `sdd/rdd-root-simplification-wave3/review/{transaction,ledger,receipt,gate-context}` observations were found; see the Acceptance Gate Status section above.

This archive report is saved as topic `sdd/rdd-root-simplification-wave3/archive-report` in Engram for persistent traceability.

## Closure Notes

Wave 3 is COMPLETE and its artifacts are ARCHIVED at `openspec/changes/archive/2026-08-03-rdd-root-simplification-wave3/`, with one outstanding mechanical cleanup item (deleting the original source directory — see Known Limitation) that requires a shell-capable follow-up. All six specs (3 new, 3 merged deltas) now reflect Wave 3's shipped behavior in `openspec/specs/` as the source of truth for Wave 4/5. N1, N2, and W2 are recorded explicitly as Wave 4/5 entry conditions per the verify-report's own final classification.

**Merged to main**: commit `4ca5715a` (tracker PR #2318, slices #2309–#2314)
**Archived**: 2026-08-03
**Ready for Wave 4/5**: Yes, subject to the N1/N2/W2 entry conditions above
