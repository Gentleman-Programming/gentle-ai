# Delta for Bounded Review Transaction and Findings Ledger

## ADDED Requirements

### Requirement: Final-Scope TargetFixDiff Receipt Identity

Each terminal `TargetFixDiff` receipt MUST bind final correction scope and evidence as one versioned identity. Readers MUST preserve legacy receipt and compact-binding bytes; unknown or incomplete identities fail closed.

#### Scenario: Receipt agrees

- GIVEN an approved correction with final scope and evidence
- WHEN its receipt validates
- THEN identity agrees with that scope and evidence

#### Scenario: Invalid identity denied

- GIVEN an unknown receipt version or missing evidence
- WHEN a lifecycle gate validates it
- THEN validation denies without treating it as a current receipt

### Requirement: Read-Only Budget-Free Lifecycle Validation

Lifecycle validation MUST be read-only: it MUST NOT start, finalize, recover, create a lineage, or allocate budget. A compatible receipt and evidence MUST allow; mismatches fail closed with machine-readable denial.

#### Scenario: Compatible receipt allowed

- GIVEN a compatible receipt, evidence, and target
- WHEN a supported gate validates
- THEN it allows and performs zero lifecycle or budget operations

#### Scenario: Evidence mismatch denied

- GIVEN a receipt with mismatched evidence
- WHEN a supported gate validates
- THEN it returns a machine-readable denial and performs zero lifecycle or budget operations

### Requirement: Durable Same-Lineage Lifecycle Recovery

The system MUST durably record same-lineage invalidation, validator escalation, and verify remediation. Exact replay survives restart without another budget; invalid, stale, or competing transitions fail closed.

#### Scenario: Restart replay

- GIVEN durably recorded verify remediation
- WHEN restart replays that transition
- THEN it resumes the same lineage with unchanged budgets

#### Scenario: Competing transition

- GIVEN a stale or competing transition predecessor
- WHEN it is applied or replayed
- THEN it is rejected and no budget or lineage changes

### Requirement: Exact Scope-Change Action

On final-scope change, validation MUST emit `scope-changed` and maintainer action. It MUST NOT automatically create, select, or reuse a lineage.

#### Scenario: Scope-change action

- GIVEN current and receipt scopes differ
- WHEN validation runs
- THEN it returns `scope-changed` and maintainer action

#### Scenario: No automatic lineage

- GIVEN a scope-change diagnostic is returned
- WHEN no maintainer action is performed
- THEN no lineage is created, selected, or reused

### Requirement: One-Lineage One-Budget Delivery Proof

The system MUST provide one realistic proof: one lineage and budget complete correction, validation, restart, verify remediation, verify, archive, stage, commit, push, and pre-PR. Any mismatch MUST deny without another budget.

#### Scenario: Complete flow

- GIVEN one approved lineage and budget
- WHEN the proof completes the supported flow including restart
- THEN every allowed gate reuses them

#### Scenario: Mismatch denies

- GIVEN a scope, evidence, identity, or transition mismatch
- WHEN the affected gate runs
- THEN it denies and does not allocate another budget
