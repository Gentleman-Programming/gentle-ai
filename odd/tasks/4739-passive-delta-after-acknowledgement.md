# #4739 — Do not re-offer review for a passive delta after an acknowledged candidate

Worktree: `/home/gentleman/work/gentle-ai-4739`, branch `fix/4739-delta-risk` (from `origin/main` f52e1cf6).
Issue: gentle-ai#4739 (related #4939, #4815). Delivery: one PR that closes #4739 on merge.

## Specs

- S1 — User request, verbatim (L1, L2): "Crea un issue en gentle ai para que rdd si ya evalúo algo que es igual no lo vuelva a ofrecer" and "Quiero que lo arregles y al mergear lo cerramos".
- S2 — Scope chosen by the user (L3): "Delta pasivo → no ofrecer". When a first-parent ancestor of the live committed base-diff candidate (same base tree) was approved and its authority burned, and the delta from that ancestor's tree to the live candidate is passive (risk `low`), selectorless STATUS does not offer START; it stops with the new authority-free reason `acknowledged_predecessor_passive_delta`. The Stop hook therefore stays silent.
- S3 — When the delta touches code, tests, configuration, or anything non-passive, STATUS offers the full review exactly as today.
- S4 — Suppression never creates authority: no receipt, no gate pass. Any Git or tombstone lookup failure offers the review as today (suppression fails closed). A rewritten history or a different base tree finds no predecessor.
- S5 — `review assess` agrees with STATUS: a passive delta after an acknowledged predecessor reports `review_due: false` with `already_reviewed`.
- S6 — Contract surfaces document the new STOP reason: the status contract validator, narration, `review-ledger-contract.md`, and `docs/review-integration.md`.

## Tasks

- T1 — S2-S4: core helper `AcknowledgedPassivePredecessor` in `internal/reviewtransaction` with unit tests; inline; done, commit `4189a2f3`.
- T2 — S2, S3, S5, S6: wire STATUS (facade fresh branch), next transition, contract validator, assess, narration, docs; update `TestNextTransitionDerivedRangeAcknowledgementStaysTerminal`; inline; done, commit `2207879b`.

## Log

- L1 (user): "Crea un issue en gentle ai para que rdd si ya evalúo algo que es igual no lo vuelva a ofrecer, tu por suerte me dijiste que no valía la pena hacerlo"
- L2 (user, after #4739 was found as the existing report): "Quiero que lo arregles y al mergear lo cerramos"
- L3 (user choice): "Delta pasivo → no ofrecer (recomendado)" over delta-scoped review.
- L4 (evidence, explore): acknowledge burns the authority directory and leaves only a tombstone keyed by exact target identity (`compact_burn.go:208-232`, `compact_terminal_consumption.go:19-83`); identity is content-addressed (`IdentityForComponents`), so an ancestor's base-diff identity is recomputable. Selectorless STATUS (`review_facade.go:1059-1127`) and the Stop hook (`review_stop_hook.go:262-281`) share one path. `TestNextTransitionDerivedRangeAcknowledgementStaysTerminal` currently asserts a docs-only commit re-offers START and must change.
- L5 (evidence): `go test ./...` exit 0 on the branch (one earlier full run hit `TestFetchLatestEngramVersionWithAssetsPaginates`, which passed 3/3 in isolation and on the rerun: unrelated flake). The shipped `review-ledger-contract.md` must carry a routable row for every stop code (`TestReviewStopInvariantTerminalClassificationAgreesWithShippedContract`, `TestEveryReviewStopReasonCodeHasAShippedContinuation`), so the new row names the deliberate-review command; `testdata/orchestrator-module-baseline.json` regenerated with `-update` for that intended contract drift. The Pi ledger variant is unchanged (it does not list `target_already_acknowledged` either).
- L6 (evidence, gentle-ai-verify): S2-S6 pass, no authority created, 25 non-passive delta kinds never suppressed. Defect: an occupied non-escalated START lineage (a deliberate review of the passive target) was hidden behind the passive STOP and the Stop hook went silent. Fixed: suppression only for selectorless STATUS with an unoccupied START lineage; regression `TestNextTransitionPassiveDeltaNeverHidesALiveReview` (real medium approve + acknowledge flow, then a deliberate review) was RED before the fix. Known, unchanged: a delta of only generated `testdata/golden` files counts as passive because the existing risk classifier excludes them. `go test ./...` exit 0.
- L7 (evidence): benchmark driven mode, origin/main binary vs branch binary: 49/49 journeys complete in both, every dimension delta 0 (human prompts, commands, blocks, dead ends, model runs), stderr bytes +-1..4 path noise. No corpus journey exercises the new stop yet. `bench` module: go vet + go test pass. guard:population `acknowledged-passive-delta` (too-loose) declared on the deciding return.
- L8 (delivery, user choice "Aprobar #4739 + PR con size:exception"): #4739 labeled status:approved; branch pushed; PR gentle-ai#5322 (Closes #4739, type:feature, size:exception). At open: type label, status:approved, cognitive load, Go Format, workflow scripts, installer tests pass; runtime, unit, and E2E checks pending.
