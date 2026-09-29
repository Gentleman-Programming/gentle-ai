---
description: Produce the reproducible G6 evidence bundle (types, test run, no fixed waits, Screenplay+POM, docs comparison, exact command+result)
agent: gentle-orchestrator
---

Load `qa-evidence` first, then use it to produce the G6 evidence bundle for
the test the user names: "$ARGUMENTS".

This is a standalone QA tool: it runs with or without an active `{change}`.
It covers exactly items 1, 2, 5, 6, 7 and 8 of the G6 evidence checklist in
`skills/_shared/qa-gate-policy.md` — never items 3 (lint) or 4 (secrets),
which stay under `qa-verify`'s real RDD invocation; never reimplement or
duplicate that review here. Never mark an item `pass` without attaching the
exact command and its observed output.
