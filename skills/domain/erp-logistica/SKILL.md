---
name: domain-erp-logistica
description: "Business logic and functional boundary contract for the erp-logistica domain. Trigger: orchestrator or qa-supervisor launches tasks affecting inventory, dispatch, or logistics."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the domain consultant or `qa-supervisor` for **Logística** unless you loaded this skill directly through the `skill()` tool.

- If you are the `qa-supervisor` or implementing backend logic, continue with the phase work below. Do not delegate.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and ensure the implementer/QA agent receives this context.

## Business Language Contract

Generated technical artifacts and code MUST use the precise business nouns of the Logística ecosystem. Do not use synonyms or translations.
- `Guía de Remisión` (Not Dispatch Note)
- `Inventario` (Not Stock Array)
- `Familia` (Not Category Group)
- `Almacén` (Not Warehouse)

## Purpose

You are responsible for ensuring that any technical implementation or QA automation strictly adheres to the real-world operational rules of the SmartClic Logistics module.

## What You Receive

From the orchestrator:
- The exact SDD tasks list or QA scenarios to implement.
- Target module (e.g., Almacenes, Kardex).

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).

- **engram**: Log major business discoveries or edge-case decisions in `sdd/{change-name}/apply-progress/{repo-slug}`.

## What to Do

### Step 1: Validate Database Boundaries
Confirm that your implementation ONLY connects to `BD: Logistica`. The logistics domain must never directly write to `BD Finanzas`.

### Step 2: Leverage Stored Procedures
Check if the requested data extraction already exists as a Stored Procedure (e.g., `PROC_LISTAUBIGEOS`). If it does, your code MUST call the SP rather than writing raw SQL queries.

### Step 3: Implement QA Automation (If applicable)
If you are writing tests, you must use **Playwright**.
1. Navigate to `pages/Logistica/`.
2. Implement the **Page Object Model (POM)**.
3. Verify using the CLI: `npx playwright test --project Logistica`.

## Implementation Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **Naming QA (POM)** | `PedidoListaPage` | `BcsPage` (ambiguous acronyms) |
| **Data Fetching** | `EXEC PROC_LISTAUBIGEOS` | `SELECT * FROM tb_ubigeos` |
| **E2E Isolation** | Isolated in `playwright.config.ts` | Tightly coupled to Point of Sale tests |

## Step 4: Return Summary

Return to the orchestrator:

```markdown
## Domain Compliance Report

**Domain**: erp-logistica
**Change**: {change-name}

### Completed Tasks
- [x] Confirmed business nouns used in code.
- [x] Confirmed adherence to `BD: Logistica`.

### Next Step
{Ready for sdd-verify}
```
