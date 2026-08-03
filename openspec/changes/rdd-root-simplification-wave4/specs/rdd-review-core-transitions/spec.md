# Delta for RDD Review Core Transitions

> **Re-base caveat**: this delta is copied from `openspec/changes/rdd-root-simplification-wave3/specs/rdd-review-core-transitions/spec.md`, which has not yet archived to `openspec/specs/rdd-review-core-transitions/spec.md`. If Wave 3 archives before Wave 4, re-diff this delta against the archived top-level spec before Wave 4 archives; the requirement text below is the authoritative base as of this writing.

## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: Offer Transition Reachable From a Real Call Site

`ReviewCore`'s post-verify offer transition MUST be invoked from SDD's post-verify, pre-archive call site (`OfferReviewAfterVerify`, Wave 4). It MUST NOT remain unwired or left in a deadcode-baselined state once Wave 4 lands. (Issue #1209)

#### Scenario: Offer transition is wired to a live caller

- GIVEN Wave 4 has landed
- WHEN SDD reaches the post-verify, pre-archive point
- THEN it calls `ReviewCore`'s offer transition through `OfferReviewAfterVerify`
- AND the offer transition has at least one live, non-test caller at SDD's verify-success exit in `internal/cli`, with `sddstatus.Resolve` remaining a pure read

#### Scenario: Offer transition is absent from pre-verify code paths

- GIVEN Wave 4 has landed
- WHEN the SDD apply or pre-verify path is inspected
- THEN no call into the offer transition exists on that path
