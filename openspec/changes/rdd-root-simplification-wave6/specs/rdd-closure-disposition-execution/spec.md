# RDD Closure Disposition Execution Specification

## Purpose

Relaxes Wave 2's `admitLeafDisposition` from `len(closure) == 1` to `len(closure) >= 1` for CLOSED anomaly classes, closing #2014's multi-node case and the classifiable half of #1656. Reuses `authorityDispositionClosure`'s transitive-closure derivation and the existing per-entry two-phase move (`quarantineCompactStoreEntry`); adds ordering, a plan-scoped manifest, and forward-only resume. No new plan shape, verb, or digest domain (see `rdd-authority-disposition-plan`).

## Requirements

### Requirement: N-Node Admission for Closed Anomaly Classes

The executor MUST admit a plan when `closure(S)` has cardinality `>= 1` and `anomaly_class` is closed and evidence-backed (per `rdd-authority-graph-classification`). Unknown, mixed, or ambiguous classes MUST still refuse with no generic fallback quarantine.

#### Scenario: Multi-node classified closure is admitted

- GIVEN a plan whose `ordered_closure` has two or more entries, classified from #2014's content-mismatched binding shape
- WHEN the executor evaluates admission
- THEN the plan is admitted for execution

#### Scenario: Unclassifiable multi-lineage shape still blocks

- GIVEN a graph shape matching #1656's unclassifiable half
- WHEN the executor evaluates admission
- THEN execution refuses with no generic fallback, and the refusal names the diagnosis (#1656) without a delivery-date promise

### Requirement: Descendant-First Ordered Disposition

`ordered_closure` execution MUST proceed deepest-descendant-first, seed-last, so every interruption prefix leaves the retained graph in a valid state.

#### Scenario: Crash after N-1 of N nodes leaves a valid graph

- GIVEN an N-node closure disposition interrupted after committing N-1 descendant nodes
- WHEN the retained graph is inspected post-crash
- THEN it classifies cleanly with no dangling reference, and only the seed remains unmoved

### Requirement: Atomic Visibility With Forward-Only Convergence

(Assumption, pending maintainer confirmation.) A plan-scoped closure manifest MUST bind all N per-entry two-phase quarantine records to one `plan_digest`. Success and retained-graph revalidation MUST be reported only after the last node commits. No rollback of already-committed nodes is provided; recovery is forward-only replay (D1).

#### Scenario: Partial closure never reports success

- GIVEN a closure disposition interrupted before the last node commits
- WHEN the operator queries the outcome
- THEN no success is reported, and the manifest shows the closure as incomplete, not failed-and-undone

### Requirement: Forward-Only Resume via Plan Digest and Residue Discriminator

(Assumption, pending maintainer confirmation.) Replaying the same `plan_digest` MUST skip nodes with committed manifest records and complete `prepared` nodes using the `residue/` presence discriminator, continuing in descendant-first order. A digest mismatch or a graph that no longer re-derives the same closure MUST refuse, name the manifest path, and escalate (D2).

#### Scenario: Exact replay resumes without a double move

- GIVEN an interrupted closure with some nodes committed and one `prepared`
- WHEN the same plan is replayed
- THEN committed nodes are skipped, the prepared node completes via `residue/`, and no entry moves twice

#### Scenario: Digest mismatch refuses and names the manifest

- GIVEN an interrupted closure and a resume attempt with a different `plan_digest`
- WHEN the executor evaluates the resume
- THEN it refuses, names the manifest path, and escalates without attempting narrowing re-derivation

### Requirement: Unrelated Lineage Preservation Across Cross-Lineage Closure

Closure derivation MUST use only report edges. Lineages not reachable from the classified anomaly's report edges MUST remain byte-identical after disposition, asserted per journey.

#### Scenario: Cross-lineage closure disposes only reachable nodes

- GIVEN a classified anomaly whose closure spans two chains via report edges, alongside an unrelated third lineage
- WHEN disposition completes
- THEN the two reachable chains are quarantined and the unrelated lineage is byte-identical to its pre-execution state

### Requirement: Reachable Through the Negotiated Transition Route

(Assumption, pending maintainer confirmation.) `reviewRepairTransition` MUST serve the closure disposition flow so `review status --next-transition` offers disposition `collect`/`execute`, rather than requiring a raw `--plan-digest/--inventory-revision/--authorization` flag triad (D4).

#### Scenario: next_transition offers disposition collect/execute

- GIVEN a `repairable`-classified graph with an authorized closure plan
- WHEN an operator runs `review status --next-transition`
- THEN the returned transition names the disposition `collect`/`execute` operation, not only raw flags

### Requirement: Exit Evidence via ds09+ Bench Journeys

The `ds09+` bench catalog MUST cover multi-chain closure, cross-lineage closure, unchanged-unrelated-graph, replay, and crash-recovery mid-closure as black-box, byte-preserving journeys.

#### Scenario: Multi-chain and crash-recovery journeys pass

- GIVEN `ds09+` journeys seeding a classified multi-chain anomaly and interrupting disposition at each ordered position
- WHEN each journey runs disposition end-to-end or replays after interruption
- THEN byte-preserving quarantine, clean retained-graph revalidation, and no double-move are asserted for every position
