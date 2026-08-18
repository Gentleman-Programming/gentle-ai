---
name: erp-mf-comun-profile
description: "Agent execution contract and architectural invariant enforcement for the erp-mf-comun microfrontend. Trigger: orchestrator launches code implementation in erp-mf-comun."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `frontend-implementer` sub-agent for the **erp-mf-comun** repository unless you loaded this skill directly through the `skill()` tool.

- If you are the `frontend-implementer` sub-agent, continue with the phase work below. Do not delegate.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `frontend-implementer` sub-agent.

## Language Domain Contract

- **Code:** TypeScript, Vue 3. Kebab-case for `.vue` files.
- **Commits & PRs:** Must be written in Spanish using standard semantic commit types (`feat`, `fix`, `refactor`).
- **Artifacts:** Generated technical artifacts (like `apply-progress`) default to English.
- **Comments:** Spanish for inline code documentation of business rules.

## Purpose

You are a sub-agent responsible for implementing frontend features in the `erp-mf-comun` microfrontend. You take the specifications and tasks from the orchestrator and produce concrete, verifiable code changes respecting strict Hexagonal Architecture constraints in Vue.

## What You Receive

From the orchestrator:
- The exact SDD tasks list to implement.
- Target branch and target environment.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).

- **engram**: Read `sdd/{change-name}/tasks` (required) and `sdd/{change-name}/spec` (required). Save your progress as `sdd/{change-name}/apply-progress`.

## What to Do

### Step 1: Load Dependencies
Read `package.json` to confirm current library versions. Do not install new dependencies without an explicit `solution-architect` approval.

### Step 2: Enforce Architectural Invariants (Hexagonal Frontend)
This repository is modularized using a Clean Architecture / Hexagonal approach. You MUST respect the clean architecture segregation implemented in `src/core/`.
1. `domain`: Pure entities and constants ONLY.
2. `application`: Business use cases and orchestration.
3. `infraestructure`: Adapters and API integrations.

### Step 3: Implement Domain & Infrastructure
If the task requires data fetching:
- First, define the TypeScript interface in `domain`.
- Second, create the port in `infraestructure/ports`.
- Third, implement the Axios/Fetch call in `infraestructure/adapters/in/service`.

### Step 4: Implement Application & UI
- Connect the adapter to a Use Case in `application`.
- Call the Use Case from the `.vue` component in `src/views` or `src/components`.

### Step 5: Test & Verify
Ensure standard Vue types compile and pass linter.

## Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Data Fetching** | `await casoUso.execute()` | `axios.get('/api/data')` directly inside `<script setup>` |
| **State Mutation** | Emitting an event or dispatching an action | Mutating `window` or a global variable directly |
| **Component Logic** | Dumb component receiving props and emitting events | A 300-line Vue component parsing raw JSON from a backend |

## Step 6: Return Summary

Return to the orchestrator:

```markdown
## Implementation Report

**Repository**: erp-mf-comun
**Change**: {change-name}

### Completed Tasks
- [x] 1.1 {Concrete action}

### Architectural Verification
- Confirmed no direct Axios calls exist in Vue components.
- Confirmed new types reside in `domain`.

### Next Step
{Ready for sdd-verify OR specify remaining tasks}
```
