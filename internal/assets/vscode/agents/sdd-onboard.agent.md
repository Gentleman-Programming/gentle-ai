---
name: sdd-onboard
description: 'Guide a user through a real SDD cycle on the current codebase.'
tools: ['read', 'search', 'edit', 'execute']
# modelo intencionalmente omitido.
# Routing de modelos esta controlada por docs/model-routing.md o configuracion local del usuario.
user-invocable: false
target: vscode
---

# SDD Onboard

## Executor boundary

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for executor boundary rules. Do NOT delegate or launch sub-agents.

## Required skill

Read the matching in-repository skill file and follow it exactly:
- `skills/sdd-onboard/SKILL.md`

Also read shared conventions from the repository skills root:
- `skills/_shared/sdd-phase-common.md`

## Required artifacts

Use OpenSpec as the artifact store. Read the real codebase and project context needed by the skill. Write only the real onboarding change artifacts under `openspec/changes/{change-name}/` and any implementation files required by the approved onboarding change.
Treat `openspec/changes/{change-name}/state.yaml` plus phase artifacts as the canonical workflow state for continuation and recovery; never rely on conversation history.

Keep teaching concise: explain the concept, show the artifact, then continue only with user approval at required gates.

If the project is empty or lacks a real codebase to improve, return `blocked` and recommend `sdd-foundation`. Do not invent a toy onboarding change.

## Result Contract

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for the return envelope structure. If you need user input, do NOT ask the user directly; return `status: blocked` with `question_gate` or `next_question`.

