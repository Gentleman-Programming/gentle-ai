# RDD Review Core Transitions Specification

## Purpose

Define `ReviewCore` as the sole transition owner for new-lineage reviews: `start`, `finalize`, `validate`. It performs consent-gated candidate freeze, assigns tier 0|1|4 with frozen lenses and correction budget, admits only candidate-causal findings, permits exactly one bounded correction, and issues the terminal receipt. This spec covers new-lineage behavior only; legacy transitions are untouched.

## Requirements

### Requirement: Sole Transition Owner for New Lineages

`ReviewCore` MUST be the only component that performs `start`, `finalize`, or `validate` for a new-lineage candidate. No other package, adapter, or gate MAY mutate new-lineage state directly.

#### Scenario: Only ReviewCore transitions new-lineage state

- GIVEN a new-lineage candidate under the activation switch
- WHEN a state transition is requested
- THEN it is performed exclusively through `ReviewCore.start`, `.finalize`, or `.validate`
- AND no gate or adapter writes new-lineage state directly

### Requirement: Consent-Gated Freeze With Immutable Tier, Lenses, and Budget, Preceded by Capability Admission

`start` MUST freeze candidate identity, tier (0|1|4, reusing existing threshold logic — adopted default), lens set, and correction budget only after (1) transport capability admission has succeeded (Issue #1247) and (2) consent is granted via the reused `gentle-ai.review-integration.consent/v2` envelope (adopted default) for tier 1|4; tier 0 proceeds without a consent question. Capability admission MUST run before consent is even requested — an unsupported transport MUST deny before any consent prompt exists. Once frozen, tier, lenses, and budget MUST NOT be recomputed later in the same lineage.

(Previously: freeze depended only on consent gating; capability was not a precondition of `start`.)

#### Scenario: Tier 1 candidate freezes only after consent

- GIVEN a tier-1 candidate awaiting the v2 consent envelope
- WHEN consent is granted
- THEN `start` freezes candidate identity, tier, lenses, and budget together, once

#### Scenario: Frozen tier is never recomputed

- GIVEN a frozen tier-4 lineage mid-review
- WHEN a later transition re-evaluates risk inputs
- THEN the persisted tier, lens set, and budget remain exactly as frozen at `start`

#### Scenario: Capability admission precedes candidate freeze

- GIVEN an adapter whose transport capability is unsupported
- WHEN `start` is invoked
- THEN capability admission denies before any consent prompt, tier assignment, lens selection, or budget freeze occurs
- AND no partial candidate state is created

### Requirement: Candidate-Causal Admission Only

`validate` MUST admit only findings caused by the frozen candidate. Pre-existing or base-only findings MUST become follow-ups, not blockers.

#### Scenario: Candidate-caused finding blocks

- GIVEN a finding whose evidence traces only to paths changed by the frozen candidate
- WHEN `validate` runs
- THEN the finding is admitted as blocking

#### Scenario: Pre-existing finding becomes a follow-up

- GIVEN a finding present in the base tree before the candidate existed
- WHEN `validate` runs
- THEN the finding is recorded as a follow-up and does not block finalize

### Requirement: One Bounded Correction, Exact Replay Exempt

A lineage MUST permit at most one correction transaction. A second correction attempt MUST refuse. Re-validating an unchanged frozen candidate (exact replay) MUST NOT consume the correction budget.

#### Scenario: Second correction attempt refuses

- GIVEN a lineage that already consumed its one correction
- WHEN a second correction is attempted
- THEN `ReviewCore` refuses with a typed reason and no state mutation

#### Scenario: Exact replay costs nothing

- GIVEN a frozen candidate re-validated with byte-identical content
- WHEN `validate` runs again
- THEN the correction budget is unchanged

### Requirement: Terminal Receipt Issuance Exactly Once

`finalize` MUST issue exactly one immutable receipt per lineage. A lineage that has already finalized MUST NOT finalize again.

#### Scenario: Finalize issues one receipt

- GIVEN an approved or escalated lineage ready to finalize
- WHEN `finalize` runs
- THEN exactly one immutable receipt is written and the lineage cannot finalize a second time

### Requirement: Offer Transition Reachable From a Real Call Site

`ReviewCore`'s post-verify offer transition MUST be invoked from SDD's post-verify, pre-archive call site (`OfferReviewAfterVerify`, Wave 4). It MUST NOT remain unwired or left in a deadcode-baselined state once Wave 4 lands. (Issue #1209)

#### Scenario: Offer transition is wired to a live caller

- GIVEN Wave 4 has landed
- WHEN SDD reaches the post-verify, pre-archive point
- THEN it calls `ReviewCore`'s offer transition through `OfferReviewAfterVerify`
- AND the offer transition has at least one live, non-test caller: `internal/sddstatus`'s `Resolve()`/`resolveEngramStatus()`, through `applyReviewOfferRouting` and `review_door.go`'s `reviewOfferForVerify` (call-site amendment, 2026-08-03, superseding the originally-named `internal/cli` verify-success exit — see design.md's "Amendment (orchestrator-resolved): decision 3 call site"); `RunSDDVerifyValidate` stays context-free and is not the caller

#### Scenario: Offer transition is absent from pre-verify code paths

- GIVEN Wave 4 has landed
- WHEN the SDD apply or pre-verify path is inspected
- THEN no call into the offer transition exists on that path

### Requirement: `validate` Is The Single Review-Context Path For Legacy Lineages

`validate` — the read-only review-context evaluation transition — MUST become the sole path invoked by all five integration hooks for legacy lineages, replacing bespoke per-hook or per-lineage discovery functions. Its output is review-only evidence and MUST NOT govern commit, push, PR, release, or archive delivery.

#### Scenario: Legacy lineage invokes the same transition as a new lineage

- GIVEN a legacy-lineage candidate and a new-lineage candidate at the same integration hook
- WHEN each review context is evaluated
- THEN both invoke `validate`; no hook calls a lineage-specific discovery function that bypasses it
- AND neither result changes ordinary delivery policy

#### Scenario: No bespoke discovery fork remains

- GIVEN the cutover has landed
- WHEN an integration hook's review-context path is inspected
- THEN it contains no per-lineage-kind discovery branch outside `validate`
- AND no review result acts as a delivery gate
