# Native Compact Review Orchestration

Pi uses the compact gentle-pi facade for this lifecycle: `gentle_review` for inspect, START, consent, bound STATUS, and acknowledgement; `gentle_review_capture` and `gentle_review_capture_group` for collection. Provider-issued authority and every opaque binding are authoritative; prompt prose never creates authority or decides delivery.

## Entry rule

After authorized source-mutating implementation is complete and normalized, and before reporting it complete, call `gentle_review` with {"operation":"inspect"}. Do this once per candidate whenever the user-owned review switch is enabled (`gentle-ai review mode status` reads it without changing it). The facade returns the only offered START route; do not infer, reconstruct, or replace it. Never skip the preflight because the user did not ask for a review: the START consent envelope lets the human decide this candidate. Skip it only for a trivial passive documentation-only edit, when the user explicitly left this candidate unreviewed, or while a transaction is already bound to it.

## Atomic lifecycle

1. **Inspect before START.** Call `gentle_review` with {"operation":"inspect"} before START. Retain only the provider-issued authority and opaque bindings returned by the facade.
2. **Freeze once.** Invoke only the START route inspect offered: `gentle_review` with operation `start`, a fresh `idempotencyKey`, and the `input` the facade documents (ordinary START is {"mode":"ordinary"}; an explicit `baseRef` requires `committedOnly: true`). Retain the returned `lineageId`, revision, target, and `workspaceRoot` as opaque values. An exact replay of an active START may return `replayed`; a genuinely new START is independent. Do not start another lineage, reuse burned authority, or perform ambient recovery.
3. **Stay bound.** For later routing, call `gentle_review` with operation `status`, the exact retained `lineageId`, and `workspaceRoot` only when needed. Never run raw shell STATUS. Route only from the returned transition; for `execute`, invoke its offered facade operation with its exact opaque binding; for `collect`, satisfy only the named slots through the capture tools below; for `stop`, run no lifecycle operation.
4. **Collect exactly.** Use `gentle_review_capture` for one current returned slot or `gentle_review_capture_group` for the complete current reviewer group. Submit only the returned opaque binding and result. After collection, use bound facade STATUS again only when the returned transition requires it.
5. **Acknowledge exactly.** Only the exact provider-issued acknowledgement continuation burns approved authority. Report the burn from its returned envelope, never from a later STATUS.

Pi never reconstructs lineage, target, revision, repository context, lens, order, or commands. It never appends, removes, parses, or rebuilds provider-issued opaque bindings. Go owns repository binding, frozen evidence, provider context, validation, admission, correction scope, and closure.

## Consent and forecast

If START returns the typed `gentle-ai.review-integration.consent/v3` envelope, relay it as a Lossless Blocking Prompt. Global RDD enabled permits review; it never grants consent for this candidate. For medium/high candidates, faithfully translate the headline, reason, `value`, risk evidence, choice labels, every choice `effect`, and the off-path note while preserving original groups/order, selection mode, allowed-answer domain, answer tokens, and the `consentBinding`. Project `value` as benefits and every `effect` as consequences. Do not translate machine answer tokens (`granted`, `declined`). Submit exactly the human's choice through `gentle_review` with operation `answer-consent` and the exact `consentBinding`; a decline is candidate-scoped and is not the kill switch.

A four-lens review is long work. The first capture of a materialize slot or group returns a `forecast` and runs nothing: relay it losslessly in the user's language, preserving every step's order and fields (`step`, `kind`, `reason_code`, `description`) and the horizon, then resubmit the same exact binding with `reviewerRunAcknowledged: true`. Forecast is informational; route only from the returned transition.

## Capture and correction

Reviewers inspect only the provider-bound immutable trees, and the gentle-pi relay owns the reviewer prompt. Never hand candidate bytes through `/tmp`, an external file, a repository scratch file, or `GENTLE_AI_FROZEN_CANDIDATE_CONTEXT`, and never substitute the live worktree, index, or `HEAD`.

Only candidate-caused severe findings block. Pre-existing/base-only findings are follow-ups; unknown causality escalates. A deterministic blocker needs no refuter; inferential blockers share one read-only refuter batch. The final reviewer, refuter, or targeted-validator capture owns closure.

A malformed, incomplete, or unavailable capture never reaches acknowledgement. Use bound facade STATUS once, and relaunch only when it reoffers the same bound slot. An approved capture awaits acknowledgement; it is not burned. On `approved`, use bound facade STATUS to obtain or replay the exact provider-issued `acknowledge-approved` continuation, then execute it unchanged. Only its successful returned envelope burns authority; do not issue STATUS after that burn. On `correction_required`, continue only through exact bound facade STATUS and the provider-issued correction route. Native Go maps edits only to corroborated frozen findings and permits at most one bounded correction. A validator that cannot inspect the immutable trees produced no verdict: surface one blocked human decision and submit nothing.

### Cross-repository lifecycle root

A session in repository A may review an explicitly selected nested target in unrelated repository B only after explicit user authorization. Pass the selected path as `workspaceRoot`; Go resolves it to the canonical B worktree root, and the facade retains it. Keep that `workspaceRoot` on every later facade call for the lineage, from inspect through acknowledgement, and do not fall back to A. The same lineage text in A and B is independent; exact acknowledgement burns B only.

### Continue after a stop reason code

A `stop` ends its transition, never approves delivery. `D` means the human disables the review switch for this clone with `gentle-ai review mode disable --scope clone`; ordinary policy then decides delivery. `S` means re-query bound facade STATUS with the retained `lineageId` and `workspaceRoot`.

| Reason codes | Continuation |
| --- | --- |
| `captured_artifacts_unverifiable`, `captured_result_selection_unavailable`, `missing_authority_binding`, `corrupted_or_unverifiable_authority`, `manual_intervention_required`, `native_stop_required` | Terminal: the maintainer inspects authority/lineage, or `D`. |
| `empty_base_diff_bootstrap_required` | Terminal: authorized empty-root bootstrap for a new target, or `D`. |
| `lens_context_budget_exceeded` | Terminal: reduce the candidate scope and start a new transaction, or `D`. |
| `staged_workspace_overlay_recovery_unavailable` | Call facade `recover` with the retained `lineageId`, or start a fresh transaction; otherwise `D`. |
| `corrected_candidate_unavailable` | Change the correction candidate, then `S`; do not reuse the pre-correction target. |
| `recovery_scope_unchanged` | Change the target identity, then retry the facade `recover` route the stop returned. |
| `rdd_disabled` | `--scope clone` only clears a clone-local off; the human runs `gentle-ai review mode enable --scope global`, then `S`. |

## Delivery follows ordinary repository policy

After exact acknowledgement burns terminal `approved` authority, the review lifecycle stops. Commit, push, PR, and release remain separate human decisions under ordinary repository policy. A review outcome is informational and never authorizes delivery.
