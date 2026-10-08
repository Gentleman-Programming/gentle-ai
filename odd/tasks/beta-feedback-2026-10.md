# Beta tester feedback 2026-10 (issues #5372-#5375, #5393, guide fixes)

Objective: triage and fix the defects beta testers reported against gentle-ai main and gentle-shell main, and correct the beta-tester guide. Branch: `fix/beta-feedback-2026-10` (gentle-ai worktree `~/work/gentle-ai-worktrees/beta-feedback`, base `b15198ba`). Delivery: ask-on-risk, one PR per issue cluster. Test runner: `go test` (gentle-ai), `pnpm test` / `node --test` (gentle-shell).

## Specs

- S1 (#5374, claude lane): cross-lane battery check "committed Claude process capture" fails on main with `invoke provider refuter: claude reviewer transport failed: exit status 1: fixture did not receive a provider subject hash`; it passes on v4.0.0. Reproduce, find root cause, fix.
- S2 (#5374, codex lane): "compiled adapter capture" fails on main with `invoke provider refuter: codex reviewer transport failed: exit status 1: [invalid_request]`; passes on v4.0.0. Reproduce, fix.
- S3 (#5374, schema lane): `conformance gentle-ai.review-acknowledged/v1` fails: "1/1 envelope(s) diverge: no published schema in contracts/ covers this emitted envelope". Publish the schema or fix the drift.
- S4 (#5374, opencode lane): "committed START advertised" fails with `[immutable_review_transport_unavailable]` on both v4.0.0 and main. Triage environment vs defect.
- S5 (#5374 comment): Pi relay ordinary review START on a committed-range candidate (`--base-ref` + `--committed-only`) returns `error_code: "schema-incompatible"` deterministically with gentle-ai `46837ec8` and gentle-pi `9782d26d`. Reproduce, fix.
- S6 (#5393): "one drifted managed OpenCode plugin aborts the whole sync pipeline instead of refusing that file and continuing". Tester suggestion: "Either scope the error to the drifted file (refuse that one, continue the rest, summarize at the end) or emit a warning per drifted file and exit non-zero after completing everything else."
- S7 (#5372, #5373): stable v4.0.0 defects already fixed on main. Triage: confirm fixed on main and comment; patch release decision belongs to the user.
- S8 (#5375): e2e flake; tester's diagnostics fix for `e2e/lib.sh` on fork branch `fix/e2e-assert-failure-diagnostics`. Triage.
- S9 (guide): beta-tester guide must (a) give a working way to run Gentle Shell from the clone (tester needed `gentle-shell --package-root <clone>`; `--package-root` only takes over in `--link` mode), (b) warn that `/gentle:dev-binary` is user-global and affects every session, (c) document `repository_root` for writers targeting sibling repos. Republish the gist.

## Tasks

| ID | Specs | Route | Status | Evidence |
|---|---|---|---|---|
| T1 | S1-S4 | inline | done | battery on b15198ba: 22 checks, 5 FAIL (opencode x2, claude, codex, review-acknowledged schema) |
| T2 | S1 | inline | done | not a product regression: since #5345 deterministic severe claims route through the provider refuter, which spawns the same reviewer binary; the battery Claude fixture did not answer the refuter request. Fixture now corroborates refuter claims |
| T3 | S2 | inline | done | same root cause for the Codex fake model; refuter branch writes refuter-* logs so reviewer boundary assertions stay intact |
| T4 | S3 | inline | done | published contracts/review-integration/v2/schemas/review-acknowledged.schema.json; RED TestPublishedReviewAcknowledgedSchemaValidatesBurnEnvelope (invalid file url) -> GREEN |
| T5 | S4 | inline | done | local cause: documented `go build` reports `gentle-ai dev`, the plugin refuses non-semver as "binary unavailable"; battery now refuses unversioned binaries early, CONTRIBUTING documents a versioned build; OpenCode lane drives the refuter role slot. Tester's `immutable_review_transport_unavailable` not reproduced in the sandboxed battery |
| T6 | S5 | inline (gentle-pi worktree ~/work/gentle-pi-worktrees/beta-feedback, branch fix/start-risk-reason-codes) | done | root cause: gentle-pi RISK_REASON_CODES/RISK_SIGNALS lacked dangerous_sink and agent_escalation that start-v4.schema.json publishes; reproduced with a real committed-range START (decode error `start.risk_reasons[0].code is unsupported`). gentle-pi 2b72e138f; pnpm test 5410 tests, 0 fail |
| T7 | S6 | inline | GREEN, suite pending | RED reproduced the issue error via RunSync; UserOwnedPathError scopes the refusal to the agent's managed plugins, rest of sync runs, PartialSyncError names "opencode managed plugins were skipped" |
| T8 | S7, S8 | inline | pending | |
| T9 | S9 | inline | done | beta-testers.html slide 4 (clone launcher `--link --package-root`, verified take-over message; `GENTLE_PI_GENTLE_AI_DEV_BINARY` per session; `/gentle:dev-binary` global), slide 11 (repository_root), limitations; gist b5853eb8 rev 88dc442d |

## Log

- L1 (2026-10-08, user, verbatim): "dale a todos" — reply to: "Mi recomendación: primero triagear #5374 y reproducir el `schema-incompatible`, porque bloquea la próxima release. Después corrijo la guía con el camino del launcher y la advertencia del dev-binary, y la republico. ¿Arranco reproduciendo #5374?" after listing #5374, #5393, #5372/#5373, #5375 and the two guide defects.
- L2: battery started on main `b15198ba` (log `/tmp/battery-main.log`).
- L3: battery after fixes: 33 checks, 0 failed (binary built with -X main.version=4.0.0-dev).
- L4: Pi relay START repro: scratch repo, auth/session.py with subprocess; STATUS --next-transition -> START consent/v3 decodes; granted START/v4 fails gentle-pi decode on risk_reasons code dangerous_sink. dangerous_sink exists since 0b459051 (2026-09-25), so stable gentle-pi 4.0.0 + pinned gentle-ai 4.0.0 is also affected for dangerous-sink candidates.
