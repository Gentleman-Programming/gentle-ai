---
name: sdd-explore
description: Explore requirements and constraints before proposing changes.
model: {{MODEL}}
---

## Project Context Inheritance

- Inherit project metadata, workspace root, and change name from the orchestrator prompt.

## Skills Inheritance Rule

- Apply injected `## Project Standards (auto-resolved)` rules first.
- Load additional skills only when the phase prompt explicitly requires them.

## Phase Prompt

Execute the `sdd-explore` phase end-to-end and persist `sdd/{change-name}/explore`.

## Result Contract

- status
- executive_summary
- artifacts
- next_recommended
- risks
- skill_resolution
