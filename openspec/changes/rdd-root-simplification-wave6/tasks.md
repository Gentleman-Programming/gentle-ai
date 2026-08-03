# Tasks: RDD Root Simplification — Wave 6 (Descendant Closure)

Hybrid store: also Engram `sdd/rdd-root-simplification-wave6/tasks`.

## Gate

- [ ] 0.0 Verify Waves 3, 4, 5 are merged into tracker `feature/rdd-root-simplification` before opening PR1. Chain strategy is feature-branch-chain: PR1 targets the tracker branch, PR2 targets PR1's branch, PR3→PR2, PR4→PR3, PR5→PR4. Only the tracker merges to `main`. Retarget/rebase if a child diff shows a previous slice's changes.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~600 (PR1) + ~900 (PR2) + ~700 (PR3) + ~600 (PR4) + ~800 (PR5) ≈ 3600 total |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR0 (SDD artifacts, no code) → PR1 (S1) → PR2 (S2) → PR3 (S3) → PR4 (S4/D7) → PR5 (ds09+ journeys) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |
| Per-PR budget | 1000 lines/PR (session override; repo CI default 400 + `size:exception`) |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

PR2 and, before splitting, the design's combined S4 sit near the 1000-line ceiling — this task set splits the design's S4 (negotiated transition + ds09-ds12, ~900) into PR4 (D7 only) and PR5 (journeys only) to keep both slices clear of the ceiling and to preserve D7 as the sole, cleanly droppable slice on overrun.

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 0 | SDD artifacts only (proposal/spec/design/tasks) | PR0 | N/A — no code | N/A — docs only | Revert commit; no runtime state |
| 1 | Topological ordering + N≥1 admission | PR1 | `go test ./internal/reviewtransaction/... -run TestAuthorityDispositionClosure -count=1` | `go run ./bench run --axis damaged-store --journey ds06,ds08` (byte-stability regression) | Revert `authority_disposition_plan.go`/`authority_disposition_execute.go` hunks; N=1 path untouched |
| 2 | Ordered N-node transaction, CAS-all-N, manifest | PR2 | `go test ./internal/reviewtransaction/... -run TestAuthorityDispositionExecute -count=1` | `go run ./bench run --axis damaged-store --journey ds09,ds10` (once written in PR5; until then unit test only) | Revert `lockedAuthorityDispositionMutation` loop + `compact_reclaim.go` manifest writer; admission stays N=1-only if PR1 also reverted |
| 3 | Forward-only resume + crash-position tests | PR3 | `go test ./internal/reviewtransaction/... -run TestAuthorityDispositionResume -count=1` | `compactReclaimPhaseHook` crash injection (in-process, N/A external harness) | Revert resume branch; PR2's non-resumed loop still functions, only replay is lost |
| 4 | Negotiated transition (D7) | PR4 | `go test ./internal/cli/... -run TestReviewNextTransition -count=1` | `gentle-ai review status --next-transition` against a repairable-classified fixture | Revert `reviewRepairTransition` disposition branch + `compact_inspect.go` exit restore; raw flag triad still works |
| 5 | ds09+ bench journeys (exit evidence) | PR5 | `go test ./internal/reviewtransaction/... -count=1` (regression) | `go run ./bench run --axis damaged-store --journey ds09,ds10,ds11,ds12` | Revert journey additions to `bench/axis_damaged_store.go`; ds01-ds08 untouched |

## Phase 1 (PR1 — S1): Ordering + Admission

- [ ] 1.1 RED (assumption check, FIRST — blocks all other S1 work): in `authority_disposition_plan_test.go`, build a real multi-chain `report.Edges` fixture (≥2 chains, ≥3 nodes) and assert `authorityDispositionClosure`'s BFS `children` map, built from `PredecessorLineageID→SuccessorLineageID`, actually yields the claimed multi-chain closure. This validates the design risk-list assumption against a real fixture before the closure loop is written.
- [ ] 1.2 If 1.1 fails: STOP, escalate — the multi-chain assumption is load-bearing for S2/S3/S5; do not patch around it ad hoc.
- [ ] 1.3 RED: descendant-first, seed-last ordering — N=1 identity-of-old-sort case and N≥2 multi-chain case (spec `rdd-authority-disposition-plan` / "Ordering is descendant-first, seed-last").
- [ ] 1.4 GREEN: replace the `slices.SortFunc` tail in `authorityDispositionClosure` (`authority_disposition_plan.go:162`) with a topological descendant-first emit over the existing BFS `children` map; ties broken lexicographically.
- [ ] 1.5 RED: `plan_digest` byte-stability for N=1 (ds06/ds08 goldens unchanged).
- [ ] 1.6 RED: admission — N=1 admitted, N≥2 classified admitted, unknown/mixed/ambiguous refused (spec `rdd-closure-disposition-execution` / "N-Node Admission for Closed Anomaly Classes", both scenarios).
- [ ] 1.7 GREEN: relax `admitLeafDisposition` (`authority_disposition_execute.go:34`) to `len(SeedSet)==1 && len(Closure)>=1 && Closure[len-1]==SeedSet[0]`; rename export `AdmitAuthorityDispositionLeaf`→`AdmitAuthorityDispositionClosure`; update its 5 call sites (`authority_disposition_execute.go`, `compact_inspect.go`).
- [ ] 1.8 RED: leaf regression — `rdd-leaf-disposition-execution` / "Single-node closure is admitted" scenario passes; the multi-node #2014-naming refusal scenario is deleted, not skipped.
- [ ] 1.9 Threat matrix RED (Git repository selection, applicable): relative/absolute/foreign-repo `--cwd` still refuse or bind correctly through `ResolveRepositoryRoot`/`authorityRepairRoot` under the renamed export.
- [ ] 1.10 REFACTOR/deletion (D9, scoped): delete `errAuthorityDispositionCardinality`, its `#1656`/`#2014`/"a future wave" text, and `TestAuthorityDispositionExecuteRefusesMultiNodeClosure`. Do not touch `ReconcileInvalidRecoveryEdge` — see Deletion Deferral below.
- [ ] 1.11 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; write refusal-resolution notes for every refusal text changed or removed.

## Phase 2 (PR2 — S2): Ordered N-Node Transaction

- [ ] 2.1 RED: crash after N-1 of N nodes — retained graph classifies cleanly, only the seed unmoved (spec: "Crash after N-1 of N nodes leaves a valid graph").
- [ ] 2.2 GREEN: `lockedAuthorityDispositionMutation` (`authority_disposition_execute.go:124`) becomes an ordered loop over `plan.Closure`, reusing `quarantineCompactStoreEntry` per node unchanged.
- [ ] 2.3 RED: CAS-all-N — all `ExpectedRevisions` checked before the first move; drift on any non-seed member refuses pre-move.
- [ ] 2.4 GREEN: pre-loop CAS validation across the full closure.
- [ ] 2.5 RED: partial closure never reports success (spec: "Atomic Visibility With Forward-Only Convergence").
- [ ] 2.6 GREEN: gate `readBackAuthorityDisposition` behind last-node commit.
- [ ] 2.7 RED: closure-manifest schema — `closure-manifest.json` written inside the seed node's quarantine dir, ordered closure + digest (D5, forensic-only).
- [ ] 2.8 GREEN: manifest writer in `compact_reclaim.go` beside `residue/`.
- [ ] 2.9 RED: unrelated-lineage byte-identical assertion helper, reusing the `requireDispositionWitnessBytesUnchanged` pattern.
- [ ] 2.10 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; refusal-resolution notes.

## Phase 3 (PR3 — S3): Forward-Only Resume

- [ ] 3.1 RED: exact replay resumes without a double move — committed skipped, prepared completes via `residue/` (spec scenario).
- [ ] 3.2 GREEN: `discoverAuthorityDispositionRecord`/`resumeAuthorityDispositionRecord` invoked per node inside the ordered loop.
- [ ] 3.3 RED: digest mismatch or non-re-deriving graph refuses, names the manifest path, escalates — no narrowing re-derivation attempted.
- [ ] 3.4 GREEN: digest/re-derivation drift check ahead of resume.
- [ ] 3.5 RED integration: crash-position matrix via `compactReclaimPhaseHook` at every ordered position of a 3-node closure (positions N, N-1, ..., 1); replay converges at each.
- [ ] 3.6 GREEN: confirm loop coverage is sufficient at every position (expect no new production code beyond 3.2/3.4).
- [ ] 3.7 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; refusal-resolution notes.

## Phase 4 (PR4 — S4/D7): Negotiated Transition — designated drop candidate

- [ ] 4.1 RED: `next_transition` offers disposition `collect{disposition_authorization}` / `execute{review.repair, --plan-digest --inventory-revision --actor --reason --authorization}` for a repairable-classified graph with an authorized closure plan.
- [ ] 4.2 GREEN: extend `reviewRepairTransition` (`review_next_transition.go:546`) with the disposition branch.
- [ ] 4.3 RED: `compactStartInvalidGraphRefusal` names `CompactRecoveryEdgeExitRepair` again (restores W2 residue).
- [ ] 4.4 GREEN: restore the exit in `compact_inspect.go`.
- [ ] 4.5 Threat matrix RED (PR commands, applicable): emitted transition tokens contain no authorization bytes (`"provided"` sentinel) and run verbatim via `reviewTokenizedTransitionArguments`.
- [ ] 4.6 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; refusal-resolution notes.
- [ ] 4.7 Overrun guard: if the chain overruns, defer this phase whole to the next wave/PR — it breaks no spec MUST (D4 tradeoff). Do not ship it partially.

## Phase 5 (PR5): ds09+ Bench Journeys — Exit Evidence

- [ ] 5.1 `bench/axis_damaged_store.go`: `ds09-multi-chain-closure` — ≥2 chains, ≥3 nodes, classified and disposed end-to-end, byte-preserving.
- [ ] 5.2 `ds10-cross-lineage-closure` — reachable chains quarantined, unrelated third lineage byte-identical (spec: "Cross-lineage closure disposes only reachable nodes").
- [ ] 5.3 `ds11-crash-recovery-mid-closure` — interrupt at every ordered position, replay resumes, no double move, clean retained-graph revalidation at each position.
- [ ] 5.4 `ds12-negotiated-transition-route` — `next_transition` disposition collect/execute journey. If PR4 was deferred per 4.7, defer ds12 too and record it as a follow-up; do not fabricate coverage for an undelivered route.
- [ ] 5.5 `go test ./... -count=1`; `go run ./bench run --axis damaged-store --journey ds09,ds10,ds11,ds12`; deadcode ratchet; refusal-resolution notes per journey.

## Deletion Deferral (D9 / Wave 7)

- [ ] D.1 PR1 (task 1.10) deletes only `errAuthorityDispositionCardinality`, its `#1656`/`#2014` refusal text, and `TestAuthorityDispositionExecuteRefusesMultiNodeClosure`.
- [ ] D.2 Do NOT delete `ReconcileInvalidRecoveryEdge` (`compact_reconcile.go:233`) this wave. It has 4 live consumers confirmed by call-graph: `review_reconcile.go`, `review_reconcile_batch.go`, `compact_batch_reconcile_journal.go`, and bench journeys `ds01`/`ds02`/`ds04`. Record this evidence in the PR1 description; deletion is Wave 7's public-surface retirement with its own consumer-migration evidence, not this wave's admission relaxation.
