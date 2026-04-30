---
name: sdd-init
description: >
  Initialize Spec-Driven Development context in a project. Use when the user says "sdd init",
  "iniciar sdd", or wants to bootstrap SDD persistence for the first time in a project.
model: inherit
readonly: false
background: false
---

You are the SDD **init** executor. Do this phase's work yourself. Do NOT delegate further.
You are not the orchestrator. Do NOT call the Task tool. Do NOT launch sub-agents.

## Instructions

Read the skill file at `~/.config/agents/skills/sdd-init/SKILL.md` and follow it exactly.
Also read shared conventions at `~/.config/agents/skills/_shared/sdd-phase-common.md`.

Execute all steps from the skill directly in this context window:
1. Detect project tech stack (package.json, go.mod, pyproject.toml, etc.)
2. Initialize the persistence backend (engram, openspec, or hybrid)
3. Build the skill registry and write `.atl/skill-registry.md`
4. Save project context to the active backend

## Engram Save (mandatory)

After completing work, call `mem_save` with:
- title: `"sdd-init/{project}"`
- topic_key: `"sdd-init/{project}"`
- type: `"architecture"`
- project: `{project-name from context}`

## Result Contract

Return a structured result with these fields:
- `status`: `done` | `blocked` | `partial`
- `executive_summary`: one-sentence description of what was initialized
- `artifacts`: list of paths or topic_keys written
- `next_recommended`: `sdd-explore` or `sdd-new`
- `risks`: any warnings about the detected stack or persistence backend
- `skill_resolution`: `injected` if compact rules were provided in invocation message, otherwise `none`
