# Delta for RDD Shadow Evaluation

**Pre-archive note**: `rdd-shadow-evaluation` exists only under `openspec/changes/rdd-root-simplification-wave1/specs/rdd-shadow-evaluation/spec.md` — Wave 1 has not archived to `openspec/specs/` yet. This delta is written against that Wave 1 filesystem copy, following the same pattern Wave 2 used for `rdd-authority-graph-classification`. If Wave 1 archives before this delta is applied, re-base this MODIFIED block against `openspec/specs/rdd-shadow-evaluation/spec.md` instead.

Wave 3 lifts the resolver and relation algebra out of shadow-only gating so `ReviewCore` can call the same functions live for new lineages (`GENTLE_AI_RDD_SHADOW` remains a distinct switch, per `rdd-new-lineage-activation`). The shadow harness's own disable switch must therefore scope to its *observer* — the agreement/divergence-recording code — not to the resolver/relation functions it shares with the live facade. Wave 2's health classifier stays read-only-consumed, not extended, and is unaffected by this delta.

## MODIFIED Requirements

### Requirement: Disable Switch Is the Observer's Rollback Boundary

The harness MUST provide a disable switch. When disabled, zero shadow *observer* code (agreement/divergence recording) executes, and every live decision at a call site observed only by the shadow harness is byte-identical to the shadow-off baseline. This switch MUST NOT gate the resolver or relation functions themselves when they are invoked directly by `ReviewCore` for a new lineage under the activation switch — those calls are live decisions, not shadow observation.
(Previously: stated that disabling forbids zero shadow code path execution, full stop, which would also forbid the live facade from calling the same resolver/relation functions; Wave 3 needs those functions live-callable, so the switch is re-scoped to the observer only.)

#### Scenario: Disabling removes shadow observer execution only

- GIVEN the shadow disable switch is off
- WHEN a legacy live call site that would otherwise trigger shadow observation executes
- THEN no agreement/divergence-recording code runs
- AND the live outcome is unchanged from a build with no shadow observer present

#### Scenario: Resolver and relation stay live-callable independent of the shadow switch

- GIVEN the shadow disable switch is off and the activation switch is on
- WHEN `ReviewCore` calls the resolver and relation functions for a new-lineage transition
- THEN those calls execute as the live deciding path
- AND they are unaffected by the shadow observer's disable switch

### Requirement: Off by Default in Live Paths (Assumption, pending maintainer confirmation)

Shadow *observation* MUST default to off in live traffic paths, with no additional Git cost attributable to the observer. This zero-cost guarantee is scoped to the observer only: `ReviewCore`'s direct, live use of the resolver/relation functions for new lineages is expected to perform its own necessary Git invocations and is not shadow-observer cost.
(Previously: framed the zero-added-Git-cost guarantee as covering the harness broadly, with no distinction from a live consumer of the same functions; Wave 3 introduces that live consumer, so the guarantee is re-scoped to the observer.)

#### Scenario: Default configuration produces no live Git cost from the observer

- GIVEN the harness is installed with default configuration
- WHEN a live gate runs at a legacy call site
- THEN no additional `merge-tree` or patch-identity Git invocation occurs due to the shadow observer

#### Scenario: New-lineage live cost is not observer cost

- GIVEN the activation switch on and a new-lineage `start`
- WHEN `ReviewCore` invokes the resolver and relation functions
- THEN any Git invocation they perform is attributed to the live new-lineage decision, not to the shadow observer's zero-cost guarantee
