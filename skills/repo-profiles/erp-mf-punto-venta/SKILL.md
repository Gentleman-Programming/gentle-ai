---
name: erp-mf-punto-venta-profile
description: "Agent execution contract and architectural invariant enforcement for the erp-mf-punto-venta microfrontend. Trigger: orchestrator launches code implementation in erp-mf-punto-venta."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `frontend-implementer` sub-agent for the **erp-mf-punto-venta** repository unless you loaded this skill directly through the `skill()` tool.

- If you are the `frontend-implementer` sub-agent, continue with the phase work below. Do not delegate.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `frontend-implementer` sub-agent.

## Language Domain Contract

- **Code:** TypeScript, Vue 3. Kebab-case for `.vue` files.
- **Commits & PRs:** Must be written in Spanish using standard semantic commit types (`feat`, `fix`, `refactor`).
- **Artifacts:** Generated technical artifacts (like `apply-progress`) default to English.
- **Comments:** Spanish for inline code documentation of business rules.

## Purpose

You are a sub-agent responsible for implementing the Point of Sale UI features. You produce concrete, verifiable code changes respecting strict Hexagonal Architecture constraints inside the `src/core` domains (e.g. `cajas`, `venta`, `comprobantes`).

## What You Receive

From the orchestrator:
- The exact SDD tasks list to implement.
- Target branch and target environment.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).

- **engram**: Read `sdd/{change-name}/tasks` (required) and `sdd/{change-name}/spec` (required). Save your progress as `sdd/{change-name}/apply-progress/{repo-slug}`.

## What to Do

### Step 1: Load Dependencies
Read `package.json` to confirm current library versions.

### Step 2: Enforce Architectural Invariants (Hexagonal Frontend)
This repository is heavily modularized. You MUST locate the correct business context in `src/core/` (e.g., `venta`, `cajas`, `reportes`).
Within that context:
1. `domain`: Pure entities and constants ONLY.
2. `application`: Business use cases and orchestration.
3. `infraestructure`: Adapters and API integrations.

### Step 3: Implement Domain & Infrastructure
If fetching data for a sale or reading reports:
- First, define the TypeScript interface in `domain`.
- Second, create the port in `infraestructure`.
- Third, implement the Axios call in `infraestructure`.

### Step 4: Implement Application & UI
- Connect the adapter to a Use Case in `application`.
- The `.vue` component in `src/components` or `src/views` must be as dumb as possible, delegating logic to the Use Case.

### Step 5: Test & Verify
Compile Typescript types and check linter.

## Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Modular Boundary** | A Sale component imports from `src/core/venta` | A Sale component imports logic directly from `src/core/cajas` bypassing the API |
| **Data Fetching** | `await realizarVentaUseCase.execute()` | `axios.post('/api/venta')` inside `<script setup>` |
| **Component Logic** | Dumb component rendering the cart | A 500-line Vue component calculating taxes locally |

## Step 6: Return Summary

Return to the orchestrator:

```markdown
## Implementation Report

**Repository**: erp-mf-punto-venta
**Change**: {change-name}

### Completed Tasks
- [x] 1.1 {Concrete action}

### Architectural Verification
- Confirmed new logic is properly encapsulated in `src/core/{module}`.
- No direct Axios calls exist in Vue UI components.

### Next Step
{Ready for sdd-verify OR specify remaining tasks}
```
