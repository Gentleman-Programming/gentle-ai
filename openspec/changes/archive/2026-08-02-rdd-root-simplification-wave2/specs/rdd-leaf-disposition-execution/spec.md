# RDD Leaf Disposition Execution Specification

## Purpose

Defines the one executor that consumes a `DispositionPlan` (see `rdd-authority-disposition-plan`) and admits exactly one anomaly shape: a classified malformed leaf where `closure(S)` has cardinality one (#1892). This closes #1892 and, if its shape reclassifies as a cardinality-one leaf, supersedes PR #2111. It explicitly does not close #2014's descendant closure (deferred to Wave 6) or #1656's multi-lineage case (deferred to Wave 6, stated in every refusal as the open boundary).

## Requirements

### Requirement: Cardinality-One Admission

The executor MUST admit a plan only when `closure(S)` has cardinality exactly one. Any other cardinality MUST refuse.

#### Scenario: Single-node closure is admitted

- GIVEN a plan whose `ordered_closure` has exactly one entry, classified from #1892's historical exact-binding edge shape
- WHEN the executor evaluates admission
- THEN the plan is admitted for execution

#### Scenario: Multi-node closure refuses

- GIVEN a plan whose `ordered_closure` has more than one entry (the #2014/#1656 shape)
- WHEN the executor evaluates admission
- THEN execution refuses, naming cardinality as the reason and Wave 6 as the escalation path, not a roadmap promise

### Requirement: No Predecessor Pointer Rewritten

Execution MUST NOT rewrite any predecessor pointer in the retained graph. Only the isolated leaf entry is moved.

#### Scenario: Retained predecessors are untouched

- GIVEN a successfully admitted leaf plan
- WHEN execution completes
- THEN every predecessor pointer in the retained graph is byte-identical to its pre-execution value

### Requirement: Lock and CAS Reinspection Before Mutation

Before mutating, the executor MUST acquire the maintenance lock and re-inspect the current `authority_inventory_revision` via `InspectCompactRecoveryEdges`, comparing it to the plan's `expected_revisions`. On any drift, execution MUST refuse (CAS failure) rather than proceed against a stale plan.

#### Scenario: Revision drift under lock refuses

- GIVEN a plan authorized against revision R, and the authority graph has since advanced to revision R+1 (e.g. #1892's shared compact-v2 authority under concurrent activity)
- WHEN the executor re-inspects under lock
- THEN execution refuses on CAS mismatch and no bytes are mutated

### Requirement: Byte-Preserving Quarantine With Forensic Residue

Quarantine MUST move the malformed entry's exact original bytes without modification and MUST retain residue evidence of the pre-quarantine location and content sufficient for forensic reconstruction.

#### Scenario: Quarantined bytes are unmodified

- GIVEN an admitted leaf plan targeting a malformed entry
- WHEN quarantine completes
- THEN the quarantined bytes are byte-identical to the original entry
- AND residue evidence records the original location and content

### Requirement: Retained-Graph Revalidation Before Success

After quarantine, the executor MUST re-run classification over the retained graph and MUST NOT report success unless the retained graph revalidates as consistent, with no dangling reference to the quarantined entry.

#### Scenario: Success requires a clean revalidation

- GIVEN a completed quarantine
- WHEN the executor revalidates the retained graph
- THEN success is reported only if revalidation finds no dangling reference and no new anomaly

### Requirement: Exact Replay Converges Without Double-Move

Replaying the same plan against a graph where the leaf was already quarantined MUST converge without moving any entry a second time.

#### Scenario: Replay after success is a no-op

- GIVEN a plan already executed to success
- WHEN the same plan is replayed
- THEN the executor detects the entry is already quarantined and completes without moving it again

### Requirement: Crash Mid-Execution Leaves a Valid Retained Graph

A crash between lock acquisition and completion MUST leave the retained graph in a valid, classifiable state. Recovery MUST NOT require manual byte-level repair.

#### Scenario: Crash after lock, before quarantine completes

- GIVEN the executor crashes after acquiring the lock but before quarantine finishes
- WHEN the graph is inspected after restart
- THEN it classifies cleanly (`healthy`, `repairable`, or `blocked`) with no corrupted intermediate state

### Requirement: Concurrent Execution Refuses Duplicate Mutation

A second concurrent execution attempt against the same target under the same lock MUST refuse rather than race with the first.

#### Scenario: Two operators execute the same plan concurrently

- GIVEN two concurrent executor invocations against the same leaf target
- WHEN both attempt to acquire the maintenance lock
- THEN exactly one proceeds and the other refuses without mutating

### Requirement: Unknown, Mixed, or Ambiguous Shapes Block With No Generic Fallback

The executor MUST refuse, never quarantine, any shape not admitted by the cardinality-one closed-class predicate — including unknown, mixed, or ambiguous classifications. No generic quarantine fallback MAY be applied.

#### Scenario: Ambiguous classification refuses

- GIVEN an anomaly the classifier could not prove as a closed, evidence-backed class (per `rdd-authority-graph-classification`)
- WHEN the executor is invoked
- THEN it refuses and reports the diagnosis, without applying any fallback quarantine

### Requirement: Refusal Names Diagnosis and Escalation Artifact, Not a Roadmap Promise

On refusal, the executor MUST name the diagnosis (why refused) and the escalation artifact (e.g. the classified anomaly detail or the relevant issue), and MUST NOT promise a specific future wave as a commitment.

#### Scenario: Multi-lineage refusal names #1656 without a delivery promise

- GIVEN a refusal caused by #1656's multi-lineage shape
- WHEN the refusal output is inspected
- THEN it names the diagnosis and references #1656 as the open escalation artifact, without committing to a wave delivery date

### Requirement: Refusal Requires Explicit Maintainer Authorization, Never Blocks a Human Elsewhere

Proceeding past a refusal MUST require explicit maintainer authorization. Absence of that authorization MUST NOT block any other operator flow (blocking budget compliance).

#### Scenario: Unauthorized refusal does not block unrelated work

- GIVEN a refused leaf disposition with no maintainer authorization granted
- WHEN an unrelated candidate proceeds through `review start` in another worktree
- THEN the unrelated candidate is unaffected by the outstanding refusal
