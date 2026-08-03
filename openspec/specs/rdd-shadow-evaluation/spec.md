# RDD Shadow Evaluation Specification

## Purpose

Define the harness that observes live start/status/five-gate call sites, runs the candidate identity resolver, relation algebra, and graph classifier alongside the live decision, and records agreement or divergence as exit evidence — without ever influencing a live outcome. This spec is Wave 1's rollback boundary and blocking-budget compliance contract.

## Requirements

### Requirement: Advisory-Only, Never Blocking

Shadow evaluation MUST NOT block, delay, or alter any human-facing consent prompt, terminal decision, or live gate outcome at any observed call site (start, status, `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`).

#### Scenario: Shadow failure does not block the live path

- GIVEN the shadow evaluation raises an internal error while computing a relation
- WHEN the live call site proceeds
- THEN the live decision completes exactly as it would with shadow evaluation absent
- AND no user-facing stop is emitted by the shadow harness

### Requirement: Disable Switch Is the Rollback Boundary

The harness MUST provide a disable switch. When disabled, zero shadow code path executes, and every live decision is byte-identical to the shadow-off baseline.

#### Scenario: Disabling removes all shadow execution

- GIVEN the disable switch is set to off
- WHEN a live call site that would otherwise trigger shadow evaluation executes
- THEN no resolver, relation, or classifier code runs
- AND the live outcome is unchanged from a build with no shadow package present

### Requirement: Zero Live-Lifecycle Behavior Change

Observation call sites in `gate.go`, `compact_gate.go`, `compact_recovery_binding.go`, and the review CLI MUST remain outcome-neutral: shadow evaluation MUST NOT alter any live decision, receipt, or authority mutation.

#### Scenario: Live outcome is identical with shadow on or off

- GIVEN the same candidate evaluated once with the shadow harness enabled and once disabled
- WHEN the live gate decision is compared between the two runs
- THEN the live decision, receipt, and authority state are byte-identical in both runs

### Requirement: No Persisted Divergence Artifact (Assumption, pending maintainer confirmation)

Agreement and divergence records MUST be written only to test/bench output, not to a persisted production artifact, contract version, or new state value.

#### Scenario: Divergence is recorded outside production authority

- GIVEN a divergence is detected during a fixture run
- WHEN the harness records it
- THEN the record exists only in test/bench output
- AND no new file, field, or contract version is added to `review-state.json` or `review-receipt.json`

### Requirement: Off by Default in Live Paths (Assumption, pending maintainer confirmation)

Shadow evaluation MUST default to off in live traffic paths. The differential matrix MUST be producible from deterministic fixtures; opt-in real-traffic sampling MAY be added as a separate, explicit configuration.

#### Scenario: Default configuration produces no live Git cost

- GIVEN the harness is installed with default configuration
- WHEN a live gate runs
- THEN no additional `merge-tree` or patch-identity Git invocation occurs due to the shadow harness

### Requirement: Differential Matrix Exit Evidence

The harness MUST produce a differential matrix covering all four selectors, all seven relations, base movement, contraction, ambiguity, and unknown cases. Every divergence MUST be explained. `ambiguous`/`unknown` rows with no live decision to compare MUST be marked "no live decision" and MUST NOT be recorded as agreement.

#### Scenario: Matrix covers the full selector-by-relation space

- GIVEN the fixture suite for all four selectors
- WHEN the differential matrix is generated
- THEN every selector × relation combination has a row
- AND every row is either an agreement, an explained divergence, or "no live decision"

### Requirement: Unexplained Divergence Blocks Wave 2 (Assumption, pending maintainer confirmation)

Any unexplained divergence on `exact`, `compatible_base_advance`, or `provable_contraction` MUST block Wave 2 entry. Explained divergences MUST be documented rather than silently accepted as passing.

#### Scenario: Unexplained core-relation divergence stops the wave boundary

- GIVEN the differential matrix contains an unexplained divergence on `compatible_base_advance`
- WHEN Wave 1 exit evidence is evaluated
- THEN Wave 2 does not start
- AND the divergence is surfaced, not silently reconciled
