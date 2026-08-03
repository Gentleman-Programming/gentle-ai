# RDD Candidate Identity Specification

## Purpose

Define the canonical candidate identity that Wave 1's read-only resolver computes from any of the four selector variants (staged, workspace, committed-range, workspace-overlay). This identity is the shared input to the relation algebra (`rdd-candidate-relation-algebra`) and the shadow harness (`rdd-shadow-evaluation`). This spec covers Wave 1 read-only resolution only; it does not authorize any live authority mutation.

## Requirements

### Requirement: Canonical Identity Structure

The resolver MUST represent every candidate as one structure containing exactly `repository_id`, `base_tree`, `candidate_tree`, `changed_paths_modes_digest`, and `policy_hash`.

#### Scenario: Structure completeness

- GIVEN a resolved candidate from any selector
- WHEN the resolver returns a `CandidateIdentity`
- THEN it contains all five fields populated with deterministic values
- AND no additional mutable or selector-specific field is present

### Requirement: Selector Normalization

The resolver MUST accept staged, workspace, committed-range, and workspace-overlay selectors as variants and normalize each into the same `CandidateIdentity` shape, without introducing a distinct lifecycle or state machine per selector.

#### Scenario: Staged and workspace selectors converge

- GIVEN a staged selector and a workspace selector that reference identical tree content
- WHEN both are resolved
- THEN the resulting `CandidateIdentity` values are identical
- AND no selector-specific relation logic is invoked

#### Scenario: Committed-range selector resolves canonically

- GIVEN a committed-range selector spanning two commits
- WHEN it is resolved
- THEN the resolver returns one `CandidateIdentity` describing the range's effective base and candidate trees

### Requirement: Read-Only Resolution

The resolver MUST NOT mutate authority state, write a persisted artifact, or introduce a new public operation or contract version.

#### Scenario: Resolution has no side effect

- GIVEN any valid selector input
- WHEN the resolver executes
- THEN no file, authority record, or receipt is written
- AND repeated resolution of the same selector is idempotent

### Requirement: Deterministic Ambiguity and Failure Reporting

The resolver MUST return a typed failure with evidence, or a complete ambiguity set, rather than inferring a candidate from recency or partial matches.

#### Scenario: Ambiguous selector returns full ambiguity set

- GIVEN a selector that matches more than one equally applicable candidate
- WHEN the resolver runs
- THEN it returns every applicable candidate rather than the most recent one
- AND it performs no mutation while ambiguous

#### Scenario: Unresolvable selector fails closed

- GIVEN a selector referencing content the resolver cannot prove any relation for
- WHEN the resolver runs
- THEN it returns a typed failure carrying the evidence it could gather
- AND it does not guess a candidate identity

### Requirement: Wave 1 Selector Scope (Assumption, pending maintainer confirmation)

Wave 1's resolver MUST cover gentle-ai staged, workspace, committed-range, and workspace-overlay selectors only. gentle-pi protocol-1.1 overlay selectors (absorbed backlog items pi#194, pi#197, pi#204) are out of scope for Wave 1 and are covered by the same algebra at the consumer wave.

#### Scenario: Pi overlay selector is explicitly out of scope

- GIVEN a gentle-pi protocol-1.1 overlay selector
- WHEN Wave 1's resolver is invoked with it
- THEN the resolver does not claim to resolve it as a supported Wave 1 selector
- AND coverage is deferred to the consumer wave, not silently assumed working
