# RDD Freeze-Expansion Policy — Wave 0

**Decision**: Wave 0 stops additive old-facade recovery and transport work on the RDD lifecycle, except for a proven security defect meeting all four criteria below. This is maintainer-internal guidance, not CI-enforced, and expires when Wave 7 completes or the tracker branch (`feature/rdd-root-simplification`) is abandoned.

This policy exists because the problem statement in `docs/architecture/rdd-root-simplification-design.md` names a repeatable failure pattern: a local patch adds a reason code, contract field, command, state field, adapter rule, or persisted artifact to fix one edge, while a sibling flow still implements the old interpretation. Every wave-0 through wave-7 PR on the tracker chain is exempt from this policy by definition — the policy freezes *additive* work on the *old* facade, not the migration that replaces it.

## Blocking budget

Added per maintainer directive (2026-08-02), refining rather than replacing the four-criterion test below: **no more blockages unless truly necessary; quality yes, but never a system so impressive nobody can maintain it.**

1. A flow governed by this policy MUST block a human only at (a) consent before freezing a medium/high-risk candidate, and (b) genuine terminal decisions (exemption granted/denied, escalation accepted/declined). Every other check in this policy is silent or advisory — it informs a decision, it does not stop one.
2. Every blocking gate in this policy MUST offer a documented self-service exit that requires no source-code archaeology. Reference pattern: `skills/gentle-ai-chained-pr`'s `size:exception` label lets a maintainer clear the 400-line budget gate without reading the budget-check implementation. This policy's exits are named explicitly in **Escalation path** below — never "read the source to find out."
3. Fail-closed must never be fail-mute. Every stop this policy causes names its reason (which of the four criteria failed) and its exit (Escalation path). A stop with no named reason or no named exit is a defect in this policy, not an acceptable outcome.
4. The following safety boundaries are exempt from softening, in any future revision of this policy: content binding (approval binds exact candidate bytes), no floating approval (a branch name or intent never substitutes for the frozen candidate), and fail-closed on unknown corruption. "No more blockages" governs process friction, never these three invariants.

## Quick path

For a contributor whose change touches a surface listed under **Scope**:

1. Check whether the change touches a frozen row/glob below. If not, this policy does not apply — proceed normally.
2. If it does, evaluate the change against all four **Proven security defect** criteria before writing code.
3. If any criterion fails, stop. This is not a rejection — bring the work to the maintainer as tracker-wave work instead (see **Escalation path**); the fix likely belongs in Wave 1–7, not the frozen facade.
4. If all four criteria hold, open the minimal fix with reproduction evidence and the declared rollback boundary in the PR description, scoped strictly inside the frozen surface.

## Scope

Frozen: any inventory row in `docs/architecture/rdd-ownership-inventory.md` whose **Target disposition** is `REMOVE`, `MERGE`, or `DERIVE` — i.e., every surface the target architecture consolidates or deletes, not the two `KEEP` review-lifecycle artifacts or the five `KEEP` review-context hooks that already match the target model. None of those hooks is delivery authority; ordinary repository policy owns delivery.

| Row ID(s) | Surface | Path glob |
|---|---|---|
| TRN-01 | Legacy 13-state transaction vocabulary | `internal/reviewtransaction/transaction.go` (const block + compat switch) |
| TRN-02 | `next_transition` producer / independent state-mutation sites | `internal/cli/review_next_transition.go`, `internal/reviewtransaction/transaction.go` |
| TRN-03 | Legacy public dispatch forms (`review-step`, bundle/validate legacy verbs) | `internal/app/app.go` |
| ART-03 | Batch-reconcile journal | `internal/reviewtransaction/compact_batch_reconcile_journal.go` |
| ART-04 | Finalize-attempt journal | `internal/reviewtransaction/finalize_attempt_journal.go` |
| ART-05 | Bundle export/import artifact | `internal/reviewtransaction/bundle.go` |
| ART-06 | Legacy fix-scope quarantine | `internal/reviewtransaction/legacy_fix_scope_quarantine.go` |
| ART-07 | Legacy quarantine (general) | `internal/reviewtransaction/legacy_quarantine.go` |
| CTR-01–CTR-06 | Hand-written contract schemas/fixtures/constants | `contracts/review-integration/{v1,v2}/**`, `internal/cli/review_status_contract.go`, `internal/cli/review_operation_contract.go` |
| CON-06, CON-07 | SDD review-gate bridge, SDD review-binding mirror | `internal/sddstatus/review_gate.go`, `internal/sddstatus/review_binding.go` |

**Explicitly not frozen**:

- Documentation of any kind.
- Tests that pin *existing* behavior (regression coverage on a `KEEP` row or on already-shipped behavior).
- Wave work on the tracker chain (`feature/rdd-root-simplification` and its child PRs) — that work builds the *replacement*, it is not additive old-facade growth.
- The two `KEEP` review-lifecycle artifacts (ART-01 authority record, ART-02 terminal receipt) and the five `KEEP` review-context hooks (CON-01–CON-05) — ordinary maintenance and bugfixes on already-target-shaped surfaces continue. Their outputs remain informational and never govern delivery or archive.
- CON-08 (SDD attempt-ledger) and CON-09–CON-11 (adapter dispatch) — flagged findings, not frozen surfaces; fixing their split-ownership is itself wave work.

## Proven security defect

All four criteria, conjunctively — three of four escalates to wave work, it does not exempt:

1. **Concrete impact.** A concrete confidentiality, integrity, or authorization impact on a named row of the design's `Non-negotiable invariants` table (e.g., Candidate binding, No floating approval, Corruption).
2. **Reproduction required.** Reproduction on current `main` at a named SHA, per `skills/rdd-defect-workflow`. A credible report alone does not satisfy this criterion — **maintainer-confirmed 2026-08-02** (was proposal Q3 assumption; see Decision log).
3. **Minimal fix inside the frozen surface.** No new state value, verb, reason code, contract version, or persisted artifact. A fix that must add one of those is wave work, not an exemption.
4. **Declared rollback boundary.** Stated in the PR description independently of commit creation.

## Required evidence

- The mechanism-not-site sweep and the class-closing guard from `backlog-triage` Phase 2b (named by the design; not yet a standalone skill in this repo — apply its intent: prove the *class* of defect is closed, not just the one reported call site).
- The ≤400 authored-line forecast from `skills/rdd-defect-workflow` (hard limit; above it, chain or get an explicit maintainer-approved exception before edits).
- Current-`main` reproduction evidence per criterion 2 above.
- The declared rollback boundary per criterion 4 above.

## Escalation path

- **Any criterion fails, or the reproduction is inconclusive**: open the fix as tracker-wave work instead (target the wave whose scope matches the surface — see the Migration waves table in the design). This is the named, documented, no-archaeology-required exit from every block in this policy.
- **Three of four criteria hold**: escalate to the maintainer with the failing criterion named; do not proceed as an exemption.
- **Genuinely proven (all four)**: open the minimal PR against `main` (not the tracker chain — this is a real-world fix, not wave work), citing this policy and the four criteria in the PR description.

## Status and expiry

Maintainer-internal guidance. Not binding on external contributors. No CI enforcement is introduced by this change — **maintainer-confirmed 2026-08-02** (was proposal Q1 assumption; see Decision log). Expires at Wave 7 completion or tracker-branch abandonment, whichever comes first.

## Decision log

| Question | Prior status | Current status |
|---|---|---|
| Q1 — guidance-only, no CI enforcement | Proposal assumption | **Maintainer-confirmed**, 2026-08-02 |
| Q3 — reproduction required for the security-defect exemption | Proposal assumption | **Maintainer-confirmed**, 2026-08-02 |
| Blocking budget principle | Not in the original design | Added 2026-08-02, refining (not contradicting) the four-criterion test above |
