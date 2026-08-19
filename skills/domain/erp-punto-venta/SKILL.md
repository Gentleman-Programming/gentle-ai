---
name: domain-erp-punto-venta
description: "Business logic and functional boundary contract for the erp-punto-venta domain (Point of Sale, Billing, Reports). Trigger: orchestrator or qa-supervisor launches tasks affecting sales, pos, or billing."
disable-model-invocation: true
user-invocable: false
license: Apache-2.0
metadata:
  author: Jhunior Gutierrez
  version: "3.0"
  delegate_only: true
---

## Execution Role

Confirm your role before acting. You are the domain consultant or `qa-supervisor` for **Punto de Venta** unless you loaded this skill directly through the `skill()` tool.

- If you are the implementer or QA agent, continue with the phase work below. Do not delegate.
- If you loaded this skill through the `skill()` tool, you are the orchestrator. Stop here and ensure the implementer/QA receives this context.

## Business Language Contract

Use the precise business nouns of the SmartClic POS ecosystem.
- `Comprobante de Pago` (Not Receipt)
- `Caja Chica` (Not Register Drawer)
- `Facturación Electrónica` (Not E-Billing)

## Purpose

Ensure technical implementations or QA automations strictly adhere to Point of Sale operational rules, specifically regarding local reporting and denormalized databases.

## What You Receive

From the orchestrator:
- The SDD tasks list or QA scenarios.

## Execution and Persistence Contract

> Follow **Section B** (retrieval) and **Section C** (persistence) from your agent's own `_shared/sdd-phase-common.md` (e.g. `~/.claude/skills/_shared/...` for Claude Code, `~/.gemini/skills/_shared/...` for Gemini, `~/.codex/skills/_shared/...` for Codex — resolve via your own agent's skills root, not necessarily Claude's).

- **engram**: Log major business discoveries in `sdd/{change-name}/apply-progress/{repo-slug}`.

## What to Do

### Step 1: Validate Database Boundaries (Denormalization Rule)
Punto de Venta uses a **denormalized copy** of the Finanzas database to support high-speed local reporting. 
You must NOT query the live `BD Finanzas` directly for reporting in POS. You must query the denormalized POS tables synchronized by background jobs.

### Step 2: Validate Billing Constraints
Any modification to `Facturación` (Invoicing) must not break electronic billing compliance limits (e.g. tax calculations, XML generation limits).

### Step 3: Implement QA Automation (If applicable)
Use Playwright and the Page Object Model (POM) within the `pages/PuntoVenta/` structure. Isolate Point of Sale tests from Logistics or Finance modules.

## Implementation Rules

| Criteria | Example ✅ | Anti-example ❌ |
|----------|-----------|----------------|
| **POS Reporting** | Querying denormalized local POS tables | Blocking HTTP call to Finanzas API for every report generation |
| **Naming QA (POM)** | `CajaAperturaPage` | `PosOpenPage` (English/Ambiguous) |
| **Tax Calculation**| Utilizing standardized `domain` tax entities | Hardcoding IGV/IVA as `0.18` in UI templates |

## Step 4: Return Summary

Return to the orchestrator:

```markdown
## Domain Compliance Report

**Domain**: erp-punto-venta
**Change**: {change-name}

### Completed Tasks
- [x] Confirmed business nouns used in code.
- [x] Confirmed adherence to denormalized reporting database rules.

### Next Step
{Ready for sdd-verify}
```
