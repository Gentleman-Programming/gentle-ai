# Proposal: RDD Root Simplification — Wave 3 (New-Lineage Facade)

## Intent

Waves 1–2 built the replacement parts read-only: a canonical `CandidateIdentity`, one seven-value relation algebra, a health classifier, and one classified disposition plan. Nothing consumes them — every live transition is still selected by `compact.go`, `review_facade.go`, status, adapters, and five gates independently, which is the split-ownership root cause Wave 0 measured (36 persisted fields, ≥28 operation forms, 13 legacy + 6 compact states).

Wave 3 promotes those parts into the only owner of NEW lineages: one `ReviewCore` with `start`/`finalize`/`validate`, five persisted states, and two active artifacts. Legacy lineages are untouched; the new path is switch-gated. Success is end-to-end journeys, not a partial cutover.

## Scope

### In Scope

- `ReviewCore` as sole transition owner for new lineages: consent-gated candidate freeze (Wave 1 resolver), tier `0|1|4` with frozen lenses and budget, `candidate_causal` admission, ONE bounded correction, targeted validation, terminal receipt.
- Five persisted states (`reviewing`, `correcting`, `validating`, `approved`, `escalated`). `invalidated`, `scope_changed`, `ambiguous`, `repairable`, `corrupted` are DERIVED, never stored, never written by a gate.
- `AuthorityStore`: `review-state.json` (CAS mutable) + `review-receipt.json` (immutable terminal). Exact-replay identity in the record; no sidecars, journals, bundles, or result files for new lineages.
- Activation switch: new starts take the LEGACY path until enabled. Coexistence precedence (Amendment C) — only a new-lineage receipt authorizes a new-lineage candidate.
- Exit evidence: candidate → consent → 0/1/4 → correction → receipt → all five gates validating through the shared algebra.
- Facade API shaped so Wave 4 can invoke review as a post-verify service (maintainer directive, 2026-08-02): a single offer/entry point, and a kill-switch OFF result that is structurally unfailable and creates nothing.

### Out of Scope

- Adapter and SDD consumer migration (Wave 4); legacy-lineage gate cutover (Wave 5); descendant closure (Wave 6); legacy deletion (Wave 7).
- External contract version bump — the new facade stays internal and switch-gated until Wave 4 exposes it.
- Candidate-decline authority, bundle transport, repair verbs.

## Coverage

22 `absorbed-into-wave-3` rows (gentle-ai #1215, #1259, #1264, #1308, #1384, #1454, #1464, #1484, #1517, #1519, #1528, #1554, #1555, #1575, #1577, #1579, #1583, #1611, #2050, #2061, #2103, #2233). Only **#1308** is OPEN — candidate-bound evidence for materially distinct behavior, which the causal-admission requirement addresses directly. The other 21 are closed/merged history; Wave 3 proves the mechanism, it closes nothing.

## Capabilities

### New Capabilities

- `rdd-review-core-transitions`: single transition owner; consent-gated freeze; tier/lens/budget immutability; causal admission; one correction; exact replay; receipt issuance.
- `rdd-authority-store`: two-artifact model, CAS mutation, five persisted states, derived-observation rule, replay identity, receipt immutability.
- `rdd-new-lineage-activation`: switch semantics, legacy default, coexistence precedence, historical read-only compatibility.

### Modified Capabilities

- `rdd-candidate-relation-algebra` (Wave 1): promoted from observation to the deciding authority for new-lineage start, finalize, validate, and gates.
- `rdd-candidate-identity` (Wave 1): identity becomes persisted frozen authority, not a computed observation.
- `rdd-shadow-evaluation` (Wave 1): its disable switch must scope to *observation* only. Today "when disabled, zero shadow code path executes" would forbid the live facade from calling the same functions — the algebra must move out from behind the shadow gate.

## Approach

Add, do not translate. `ReviewCore` is a new owner beside the existing state machine; lineage kind is decided once at `start` and never mixed afterwards. The relation algebra and resolver are lifted out of the `shadow_*` gating (same functions, new home) so exactly one implementation decides for both the shadow matrix and the live facade — re-deriving them would recreate the root cause the wave exists to remove. Wave 2's `loadCompactRecoveryRecords` seam and health classifier are consumed read-only for authority-health reporting; they are not extended here.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/reviewtransaction/review_core*.go` | New | Transition owner: start/finalize/validate, admission, correction budget |
| `internal/reviewtransaction/authority_store*.go` | New | Two-artifact CAS store, five states, replay identity, receipt publication |
| `internal/reviewtransaction/shadow_relation.go`, `shadow_identity.go` | Modified | Lift algebra/resolver out of shadow gating; shadow observer keeps its switch |
| `internal/reviewtransaction/compact.go`, `compact_store.go` | Modified | Lineage-kind routing at start only; legacy semantics unchanged |
| `internal/reviewtransaction/gate.go`, `compact_gate.go` | Modified | New-lineage validation branch + coexistence precedence; legacy branch untouched |
| `internal/cli/review_facade.go`, `review_next_transition.go` | Modified | New-lineage transitions behind the activation switch |
| `bench/` + tests | New | End-to-end journeys, precedence matrix, switch-off equivalence |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Coexistence precedence violated — a legacy receipt authorizes a new-lineage candidate (Amendment C) | Med | Per-gate precedence matrix over {legacy, new} × {exists, absent}; new-lineage candidate with only legacy authority denies |
| Gates are touched in Wave 3 while cutover is Wave 5 — a de-facto partial cutover | High | Strictly additive branch keyed on lineage kind; legacy branch byte-identical, proven by switch-off equivalence tests |
| Two live state machines coexist indefinitely if Wave 4/7 slip | Med | New lineages provably cannot write legacy artifacts; the switch stays default-off until Wave 4 consumes it |
| Five states lose diagnostics the 36-field compact record carries | Med | Derived categories + reason taxonomy must reproduce every existing user-visible refusal; regression matrix as exit evidence |
| Replay identity differs from the legacy notion, double-consuming correction budget | Med | One replay-identity definition in the store; explicit replay-vs-correction test per state |
| Rollback strands an in-flight new lineage with no way to finalize | Med | Rollback disables new **starts** only; existing new-lineage authority stays fully executable and readable |

## Rollback Plan

Turn the activation switch off: every new `start` takes the legacy path, no new-lineage artifact is created, and live behavior is byte-identical to Wave 2's tip. Already-created new lineages remain readable AND finalizable — a read-only-only rollback would strand them mid-lifecycle. Historical legacy authority is never rewritten. Full structural revert deletes `review_core*.go` / `authority_store*.go` and the lineage-kind branch; the lifted algebra returns behind the shadow gate.

## Dependencies

- Wave 1 merged onto the tracker: `CandidateIdentity`, `shadowRelate`, `ObserveShadowRelation`.
- Wave 2 exit evidence: `DispositionPlan`, `loadCompactRecoveryRecords` seam, health classifier.
- Adopted decisions 1 (five states + two artifacts), 2 (shared algebra, read-only gates), 3 (declined/unsupported = unmanaged), 7/Amendment C (coexistence precedence).
- Maintainer Wave-4 directive (post-verify offer, total invisibility when off) constrains the facade API shape.

## Success Criteria

- [ ] End-to-end journey passes for each tier: candidate → consent → 0/1/4 → (correction) → receipt → each of the five gates validating via the shared algebra.
- [ ] A new lineage writes exactly two artifacts and zero legacy state/artifacts, proven by store inspection.
- [ ] Only the five states are ever persisted; every other observation is derived, and no gate mutates authority.
- [ ] Exactly one candidate-causal correction is possible; exact replay consumes no budget in any state.
- [ ] Coexistence precedence proven: a legacy receipt never authorizes a new-lineage candidate, at all five gates.
- [ ] Switch OFF ⇒ live outcomes byte-identical to the pre-wave tip; switch flip strands no in-flight lineage.
- [ ] Kill switch OFF ⇒ the facade creates nothing and cannot fail (Wave-4 precondition).

## Proposal question round

Auto execution mode — asked here rather than interactively. Assumptions stand unless corrected.

1. **Consent UX shape for the new start.** Reuse the existing `gentle-ai.review-integration.consent/v2` envelope verbatim, or issue a new-lineage envelope? *Assumption: reuse v2 unchanged. It already carries candidate, tier, lenses, budget, and answer tokens; a second envelope would fork the one surface adapters implement, and Wave 4 is the wave allowed to change consumer contracts.*
2. **Tier thresholds: reuse or redefine.** Wave 3 could re-derive low/standard/high risk selection. *Assumption: reuse the existing threshold logic unchanged and freeze the result into the new record. Redefining risk selection in the same wave that replaces the state machine makes any journey divergence unattributable.*
3. **Receipt schema versioning.** Does `review-receipt.json` get a new schema identity, or extend the current receipt schema? *Assumption: a distinct new-lineage schema identity (e.g. `…review-receipt/v1`) with a `lineage_kind` discriminator. Extending the current schema makes "new lineages stop writing legacy artifacts" unprovable and blocks Wave 7 deletion.*
4. **Activation switch identity and default.** Env switch (Wave 1 precedent) or a persisted repo-level setting? *Assumption: env switch, default OFF, distinct from `GENTLE_AI_RDD_SHADOW` and from the user-owned RDD kill switch — three switches with three meanings, never overloaded.*
5. **Gate scope in this wave.** Do all five gates gain the new-lineage branch now, or only the two needed to prove a journey (`pre-commit`, `pre-pr`)? *Assumption: all five, because the exit evidence names all five and a partial set would leave a new-lineage candidate undeliverable at `release`.*
6. **Wave-4 API surface now or later.** Should the post-verify offer entry point ship in Wave 3 (unwired) or wait? *Assumption: ship the API shape unwired behind the deadcode ratchet, so Wave 4 is a consumer change only — but no SDD call site in this wave.*
