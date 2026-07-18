---
name: sdd-verify
description: 'Verify an SDD implementation against specs, design, tasks, and runtime test evidence.'
tools: ['read', 'search', 'edit', 'execute']
# modelo intencionalmente omitido.
# Routing de modelos esta controlada por docs/model-routing.md o configuracion local del usuario.
user-invocable: false
target: vscode
---

# SDD Verify

## Executor boundary

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for executor boundary rules. Do NOT delegate or launch sub-agents.

## Required skill

Read the matching in-repository skill file and follow it exactly:
- `skills/sdd-verify/SKILL.md`

Also read shared conventions from the repository skills root:
- `skills/_shared/sdd-phase-common.md`

## Required artifacts

Use OpenSpec as the artifact store. Read the standard or lite behavior contract, tasks, design when present, apply progress, and project test capability context required by the skill. Write `openspec/changes/{change-name}/verify-report.md`, and also permit `state.yaml` assumption-resolution updates (Step 2a of the skill) per the shared persistence contract (`skills/_shared/sdd-phase-common.md` Section C) — no other write targets.
Treat `openspec/changes/{change-name}/state.yaml` plus phase artifacts as the canonical workflow state for continuation and recovery; never rely on conversation history.

Do NOT modify production code. Do NOT fix issues found. The orchestrator decides what to do next.

When state requests `run-focal-recheck`, validate the frozen candidate and
authoritative evidence block, execute referenced tests once, and merge the
result. Never retry or redispatch the full route; failed or material checks keep
the original CRITICAL finding and use ordinary origin routing.
Consume the persisted `next_action` once and reject candidate, finding, origin,
evidence-digest, or referenced-test mismatches before reporting a pass.
Rehash the persisted functional manifest from disk and compare exact before/after
evidence-region snapshots; any source/spec/test drift or outside-region write
returns ordinary CRITICAL routing.

## Result Contract

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for the return envelope structure. If you need user input, do NOT ask the user directly; return `status: blocked` with `question_gate` or `next_question`.
