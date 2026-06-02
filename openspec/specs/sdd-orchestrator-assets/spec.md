# SDD Orchestrator Assets Specification

## Purpose

Defines required behavior for agent-specific SDD orchestrator guidance assets so Claude and OpenCode guidance remain semantically aligned where intended, while preserving platform-accurate delegation wording.

---

## Requirements

### Requirement: Chain Strategy Guidance Parity

For Claude-targeted SDD orchestrator assets, the system MUST include explicit chain strategy guidance equivalent to OpenCode for reviewer-safe delivery planning.

The guidance MUST enumerate exactly these selectable strategies:
- `stacked-to-main`
- `feature-branch-chain`

#### Scenario: Claude guidance lists both required strategies

- GIVEN Claude orchestrator guidance is generated from embedded assets
- WHEN the chain strategy section is rendered
- THEN it includes `stacked-to-main`
- AND it includes `feature-branch-chain`

#### Scenario: Strategy naming remains canonical

- GIVEN a static validation checks chain strategy labels in Claude guidance
- WHEN the check runs
- THEN no alias or renamed value replaces the canonical strategy names

---

### Requirement: Chain Strategy Propagation to Downstream Phases

Claude orchestrator guidance MUST require passing `chain_strategy` together with `delivery_strategy` into `sdd-tasks` and `sdd-apply` whenever delivery planning is relevant.

#### Scenario: Guidance forwards both strategy fields to sdd-tasks

- GIVEN the orchestrator invokes `sdd-tasks` for a change requiring delivery planning
- WHEN invocation guidance is rendered
- THEN `delivery_strategy` is forwarded
- AND `chain_strategy` is forwarded

#### Scenario: Guidance forwards both strategy fields to sdd-apply

- GIVEN the orchestrator invokes `sdd-apply` after planning
- WHEN invocation guidance is rendered
- THEN `delivery_strategy` is forwarded
- AND `chain_strategy` is forwarded

---

### Requirement: Non-Claude Chain Strategy Guidance Parity

For non-Claude SDD orchestrator assets that support SDD delivery planning, the system MUST include explicit chain strategy guidance equivalent in intent to the Claude/OpenCode reference guidance.

The guidance MUST enumerate exactly these selectable strategies:
- `stacked-to-main`
- `feature-branch-chain`

#### Scenario: Non-Claude delivery-planning guidance lists both required strategies

- GIVEN a non-Claude orchestrator asset that includes delivery planning guidance
- WHEN the chain strategy section is rendered
- THEN it includes `stacked-to-main`
- AND it includes `feature-branch-chain`

#### Scenario: Non-Claude strategy naming remains canonical

- GIVEN static validation checks chain strategy labels in non-Claude orchestrator assets
- WHEN the check runs
- THEN no alias or renamed value replaces the canonical strategy names

---

### Requirement: Non-Claude Strategy Propagation to Downstream Phases

For non-Claude SDD orchestrator assets that invoke downstream delivery phases, the system MUST require passing `chain_strategy` together with `delivery_strategy` into `sdd-tasks` and `sdd-apply` whenever delivery planning is relevant.

#### Scenario: Non-Claude guidance forwards both strategy fields to sdd-tasks

- GIVEN a non-Claude orchestrator asset invokes `sdd-tasks` for a change requiring delivery planning
- WHEN invocation guidance is rendered
- THEN `delivery_strategy` is forwarded
- AND `chain_strategy` is forwarded

#### Scenario: Non-Claude guidance forwards both strategy fields to sdd-apply

- GIVEN a non-Claude orchestrator asset invokes `sdd-apply` after planning
- WHEN invocation guidance is rendered
- THEN `delivery_strategy` is forwarded
- AND `chain_strategy` is forwarded

---

### Requirement: Platform-Native Solo Inline Semantics Preservation

For non-Claude solo-inline and platform-native orchestrator assets, the system MUST preserve platform-accurate inline execution wording and MUST NOT add inaccurate subagent or OpenCode persistence claims.

#### Scenario: Windsurf and Antigravity keep inline-accurate wording

- GIVEN Windsurf and Antigravity orchestrator assets are reviewed
- WHEN delegation and execution wording is validated
- THEN wording remains accurate to inline/platform-native execution
- AND wording does not claim persisted OpenCode plugin-backed background delegation

#### Scenario: Other non-Claude platform-native assets avoid inaccurate persistence claims

- GIVEN non-Claude platform-native orchestrator assets are reviewed
- WHEN async/delegation wording is validated
- THEN wording stays platform-accurate
- AND wording does not introduce inaccurate subagent or OpenCode persistence guarantees

---

### Requirement: Claude-Native Delegation Semantics

Claude-targeted guidance MUST describe delegation behavior accurately for Claude Code and MUST NOT imply OpenCode plugin-backed persisted background delegation.

#### Scenario: Guidance avoids persisted-plugin delegation claims

- GIVEN Claude orchestrator guidance content is reviewed for async/delegation wording
- WHEN delegation behavior is described
- THEN wording does not claim OpenCode plugin persistence or background task storage

#### Scenario: Guidance states Claude-accurate delegation behavior

- GIVEN a reader follows the Claude guidance
- WHEN they interpret delegation expectations
- THEN wording reflects Claude Code semantics without OpenCode-specific guarantees

---

### Requirement: Claude Golden and Static Validation Coverage

The system MUST keep Claude golden outputs and static text assertions synchronized with the current Claude orchestrator guidance asset.

The system MUST also keep direct static assertions and impacted golden fixtures synchronized for non-Claude orchestrator assets affected by this capability.

#### Scenario: Claude SDD golden reflects updated chain strategy and forwarding guidance

- GIVEN Claude standalone and combined golden fixtures are generated
- WHEN compared with expected outputs
- THEN both include the updated chain strategy and propagation guidance

#### Scenario: Static assertions align with revised Claude delegation wording

- GIVEN static tests assert expected Claude guidance text fragments
- WHEN tests execute after wording changes
- THEN assertions reflect Claude-native delegation wording
- AND no assertion requires OpenCode plugin-backed persistence phrasing

#### Scenario: Non-Claude static assertions validate strategy and platform wording

- GIVEN direct static tests assert expected non-Claude orchestrator text fragments
- WHEN tests execute after non-Claude asset updates
- THEN assertions verify canonical chain strategy names where delivery planning is present
- AND assertions verify platform-native wording constraints for solo-inline hosts

#### Scenario: Impacted non-Claude golden fixtures reflect current orchestrator guidance

- GIVEN golden fixtures for directly affected non-Claude orchestrator outputs are generated
- WHEN compared with expected outputs
- THEN fixtures include required strategy guidance and propagation where relevant
- AND fixtures preserve accurate platform-native wording without unrelated churn
