# SDD orchestrator assets

## ADDED Requirements

### Requirement: Antigravity uses dynamic subagent orchestration

The `antigravity` SDD orchestrator asset MUST instruct the agent to define and invoke phase subagents dynamically at runtime.

#### Scenario: Antigravity orchestrator includes dynamic subagent delegation

- GIVEN the Antigravity orchestrator asset is reviewed
- WHEN the asset content is inspected
- THEN it includes `define_subagent`
- AND it includes `invoke_subagent`
- AND it directs Antigravity to read phase skills from `~/.gemini/antigravity-cli/skills/{phase}/SKILL.md` or workspace `.agents/skills/{phase}/SKILL.md`
- AND it avoids inline phase execution as the primary execution strategy
