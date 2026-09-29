---
name: qa-doc-access
description: "Trigger: BookStack MCP search, cite pages, STOP on gap, never self-decide divergence. Provides document access guidelines."
license: Apache-2.0
metadata:
  author: JhuniorBrayan123
  version: "1.0"
---

## BookStack Search Protocol (MANDATORY)

1. Use the BookStack MCP to search for official documentation before making assumptions.
2. Cite all pages used with exact URLs, rendered as a ficha de documentación PRD (see `qa-doc-reference`).
3. STOP and ask the user if there is a gap or divergence between docs and code.
4. NEVER self-decide a divergence; always defer to official docs or human judgment.

Reglas G1-G6: lee `skills/_shared/qa-gate-policy.md` (fuente única in-repo).

Contrato de herramienta standalone (uso sin `{change}` activo, nunca avanza
el ledger): sección "Contrato de herramientas QA standalone" del mismo
archivo.
