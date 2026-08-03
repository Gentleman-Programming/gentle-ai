# RDD Authority Store Specification

## Purpose

Define the two-artifact persistence model for new lineages: `review-state.json` (CAS mutable) and `review-receipt.json` (immutable terminal). Exactly five states persist; every other observation is derived, never stored, never gate-written. Covers new-lineage persistence only.

## Requirements

### Requirement: Two-Artifact Model, No Sidecars

A new lineage MUST persist exactly two artifacts: one CAS-mutable state artifact and one immutable terminal receipt. No sidecar file, journal, bundle, or result file MAY be created for a new lineage.

#### Scenario: New lineage writes exactly two artifacts

- GIVEN a new lineage from `start` through `finalize`
- WHEN the store's writes are inspected
- THEN exactly `review-state.json` and, after finalize, `review-receipt.json` exist for that lineage
- AND no sidecar, journal, bundle, or result file is present

### Requirement: Five Persisted States, Everything Else Derived

Only `reviewing`, `correcting`, `validating`, `approved`, and `escalated` MAY be persisted. `invalidated`, `scope_changed`, `ambiguous`, `repairable`, and `corrupted` MUST be derived at read time and MUST NOT be written by any gate.

#### Scenario: Derived category is never a stored value

- GIVEN a lineage whose live comparison currently classifies as `ambiguous`
- WHEN its persisted state is inspected
- THEN the stored value is one of the five persisted states, not `ambiguous`

#### Scenario: No gate writes a derived category

- GIVEN any of the five gates evaluating a new-lineage candidate
- WHEN the gate completes
- THEN it has not written `invalidated`, `scope_changed`, `ambiguous`, `repairable`, or `corrupted` to the state artifact

### Requirement: CAS Mutation With In-Record Replay Identity

The state artifact MUST be mutated only under compare-and-swap on its current revision. Exact-replay identity MUST be recorded in the same artifact so replay detection needs no external lookup.

#### Scenario: Stale revision refuses the write

- GIVEN a state write whose expected revision no longer matches the current artifact
- WHEN the write is attempted
- THEN it refuses on CAS mismatch and the artifact is unchanged

#### Scenario: Replay identity is self-contained

- GIVEN a completed transition
- WHEN a later transition checks whether the candidate is an exact replay
- THEN it decides using only the fields already in `review-state.json`, with no separate journal

### Requirement: Receipt Immutability After Issuance

Once `review-receipt.json` is written for a lineage, it MUST NOT be modified, appended to, or replaced.

#### Scenario: Post-issuance write attempt is rejected

- GIVEN a lineage with an already-issued receipt
- WHEN any process attempts to modify that receipt
- THEN the write is rejected and the original receipt bytes are unchanged

### Requirement: Reason Taxonomy Regression Coverage

Every user-visible refusal reason produced by the legacy 36-field record MUST have a mapped equivalent reachable through the five-state-plus-derived-observation model. This mapping MUST be exit evidence for the wave.

#### Scenario: Legacy refusal reason has a new-model equivalent

- GIVEN the enumerated set of legacy user-visible refusal reasons
- WHEN the regression matrix is built against the new model
- THEN every legacy reason maps to exactly one reachable new-model reason
- AND no legacy reason is left unmapped
