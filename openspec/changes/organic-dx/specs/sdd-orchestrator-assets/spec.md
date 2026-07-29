# Delta for SDD Orchestrator Assets

## ADDED Requirements

### Requirement: Auto Execution Mode Default

When execution mode is unspecified, the SDD orchestrator MUST default to Auto mode. Interactive mode MUST remain explicitly selectable.

#### Scenario: Unspecified mode defaults to Auto

- GIVEN the orchestrator starts with no explicit execution-mode selection
- WHEN it resolves the effective mode
- THEN it runs in Auto mode

#### Scenario: Interactive remains selectable

- GIVEN an operator explicitly selects Interactive mode
- WHEN the orchestrator resolves the effective mode
- THEN it runs in Interactive mode

### Requirement: Bounded Prompt Budget

The orchestrator MUST emit zero prompts after scope approval on the happy path, and MUST emit at most one actionable prompt per recoverable failure. The gatekeeper MUST summarize phase progress instead of interrupting, and MUST interrupt only on a second consecutive gate failure or a genuine scope/product decision.

#### Scenario: Zero prompts after scope approval on the happy path

- GIVEN scope has been approved and no failure occurs
- WHEN the orchestrator runs the remaining phases
- THEN it emits zero further prompts

#### Scenario: At most one actionable prompt per recoverable failure

- GIVEN a single recoverable failure occurs during a phase
- WHEN the orchestrator handles it
- THEN it emits at most one actionable prompt for that failure

#### Scenario: Gatekeeper summarizes instead of interrupting

- GIVEN a phase completes without a genuine decision point
- WHEN the gatekeeper reports progress
- THEN it summarizes the phase result
- AND it does not interrupt the operator

#### Scenario: Gatekeeper interrupts on second gate failure or a real decision

- GIVEN either a second consecutive gate failure or a genuine scope/product decision
- WHEN the gatekeeper evaluates whether to interrupt
- THEN it interrupts and presents the decision

### Requirement: Asset Text Pin Consistency

Any asset-text requirement introduced or modified for auto-by-default execution mode and prompt budgets MUST remain consistent with the existing `assets_test` pinned assertions and their golden fixtures.

#### Scenario: Updated asset text stays pinned

- GIVEN the auto-by-default and prompt-budget wording is added to the orchestrator assets
- WHEN `assets_test` runs
- THEN the pinned assertions and golden fixtures pass against the updated wording
