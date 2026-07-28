# Delta for SDD Orchestrator Assets

## ADDED Requirements

### Requirement: Published asset migration is explicit

Published managed asset paths MUST follow systemic migration order. A binary update MUST NOT pretend already-installed assets changed: digest mismatch MUST report `sync_required`, and the next explicit install/sync MUST transactionally replace only the managed block from its canonical source while preserving unmanaged user content. The migration MUST reconcile both released pre-WorkRun assets and unpublished branch-build WorkRun blocks without retaining obsolete commands.

#### Scenario: Updated binary sees old installed assets

- GIVEN an installed managed asset has the previous digest
- WHEN diagnostics run before sync
- THEN `sync_required` is reported
- AND the installed file is not silently mutated

#### Scenario: Explicit sync migrates managed guidance

- GIVEN a released or branch-build managed block is stale
- WHEN explicit sync reconciles it
- THEN corrected organic guidance replaces only that managed block
- AND a second identical sync is a no-op

## MODIFIED Requirements

### Requirement: Bounded Review Contract Parity Across Catalog Agents

Every AgentID returned by `catalog.AllAgents()` MUST render the canonical bounded review execution contract. The 12 source orchestrator templates MAY share mappings, but every one of the 16 supported IDs MUST be covered, including OpenCode/Kilocode sharing and Pi/VS Code Copilot/OpenClaw/Trae using the generic runtime family. The canonical contract MUST additionally apply post-candidate proportional tiers, one scoped ordinary correction, receipt reuse, and prohibit WorkRun authority; Judgment Day retains its independent two-round budget.
(Previously: parity did not require proportional post-candidate policy, receipt reuse, or WorkRun prohibition.)

#### Scenario: Catalog-derived parity test renders all agents

- GIVEN the current catalog contains 16 supported AgentIDs
- WHEN each ID is passed through the real orchestrator asset renderer
- THEN every rendered variant contains explicit `review/start(target)`, one transaction-wide refuter batch, non-iterative ordinary scoped validation, Judgment Day-only re-judgment, exact persistence references, and user-owned runtime selection
- AND no variant contains the old standard=1/full-4R=3 refuter fan-out

#### Scenario: Proportional policy is rendered

- GIVEN a supported AgentID renders the canonical contract
- WHEN post-candidate checking is selected
- THEN proportional tiers, one ordinary correction, receipt reuse, and no WorkRun authority apply
- AND Judgment Day retains its independent two-round budget

### Requirement: Shared Contract Expansion

Reusable bounded-review algorithms MUST live in `skills/_shared/review-ledger-contract.md`. Orchestrator and native review-agent generation MUST expand that canonical source instead of maintaining divergent hand-copied algorithms. The same canonical embedded asset MUST expand proportional post-candidate policy without changing canonical-source ownership or installed bounded-contract content.
(Previously: the canonical source did not include proportional post-candidate policy.)

#### Scenario: Dedicated reviewer is installed

- GIVEN an adapter supports native review sub-agents
- WHEN SDD injection writes the reviewer file
- THEN the installed prompt contains the canonical bounded contract
- AND the role-specific source contributes only lens and tool-boundary instructions

