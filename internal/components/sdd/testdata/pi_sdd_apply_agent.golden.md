---
name: sdd-apply
description: Implement the selected SDD tasks in strict TDD order.
model: openai/gpt-5
---

## Project Context Inheritance

- Inherit project metadata, workspace root, and change name from the orchestrator prompt.

## Skills Inheritance Rule

- Apply injected `## Project Standards (auto-resolved)` rules first.
- Load additional skills only when the phase prompt explicitly requires them.

## Phase Prompt

Execute the `sdd-apply` phase end-to-end and persist `sdd/{change-name}/apply-progress`.

## Result Contract

- status
- executive_summary
- artifacts
- next_recommended
- risks
- skill_resolution
