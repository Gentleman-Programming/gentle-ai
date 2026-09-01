# Proposal: RDD Root Simplification — Wave 4 (Thin Consumers)

## Intent

Wave 3 built the new-lineage facade; nothing outside RDD consumes it. Two consumer-side defects remain, and they generate the 21 `absorbed-into-wave-4` rows:

1. **SDD holds a second copy of review truth.** `review_binding.go` persists a `gentle-ai.sdd-review-binding/v1` mirror (CON-07) and `review_gate.go` re-derives gate meaning from receipts (CON-06), so every review change needs a matching SDD fix.
2. **RDD supervises the SDD cycle from before apply.** `applyPreVerifyReviewRouting` blocks verify until a review transaction exists, so a review problem stops implementation — and with the kill switch OFF the archive gate still runs a disabled/unmanaged ceremony that can fail.

The maintainer directive (2026-08-02, Engram decision 10123) reorders this: **apply → verify → offer RDD → archive**. RDD becomes a service SDD invokes at one point, never a supervisor. With the kill switch OFF, RDD must not exist for SDD at all — no offer, no status consultation, no ceremony that can block.

Separately, transport capability is still decided inside consent/status/collection routes, so a runtime limitation becomes lifecycle state and a contract version (#2076/#2207/#2221/#2225/#2191).

## Scope

### In Scope

- **ReceiptRef only.** SDD stores a terminal `ReceiptRef` plus its own work-unit attempts. Binding/remediation mirrors retire; every review-validity question is one native validation call.
- **Post-verify offer point wired** (Wave 3 decision 8 `OfferReviewAfterVerify`). Pre-apply/pre-verify status control removed; the post-apply gate's binding role repositions to the offer point.
- **Kill switch OFF = structural absence.** No offer, no consultation, no `disabled/unmanaged` disposition — not a no-op path that could fail.
- **Post-verify correction loop.** A bounded correction invalidates prior verify evidence for touched paths → targeted re-verify → archive.
- **Capability before lifecycle mutation.** A runtime declares transport capability *before* any authority, tier, lens, budget, or collection slot exists.
- **Attempt-ledger reconciliation** (decision 9) as an explicit written decision.
- **Bundled in-repo adapter assets** consume opaque transitions only.

### Out of Scope

- Gate cutover (W5), descendant closure (W6), legacy deletion (W7); new contract major.
- **gentle-pi's own adapter** — a separate repository. This wave ships the *provider* side (opaque transitions + capability declaration) plus *in-repo* consumers; Pi's thinning is gentle-pi's chain, gated by its declared capability.
- Out-of-repo host-runtime behavior (CON-12), which this repository does not contain.

## Coverage

21 `absorbed-into-wave-4` rows: #1013, #1204, #1209, #1227, #1247, #1296, #1358, #1385, #1533, #1552, #1569, #1581, #1620, #1708, #1715, #2076, #2131, #2191, #2207, #2221, #2225. Six are OPEN (#1013, #1204, #1209, #1227, #1247, #1385) — all SDD/budget-boundary reports the receipt-only bridge addresses directly. The rest are closed history; this wave proves the mechanism, it closes nothing.

## Capabilities

### New Capabilities

- `rdd-sdd-receipt-consumption`: SDD persists a `ReceiptRef` and its own attempts only; no review-state mirror, no re-derived gate meaning; validation is requested, never reimplemented.
- `rdd-post-verify-review-offer`: the apply → verify → offer → (correction → targeted re-verify) → archive sequence, the offer's decline semantics, and kill-switch-OFF structural absence.
- `rdd-transport-capability`: capability declared and decided before authority/tier/lens/budget/collection creation; unsupported transport creates no review state and no recoverable remnant.

### Modified Capabilities

- `rdd-review-core-transitions` (Wave 3): the offer API gains its call site, and capability admission becomes a precondition of candidate freeze.

## Approach

Invert the dependency: **SDD calls RDD; RDD never calls SDD.** Delete mirrors instead of synchronising them.

`applyPreVerifyReviewRouting`/`applyPreVerifyCompactBridgeRouting` are removed rather than reordered — a reordered control is still a control. `OfferReviewAfterVerify` becomes the one call site, guarded before any repository read so the OFF path is unfailable by construction. `ReviewBinding` collapses to a `ReceiptRef`; existing binding files parse read-only and are never written again.

On decision 9 the evidence already answers the condition: `runtime_ledger.go` is an append-only record chain with `previous_revision`, CAS `expected_revision`, and `request_digest` replay identity — durable cumulative CAS-like properties. **Recommendation: attempts stay in SDD**, and CON-08's split ownership is closed by naming one owner of work-unit scope across `runtime_ledger.go` and `runtime_compact.go` (the #2133/#2151 defect class).

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/sddstatus/review_binding.go` | Removed | CON-07 mirror → `ReceiptRef`; ledger/digest duplication deleted |
| `internal/sddstatus/review_gate.go` | Modified | CON-06: re-derived gate reasoning → one native validation call |
| `internal/sddstatus/status.go` (`applyPreVerifyReviewRouting`, `applyPreVerifyCompactBridgeRouting`) | Removed | Pre-verify RDD status control deleted |
| `internal/sddstatus/status_v1.go` | Modified | `reviewGateStateV1` retained for legacy clients; never populated when OFF |
| `internal/sddstatus/runtime_ledger.go`, `runtime_compact.go` | Modified | Decision 9 / CON-08: attempts stay in SDD, one work-unit owner |
| `internal/reviewtransaction/review_offer.go` | Modified | Wave 3's unwired API wired; leaves `.deadcode-baseline.txt` |
| `internal/reviewtransaction/` (capability) | New | Pre-freeze transport capability admission |
| `internal/agents/{opencode,claude,pi}/adapter.go` | Modified | Opaque transitions only (behavioral depth is a Wave 0 gap — trace first) |
| `internal/assets/skills/_shared/review-ledger-contract.md`, `internal/assets/{claude,opencode}/commands/sdd-apply.md`, `testdata/golden/*` | Modified | Sequence + offer point; client planner text removed |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Removing the pre-verify control removes a real safeguard against unreviewed apply | Med | The safeguard moves, not disappears: the post-verify offer plus five unchanged delivery gates still deny unreviewed *delivery*; apply was never the enforcement point |
| Pi's adapter lives in another repo and still speaks the old shape | High | Provider keeps both surfaces; capability declaration decides which; per-adapter unavailable mode, no unsafe fallback |
| Kill-switch-OFF invisibility is asserted by a passing test rather than proven | Med | Prove *absence of calls* (no review code executes on any SDD path), not a green disabled path |
| Targeted re-verify scope under-specified — a correction could skip evidence it invalidated | Med | Re-verify scope is derived from correction changed paths; an empty intersection still re-runs the objective's evidence goal |
| Retiring the binding record orphans in-flight changes holding one | Med | Read-only compatibility: an existing binding parses into a `ReceiptRef`; nothing new is written |
| Asset/golden churn (12+ goldens) dwarfs behavioral change and hides regressions | Med | One contract source regenerated; a negative assertion that offer text is absent when OFF |
| SDD verify and RDD correction disagree about what "passed" means | Med | One evidence-revision identity shared by both; correction records which revision it invalidated |

## Rollback Plan

Per-adapter and per-consumer, no unsafe fallback:

1. **Adapter**: an adapter that cannot declare capability enters *unavailable* mode — RDD is not offered for it. It never falls back to constructing its own transitions.
2. **Sequence**: revert the offer call site and restore `applyPreVerifyReviewRouting`; the offer API returns to unwired-and-baselined (Wave 3 state).
3. **ReceiptRef**: binding-record removal is the only destructive step — it lands last, after the sequence change is proven, and historical binding files stay parseable throughout.

No authority is rewritten; no receipt is invalidated by rollback.

## Dependencies

- Wave 3 exit evidence: `ReviewCore`, `AuthorityStore`, five states, `OfferReviewAfterVerify` shape.
- Maintainer directive (Engram decision 10123) — binding on the sequence and on OFF-invisibility.
- Wave 0 inventory rows CON-06 through CON-12 and its named gap: **CON-09/10/11 behavioral depth must be traced before this wave's adapter work** ("A future wave should verify this before Wave 4").
- Adopted decision 3 (unsupported runtime ⇒ fail before freeze, unmanaged ordinary delivery).

## Success Criteria

- [ ] SDD persists no review state beyond a `ReceiptRef`; `gentle-ai.sdd-review-binding/v1` is never written, proven by store inspection.
- [ ] No review-gate result is re-derived inside `internal/sddstatus`; every answer comes from one native validation entry point.
- [ ] End-to-end: apply → verify → offer → (bounded correction → targeted re-verify) → archive, for accept and decline.
- [ ] Kill switch OFF ⇒ **zero** review code executes on any SDD path (status, continue, verify, archive) and no review field appears in output; proven by call absence, not by a passing disabled path.
- [ ] Unsupported transport denies before any authority, tier, lens, budget, or collection slot exists — nothing to recover, nothing to repair.
- [ ] Each in-repo adapter executes a provider-issued opaque transition in a black-box run and constructs no flag, revision, target, or binding.
- [ ] Decision 9 recorded in writing; CON-08's work-unit owner named exactly once.

## Proposal question round

Auto execution mode — asked here rather than interactively. Assumptions stand unless corrected.

1. **Decline semantics at the offer point.** The user is offered review post-verify and says no. *Assumption: archive proceeds under ordinary repository policy with an explicit unmanaged outcome (adopted decision 3) — a decline is not a block and persists no receipt-like authorization. Confirm archive is not gated on the decline being recorded.*
2. **Re-verify granularity after a bounded correction.** *Assumption: targeted re-verify re-runs the verify objective's evidence goal scoped to the correction's changed paths, and the archive requires a verify evidence revision at least as new as the correction. Full re-verify is the fallback when scoping is not provable, never the default.*
3. **Where the `ReceiptRef` lives.** *Assumption: in SDD's existing runtime ledger (which already has CAS + replay identity), not as a new OpenSpec artifact file — a file would recreate the mirror this wave deletes.*
4. **Who declares transport capability, and is it trusted?** *Assumption: the adapter declares; the provider treats an absent or unrecognised declaration as unsupported (fail closed). No provider-side probing of host runtimes, because that recreates runtime-specific lifecycle routes.*
5. **Legacy `reviewGate` field for Pi clients while Pi is unmigrated.** *Assumption: `reviewGateStateV1` stays in the v1 projection so legacy client bytes are unchanged on paths that already produced a gate, but it is simply absent when the kill switch is OFF. Removing the field is Wave 7.*
6. **Order within the wave.** *Assumption: sequence change first (offer + control removal), capability second, mirror deletion last — so the destructive step lands only after the new sequence has real runtime evidence.*
