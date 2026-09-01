# RDD Ownership Inventory Specification

## Purpose

Defines the one-owner mapping instrument for every RDD lifecycle transition, persisted artifact, contract surface, and consumer, pinned to a fixed snapshot.

## Requirements

### Requirement: Inventory Row Completeness and Single Ownership

The inventory MUST list every RDD lifecycle transition, persisted artifact, contract surface, and consumer (including adapters, SDD, and the five gates), each appearing exactly once, with exactly one named owner per row.

#### Scenario: Every surface appears once with one owner

- GIVEN the completed inventory
- WHEN any transition, artifact, contract surface, or consumer row is inspected
- THEN it appears exactly once and names exactly one owner

#### Scenario: Unowned or multi-owner surface

- GIVEN a surface with no clear owner or with competing owners
- WHEN the inventory is built
- THEN the row is recorded as a finding, not silently resolved or omitted

### Requirement: Snapshot-Pinned, Non-Live Enumeration

The inventory MUST be derived via CodeGraph enumeration at snapshot `ece470dacd0041f394e7f6f3877a6a9fcb3482af` and MUST state explicitly that it is not live authority.

#### Scenario: Snapshot statement present

- GIVEN the inventory document
- WHEN its header or preamble is inspected
- THEN it states the pinned snapshot SHA and that the inventory is not live authority

### Requirement: SDD Attempt-Ledger Ownership Default

The inventory MUST record SDD attempt-ledger ownership per decision 9's adopted default: SDD owns attempts only conditional on its store gaining durable cumulative CAS-like properties; otherwise native authority owns attempts.

#### Scenario: Attempt-ledger row reflects the conditional default

- GIVEN the inventory row for the SDD attempt ledger
- WHEN its owner field is inspected
- THEN it states the conditional default from decision 9, not an unconditional single owner
