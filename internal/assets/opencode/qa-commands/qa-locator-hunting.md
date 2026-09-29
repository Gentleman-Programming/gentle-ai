---
description: Hunt a UI locator/selector for an erp-mf-* microfront E2E test — POM, live DOM, GitLab, never invented
agent: gentle-orchestrator
---

Load `qa-locator-hunting` first, then use it to resolve the locator/selector
the user needs: "$ARGUMENTS".

This is a standalone QA tool: it runs with or without an active `{change}`
and never advances the QA ledger (`qa-begin`/`qa-finish`) by itself. Follow
its strict NIVEL 0 → NIVEL 1 → NIVEL 2 search order and its attribute
priority (`data-testid > id > name > formControlName > aria-label > clases
CSS`) — never invent a selector, a `data-testid`, or any other unconfirmed
attribute.
