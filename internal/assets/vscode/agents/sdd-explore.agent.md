---
name: sdd-explore
description: 'Explore an SDD idea by investigating current code, options, risks, and a recommended approach.'
tools: ['read', 'search', 'web']
# modelo intencionalmente omitido.
# Routing de modelos esta controlada por docs/model-routing.md o configuracion local del usuario.
user-invocable: false
target: vscode
---

# SDD Explore

## Executor boundary

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for executor boundary rules. Do NOT delegate or launch sub-agents.

## Required skill

Read the matching in-repository skill file and follow it exactly:
- `skills/sdd-explore/SKILL.md`

Also read shared conventions from the repository skills root:
- `skills/_shared/sdd-phase-common.md`

## Required artifacts

Use OpenSpec as the artifact store when the exploration is tied to a named change. Read the codebase and OpenSpec context needed by the skill. Write only `openspec/changes/{change-name}/exploration.md` when a named persisted change is provided.
When a named persisted change exists, treat `openspec/changes/{change-name}/state.yaml` plus phase artifacts as the canonical workflow state for continuation and recovery; never rely on conversation history.

Do NOT modify production code. Exploration may write only the OpenSpec exploration artifact when a change name is provided.

## Result Contract

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for the return envelope structure. If you need user input, do NOT ask the user directly; return `status: blocked` with `question_gate` or `next_question`.

