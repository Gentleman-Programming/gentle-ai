# Design: RDD Root Simplification — Wave 2 (Leaf Disposition)

## Technical Approach

Promote Wave 1's read-only leaf observation into a derived `AuthorityDispositionPlan` and give it exactly one executor: a cardinality-one leaf of a closed content-mismatch class. Reuse existing machinery — `InspectCompactRecoveryEdges` (graph source of record), `classifyCompactRecoveryEdgeAnomalies` (pure record classifier), `quarantineCompactStoreEntry` (byte-preserving residue), and `RepairClassifiedAuthority`'s assess → lock → replay → execute → readback skeleton on the existing `review repair` verb. No new verb, no new store format.

## Architecture Decisions

| # | Choice | Rejected | Rationale |
|---|---|---|---|
| 1 | New `internal/reviewtransaction/authority_disposition_plan.go`: `AuthorityDispositionPlan{Schema, RepositoryBinding, AuthorityInventoryRevision, AnomalyClass, SeedSet []string, Closure []string, ExpectedRevisions map[string]string, Actor, Reason, PlanDigest, Authorization string}`; digests via existing `classifiedAuthorityRepairDigest(domain, value)` with domains `…disposition-plan-digest/v1`, `…authority-inventory-revision/v1`. `Authorization` is populated at execution time from the maintainer input and is EXCLUDED from the `plan_digest` pre-image. `Actor` and `Reason` are likewise EXCLUDED from the `plan_digest` pre-image — they are execution-time PROVENANCE (still carried as plan fields per the spec's ten-field carry requirement, still required non-empty at execution time), not plan IDENTITY, mirroring `Authorization`'s treatment. `plan_digest` is computed over the seven derived identity fields only (`schema`, `repository_id`, `authority_inventory_revision`, `anomaly_class`, `ordered_seed_set`, `ordered_closure`, `expected_revisions`), so it is derivable pre-authorization AND before actor/reason are known at all — the digest `review repair --preflight` publishes (always derived with empty actor/reason) is exactly the digest a later execution re-derives (with the real actor/reason) for the same graph state; the executor validates the populated `Authorization` against the digest-bound plan at execution time. `Schema` is an explicitly-permitted eleventh serialization field beyond the spec's ten | Bespoke canonical encoder; extending `AuthorityRepairAssessment`; dropping `Authorization` from the struct (rejected: spec's Plan Field Set requirement mandates all ten fields including `authorization`); including `Actor`/`Reason` in the digest pre-image (rejected: this was the original S1/S2/S3 shape and S4's bench work discovered it makes the preflight-published digest architecturally unusable by any real execution, since `--preflight` always derives with empty actor/reason while execution requires them non-empty; provenance identity is exactly the property `Authorization`'s own exclusion already establishes as correct) | `encoding/json` sorts map keys, so the existing domain-separated digest idiom is already deterministic; the assessment is a legacy-v1 scanner, not a graph plan |
| 2 | Closed class `content_mismatched_recovery_authorization`. Predicate already isolated in `classifyCompactRecoveryEdgeAnomalies`: edge fails `errCompactRecoveryAuthorizationInexact` (or unchanged-target) **and** `recovery.MaintainerAuthorization` carries the `compactRecoveryAuthorizationSchema` prefix but ≠ exact binding. Those branches gain a pure `DispositionClass` field beside today's `NonReconcilableError` | Adding it to `AnomalyClasses` | `AnomalyClasses` makes reconcile and `SanctionedCompactRecoveryExits` advertise `review reconcile-authority`, which would then refuse — a dead end. `CompactRecoveryEdgeInspection` JSON stays byte-identical, so batch-reconcile binding identity is untouched |
| 2b | Leaf predicate promoted verbatim from Wave 1: leaf ⇔ no report edge has `PredecessorLineageID == successor`; `closure(S) = {seed}` only then | Recomputing descendants from records | Wave 1 already proved it read-only over the same report |
| 3 | `gentle-ai.review-disposition-authorization/v1` = schema + repository + class + plan_digest + inventory_revision + actor + reason, shaped like `authorityRepairAuthorizationBinding`; no wall-clock expiry. **(Assumption, pending maintainer confirmation.)** Supplied on the existing verb: `review repair --preflight` emits path-free plan digest + inventory revision, execution adds `--plan-digest --inventory-revision --actor --reason --authorization` | Expiry timestamps; a new `review dispose` verb | CAS on `ExpectedRevisions` plus under-lock re-derivation already refuses stale plans; expiry only adds a blocking failure mode. Control-reduction matrix: recovery verbs MERGE. This deviates from the ratified `rdd-simplification-design` decision 4 default ("maintainer-bound disposition plan and a short version-pinned read-only horizon"), so it stays an open, unratified assumption rather than a settled choice |
| 4 | Reuse `quarantineCompactStoreEntry` → `quarantine/<lineage>-<rand>/{reclaim-record.json,residue/}`; new `AuthorityDisposition *AuthorityDispositionProof` on `CompactReclaimRecord` (plan digest, inventory revision, class, seed, closure, expected revisions, recorded-authorization SHA-256) | Fresh quarantine layout | Record/residue split, crash phases (`compactReclaimPhaseHook`), and prefix replay discovery already exist and are tested |
| 5 | Leaf constraint lives in executor admission: `admitLeafDisposition(plan)` requires `len(Closure) == 1 && Closure[0] == SeedSet[0]`; derivation stays generic | A single `LineageID` field on the plan | Wave 6 relaxes cardinality by replacing admission only — no plan-shape or digest-domain change |
| 6 | Add `CompactRecoveryEdgeExitRepair = "review repair"`, emitted by `SanctionedCompactRecoveryExits` only when derivation **and** leaf admission accept the edge; otherwise today's `Blocked` prose stands | Always naming `review repair` for the class | Existing rule: never advertise an operation whose own prediction would refuse |

**Spec-field → Go-field mapping** (`rdd-authority-disposition-plan/spec.md` Requirement: Plan Field Set):

| Spec field | Go field |
|---|---|
| `repository_id` | `RepositoryBinding` |
| `authority_inventory_revision` | `AuthorityInventoryRevision` |
| `anomaly_class` | `AnomalyClass` |
| `ordered_seed_set` | `SeedSet` |
| `ordered_closure` | `Closure` |
| `expected_revisions` | `ExpectedRevisions` |
| `plan_digest` | `PlanDigest` |
| `actor` | `Actor` |
| `reason` | `Reason` |
| `authorization` | `Authorization` |
| — (not in spec, permitted 11th field) | `Schema` |

**#2111 supersession test**: superseded iff its anomaly re-derives with a non-empty `DispositionClass` **and** the successor is a leaf. Non-leaf, mixed, unknown, or entry-diagnostic-bearing inspections stay `blocked`; the PR then stays open for Wave 6.

Decision numbering: this design's row 3 (authorization binding, no wall-clock expiry) corresponds to `rdd-simplification-design`'s decision 4 ("repair authorization and compatibility horizon"), so cross-references between the two documents resolve as row 3 ↔ decision 4.

## Data Flow

    review repair --preflight ─→ loadCompactRecoveryRecords (extracted seam, compact_inspect.go)
                                   │                     │
                         report ───┴──→ derive plan ←────┴─ records ─→ DispositionClass
    maintainer authorization ─→ review repair ─→ admitLeafDisposition
        └→ exclusive maintenance lock ─→ re-derive plan ─→ CAS ExpectedRevisions
        └→ replay discovery by plan_digest ─→ quarantine ─→ retained-graph readback

Both derivation inputs (`report` and `records`) MUST come from the single `loadCompactRecoveryRecords` seam — sdd-tasks should carry a test asserting this (no second/independent record-loading path feeds derivation).

Readback = `compactAuthorityRemovalRegression(records, compactRecordsWithout(records, seed))` **plus** a fresh inspection that must be `Complete && Valid` before success is reported. Replay: a committed record whose proof carries the same plan digest returns the same identity and moves nothing; a different digest refuses.

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/authority_disposition_plan.go` | Create | Plan type, seed/closure derivation, digests, authorization binding |
| `internal/reviewtransaction/authority_disposition_execute.go` | Create | Leaf admission, lock+CAS, replay, quarantine, readback |
| `internal/reviewtransaction/compact_inspect.go` | Modify | Extract `loadCompactRecoveryRecords`; add repair exit; no semantics or JSON change |
| `internal/reviewtransaction/compact_reconcile.go` | Modify | `DispositionClass` on the two content-mismatch branches |
| `internal/reviewtransaction/compact_reclaim.go` | Modify | `AuthorityDispositionProof` on `CompactReclaimRecord` |
| `internal/reviewtransaction/authority_repair.go` | Modify | Route classified execution through the plan; legacy-alias class unchanged |
| `internal/cli/review_repair.go`, `review_next_transition.go` | Modify | Plan-bound preflight output and execution inputs, same verb |
| `bench/axis_damaged_store.go` | Modify | Black-box repair and refusal journeys |

## Interfaces / Contracts

```go
func deriveAuthorityDispositionPlan(report CompactRecoveryInspectionReport,
    records map[string]CompactRecord, binding, actor, reason string) (AuthorityDispositionPlan, error)
func admitLeafDisposition(plan AuthorityDispositionPlan) error // refuses cardinality != 1
```

## Testing Strategy

| Exit-evidence family | Layer | Approach (reused idiom) |
|---|---|---|
| Classification / refusal | Unit | `compact_reconcile_test.go`; `compact_forged_authorization_test.go`'s blocked assertion becomes the runnable-exit assertion |
| Plan determinism | Unit | Same records ⇒ same digest; any revision change ⇒ new digest |
| Repair (black-box) | Bench | New `damagedStoreJourneys` fixture that proves the damage through `review inspect-authority` first |
| Replay | Integration | Identical second execution returns the same record identity, moves nothing twice |
| Crash | Integration | `compactReclaimPhaseHook` at `prepared`/`renamed`/`committed` (`compact_damaged_store_exit_test.go`) |
| Concurrency | Integration | `maintenance_lock_test.go` idiom; stale `ExpectedRevisions` ⇒ `ErrConcurrentUpdate` |
| Retained graph | Integration | Post-quarantine inspection `Complete && Valid`; unrelated lineages byte-unchanged |

**Rejected**: one end-to-end bench journey covering all seven families. Crash and CAS-drift phases are not reachable black-box — the phase hooks in `compact_reclaim.go` (`compactReclaimPhaseHook`) exist precisely for white-box injection, so those two families must stay integration-level, not bench.

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A: no file classification or execution | — | — |
| Git repository selection | Applicable: `--cwd`, worktrees, `.git` gitfile, `commondir`, symlinks | `authorityRepairRoot` + `ensureCanonicalReviewQuarantineRoot` resolve and refuse noncanonical roots; `RepositoryBinding` is inside the plan digest | One per selector: relative `--cwd`, linked worktree, symlinked common dir, foreign-binding refusal |
| Commit state | N/A: authority store only, no index/worktree read | — | — |
| Push state | N/A: no remote interaction | — | — |
| PR commands | N/A: no PR automation | — | — |

## Migration / Rollout

No data migration. Rollback disables the executor: derivation and inspection stay read-only, authority bytes untouched, quarantined entries keep residue. `ensureNoPreparedCompactBatchReconciliation` already blocks inspection while a batch is prepared, so no cross-build plan straddles the upgrade.

## PR Slicing Preview (for sdd-tasks)

Chained on `feature/rdd-root-simplification` after Wave 1 merges; ≤1000 authored lines/slice; deadcode ratchet per slice.

| Slice | Work unit | Forecast |
|---|---|---|
| S1 | Plan type, digests, derivation seam, `DispositionClass` + unit tests | ~400 |
| S2 | Leaf admission, lock+CAS, quarantine proof, replay, readback + tests | ~600 |
| S3 | `review repair` plan-bound preflight/execution, sanctioned repair exit | ~350 |
| S4 | Bench damaged-store journeys, crash and concurrency evidence | ~350 |

**Rejected**: a single PR for plan + executor (S1+S2 combined). It exceeds the 1000-line slice budget and merges derivation evidence (plan determinism, classification) with mutation evidence (lock, CAS, quarantine, replay) in one review surface.

## Open Questions

- [ ] Does #2111's fixture re-derive with a non-empty `DispositionClass`? Decided by the S1 classifier test; if not, supersession is withdrawn.
- [ ] `AuthorityInventoryRevision` covers entry diagnostics as well as edges here — any diagnostic makes the plan underivable. Confirm that is the intended fail-closed strictness.
- [ ] No-wall-clock-expiry authorization (row 3 / decision 4): confirm with the maintainer that CAS-on-`ExpectedRevisions` is accepted as sufficient staleness protection in place of the decision-4 default of a short version-pinned read-only horizon.
