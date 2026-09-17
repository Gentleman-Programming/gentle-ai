# Delta for sdd-orchestrator-assets

## MODIFIED Requirements

### Requirement: Orchestrator-Owned Discovery and Confirmed Handoff

Orchestrators MUST own discovery after exploration/research. Interactive mode MAY question; automatic mode MUST use confirmed facts or block. They MUST invoke `sdd-propose` only with selected research `done`, decisions `confirmed`, valid references/readiness, and a native-selected action/context where a native handoff is required. Proposal executors MUST NOT interview. Orchestrator and managed-child guidance MUST treat native status as authoritative for the next action and existing authorized artifact scope; they MUST NOT infer transitions from prose, add launch/review/delivery gates to read-only status, or broaden write authority.
(Previously: orchestrators owned post-research discovery and invoked proposal only from confirmed readiness.)

#### Scenario: OA-01 confirmed proposal handoff

- GIVEN research is done or unselected, state matches, and decisions are confirmed
- WHEN the orchestrator reaches proposal
- THEN proposal starts without questions

#### Scenario: OA-02 automatic unresolved choices

- GIVEN automatic mode has unresolved product choices
- WHEN the pre-proposal gate runs
- THEN it emits one lossless grouped prompt, persists state, and blocks proposal

#### Scenario: OA-03 native action governs managed handoff

- GIVEN native status selects an action and supplies bounded action context
- WHEN orchestrator guidance starts the managed next phase
- THEN it carries the selected action and existing authorized artifact scope
- AND it does not recompute a transition or widen authority

#### Scenario: OA-04 read-only status remains ungated

- GIVEN status inspection has no mutation request
- WHEN the orchestrator renders or follows status guidance
- THEN it does not require launch, review, delivery, or archive authorization
- AND it does not represent inspection as write authority
