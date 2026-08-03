# Delta for RDD Authority Disposition Plan

## MODIFIED Requirements

### Requirement: Deterministic Closure Derivation From the Graph Source of Record

`ordered_seed_set` and `ordered_closure` MUST be derived deterministically from `InspectCompactRecoveryEdges` (`compact_inspect.go`). Plan derivation MUST NOT re-derive graph state independently or from a cached/inferred view. As of Wave 6, `ordered_closure` ordering is normative: entries MUST be ordered deepest-descendant-first with the seed last, so any executor consuming the closure (e.g. `rdd-closure-disposition-execution`) can rely on order for interruption safety, not just for determinism.

(Previously: ordering was determinism-only — "same graph state derives the same closure" — with no normative ordering requirement, since Wave 2's executor consumed only cardinality-one closures where order was immaterial.)

#### Scenario: Same graph state derives the same closure

- GIVEN the same authority graph inspected twice with no state change
- WHEN a plan is derived both times for the same anomaly
- THEN `ordered_seed_set` and `ordered_closure` are identical both times

#### Scenario: Ordering is descendant-first, seed-last

- GIVEN a derived plan for a multi-node closure
- WHEN `ordered_closure` is inspected
- THEN entries appear deepest-descendant-first, with the seed entry last

### Requirement: Cardinality Is an Executor Admission Policy, Not a Plan-Shape Constraint

The plan shape MUST NOT hard-code closure cardinality. Cardinality restrictions are enforced by the consuming executor: Wave 2 admitted only cardinality-one closures (`rdd-leaf-disposition-execution`); Wave 6 admits cardinality `>= 1` for closed anomaly classes (`rdd-closure-disposition-execution`) using the identical plan shape.

(Previously: referenced "a future wave (#2014, #1656)" as the wider-cardinality consumer; Wave 6 is that wave, so the requirement now names the shipped executor rather than a forward-looking placeholder.)

#### Scenario: Same plan shape serves a single-node closure

- GIVEN a plan whose `ordered_closure` has cardinality one
- WHEN the plan shape is inspected
- THEN it uses the same fields as a plan with a larger closure would, with no leaf-specific field
