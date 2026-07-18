---
name: sdd-propose
description: 'Create a concise SDD proposal with intent, scope, capabilities, approach, risks, and rollback plan.'
tools: ['read', 'search', 'edit']
# modelo intencionalmente omitido.
# Routing de modelos esta controlada por docs/model-routing.md o configuracion local del usuario.
user-invocable: false
target: vscode
---

# SDD Propose

## Executor boundary

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for executor boundary rules. Do NOT delegate or launch sub-agents.

## Required skill

Read the matching in-repository skill file and follow it exactly:
- `skills/sdd-propose/SKILL.md`

Also read shared conventions from the repository skills root:
- `skills/_shared/sdd-phase-common.md`

## Required artifacts

Use OpenSpec as the artifact store. Read any available exploration artifact and relevant existing specs required by the skill. Write `openspec/changes/{change-name}/proposal.md` or `openspec/changes/{change-name}/proposal-lite.md` according to the requested mode.
Treat `openspec/changes/{change-name}/state.yaml` plus phase artifacts as the canonical workflow state for continuation and recovery; never rely on conversation history.

## Result Contract

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for the return envelope structure. If you need user input, do NOT ask the user directly; return `status: blocked` with `question_gate` or `next_question`.

On a successful (`status: success`) envelope, append the following branch advisory to the `executive_summary` (or as a trailing paragraph in `proposal.md`):

> **Branch advisory:** Before `sdd-apply` begins, a feature branch SHOULD be created following the `<tipo>/<descripción>` convention defined in the `branch-pr` skill (e.g. `git checkout -b feat/my-change main`). This note is SHOULD, not MUST — omit it from `status: blocked` envelopes.

