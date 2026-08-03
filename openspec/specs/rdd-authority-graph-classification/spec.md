# RDD Authority Graph Classification Specification

## Purpose

Define the read-only classifier that inspects an authority graph and returns one of the target architecture's three health values. Wave 1 delivers classification only — no disposition plan derivation, quarantine, or repair execution. Those are Wave 2 (#1892 leaf-only) and Wave 6 (#2014 descendant closure) scope.

## Requirements

### Requirement: Three-Value Health Classification

The classifier MUST return exactly one of `healthy`, `repairable`, or `blocked` for any inspected authority graph, and MUST NOT introduce a fourth value or an intermediate state.

#### Scenario: Consistent graph classifies healthy

- GIVEN an authority graph with no detected anomaly
- WHEN the classifier inspects it
- THEN it returns `healthy`

#### Scenario: Classified leaf anomaly classifies repairable

- GIVEN an authority graph containing a closed, evidence-backed anomaly class with a leaf-only closure
- WHEN the classifier inspects it
- THEN it returns `repairable`

### Requirement: No Mutation or Execution

The classifier MUST NOT mutate the authority graph, derive or execute a disposition plan, quarantine bytes, or acquire the maintenance lock. Classification is inspection-only. A separate, deterministic plan derivation (owned by `rdd-authority-disposition-plan`) MAY consume a `repairable` classification's evidence to produce a `DispositionPlan`; that derivation is not part of the classifier and MUST NOT be implemented inside it.

#### Scenario: Classification has no side effect

- GIVEN any authority graph
- WHEN the classifier runs
- THEN no authority record, receipt, or graph edge is modified
- AND no disposition plan is created or persisted

#### Scenario: Repairable result feeds a separate plan derivation, not the classifier itself

- GIVEN a `repairable` classification for #1892's leaf anomaly shape
- WHEN a `DispositionPlan` is subsequently derived from that classification's evidence
- THEN the derivation happens in `rdd-authority-disposition-plan`, and re-running the classifier alone still produces no plan and no mutation

### Requirement: Fail-Closed on Unknown Shape

An anomaly shape the classifier cannot prove as a closed, evidence-backed class MUST classify as `blocked`, never `healthy` or `repairable`.

#### Scenario: Unclassifiable shape blocks

- GIVEN an authority graph anomaly that matches no closed, evidence-backed anomaly class
- WHEN the classifier inspects it
- THEN it returns `blocked`
- AND no generic repairable fallback is offered

### Requirement: Deterministic, Evidence-Backed Classification

The classifier MUST derive its result deterministically from the inspected graph and named evidence, not from inference, recency, or partial signals.

#### Scenario: Same graph state classifies identically

- GIVEN the same authority graph inspected twice with no state change between inspections
- WHEN the classifier runs both times
- THEN it returns the same classification both times
