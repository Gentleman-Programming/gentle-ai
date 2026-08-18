---
name: erp-mf-punto-venta-menu-profile
description: "Agent execution contract and architectural invariant enforcement for the erp-mf-punto-venta-menu repository. Trigger: orchestrator launches code implementation in erp-mf-punto-venta-menu."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `frontend-implementer` sub-agent for the **erp-mf-punto-venta-menu** repository unless you loaded this skill directly through the `skill()` tool.

- If you are the `frontend-implementer` sub-agent, continue with the phase work below. Do not delegate.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `frontend-implementer` sub-agent.

## Language Domain Contract

- **Code:** TypeScript, Vue 3. Kebab-case for `.vue` files.
- **Commits & PRs:** Must be written in Spanish using standard semantic commit types (`feat`, `fix`, `refactor`).
- **Artifacts:** Generated technical artifacts (like `apply-progress`) default to English.

## Purpose

You are a sub-agent responsible for implementing changes in the `erp-mf-punto-venta-menu` repository.
*Note: As of August 2026, this repository appears to be an abandoned or empty stub in GitLab (no package.json or src).*

## What You Receive

From the orchestrator:
- The exact SDD tasks list to implement.
- Target branch and target environment.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).

- **engram**: Read `sdd/{change-name}/tasks` (required) and `sdd/{change-name}/spec` (required). Save your progress as `sdd/{change-name}/apply-progress/{repo-slug}`.

## What to Do

### Step 1: Verify State
Check if the repository has been initialized. If it is empty, ask the `solution-architect` for a bootstrap.

### Step 2: Enforce Architectural Invariants (Hexagonal Frontend)
If initialized, it should follow the standard Clean Architecture / Hexagonal approach. You MUST respect the clean architecture segregation implemented in `src/core/`.
1. `domain`: Pure entities and constants ONLY.
2. `application`: Business use cases and orchestration.
3. `infraestructure`: Adapters and API integrations.

### Step 6: Return Summary

Return to the orchestrator:

```markdown
## Implementation Report

**Repository**: erp-mf-punto-venta-menu
**Change**: {change-name}

### Completed Tasks
- [x] 1.1 {Concrete action}

### Architectural Verification
- Confirmed repository state.

### Next Step
{Ready for sdd-verify OR specify remaining tasks}
```
