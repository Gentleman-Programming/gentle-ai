# Tasks: RDD Root Simplification — Wave 2 (Leaf Disposition)

## Pending-Confirmation Assumptions (apply agents: do NOT silently harden)

1. **No wall-clock expiry** on `Authorization` — binds only to `plan_digest` + `authority_inventory_revision`; CAS is the sole staleness guard. Spec `rdd-authority-disposition-plan` / "Authorization Binds to Digest and Revision, No Wall-Clock Expiry". Deviates from ratified `rdd-simplification-design` decision 4's short-expiry default (design row 3 ↔ decision 4).
2. **No new public repair verb** — plan derivation stays internal behind existing `review repair`. Spec `rdd-authority-disposition-plan` / "No New Public Repair Verb".

Both ship as-implemented this wave. `sdd-apply` MUST implement exactly what the spec/design state and MUST NOT add an expiry mechanism or a new CLI verb on its own initiative — these stay open in design.md's Open Questions pending maintainer confirmation.

## Review Workload Forecast

| Slice | PR | Content | Est. authored lines | vs 1000 (design cap) | vs 400 (CI gate) |
|---|---|---|---|---|---|
| PR0 | tracker | Land Wave 2 SDD artifacts (proposal, 3 specs, design, tasks) | ~650 | within | exceeds → `size:exception` |
| S1 | PR1 | Plan type, digests, `loadCompactRecoveryRecords` seam, `DispositionClass` + unit tests | ~400 | within | exceeds → `size:exception` |
| S2 | PR2 | Leaf admission, lock+CAS, quarantine proof, replay, readback + tests | ~600 | within | exceeds → `size:exception` |
| S3 | PR3 | `review repair` plan-bound preflight/execution, sanctioned repair exit | ~350 | within | likely within 400 |
| S4 | PR4 | Bench damaged-store journeys, crash + concurrency evidence | ~350 | within | likely within 400 |
| **Total** | | | **~2350** | | 3 of 5 slices need `size:exception` |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

Chain strategy: feature-branch-chain on tracker `feature/rdd-root-simplification`, PR0→PR1→PR2→PR3→PR4 each targeting the prior branch. **Gating**: this chain MUST NOT open PR0 until Wave 1's tracker→main merge has landed on `main` — Wave 2 builds on Wave 1's `shadow_authority_health.go` leaf predicate and the `feature/rdd-root-simplification` branch must be re-cut/rebased from a `main` that already contains Wave 1.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 0 | Land SDD docs | PR0 | N/A (docs) | N/A — docs only | Revert docs commit |
| 1 | Plan type + seam + classification | PR1 | `go test ./internal/reviewtransaction/... -run 'TestAuthorityDispositionPlan|TestLoadCompactRecoveryRecords|TestDispositionClass'` | N/A — not wired to any live path until PR3 | Revert `authority_disposition_plan.go` + `compact_reconcile.go`/`compact_inspect.go` diffs |
| 2 | Leaf executor | PR2 | `go test ./internal/reviewtransaction/... -run 'TestAdmitLeafDisposition|TestAuthorityDispositionExecute'` | N/A — not wired to `review repair` CLI until PR3 | Revert `authority_disposition_execute.go` + `compact_reclaim.go`/`authority_repair.go` diffs |
| 3 | `review repair` wiring | PR3 | `go test ./internal/cli/... -run 'TestReviewRepairPreflight|TestReviewRepairExecute'` | `gentle-ai review repair --preflight` against a fixture-damaged store, real CLI invocation | Revert `review_repair.go`/`review_next_transition.go` diffs; verb reverts to legacy-alias-only |
| 4 | Bench + crash/concurrency evidence | PR4 | `go test ./internal/reviewtransaction/... -run 'TestCompactDamagedStoreExit|TestMaintenanceLock'` | `bench/axis_damaged_store.go` journeys, real repo fixtures | Revert bench fixture + integration test additions; no production code touched |

## Phase 0 (PR0): Land Wave 2 SDD Artifacts

- [x] 0.1 Confirm Wave 1 tracker→main merge landed; rebase/re-cut `feature/rdd-root-simplification` from post-merge `main`.
- [x] 0.2 `git add openspec/changes/rdd-root-simplification-wave2/{proposal.md,specs/*,design.md,tasks.md}`; commit `docs(sdd): land Wave 2 SDD artifacts` on tracker.

## Phase 1 (Slice S1 / PR1): Plan Type, Digests, Derivation Seam, Classification

Satisfies `rdd-authority-disposition-plan` (all 7 requirements) + `rdd-authority-graph-classification` MODIFIED "No Mutation or Execution".

- [x] 1.1 RED: `compact_inspect_test.go` — extract `loadCompactRecoveryRecords(repo) (report, records)`; assert it is the ONLY function both `InspectCompactRecoveryEdges` and `deriveAuthorityDispositionPlan` call for their `report`/`records` inputs (**mandatory obligation (a)**: single-seam test, no independent second record-loading path).
- [x] 1.2 GREEN: extract seam in `compact_inspect.go`; no inspection-semantics or JSON change.
- [x] 1.3 RED: `authority_disposition_plan_test.go` — Plan Field Set: derived plan carries all ten spec fields (`repository_id`…`authorization`) plus permitted `Schema` per the spec-field→Go-field mapping table.
- [x] 1.4 RED: Deterministic Closure Derivation — same graph inspected twice ⇒ identical `SeedSet`/`Closure`; derivation reads only via 1.1's seam, no cache/inferred view.
- [x] 1.5 RED: Closed Anomaly Classification Required — unknown/mixed/ambiguous (blocked classification, e.g. #1656 multi-lineage) ⇒ no plan, typed refusal, no generic fallback.
- [x] 1.6 RED: Plan digest determinism family — same records ⇒ same `plan_digest`; any `Closure`/`ExpectedRevisions`/`AnomalyClass` change ⇒ different digest; `Authorization` excluded from the digest pre-image (nine-field digest, ten-field struct).
- [x] 1.7 RED: Authorization Binds to Digest+Revision, No Expiry — stale `authority_inventory_revision` refuses regardless of elapsed time; valid revision proceeds with no expiry check (pending-confirmation assumption 1 — implement exactly as specified, no expiry code path).
- [x] 1.8 RED: Cardinality Is Executor Policy — plan shape has no leaf-specific field; cardinality-one and cardinality-N closures use the identical struct.
- [x] 1.9 RED: No New Public Repair Verb — CLI command-set fixture asserts plan derivation is reachable only through pre-existing `review repair` (pending-confirmation assumption 2).
- [x] 1.10 RED: classification regression — re-running the classifier alone (no plan derivation call) still produces no plan/mutation (`rdd-authority-graph-classification` delta scenario).
- [x] 1.11 RED: `DispositionClass` on the two content-mismatch branches in `classifyCompactRecoveryEdgeAnomalies`; `CompactRecoveryEdgeInspection` JSON stays byte-identical.
- [x] 1.12 RED: #2111 supersession probe — its fixture re-derives with non-empty `DispositionClass` and leaf cardinality, or the test documents withdrawal (design Open Question 1).
- [x] 1.13 GREEN: `internal/reviewtransaction/authority_disposition_plan.go` — struct, `deriveAuthorityDispositionPlan`, digests via `classifiedAuthorityRepairDigest`; `compact_reconcile.go` `DispositionClass` field.
- [x] 1.14 Ratchet: new unwired functions this slice — `scripts/deadcode-ratchet.sh --update`.

## Phase 2 (Slice S2 / PR2): Leaf Admission, Lock+CAS, Quarantine, Replay, Readback

Satisfies `rdd-leaf-disposition-execution` (all 11 requirements).

- [x] 2.1 RED: Cardinality-One Admission — single-node closure admitted; multi-node (#2014/#1656 shape) refuses, names cardinality + Wave 6 as escalation (not a promise).
- [x] 2.2 RED: No Predecessor Pointer Rewritten — every predecessor pointer byte-identical pre/post execution.
- [x] 2.3 RED: Lock and CAS Reinspection — revision drift under lock (revision R→R+1) refuses on CAS mismatch, zero bytes mutated.
- [x] 2.4 RED: **mandatory obligation (b)** — executor validates the populated `Authorization` against the digest-bound plan at execution time; forged/mismatched `Authorization` (bound to a different `plan_digest`) refuses even when cardinality and CAS both pass.
- [x] 2.5 RED: Byte-Preserving Quarantine With Forensic Residue — quarantined bytes byte-identical to original; residue records original location+content.
- [x] 2.6 RED: Retained-Graph Revalidation Before Success — success reported only if post-quarantine re-classification is `Complete && Valid`, no dangling reference.
- [x] 2.7 RED: Exact Replay Converges Without Double-Move — replaying an already-succeeded plan detects existing quarantine, converges, does not move the entry twice.
- [x] 2.8 RED: Crash Mid-Execution Leaves a Valid Retained Graph — via `compactReclaimPhaseHook` at `prepared`/`renamed`/`committed`; post-restart inspection classifies cleanly, no corrupted intermediate state.
- [x] 2.9 RED: Concurrent Execution Refuses Duplicate Mutation — two concurrent invocations against the same target under the same lock: exactly one proceeds, the other refuses without mutating.
- [x] 2.10 RED: Unknown/Mixed/Ambiguous Shapes Block, No Generic Fallback — non-admitted shape refuses, never quarantines.
- [x] 2.11 RED: Refusal Names Diagnosis + Escalation Artifact, Not a Roadmap Promise — #1656 multi-lineage refusal names the diagnosis and #1656, no delivery-date commitment.
- [x] 2.12 RED: Refusal Requires Explicit Authorization, Never Blocks Elsewhere — unauthorized refusal on one candidate does not affect an unrelated `review start` in another worktree (blocking budget compliance).
- [x] 2.13 GREEN: `internal/reviewtransaction/authority_disposition_execute.go` — `admitLeafDisposition`, lock+CAS, quarantine call-through, replay detection, readback; `AuthorityDispositionProof` on `CompactReclaimRecord` (`compact_reclaim.go`); `authority_repair.go` routes classified execution through the plan.
- [x] 2.14 Ratchet: new unwired functions this slice — `scripts/deadcode-ratchet.sh --update`.

## Phase 3 (Slice S3 / PR3): `review repair` Plan-Bound Preflight/Execution

Satisfies `rdd-authority-disposition-plan` "No New Public Repair Verb" (wiring) + sanctioned exit surfacing.

- [x] 3.1 RED: `review repair --preflight` emits plan digest + inventory revision only, no bytes mutated, no new command added to the CLI surface.
- [x] 3.2 RED: `review repair` execution requires `--plan-digest --inventory-revision --actor --reason --authorization`; missing any flag refuses before lock acquisition.
- [x] 3.3 RED: `SanctionedCompactRecoveryExits` emits `CompactRecoveryEdgeExitRepair = "review repair"` only when derivation AND leaf admission both accept the edge; otherwise existing `Blocked` prose stands unchanged.
- [x] 3.4 GREEN: `internal/cli/review_repair.go`, `review_next_transition.go` — plan-bound preflight/execution inputs on the existing verb.
- [x] 3.5 Ratchet: `scripts/deadcode-ratchet.sh --update` — this slice WIRES S1/S2's previously-unwired functions; confirm baseline entries drop.

## Phase 4 (Slice S4 / PR4): Bench Damaged-Store Journeys, Crash + Concurrency Evidence

Satisfies exit-evidence families: black-box repair, replay, crash, concurrency, retained-graph (bench layer, complementing S2's integration-layer proofs).

- [x] 4.1 RED: `bench/axis_damaged_store.go` — `damagedStoreJourneys` fixture proves the damage via `review inspect-authority` first, then repairs black-box through `review repair`.
- [x] 4.2 RED: bench refusal journey — multi-node, ambiguous, and unauthorized cases each refuse with named diagnosis, no bytes mutated.
- [x] 4.3 RED: bench retained-graph journey — post-repair inspection is `Complete && Valid`, unrelated lineages byte-unchanged.
- [x] 4.4 GREEN: implement fixtures; wire into existing bench harness.
- [x] 4.5 Ratchet: `scripts/deadcode-ratchet.sh --update` if bench fixtures introduce any unwired helper — no new unwired functions this slice, ratchet unchanged (exit 0, "no new unreachable functions").
