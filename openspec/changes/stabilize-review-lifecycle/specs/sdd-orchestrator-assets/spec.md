# Delta for SDD Orchestrator Assets Specification

## ADDED Requirements

### Requirement: Supported Delivery Flow Guidance

Guidance MUST cover only verify → archive → stage → commit → push → pre-PR, archive relocation through the compact binding, and read-only budget-free validation.

#### Scenario: Supported flow

- GIVEN a rendered supported guidance asset
- WHEN its flow is inspected
- THEN it names every supported operation in order and archive relocation through the compact binding

#### Scenario: No unsupported operations

- GIVEN a rendered supported guidance asset
- WHEN lifecycle instructions are inspected
- THEN it does not direct lifecycle allocation or an unsupported flow operation

### Requirement: Scope-Change Guidance Parity

Guidance MUST state `scope-changed` and maintainer action. It MUST NOT direct automatic lineage creation, selection, or reuse.

#### Scenario: Maintainer action

- GIVEN guidance describing a scope change
- WHEN an operator follows it
- THEN it receives `scope-changed` and maintainer action

#### Scenario: No automatic lineage

- GIVEN guidance describing a scope change
- WHEN its prescribed action is inspected
- THEN it does not instruct automatic lineage creation, selection, or reuse
