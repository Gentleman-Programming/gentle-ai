---
description: Start a medium or large change with the SDD planning lifecycle in Windsurf
---

# /sdd-new

Use this workflow for a new feature, a multi-file change, architectural risk, or any request that needs a formal implementation contract. Small maintenance tasks do not need this workflow.

## Operating rules

- Enter Plan Mode immediately. Do not write production code or switch to Code Mode during planning.
- The orchestrator owns lifecycle state and product discovery. Phase executors consume explicit handoffs and do not invent decisions.
- Generated technical artifacts default to English unless the user explicitly requests another artifact language.
- Stop whenever required state is missing, selected research is incomplete, product decisions are unresolved, or hybrid stores disagree.

## Required sequence

### 1. Initialize OpenSpec and Engram lifecycle

Run the installed `sdd-init` flow before creating planning artifacts. Resolve the change name, artifact store, project root, and allowed edit roots.

- OpenSpec artifacts live under `openspec/changes/<change>/`.
- Engram artifacts use the `sdd/<change>/` topic family.
- In hybrid mode, persist the same lifecycle state to both stores and require matching revisions and bytes.

Recover prior decisions with Engram when available, then read repository instructions and relevant architecture context. Never invent conventions when context is missing.

### 2. Run local exploration

Run `sdd-explore` for repository-local investigation before external research or proposal work. Record the exploration outcome and references in pre-proposal state.

Exploration may inspect current code, architecture, constraints, and risks. It does not make unconfirmed product decisions and does not create the proposal or specification.

### 3. Run optional admitted research

Research is optional until explicitly selected. If the user or orchestrator selects it, persist the request before source access and run `sdd-research` only with exact admitted evidence grants.

Selected research must finish with a source-backed `done` artifact and valid references. Denied, partial, blocked, missing, or divergent research remains persisted and blocks proposal readiness. If research is not selected, skip only this step.

### 4. Confirm product decisions

After exploration and any selected research, resolve product choices separately from evidence claims.

- In interactive mode, ask only for unresolved choices and persist the answers.
- In automatic mode, use confirmed facts or emit one lossless grouped decision prompt, persist the pending state, and stop.
- Continue only when decisions are confirmed and all required references are valid.

### 5. Create proposal and specification

When pre-proposal state is ready, invoke `sdd-propose` with the confirmed handoff. The proposer must not interview the user. Persist the proposal through the initialized store, then invoke `sdd-spec` to define requirements and scenarios.

The strict order is:

1. Initialize lifecycle state.
2. Complete local exploration.
3. Complete optional selected research.
4. Confirm product decisions.
5. Run `sdd-propose`.
6. Run `sdd-spec`.

## Approval gate

Present a concise planning summary with the objective, scope, risks, unresolved blockers, and persisted artifact locations. Ask for explicit approval before implementation.

After approval, continue through design and tasks before `sdd-apply`. Never treat silence as approval, create commits, or begin partial implementation from this workflow.

## Completion criteria

This workflow is complete only when lifecycle state is persisted, local exploration is recorded, selected research is done or explicitly unselected, product decisions are confirmed, proposal and specification artifacts exist in the initialized store, and the user has explicitly approved the plan.
