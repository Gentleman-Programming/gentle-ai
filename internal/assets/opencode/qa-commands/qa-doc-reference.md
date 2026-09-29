---
description: Render one or more BookStack PRD pages as the 13-field ficha de documentación, with exact URLs
agent: gentle-orchestrator
---

Load `qa-doc-reference` first, then use it to cite the BookStack
documentation the user is asking about: "$ARGUMENTS".

This is a standalone QA tool: it runs with or without an active `{change}`
and never advances the QA ledger by itself. Render every page consulted as
the full ficha de documentación PRD (13-field metadata table plus a "Qué
contiene" summary), and STOP to present any divergence between BookStack and
the code to the user instead of deciding it yourself.
