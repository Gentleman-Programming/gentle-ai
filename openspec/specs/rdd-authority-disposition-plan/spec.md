# RDD Authority Disposition Plan Specification

## Purpose

Defines the generic `DispositionPlan` derived deterministically from `InspectCompactRecoveryEdges` (the graph source of record) for a `repairable`-classified authority graph, and the authorization binding required before any executor may consume it. The plan is the one reusable surface for every future disposition wave (#1892 here, #2014 and #1656's multi-lineage case in Wave 6); cardinality restrictions live in consuming executors, not in this shape.

## Requirements

### Requirement: Plan Field Set

A `DispositionPlan` MUST carry exactly `repository_id`, `authority_inventory_revision`, `anomaly_class`, `ordered_seed_set`, `ordered_closure`, `expected_revisions`, `plan_digest`, `actor`, `reason`, and `authorization`. No field MAY be dropped or renamed without a spec change.

#### Scenario: Plan carries all required fields

- GIVEN a plan derived for a `repairable`-classified graph
- WHEN the plan is inspected
- THEN all ten fields listed above are present and populated

### Requirement: Deterministic Closure Derivation From the Graph Source of Record

`ordered_seed_set` and `ordered_closure` MUST be derived deterministically from `InspectCompactRecoveryEdges` (`compact_inspect.go`). Plan derivation MUST NOT re-derive graph state independently or from a cached/inferred view. As of Wave 6, `ordered_closure` ordering is normative: entries MUST be ordered deepest-descendant-first with the seed last, so any executor consuming the closure (e.g. `rdd-closure-disposition-execution`) can rely on order for interruption safety, not just for determinism.

#### Scenario: Same graph state derives the same closure

- GIVEN the same authority graph inspected twice with no state change
- WHEN a plan is derived both times for the same anomaly
- THEN `ordered_seed_set` and `ordered_closure` are identical both times

#### Scenario: Ordering is descendant-first, seed-last

- GIVEN a derived plan for a multi-node closure
- WHEN `ordered_closure` is inspected
- THEN entries appear deepest-descendant-first, with the seed entry last

### Requirement: Closed Anomaly Classification Required for Derivation

Plan derivation MUST refuse (produce no plan) when `anomaly_class` is unknown, mixed, or ambiguous. Only a closed, evidence-backed class (per `rdd-authority-graph-classification`) yields a plan. No generic fallback plan MAY be produced.

#### Scenario: Unknown shape yields no plan

- GIVEN a `blocked` classification for an unclassifiable shape (e.g. the multi-lineage case in #1656)
- WHEN plan derivation is attempted
- THEN no plan is produced and the caller receives a typed refusal, not a partial plan

#### Scenario: Content-mismatched binding classifies before it can plan

- GIVEN an anomaly matching #2014's content-mismatched recovery binding shape
- WHEN plan derivation is attempted
- THEN a plan is produced only if the shape classifies as a closed, evidence-backed class; otherwise it refuses and stays deferred (Wave 6)

### Requirement: Plan Digest Binds Exact Content

`plan_digest` MUST be computed over the full `ordered_closure`, `expected_revisions`, and `anomaly_class`. Any change to that content MUST invalidate the digest. `plan_digest` MUST NOT be computed over `actor` or `reason`: those are execution-time provenance a maintainer supplies (and that a read-only preflight cannot yet know), not plan identity — the same treatment `authorization` already receives. A plan derived read-only with no `actor`/`reason` MUST publish the exact same `plan_digest` a later execution re-derives with the real `actor`/`reason`, for the same graph state.

#### Scenario: Content change invalidates the digest

- GIVEN a derived plan and its `plan_digest`
- WHEN any entry in `ordered_closure` or `expected_revisions` changes
- THEN re-deriving the digest over the changed content produces a different value than the original

#### Scenario: Actor and reason do not affect plan_digest

- GIVEN a plan derived read-only with empty `actor` and `reason` (e.g. `review repair --preflight`)
- WHEN the identical graph state is re-derived at execution time with a real, non-empty `actor` and `reason`
- THEN both derivations produce the identical `plan_digest`

### Requirement: Authorization Binds to Digest and Revision, No Wall-Clock Expiry

(Assumption, pending maintainer confirmation.) Authorization MUST bind to `plan_digest` and `authority_inventory_revision` and MUST NOT carry a wall-clock expiry. Staleness is guarded by CAS revalidation against the current `authority_inventory_revision` at execution time, not by elapsed time. This deviates from design decision 4's short-expiry default; the CAS check is asserted as sufficient because expiry adds a blocking failure mode the freeze-expansion budget discourages.

#### Scenario: Stale revision refuses regardless of elapsed time

- GIVEN an authorized plan whose `authority_inventory_revision` no longer matches the graph's current revision
- WHEN an executor attempts to consume the plan
- THEN execution refuses on CAS mismatch, independent of how much time has elapsed since authorization

#### Scenario: Valid, unexpired-by-clock plan proceeds

- GIVEN an authorized plan bound to the current `authority_inventory_revision`
- WHEN an executor attempts to consume the plan
- THEN execution proceeds without any wall-clock expiry check

### Requirement: Cardinality Is an Executor Admission Policy, Not a Plan-Shape Constraint

The plan shape MUST NOT hard-code closure cardinality. Cardinality restrictions are enforced by the consuming executor: Wave 2 admitted only cardinality-one closures (`rdd-leaf-disposition-execution`); Wave 6 admits cardinality `>= 1` for closed anomaly classes (`rdd-closure-disposition-execution`) using the identical plan shape.

#### Scenario: Same plan shape serves a single-node closure

- GIVEN a plan whose `ordered_closure` has cardinality one
- WHEN the plan shape is inspected
- THEN it uses the same fields as a plan with a larger closure would, with no leaf-specific field

### Requirement: No New Public Repair Verb

(Assumption, pending maintainer confirmation.) Plan derivation MUST occur internally behind the existing maintainer `review repair` verb. It MUST NOT introduce a new CLI command, subcommand, or public API.

#### Scenario: Plan derivation has no new command

- GIVEN the Wave 2 CLI surface (`review_repair.go`, `review_next_transition.go`)
- WHEN the command set is inspected
- THEN plan derivation is reachable only through the pre-existing `review repair` verb, with no added command
