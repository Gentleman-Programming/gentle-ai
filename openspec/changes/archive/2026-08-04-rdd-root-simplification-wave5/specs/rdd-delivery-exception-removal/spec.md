# RDD Delivery Exception Removal Specification

## Purpose

Define removal of the two gate-specific delivery-authorization exceptions — pre-PR receipt-graph composition and candidate-decline authorization — so "one immutable receipt authorizes delivery" holds without exception, guarded by characterization tests landing before removal (Wave 1 precedent).

## Requirements

### Requirement: Pre-PR Chain Composition Removed

`EvaluateCompactPrePRChain` (compact_chain.go) MUST NOT be invoked to authorize delivery. Pre-PR MUST deny on the same relation-derived path as the other four gates when no exact or compatible receipt governs.

#### Scenario: Pre-PR base-mismatch denies without composing a chain

- GIVEN a pre-PR evaluation whose receipt binding fails with a base-mismatch denial
- WHEN the gate reports its verdict
- THEN it returns the derived denial directly, without attempting chain composition as a fallback

### Requirement: Candidate-Decline Delivery Authorization Removed

A candidate decline MUST NOT authorize delivery at any gate and MUST NOT create or persist a receipt-like record. The outcome MUST resolve to ordinary unmanaged repository policy, identical in shape to "no receipt discovered." (RATIFIED, maintainer, 2026-08-02: decline finality — full removal of `candidate_decline.go`; decline resolves to unmanaged ordinary delivery permanently; old decline records remain read-only and never grant approval, even on reconsideration.)

#### Scenario: Declined candidate reaches ordinary unmanaged delivery

- GIVEN a candidate with a decline record and no terminal receipt
- WHEN a gate evaluates it
- THEN delivery resolves via ordinary unmanaged policy with no receipt-like record created or read as authority

### Requirement: Characterization Tests Precede Removal

`ValidatedChain`-related pre-PR chain composition and `compactPrePRChainProof` behavior MUST have committed, covering characterization tests before `compact_chain.go` and `candidate_decline.go` removal, mirroring Wave 1's precondition for `deriveBaseAdvanceCompatibility`.

#### Scenario: Characterization tests land before deletion

- GIVEN the removal of pre-PR chain composition and candidate-decline authorization is planned
- WHEN the removal change is prepared
- THEN characterization tests covering both behaviors already exist and pass before either source file is deleted

### Requirement: No Composed Or Decline-Sourced Authority Remains Reachable

After removal, no code path MUST be able to construct delivery authorization from a composed receipt graph or a decline record. Call-absence MUST be provable.

#### Scenario: Zero callers of removed authorization constructors

- GIVEN the post-removal codebase
- WHEN gate call sites are inspected
- THEN no gate calls a composed-graph or decline-authorization constructor to authorize delivery
