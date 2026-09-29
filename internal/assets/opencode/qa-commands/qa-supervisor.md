---
description: Automate, create, or modify a QA test case — the QA-Orchestrator entrypoint that routes through gentle-ai qa-status
agent: gentle-orchestrator
---

Load `qa-supervisor` first, then use it to handle the user's QA automation request: "$ARGUMENTS".

`qa-supervisor` is the entry point for the whole QA-Orchestrator flow, not an
executor — it never writes tests directly. It supervises G1-G6 (see
`skills/_shared/qa-gate-policy.md`) and routes exclusively by
`gentle-ai qa-status` (`next_action`/`stage`) before delegating to the
ledger-bound stage skills (`qa-explore`, `qa-spec`, `qa-apply`, `qa-verify`,
`qa-docs`), which depend on `QAStateMachine`/`{change}` and are never invoked
directly as standalone commands.

If the request is ambiguous — missing `{change}`, unclear scope, or no
BookStack documentation to cite — ask one focused clarification before
delegating.
