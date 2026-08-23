# RDD Shadow Evaluation Specification

## Purpose

Define the harness that observes live review start/status and five review-context hook call sites, runs the candidate identity resolver, relation algebra, and graph classifier alongside the live review-lifecycle decision, and records agreement or divergence as exit evidence — without ever influencing a live outcome. Every shadow input, result, agreement, divergence, and decision is review-lifecycle context only: it MUST NOT authorize, deny, block, or route ordinary delivery or SDD archive. This spec is Wave 1's rollback boundary and blocking-budget compliance contract.

## Requirements

### Requirement: Advisory-Only, Never Blocking

Shadow evaluation MUST NOT block, delay, or alter any human-facing consent prompt, terminal review-lifecycle decision, or live review-context outcome at any observed call site (start, status, `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, `release`). It MUST NOT influence ordinary delivery or SDD archive.

#### Scenario: Shadow failure does not block the live path

- GIVEN the shadow evaluation raises an internal error while computing a relation
- WHEN the live call site proceeds
- THEN the live review-lifecycle decision completes exactly as it would with shadow evaluation absent
- AND no user-facing stop is emitted by the shadow harness

### Requirement: Disable Switch Is the Observer's Rollback Boundary

The harness MUST provide a disable switch. When disabled, zero shadow *observer* code (agreement/divergence recording) executes, and every live review-context result at a call site observed only by the shadow harness is byte-identical to the shadow-off baseline. This switch MUST NOT gate the resolver or relation functions themselves when they are invoked directly by `ReviewCore` for a new lineage under the activation switch — those calls are live review-lifecycle decisions, not shadow observation.
(Previously: stated that disabling forbids zero shadow code path execution, full stop, which would also forbid the live facade from calling the same resolver/relation functions; Wave 3 needs those functions live-callable, so the switch is re-scoped to the observer only.)

#### Scenario: Disabling removes shadow observer execution only

- GIVEN the shadow disable switch is off
- WHEN a legacy live review-context call site that would otherwise trigger shadow observation executes
- THEN no agreement/divergence-recording code runs
- AND the live review-lifecycle outcome is unchanged from a build with no shadow observer present

#### Scenario: Resolver and relation stay live-callable independent of the shadow switch

- GIVEN the shadow disable switch is off and the activation switch is on
- WHEN `ReviewCore` calls the resolver and relation functions for a new-lineage transition
- THEN those calls execute as the live review-lifecycle deciding path
- AND they are unaffected by the shadow observer's disable switch

### Requirement: Zero Live Review-Lifecycle Behavior Change

Observation call sites in `gate.go`, `compact_gate.go`, `compact_recovery_binding.go`, and the review CLI MUST remain outcome-neutral: shadow evaluation MUST NOT alter any live review-lifecycle decision, receipt, or authority mutation, and no observation output may influence ordinary delivery or SDD archive.

#### Scenario: Live outcome is identical with shadow on or off

- GIVEN the same candidate evaluated once with the shadow harness enabled and once disabled
- WHEN the live review-context result is compared between the two runs
- THEN the live review-lifecycle decision, receipt, and authority state are byte-identical in both runs

### Requirement: No Persisted Divergence Artifact (Assumption, pending maintainer confirmation)

Agreement and divergence records MUST be written only to test/bench output, not to a persisted production artifact, contract version, or new state value.

#### Scenario: Divergence is recorded outside production authority

- GIVEN a divergence is detected during a fixture run
- WHEN the harness records it
- THEN the record exists only in test/bench output
- AND no new file, field, or contract version is added to `review-state.json` or `review-receipt.json`

### Requirement: Off by Default in Live Paths (Assumption, pending maintainer confirmation)

Shadow *observation* MUST default to off in live traffic paths, with no additional Git cost attributable to the observer. This zero-cost guarantee is scoped to the observer only: `ReviewCore`'s direct, live use of the resolver/relation functions for new lineages is expected to perform its own necessary Git invocations and is not shadow-observer cost.
(Previously: framed the zero-added-Git-cost guarantee as covering the harness broadly, with no distinction from a live consumer of the same functions; Wave 3 introduces that live consumer, so the guarantee is re-scoped to the observer.)

#### Scenario: Default configuration produces no live Git cost from the observer

- GIVEN the harness is installed with default configuration
- WHEN a live review-context hook runs at a legacy call site
- THEN no additional `merge-tree` or patch-identity Git invocation occurs due to the shadow observer

#### Scenario: New-lineage live cost is not observer cost

- GIVEN the activation switch on and a new-lineage `start`
- WHEN `ReviewCore` invokes the resolver and relation functions
- THEN any Git invocation they perform is attributed to the live new-lineage review decision, not to the shadow observer's zero-cost guarantee

### Requirement: Differential Matrix Exit Evidence

The harness MUST produce a differential matrix covering all four selectors, all seven relations, base movement, contraction, ambiguity, and unknown cases. Every divergence MUST be explained. `ambiguous`/`unknown` rows with no live decision to compare MUST be marked "no live decision" and MUST NOT be recorded as agreement.

#### Scenario: Matrix covers the full selector-by-relation space

- GIVEN the fixture suite for all four selectors
- WHEN the differential matrix is generated
- THEN every selector × relation combination has a row
- AND every row is either an agreement, an explained divergence, or "no live decision"

### Requirement: Unexplained Divergence Blocks Wave 2 (Assumption, pending maintainer confirmation)

Any unexplained divergence on `exact`, `compatible_base_advance`, or `provable_contraction` MUST block Wave 2 entry only. Explained divergences MUST be documented rather than silently accepted as passing, and neither outcome may control ordinary delivery or SDD archive.

#### Scenario: Unexplained core-relation divergence stops the wave boundary

- GIVEN the differential matrix contains an unexplained divergence on `compatible_base_advance`
- WHEN Wave 1 exit evidence is evaluated
- THEN Wave 2 does not start; ordinary delivery and SDD archive remain governed independently
- AND the divergence is surfaced, not silently reconciled
