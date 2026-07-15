# Exploration: Stabilize Review Lifecycle

## Decision

Narrow the current change to the existing compact receipt, compact store, SDD binding, and lifecycle commands. Fix final-scope identity, make validation structurally read-only, add durable same-lineage remediation, and support OpenSpec archive relocation with one focused native archive coordinator. Do not introduce a general authority resolver, repository identity, hooks, risk accounting, telemetry, or provider cleanup.

## Current Evidence

Repository evidence remains `HEAD == origin/main == 1b5a5f59f74d3f6dab7de01c1603d5ce1b77af17`; this OpenSpec change is untracked. Current defects relevant to this slice are:

- `CompactState.Receipt()` combines `CurrentSnapshot.CandidateTree` with `InitialSnapshot.PathsDigest`, so `TargetFixDiff` lacks one internally consistent final scope.
- `EvaluateCompactGate` is read-oriented but accepts no `archive` gate and directly derives a mutable `CompactStore`.
- `BindApprovedReview` binds only `openspec/changes/<change>`; `resolveBindingChangeRoot` rejects the dated archive location.
- `CompleteVerification(..., false)` makes compact state terminally escalated, while `sddstatus` remediation understands only legacy transaction states. Compact verification failure therefore cannot resume the same lineage.
- `sdd-status` is read-only, and no current review facade command owns archive preparation, rename, projection publication, or crash reconciliation.

## Narrowed Model

`CompactReceipt` v3 binds `CorrectionBaseTree`, final candidate tree, final paths digest, intended-untracked proof, frozen ledger IDs, fix delta, and verification evidence in `FinalScope`. Genesis scope remains audit data. Unknown or incomplete receipt identities deny; v2 bytes are preserved, never reinterpreted.

Validation uses a `CompactGateReader` exposing only receipt/state/event reads and a locked final recheck. `EvaluateCompactGate`, `applyReviewGate`, and `RunReviewFacadeValidate` have no callable start, finalize, recover, lineage creation, state replacement, or budget-allocation method. Scope drift returns exact expected/actual identities, complete sorted added/removed paths, `scope-changed`, and explicit maintainer action; it never creates or selects a lineage.

## Archive Ownership and Invocation

Add `gentle-ai review archive-sdd <capture|relocate|reconcile>`, parsed by `internal/cli.RunReviewArchiveSDD`. Focused `internal/sddarchive.Service` owns archive evidence and filesystem mutation; `sddstatus.Resolve` remains read-only. Existing `sddstatus.ReviewBinding` APIs expose versioned load/CAS operations used by the service.

The archive agent performs this exact sequence:

1. Reconcile/verify tasks, then call `archive-sdd capture`; native code freezes active root, destination, binding/authority/receipt/lifecycle revisions, budget fingerprint, verify hash, delta inputs, main-spec preimages, and active-change manifest.
2. Apply delta-spec sync.
3. `archive-sdd relocate` recomputes the target/projection. It admits only deterministic main-spec outputs from captured deltas and proven task-checkbox reconciliation under OpenSpec roots. Any executable/non-OpenSpec change or unexpected lifecycle byte is `scope-changed`.
4. Run read-only `GateArchive`; prove unchanged lineage, receipt hash, authority revision, frozen findings, and budget.
5. CAS binding v2 from `active` to `archive-prepared`, recording the refreshed projection and relocation manifest.
6. Require one filesystem/device and perform `os.Rename(active, archive)`.
7. Re-read destination bytes/modes and CAS `archive-prepared` to `archived`.

Crash before prepare leaves only baseline evidence; retry refreshes. Crash after prepare but before rename retries the rename. Crash after rename but before final CAS is completed by `reconcile` only when active is absent and the sole destination exactly matches the prepared manifest. Final-state replay is a no-op; ambiguity, source reappearance, EXDEV, or revision/digest drift fails closed without deletion.

The archive projection authorizes only the deterministic lifecycle relocation; it does not replace receipt final scope or recalculate risk/budget. Pre-commit/push/pre-PR gates require both the unchanged receipt and exact archived projection.

## Same-Lineage Verification Remediation

Extend `CompactState` with `StateVerifyFailed`, `StateVerifyRemediating`, `FailedEvidenceRevision`, `VerifyRemediationCount`, and canonical `CompactLifecycleEvent/v1` entries embedded in the CAS-protected state record.

Events contain `Schema`, `Kind`, `PreviousRevision`, failed/replacement evidence revisions, actor/reason, and budget fingerprint. Missing additive fields in legacy compact v2 state normalize to empty; old bytes are not rewritten until an explicit mutation.

- `RecordVerifyFailure(expectedRevision,evidence)` moves `approved → verify_failed`; gates deny the now-stale receipt.
- `AuthorizeVerifyRemediation(expectedRevision,failedEvidence,proposed,actor)` moves `verify_failed → verify_remediating` only once and only within `CorrectionBudget-CumulativeCorrectionLines`.
- `CompleteVerifyRemediation(expectedRevision,snapshot,actual,validation,evidence)` requires `TargetFixDiff`, frozen `FixFindingIDs`, genesis-path subset, exact failed evidence, and passing scoped checks; it appends a correction attempt and moves to `validating`.
- Existing `CompleteVerification(replacement,true)` moves `validating → approved` and atomically replaces the receipt; replacement evidence must differ from failed evidence.

`RunReviewFacadeFinalize` invokes these APIs through new `review finalize --verify-failed`, `--authorize-verify-remediation`, and `--complete-verify-remediation` modes; each requires `--expected-revision`.

A failed sole scoped validator calls `EscalateValidatorFailure` and moves directly to `escalated`; it is not verification remediation. Exhausted budget, second remediation, stale evidence, or failed replacement verification also escalates. Every successor preserves lineage, generation, initial snapshot, policy, frozen findings, original lines, and budget. Exact state/event replay returns the existing revision; stale, altered, or competing predecessors fail. Restart loads the state/event array and resumes the recorded state without budget allocation.

## Current Slice and Follow-Ups

Current affected areas are only `internal/reviewtransaction`, `internal/sddstatus`, `internal/sddarchive`, `internal/cli`, three shared/archive assets, their focused tests, and one real Git lifecycle E2E.

Future follow-ups/non-goals: general authority registry or precedence unification; repository/clone/worktree identity; Engram/legacy authority cleanup; hook adoption/suppression; lifecycle risk-accounting redesign; telemetry; broad generated-provider cleanup. None is a prerequisite or forecast item.

The honest implementation forecast exceeds 3,000 authored lines. Use the smallest durable split: PR1 delivers final-scope receipt, pure gates, exact diagnostics, and same-lineage remediation through a replacement approved receipt; PR2 consumes that contract for archive capture/relocate/reconcile, binding projection, guidance, and the complete Git E2E.
