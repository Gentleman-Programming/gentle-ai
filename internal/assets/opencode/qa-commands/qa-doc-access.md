---
description: Search BookStack for official documentation, cite exact URLs, and STOP on any gap or divergence
agent: gentle-orchestrator
---

Load `qa-doc-access` first, then use it to search BookStack for the
documentation the user's request needs: "$ARGUMENTS".

This is a standalone QA tool: it runs with or without an active `{change}`
and never advances the QA ledger by itself. Cite every page used (rendered
per `qa-doc-reference`) and STOP to ask the user if there is a gap or a
divergence between the docs and the code — never self-decide it.
