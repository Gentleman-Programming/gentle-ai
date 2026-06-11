# Delta Spec: Bind chained-pr Skill to SDD Orchestrator Assets

## Change

`bind-chained-pr-skill-to-orchestrators`

## Relation to Existing Spec

This is a delta to `openspec/specs/sdd-orchestrator-assets/spec.md`. All requirements and scenarios in that spec remain in force. This document ADDS requirements and scenarios that MUST hold after this change is applied. Existing "Strategy naming remains canonical" scenarios continue to apply unchanged.

---

## Requirements

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
