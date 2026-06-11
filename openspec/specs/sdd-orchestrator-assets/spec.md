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

### Requirement: chained-pr Skill Binding in Chain Strategy Guidance

All 11 SDD orchestrator assets MUST bind the `chained-pr` skill into their chain-strategy guidance. When delivery planning produces chained or stacked PRs, the orchestrator MUST resolve the `chained-pr` skill by its registry name (not a hardcoded filesystem path) through its existing Sub-Agent Launch Pattern, and MUST inject the resolved skill path into `sdd-tasks` and `sdd-apply` phase prompts under `## Skills to load before work`. Those phases MUST read and follow the skill BEFORE planning or creating any PR.

The binding MUST reference the skill by registry name `chained-pr` (frontmatter `gentle-ai-chained-pr`). No hardcoded path to a `SKILL.md` file is permitted as the binding reference.

#### Scenario: Orchestrator resolves chained-pr skill by registry name on chained delivery

- GIVEN an SDD orchestrator asset has chain strategy guidance
- WHEN delivery planning determines that chained or stacked PRs are required
- THEN the orchestrator resolves the `chained-pr` skill by its registry name through the Sub-Agent Launch Pattern
- AND the resolved skill path is injected into the `sdd-tasks` prompt under `## Skills to load before work`
- AND the resolved skill path is injected into the `sdd-apply` prompt under `## Skills to load before work`
- AND no hardcoded filesystem path is used as the binding reference

#### Scenario: Binding is present in all 11 orchestrator templates

- GIVEN the 11 SDD orchestrator templates are reviewed (antigravity, claude, codex, cursor, gemini, generic, kimi, kiro, opencode, qwen, windsurf)
- WHEN the chain strategy section of each template is inspected
- THEN each template contains a binding instruction referencing the `chained-pr` skill by registry name
- AND the binding instruction is located inside or immediately adjacent to the chain strategy section

#### Scenario: sdd-tasks sub-agent reads chained-pr skill before planning PRs

- GIVEN an `sdd-tasks` sub-agent is launched with a `chain_strategy` requiring chained PRs
- WHEN the sub-agent begins planning
- THEN the prompt includes `chained-pr` skill under `## Skills to load before work`
- AND the sub-agent reads and follows the skill before producing a PR plan

#### Scenario: sdd-apply sub-agent reads chained-pr skill before creating PRs

- GIVEN an `sdd-apply` sub-agent is launched for a change requiring chained PRs
- WHEN the sub-agent begins implementation
- THEN the prompt includes `chained-pr` skill under `## Skills to load before work`
- AND the sub-agent reads and follows the skill before creating any PR

---

### Requirement: Inline Chain Strategy Summary Retained

All 11 SDD orchestrator assets MUST retain a short inline summary of both `stacked-to-main` and `feature-branch-chain` strategies in their chain strategy section. The binding supplements, and does NOT replace, the existing inline summary. Canonical strategy names MUST continue to appear verbatim so that existing "Strategy naming remains canonical" scenarios continue to hold.

#### Scenario: Chain strategy inline summary coexists with skill binding

- GIVEN an SDD orchestrator asset contains a chain strategy section with the skill binding
- WHEN the section is inspected
- THEN `stacked-to-main` appears as a canonical strategy name
- AND `feature-branch-chain` appears as a canonical strategy name
- AND a binding instruction referencing the `chained-pr` skill by registry name also appears
- AND the binding instruction does not replace the inline strategy summaries

#### Scenario: Static validation passes after binding is added

- GIVEN static-validation tests assert canonical strategy names and chain strategy section presence
- WHEN tests execute against the updated orchestrator templates
- THEN all assertions for canonical strategy names continue to pass
- AND new binding assertions also pass
- AND no test requires the binding to have replaced the inline summary

---

### Requirement: Cursor Chain Strategy Section Parity

The `cursor/sdd-orchestrator.md` template MUST include a `### Chain Strategy` section that is at parity with the other 10 templates. The section MUST enumerate both canonical strategy names, forward `chain_strategy` to downstream phases, and include the `chained-pr` skill binding.

#### Scenario: Cursor template includes a Chain Strategy section

- GIVEN the cursor SDD orchestrator template is inspected
- WHEN the document structure is reviewed
- THEN a `### Chain Strategy` section is present
- AND it includes `stacked-to-main`
- AND it includes `feature-branch-chain`

#### Scenario: Cursor forwards chain_strategy to downstream phases

- GIVEN the cursor SDD orchestrator template invokes `sdd-tasks` or `sdd-apply`
- WHEN the invocation guidance is rendered
- THEN `delivery_strategy` is forwarded
- AND `chain_strategy` is forwarded

#### Scenario: Cursor includes the chained-pr skill binding

- GIVEN the cursor SDD orchestrator template chain strategy section is inspected
- WHEN the binding instruction is reviewed
- THEN the `chained-pr` skill is referenced by registry name
- AND the binding instructs injection into `sdd-tasks` and `sdd-apply` prompts

#### Scenario: Cursor chain strategy naming is canonical

- GIVEN a static validation checks chain strategy labels in the cursor template
- WHEN the check runs
- THEN no alias or renamed value replaces the canonical strategy names `stacked-to-main` and `feature-branch-chain`

---

### Requirement: Platform-Accurate Binding Wording for Solo-Inline and Platform-Native Hosts

For solo-inline and platform-native orchestrator hosts (windsurf, antigravity, kimi, kiro), the skill-binding instruction MUST phrase "inject under `## Skills to load before work`" in a way that maps to each host's actual phase-context mechanism. The binding MUST NOT reintroduce OpenCode-style persisted-delegation claims that the existing spec forbids for these hosts.

#### Scenario: Solo-inline binding wording is platform-accurate

- GIVEN a solo-inline orchestrator asset (windsurf or antigravity) is reviewed
- WHEN the skill binding instruction is read
- THEN the binding references the `chained-pr` skill by registry name
- AND the wording maps skill injection to the host's inline or platform-native phase-context mechanism
- AND the wording does not claim persisted OpenCode plugin-backed background delegation

#### Scenario: Platform-native non-Claude binding wording avoids inaccurate persistence claims

- GIVEN a platform-native non-Claude orchestrator asset (kimi or kiro) is reviewed
- WHEN the skill binding instruction is read
- THEN the binding references the `chained-pr` skill by registry name
- AND the wording does not introduce inaccurate subagent or OpenCode persistence guarantees
- AND the wording stays platform-accurate per the existing solo-inline semantics requirement

---

### Requirement: Golden Fixture and Static Assertion Coverage for the Binding

Static-validation tests and golden fixtures that cover chain strategy content MUST be updated to assert the presence of the `chained-pr` skill binding. Golden churn MUST be limited to the intended binding wording and the new Cursor chain strategy section; no unrelated content changes are permitted.

#### Scenario: New static assertions verify binding presence in all 11 templates

- GIVEN static-validation tests run after the change is applied
- WHEN binding assertions execute
- THEN each of the 11 orchestrator templates has a binding assertion that passes
- AND the assertions verify the registry-name reference, not a hardcoded path

#### Scenario: Cursor is added to chain-strategy parity static assertions

- GIVEN the existing chain-strategy parity assertions are updated to include cursor
- WHEN static tests execute
- THEN a chain-strategy parity assertion for the cursor template passes
- AND the assertion verifies that canonical strategy names are present

#### Scenario: Golden fixtures updated without unrelated churn

- GIVEN the 12 golden fixtures that reference Chain Strategy are regenerated
- WHEN the regenerated fixtures are compared with the previous versions
- THEN the diff is limited to the binding wording and the new Cursor chain strategy section
- AND no unrelated content is modified in the golden fixtures

#### Scenario: inject_test.go binding assertion passes

- GIVEN `internal/components/sdd/inject_test.go` is updated with a binding assertion
- WHEN the test executes
- THEN the assertion verifies that the `chained-pr` skill binding is present in generated prompts for `sdd-tasks` and `sdd-apply` when chain strategy is active

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
