# RDD Post-Verify Review Offer Specification

## Purpose

Define the sequence SDD MUST follow per maintainer directive (Engram decision #10123, 2026-08-02): apply -> verify -> offer RDD review -> (optional correction -> targeted re-verify) -> archive. RDD is a service SDD invokes at exactly one point, never a supervisor of the SDD cycle. These are hard MUSTs, not defaults.

## Requirements

### Requirement: Offer Occurs Strictly Post-Verify, Pre-Archive

SDD MUST offer RDD review only after verify completes and before archive begins. SDD MUST NOT consult, block on, or offer RDD review before or during apply. The pre-apply status control (`applyPreVerifyReviewRouting`, `applyPreVerifyCompactBridgeRouting`) MUST be removed, not reordered. (Issue #1209)

#### Scenario: Offer fires only after verify completes

- GIVEN apply and verify have both completed for a change
- WHEN SDD reaches its verify-success exit in `internal/cli` — not `sddstatus.Resolve`, which remains a pure read, because an offer inside `Resolve` would re-create RDD as a supervisor of every status read
- THEN it calls `OfferReviewAfterVerify` as the sole review entry point
- AND no code path offers or consults RDD before verify completes

#### Scenario: No pre-verify blocking control remains

- GIVEN a change mid-apply, verify not yet run
- WHEN SDD status is queried
- THEN SDD does not block verify on any review transaction
- AND `applyPreVerifyReviewRouting` is absent from the call graph, not merely bypassed

### Requirement: Kill-Switch-Off Is Structural Absence, Proven by Call-Absence

When the RDD kill switch is OFF, zero review code MUST execute on any SDD path: no offer, no status consultation, no disabled/unmanaged ceremony capable of failing or blocking. This MUST be proven by a call-absence assertion (static call-graph/AST guard demonstrating no edge from any SDD apply/verify/archive path into `ReviewCore`/offer symbols). A passing unit test of the disabled branch alone is explicitly NOT acceptable evidence for this requirement. (Issue #1227)

#### Scenario: Call-absence guard proves invisibility

- GIVEN the kill switch is OFF
- WHEN the call-absence guard runs (CodeGraph/AST-based, not a runtime test)
- THEN it asserts zero call edges from `internal/sddstatus` apply/verify/archive paths into any `ReviewCore` or offer-transition symbol
- AND a green "disabled path" behavioral test alone MUST NOT be accepted as satisfying this scenario

#### Scenario: Archive is unfailable on review grounds when OFF

- GIVEN the kill switch is OFF
- WHEN archive runs
- THEN archive consults no `reviewGate` structured status
- AND archive cannot fail or block for review reasons

### Requirement: Decline Proceeds to Unmanaged Ordinary Archive

WHEN the offer is declined, SDD MUST proceed to unmanaged ordinary archive under existing repository policy. SDD MUST NOT block archive on a decline and MUST NOT create a receipt-like record for a declined offer.

#### Scenario: Decline does not block archive

- GIVEN the post-verify offer is presented and declined
- WHEN SDD proceeds to archive
- THEN archive completes under ordinary `disabled/unmanaged` policy
- AND no receipt or receipt-like artifact is created for the declined offer

### Requirement: Post-Offer Correction Triggers Targeted Re-Verify Before Archive

A bounded correction issued after the offer MUST invalidate prior verify evidence for the paths it touches. SDD MUST run a targeted re-verify scoped to the correction's changed paths before archive proceeds; SDD MUST fall back to a full re-verify only when path-level scoping cannot be proven.

#### Scenario: Targeted re-verify for a scoped correction

- GIVEN a post-verify correction that touches a known, provable subset of paths
- WHEN SDD re-runs verify before archive
- THEN re-verify is scoped to exactly those changed paths
- AND unrelated prior verify evidence remains valid

#### Scenario: Full re-verify when scoping is not provable

- GIVEN a post-verify correction whose changed-path set cannot be reliably derived
- WHEN SDD re-runs verify before archive
- THEN SDD runs a full re-verify covering the objective's entire evidence goal
- AND archive does not proceed until that full re-verify passes

### Requirement: Intra-Wave Rollout Sequencing

Wave 4 delivery MUST land in this order: (1) the offer call site wired first, (2) transport capability admission wired second, (3) review-binding mirror deletion last, since binding removal is destructive and irreversible.

#### Scenario: Mirror deletion lands after offer and capability are live

- GIVEN Wave 4 implementation in progress
- WHEN the offer call site and capability admission are both live and verified
- THEN only then does the review-binding mirror deletion PR land
- AND no PR deletes the mirror before the offer call site exists
