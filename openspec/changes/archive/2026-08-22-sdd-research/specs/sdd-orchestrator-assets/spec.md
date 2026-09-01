# Delta for sdd-orchestrator-assets

## ADDED Requirements

### Requirement: Orchestrator-Owned Discovery and Confirmed Handoff

Orchestrators MUST own discovery after exploration/research. Interactive mode MAY question; automatic mode MUST use confirmed facts or block. They MUST invoke `sdd-propose` only with selected research `done`, decisions `confirmed`, valid references/readiness; proposal executors MUST NOT interview.

#### Scenario: Confirmed proposal handoff

- GIVEN research is done/unselected, state matches, and decisions are confirmed
- WHEN the orchestrator reaches proposal
- THEN proposal starts without questions

#### Scenario: Automatic unresolved choices

- GIVEN automatic mode has unresolved product choices
- WHEN the pre-proposal gate runs
- THEN it emits one lossless grouped prompt, persists state, and blocks proposal

### Requirement: Optional-Lane and Status-v1 Compatibility

Research MUST be optional before selection; no-request changes MUST skip only that branch. Native status-v1 shape, tokens, and routing MUST stay unchanged; orchestrators gate `propose` with readiness.

#### Scenario: Legacy change without research

- GIVEN a change has no research request
- WHEN status-v1 returns its existing `propose` recommendation
- THEN the route is preserved and research is not required

#### Scenario: Status-v1 remains frozen

- GIVEN a consumer asks for a research route
- WHEN status-v1 is validated
- THEN no research token is emitted/accepted; the orchestrator gate handles it

## MODIFIED Requirements

### Requirement: Bounded Review Contract Parity Across Catalog Agents

All 12 source orchestrator templates MUST render equivalent optional research lifecycle guidance; mappings MAY be shared. Every AgentID returned by `catalog.AllAgents()` MUST render the canonical bounded review execution contract, and all 16 supported IDs MUST be covered, including OpenCode/Kilocode sharing and Pi/VS Code Copilot/OpenClaw/Trae using the generic runtime family.
(Previously: bounded-review parity covered all 16 IDs but not research lifecycle.)

#### Scenario: Catalog-derived parity test renders all agents

- GIVEN the current catalog contains 16 supported AgentIDs
- WHEN each ID is passed through the real orchestrator asset renderer
- THEN every rendered variant contains explicit `review/start(target)`, one transaction-wide refuter batch, non-iterative ordinary scoped validation, Judgment Day-only re-judgment, exact persistence references, and user-owned runtime selection
- AND no variant contains the old standard=1/full-4R=3 refuter fan-out
- AND optional research guidance does not change these guarantees
