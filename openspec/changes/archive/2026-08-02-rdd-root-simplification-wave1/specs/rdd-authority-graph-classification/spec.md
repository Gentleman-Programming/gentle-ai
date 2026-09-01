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

The classifier MUST NOT mutate the authority graph, derive or execute a disposition plan, quarantine bytes, or acquire the maintenance lock. Classification is inspection-only.

#### Scenario: Classification has no side effect

- GIVEN any authority graph
- WHEN the classifier runs
- THEN no authority record, receipt, or graph edge is modified
- AND no disposition plan is created or persisted

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
