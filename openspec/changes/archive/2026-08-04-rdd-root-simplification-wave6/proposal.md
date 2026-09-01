# Proposal: RDD Root Simplification — Wave 6 (Descendant Closure)

Hybrid store: also Engram `sdd/rdd-root-simplification-wave6/proposal`.

## Intent

Wave 2 shipped the generic `AuthorityDispositionPlan` and one executor that deliberately admits only `len(closure) == 1` (`admitLeafDisposition`, called before and under the maintenance lock). The constraint was placed in **admission**, not plan shape, precisely so this wave relaxes it without a new plan, verb, or digest domain — Wave 2's verify PASS confirmed `authorityDispositionClosure` already derives full transitive closures.

Today an operator holding a content-mismatched binding whose closure spans two or more nodes is refused with a refusal that names #2014, #1656, and "a future wave". That is this wave. Until it lands, the design's deletion outcome is also blocked: overlapping anomaly-specific repair paths cannot be removed while only the leaf case executes.

## Scope

### In Scope

- Relax executor admission to `len(closure) >= 1` for a **closed** anomaly class; unknown/mixed/ambiguous still block with no generic fallback.
- **Descendant-first ordered disposition**: `ordered_closure` becomes normative — deepest descendant first, seed last — so every interruption prefix leaves a valid retained graph.
- Plan-scoped **closure manifest** binding all N per-entry two-phase quarantine records to one `plan_digest`; success (and retained-graph revalidation) reported only after the last node commits.
- Forward-only resume: exact replay of the same digest completes an interrupted closure without moving an entry twice; a different digest refuses.
- Preserve valid unrelated history: unrelated lineages byte-identical, proven per journey.
- Remove overlapping **internal** anomaly-specific repair paths made redundant (`compact_reconcile.go` edge-anomaly repair), with call-graph + bench consumer evidence.
- Bench journeys `ds09+`: multi-chain closure, cross-lineage closure, unchanged-unrelated-graph, replay, crash-recovery mid-closure.

### Out of Scope

- Legacy verb deletion, `quarantine-legacy*` families, public surface retirement — Wave 7.
- New plan shape, new digest domain, new public verbs, wall-clock expiry.
- Stale-but-healthy lineage cleanup (#1529) — see D3.
- Genuinely unclassifiable multi-lineage graphs — stay `blocked` by design.

## Coverage

Closes #2014 (multi-node case; leaf already closed by Wave 2 `ds06`). Closes the **classifiable** half of #1656; the unclassifiable half remains `blocked` with no generic fallback. #1529 assessed, not absorbed (D3).

## Capabilities

### New

- `rdd-closure-disposition-execution`: N-node admission, descendant-first ordering, closure manifest, atomic visibility, resume semantics, cross-lineage closure, retained-graph revalidation.

### Modified

- `rdd-leaf-disposition-execution`: "Cardinality-One Admission" becomes the N=1 case; the multi-node refusal and its #2014-naming scenario retire.
- `rdd-authority-disposition-plan`: `ordered_closure` ordering becomes normative (was "an order"); cardinality remains executor policy; plan field set and digest pre-image unchanged.

## Approach

Reuse, do not re-derive — again. `authorityDispositionClosure` already produces the transitive closure; this wave only orders it and lifts one admission guard. The N-node transaction is a **composition of the existing per-entry two-phase move** (`quarantineCompactStoreEntry`: prepared record → residue rename → committed record, with `residue/` presence as the crash discriminator), not a new mutation primitive. The added artifact is one manifest record making an interrupted closure discoverable and resumable.

## Unresolved decisions (recommended defaults; auto mode — defaults stand unless corrected)

**D1 — Atomicity mechanism across N quarantine moves.**
*Default*: **atomic visibility with forward-only convergence**, not rollback. N directory renames cannot be one POSIX-atomic operation; an undo log would move authority bytes *back into* the store, contradicting byte-preserving quarantine and creating the double-move Wave 2 forbids. Instead: descendant-first order means every prefix is a valid retained graph; the closure is "disposed" only when the manifest commits; nothing partial is ever reported as success.
*Tradeoff*: a crashed closure is resumable, not automatically undone — the operator must replay the same plan.

**D2 — Partial-failure recovery semantics.**
*Default*: **replay resumes, never reclassifies**. On the same `plan_digest`, skip nodes with committed records, complete `prepared` ones using the `residue/` discriminator, continue in order. On digest mismatch or a graph that no longer re-derives the same closure: refuse, name the manifest path, escalate. No heuristic partial-state repair.

**D3 — #1529 absorption verdict.**
*Default*: **do not absorb.** #1529 is audited bulk cleanup of *stale* (healthy, obsolete) lineages — a retention/GC policy, not a classified anomaly. Absorbing it needs a non-anomaly disposition class, i.e. exactly the generic quarantine fallback the design forbids. The ordered-closure machinery stays reusable if a maintainer later defines a retention class; #1529 routes to that decision or Wave 7.

**D4 — Negotiated-transition ownership (Wave 2 W3/W2 residue).**
*Default*: **Wave 6 owns it.** `reviewRepairTransition` still serves only the legacy path, so `review status --next-transition` never offers a disposition `collect`/`execute`; and `compactStartInvalidGraphRefusal` still drops `CompactRecoveryEdgeExitRepair`. Driving an N-node closure through a raw `--plan-digest/--inventory-revision/--authorization` flag triad is exactly what the negotiated route exists to prevent, and Wave 7's job is deletion, not adding routes.
*Tradeoff*: adds ~1 slice. If the chain overruns, this is the first item to defer — it breaks no spec MUST.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/reviewtransaction/authority_disposition_execute.go` | Modified | Admission relaxed to N≥1; ordered N-node transaction; resume path |
| `internal/reviewtransaction/authority_disposition_plan.go` | Modified | Normative descendant-first closure ordering; manifest binding |
| `internal/reviewtransaction/compact_reclaim.go` | Modified | Plan-scoped closure manifest over the existing two-phase move |
| `internal/reviewtransaction/compact_reconcile.go` | Removed | Overlapping internal edge-anomaly repair path (with consumer evidence) |
| `internal/reviewtransaction/compact_inspect.go` | Modified | START refusal names the repair exit (W2 residue) |
| `internal/cli/review_next_transition.go` | Modified | Negotiated disposition transition (D4) |
| `bench/axis_damaged_store.go` | New | `ds09+` multi-chain, cross-lineage, replay, crash journeys |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Order inversion (seed first) orphans descendants mid-closure | Med | Descendant-first ordering is a spec MUST with a crash-prefix journey per position |
| Resume double-moves an entry | Med | Per-node committed record + `residue/` discriminator checked before every move; replay-convergence test |
| Cross-lineage closure over-collects and quarantines valid history | High | Closure derived only from report edges; unchanged-unrelated-graph byte assertion in every journey; retained-graph revalidation before success |
| Removing the reconcile path strands a real consumer | Med | Call-graph + bench evidence required before deletion; otherwise deprecate in place and defer to Wave 7 |
| Slice budget overrun (all 5 Wave 2 slices overran forecast) | High | Forecast from Wave 2 measured actuals, not estimates; D4 is the designated drop candidate |
| Manifest becomes a third RDD artifact (design caps at two) | Med | Manifest lives inside quarantine residue as forensic evidence, never as an operational lifecycle dependency |

## Rollback Plan

Disable the closure executor: admission reverts to `len(closure) == 1`, the Wave 2 leaf path is untouched, inspection and plan derivation stay read-only. Already-quarantined closures retain residue **and** manifest, so a disabled executor never leaves an unrecoverable or unreadable state — an interrupted closure remains forensically complete and re-enablable. Full revert deletes the closure executor path and the manifest writer.

## Dependencies

- Wave 5 exit evidence and its chain position (feature-branch-chain on the tracker, after Wave 5).
- Wave 2's shipped plan, executor, lock-safe reader, and `compactReclaimPhaseHook` crash phases.
- Wave 2 verify W1 (stale OpenSpec mirror) resolved before Wave 2 archive, or Wave 6 inherits an unarchivable predecessor.

## Success Criteria

- [ ] A classified multi-node closure (≥2 nodes, ≥2 chains) is disposed end-to-end, black-box, byte-preserving.
- [ ] A cross-lineage closure from one classified anomaly is disposed; unrelated lineages are byte-identical after.
- [ ] Crash at every ordered position leaves a valid retained graph; exact replay resumes and converges without a double move.
- [ ] Digest mismatch and non-re-deriving graphs refuse with a named diagnosis and escalation artifact.
- [ ] At least one overlapping internal repair path is deleted with consumer evidence, or the deletion is explicitly deferred with a reason.
- [ ] Zero new public verbs, zero plan-shape changes, zero digest-domain changes.

## Proposal question round (auto mode)

1. D1 atomicity: is forward-only convergence acceptable, or does a maintainer require true undo? *Assumption: forward-only.*
2. D2 partial failure: refuse on any closure drift, or attempt narrowing re-derivation? *Assumption: refuse.*
3. D3 #1529: confirm non-absorption? *Assumption: not absorbed, routed to a retention decision.*
4. D4 negotiated route: Wave 6 or Wave 7? *Assumption: Wave 6, first drop candidate on overrun.*
5. #1656: is closing only the classifiable half acceptable as the wave's answer? *Assumption: yes; the unclassifiable half stays blocked permanently by design, not deferred.*
6. Deletion outcome: does removing an internal reconcile path require a deprecation window even with zero consumers? *Assumption: no window for internal, non-exported paths.*

## Session parameters

auto; auto-chain; feature-branch-chain on tracker `feature/rdd-root-simplification` (after Wave 5); `review_budget_lines: 1000`/PR (repo CI 400 + `size:exception`).
