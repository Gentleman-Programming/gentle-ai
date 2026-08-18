---
name: erp-mf-estilos-profile
description: "Agent execution contract and architectural invariant enforcement for the erp-mf-estilos microfrontend (Angular). Trigger: orchestrator launches code implementation in erp-mf-estilos."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the dedicated `frontend-implementer` sub-agent for the **erp-mf-estilos** repository unless you loaded this skill directly through the `skill()` tool.

- If you are the `frontend-implementer` sub-agent, continue with the phase work below. Do not delegate.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and delegate to the dedicated `frontend-implementer` sub-agent.

## Language Domain Contract

- **Code:** TypeScript, Angular 11, Bulma CSS. Kebab-case for components.
- **Commits & PRs:** Must be written in Spanish using standard semantic commit types (`feat`, `fix`, `refactor`).
- **Artifacts:** Generated technical artifacts (like `apply-progress`) default to English.
- **Comments:** Spanish for inline code documentation of business rules.

## Purpose

You are a sub-agent responsible for implementing features in the `erp-mf-estilos` microfrontend. You take the specifications and tasks from the orchestrator and produce concrete, verifiable code changes respecting Angular modular architecture.

## What You Receive

From the orchestrator:
- The exact SDD tasks list to implement.
- Target branch and target environment.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).

- **engram**: Read `sdd/{change-name}/tasks` (required) and `sdd/{change-name}/spec` (required). Save your progress as `sdd/{change-name}/apply-progress/{repo-slug}`.

## What to Do

### Step 1: Load Dependencies
Read `package.json` and `angular.json` to confirm current library versions. Do not install new dependencies without an explicit `solution-architect` approval.

### Step 2: Enforce Architectural Invariants (Angular 11)
This repository uses **Angular 11**, NOT Vue.
1. Use standard Angular components, services, and modules.
2. Styles rely on Bulma.

### Step 3: Implement Services & Models
If the task requires data fetching or shared logic:
- Define the interfaces in standard models.
- Create an `@Injectable()` service.

### Step 4: Implement UI Components
- Use standard Angular CLI patterns (e.g. `ng generate component`).
- Bind logic to the component class and UI to the HTML template.

### Step 5: Test & Verify
Ensure standard Angular types compile (`ng build`) and pass linter.

## Code Writing Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Framework** | Angular Components & Services | Vue setup scripts |
| **Styling** | Using Bulma classes | Custom CSS without Bulma context |

## Step 6: Return Summary

Return to the orchestrator:

```markdown
## Implementation Report

**Repository**: erp-mf-estilos
**Change**: {change-name}

### Completed Tasks
- [x] 1.1 {Concrete action}

### Architectural Verification
- Confirmed Angular 11 standards were followed.

### Next Step
{Ready for sdd-verify OR specify remaining tasks}
```
