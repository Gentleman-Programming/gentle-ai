# RDD Delivery Exception Removal Specification

## Purpose

Define removal of the two review-authority exceptions — pre-PR receipt-graph composition and candidate-decline authorization — so neither can create, bypass, or reconstruct review authority. Receipts remain review evidence only; ordinary repository and SDD policy own commit, push, PR, release, and archive decisions. Characterization tests MUST land before removal (Wave 1 precedent).

## Requirements

### Requirement: Pre-PR Chain Composition Removed

`EvaluateCompactPrePRChain` (compact_chain.go) MUST NOT be invoked to construct review authority, select a review-lifecycle result, or provide a fallback for a missing or mismatched receipt. A pre-PR review-context evaluation MUST report its derived relation directly and MUST NOT turn that relation into a delivery decision.

#### Scenario: Pre-PR base mismatch reports review context without composing a chain

- GIVEN a pre-PR review-context evaluation whose receipt binding has a base mismatch
- WHEN the review evaluator reports its result
- THEN it returns the derived review-integrity result directly, without attempting chain composition as a fallback
- AND ordinary repository policy, not that result, decides whether delivery proceeds

### Requirement: Candidate-Decline Delivery Authorization Removed

A candidate decline MUST NOT create review authority, authorize delivery, or persist a receipt-like record. The outcome MUST leave delivery to ordinary unmanaged repository policy, identical in shape to no review receipt being discovered. (RATIFIED, maintainer, 2026-08-02: decline finality — full removal of `candidate_decline.go`; old decline records remain read-only and never grant approval, even on reconsideration.)

#### Scenario: Declined candidate reaches ordinary unmanaged delivery

- GIVEN a candidate with a decline record and no terminal receipt
- WHEN review context is evaluated
- THEN no receipt-like record is created or read as authority
- AND ordinary repository policy decides delivery without a review-derived approval or denial

### Requirement: Characterization Tests Precede Removal

`ValidatedChain`-related pre-PR chain composition and `compactPrePRChainProof` behavior MUST have committed, covering characterization tests before `compact_chain.go` and `candidate_decline.go` removal, mirroring Wave 1's precondition for `deriveBaseAdvanceCompatibility`.

#### Scenario: Characterization tests land before deletion

- GIVEN the removal of pre-PR chain composition and candidate-decline authorization is planned
- WHEN the removal change is prepared
- THEN characterization tests covering both review-authority behaviors already exist and pass before either source file is deleted

### Requirement: No Composed Or Decline-Sourced Review Authority Remains Reachable

After removal, no code path — including an emergency or exception path — MUST be able to construct review authority, a review-lifecycle result, or a new lineage from a composed receipt graph or a decline record. Call-absence MUST be provable. This prohibition applies inside review authority only and MUST NOT create a review control over ordinary delivery.

#### Scenario: Zero callers of removed authority constructors

- GIVEN the post-removal codebase
- WHEN review-context call sites are inspected
- THEN no review-validation path calls a composed-graph or decline-authorization constructor to create review authority or lifecycle state
- AND receipt presence, absence, or validation never decides delivery
