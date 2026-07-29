# Delta for SDD Orchestrator Assets

## ADDED Requirements

### Requirement: Lifecycle gate contract parity

Embedded bounded-review contract guidance MUST promise only supported lifecycle CLI gates: `post-apply`, `pre-commit`, `pre-push`, `pre-pr`, and `release`. It MUST NOT promise an `archive` CLI gate. Archive guidance MUST preserve existing structured-status and allowed-receipt checks.

#### Scenario: Supported gate list is rendered

- GIVEN a bounded-review contract asset is rendered
- WHEN lifecycle gate guidance is inspected
- THEN `archive` is absent from the generic CLI gate list

#### Scenario: Archive checks remain documented

- GIVEN archive guidance is inspected after the gate-list update
- WHEN archive readiness is evaluated
- THEN existing status and allowed-receipt checks remain required
