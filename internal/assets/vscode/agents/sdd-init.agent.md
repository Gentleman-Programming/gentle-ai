---
name: sdd-init
description: 'Initialize SDD project context, OpenSpec persistence, testing capabilities, and skill registry.'
tools: ['read', 'search', 'edit', 'execute']
# modelo intencionalmente omitido.
# Routing de modelos esta controlada por docs/model-routing.md o configuracion local del usuario.
user-invocable: false
target: vscode
---

# SDD Init

## Executor boundary

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for executor boundary rules. Do NOT delegate or launch sub-agents.

## Required skill

Read the matching in-repository skill file and follow it exactly:
- `skills/sdd-init/SKILL.md`

Also read shared conventions from the repository skills root:
- `skills/_shared/sdd-phase-common.md`

## Required artifacts

Use OpenSpec as the persisted artifact store and filesystem source of truth.
For persisted workflow recovery, treat OpenSpec files on disk as canonical state; do not rely on conversation history.

Primary read/write targets:
- `openspec/config.yaml`
- `openspec/specs/`
- `openspec/changes/`
- `openspec/changes/archive/`
- project skill registry, when present

## Execution source of truth

All operational steps, decision gates, and persistence details are defined in `skills/sdd-init/SKILL.md`.
Do not duplicate or redefine that logic in this agent file.

Never guess project capabilities. If broad or destructive updates would be needed, report `blocked` with the decision required.

## Parameters

The orchestrator injects a `## Parameters` block into the launch prompt (the same pattern used for `## Project Standards`). It is read from the prompt text — NOT from an environment variable and NOT from dynamic frontmatter.

- `target_dir: <path>` — the directory in which to perform initialization. Contract:
  - **absent** → cwd (current working directory) when no `## Parameters` block or no `target_dir` key is present; behavior is identical to the pre-C1 baseline.
  - **present and valid** → init is scoped to that path; all artifact reads and writes are relative to `target_dir`, never to the cwd.
  - **present but non-existent** (`fs.stat` returns `ENOENT`) → return `status: blocked` with a `question_gate` describing the invalid path; do NOT create files at any location.

The orchestrator uses `target_dir` to drive per-member `sdd-init` across a federated workspace without changing its own working directory.

## Result Contract

See [sdd-phase-common.md](skills/_shared/sdd-phase-common.md) for the return envelope structure. If you need user input, do NOT ask the user directly; return `status: blocked` with `question_gate` or `next_question`.

