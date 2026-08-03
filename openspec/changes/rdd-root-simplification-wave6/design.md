# Design: RDD Root Simplification — Wave 6 (Descendant Closure)

## Technical Approach

Three seams change; nothing else. (1) `admitLeafDisposition` (`authority_disposition_execute.go:34`) is the only cardinality-one guard — its predicate `len(SeedSet)!=1 || len(Closure)!=1 || Closure[0]!=SeedSet[0]` relaxes to `len(SeedSet)==1 && len(Closure)>=1 && Closure[len-1]==SeedSet[0]` (seed LAST). (2) `authorityDispositionClosure` (`authority_disposition_plan.go:162`) replaces its lexicographic `slices.SortFunc` tail with a descendant-first topological emit over the same BFS `children` map built from report `PredecessorLineageID→SuccessorLineageID` edges. (3) `lockedAuthorityDispositionMutation`'s single-seed body becomes an ordered loop over `plan.Closure`, reusing `quarantineCompactStoreEntry` per node unchanged. No new plan field, verb, digest domain, or mutation primitive.

`Closure` already sits inside the seven-field `plan_digest` pre-image, so ordering becomes digest-bound with zero schema change. For N=1 the topological emit is the identity of the old sort, so every Wave 2 leaf digest, golden, and `ds06`/`ds08` journey stays byte-stable.

## Architecture Decisions

| # | Decision | Choice | Rejected alternative | Rationale |
|---|---|---|---|---|
| D1 | Admission | One relaxed predicate; export renamed `AdmitAuthorityDispositionLeaf`→`AdmitAuthorityDispositionClosure`, all 5 call sites updated | A second `admitClosureDisposition` beside the leaf one | Two predicates drift; `compact_inspect.go` `SanctionedCompactRecoveryExits` and `review_repair.go` must advertise exactly what the executor accepts |
| D2 | Ordering | Topological, deepest descendant first, seed last; ties broken lexicographically for determinism | Keep lexicographic closure, order at execution time | Execution-time order is not in `plan_digest`, so an interrupted closure could not prove which order it was disposed in |
| D3 | Atomicity | Atomic *visibility*: all N `ExpectedRevisions` CAS-checked before the first move; success + `readBackAuthorityDisposition` only after the last node commits | Undo log / rollback | Reverse moves would write authority bytes back into the store, breaking byte-preserving quarantine and the W2 no-double-move rule |
| D4 | Resume | Forward-only, per node: `discoverAuthorityDispositionRecord(base, node, plan.PlanDigest)` → committed = skip, prepared = `resumeAuthorityDispositionRecord` (the `residue/` discriminator), absent = move. Digest mismatch or closure re-derivation drift → refuse, name the quarantine path, escalate | Manifest-driven resume | Keeps the manifest forensic-only; per-node `AuthorityDispositionProof` records are already the durable resume state |
| D5 | Manifest | `closure-manifest.json` written inside the **seed** node's quarantine dir alongside `residue/`; records ordered closure + digest for humans | Manifest as a lifecycle dependency at authority root | Design caps RDD at two operational artifacts; a third would need its own recovery semantics |
| D6 | Over-collection | Closure from report edges only (unchanged); readback refuses if any retained edge names **any** closure member; each journey asserts unrelated `v2/` entries byte-identical | Reachability re-derived from on-disk state | Report-only derivation is the property W1 already proved read-only |
| D7 | Negotiated route | Extend `reviewRepairTransition` (`review_next_transition.go:546`): when no classified candidate is eligible but a disposition plan derives, emit `collect{disposition_authorization}` then `execute{review.repair, --plan-digest --inventory-revision --actor --reason --authorization}`; restore `CompactRecoveryEdgeExitRepair` in `compactStartInvalidGraphRefusal` | New `review.dispose` operation | Wave 7 deletes routes; W6 must not add public surface |
| D8 | #1529 | Excluded — stale-but-healthy is retention policy, not a classified anomaly | Absorb as a disposition class | Would require the generic quarantine fallback the design forbids |
| D9 | Deletion outcome | **Deferred to Wave 7 with evidence**: `ReconcileInvalidRecoveryEdge` has live consumers (`review_reconcile.go`, `review_reconcile_batch.go`, `compact_batch_reconcile_journal.go`, journeys ds01/ds02/ds04). W6 deletes only the cardinality-one refusal machinery (`errAuthorityDispositionCardinality`, its `#1656/#2014` text, `TestAuthorityDispositionExecuteRefusesMultiNodeClosure`) | Delete the reconcile path now | Deleting a public verb backend with four live journeys is Wave 7's public-surface retirement, not an admission relaxation |

## Data Flow

    preflight ──→ deriveAuthorityDispositionPlan ──→ ordered_closure [d2, d1, seed]
                                                            │
    execute ──→ maintenance lock ──→ re-derive + digest match + CAS all N
                     │
                     └─→ for node in ordered_closure:
                            discover(node, digest) ─ committed? skip
                                                   ─ prepared?  resume via residue/
                                                   ─ absent?    quarantineCompactStoreEntry
                     │  (crash here: prefix already disposed = valid retained graph)
                     └─→ closure-manifest.json ──→ readBackAuthorityDisposition (whole closure)

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/authority_disposition_execute.go` | Modify | N≥1 admission, ordered loop, per-node resume, closure readback |
| `internal/reviewtransaction/authority_disposition_plan.go` | Modify | Descendant-first topological `authorityDispositionClosure` |
| `internal/reviewtransaction/compact_reclaim.go` | Modify | Closure-manifest writer beside `residue/` |
| `internal/reviewtransaction/compact_inspect.go` | Modify | Restore `CompactRecoveryEdgeExitRepair` in START refusal |
| `internal/cli/review_next_transition.go` | Modify | Disposition `collect`/`execute` inside `reviewRepairTransition` |
| `bench/axis_damaged_store.go` | Modify | `ds09`–`ds12` |

## Interfaces / Contracts

```go
// seed is LAST; every prefix of ordered_closure is a valid retained graph.
func admitClosureDisposition(plan AuthorityDispositionPlan) error
type AuthorityDispositionClosureManifest struct {
    Schema, PlanDigest, AuthorityInventoryRevision, AnomalyClass string
    OrderedClosure []string
    Disposed       []string // append-only, ordered
}
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Topological order, seed-last, N=1 identity, digest stability | Table-driven, `t.TempDir()` |
| Unit | CAS drift on a non-seed member; digest mismatch on resume | Explicit refusal-text assertions |
| Integration | Crash at every ordered position via `compactReclaimPhaseHook`; replay converges | Phase-hook injection, then `InspectCompactRecoveryEdges` |
| E2E (bench) | `ds09` multi-chain closure, `ds10` cross-lineage + unrelated-bytes assertion, `ds11` crash/replay, `ds12` negotiated `next_transition` route | Black-box journeys |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED test |
|---|---|---|---|
| Documentation-like paths | N/A — no file classification | — | — |
| Git repository selection | Applicable — `--cwd` resolves the authority root and `RepositoryBinding` | Reuse `ResolveRepositoryRoot` + `authorityRepairRoot`; mismatched binding refuses pre-lock | Relative, absolute, and foreign-repo `--cwd` each refuse or bind correctly |
| Commit state | N/A — authority store only, never the index |  — | — |
| Push state | N/A — no ref resolution | — | — |
| PR commands | Applicable — `reviewRepairTransition` composes a runnable `review.repair` command line | Reuse `reviewTokenizedTransitionArguments`; authorization never appears in emitted tokens (`"provided"` sentinel) | Emitted transition tokens contain no authorization bytes and run verbatim |

## Migration / Rollout

No data migration. Rollback restores the cardinality-one predicate; interrupted closures keep residue + manifest and stay forensically complete and re-enablable.

## PR Slicing (auto-chain, ≤1000 lines/slice, after Wave 5)

| Slice | Content | Est. |
|---|---|---|
| S1 | Topological ordering + relaxed admission + rename + unit tests | ~600 |
| S2 | Ordered multi-node loop, CAS-all-N, closure readback, manifest | ~900 |
| S3 | Forward-only resume + crash-position integration tests | ~700 |
| S4 | Negotiated transition (D7) + `ds09`–`ds12` | ~900 |

Cross-slice fixes ride S1. `400-line budget risk: High` — ratchet each slice against W2 measured actuals; S4 is the designated drop candidate.

## Open Questions

- [ ] None blocking. D1–D9 stand as auto-mode defaults.
