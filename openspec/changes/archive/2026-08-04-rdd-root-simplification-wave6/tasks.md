# Tasks: RDD Root Simplification — Wave 6 (Descendant Closure)

Hybrid store: also Engram `sdd/rdd-root-simplification-wave6/tasks`.

## Archive Reconciliation Note

The checkboxes below were reconciled at archive time (sdd-archive), not by sdd-apply. Rationale: (1) the Chain summary and Fix Cycle sections in this same document already record every slice/fix commit hash (S1@41a471ff, S2@8768b7cf, S3@d17b088d, S4@48a70562, S5@40176a8f, F1@bba17974, F2@e174bc2b) as committed; (2) `verify-report.md` cycle 3 (final) reports `Tasks total 44 / Tasks complete 44 / Tasks incomplete 0` and verdict `pass_with_warnings` with 10/10 requirements and 13/13 scenarios compliant; (3) the orchestrator's archive launch prompt states HEAD `e6ac4176` is `main` WITH Wave 6 merged, an explicit final-state fact under the Final-State Authority hierarchy that outranks these intermediate unchecked snapshot boxes. Exactly 44 checkboxes were reconciled: Gate 0.0, Phase 1-5 (40 items), Deletion Deferral D.1/D.2, and Fix-Cycle-1 F1.4 (superseded by F2.1's genuine six-position interruption, per the tasks memory's own note that "F2.1 resolves WARNING-5's letter and spirit for the journey layer"). F1.5 is left unchecked: it remains a genuinely open, non-blocking item, tracked as WARNING-5 in the final (cycle 3) verify-report, not stale bookkeeping for completed work.

## Gate

- [x] 0.0 Verify Waves 3, 4, 5 are merged into tracker `feature/rdd-root-simplification` before opening PR1. Chain strategy is feature-branch-chain: PR1 targets the tracker branch, PR2 targets PR1's branch, PR3→PR2, PR4→PR3, PR5→PR4. Only the tracker merges to `main`. Retarget/rebase if a child diff shows a previous slice's changes.

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

- [x] 1.1 RED (assumption check, FIRST — blocks all other S1 work): in `authority_disposition_plan_test.go`, build a real multi-chain `report.Edges` fixture (≥2 chains, ≥3 nodes) and assert `authorityDispositionClosure`'s BFS `children` map, built from `PredecessorLineageID→SuccessorLineageID`, actually yields the claimed multi-chain closure. This validates the design risk-list assumption against a real fixture before the closure loop is written.
- [x] 1.2 If 1.1 fails: STOP, escalate — the multi-chain assumption is load-bearing for S2/S3/S5; do not patch around it ad hoc.
- [x] 1.3 RED: descendant-first, seed-last ordering — N=1 identity-of-old-sort case and N≥2 multi-chain case (spec `rdd-authority-disposition-plan` / "Ordering is descendant-first, seed-last").
- [x] 1.4 GREEN: replace the `slices.SortFunc` tail in `authorityDispositionClosure` (`authority_disposition_plan.go:162`) with a topological descendant-first emit over the existing BFS `children` map; ties broken lexicographically.
- [x] 1.5 RED: `plan_digest` byte-stability for N=1 (ds06/ds08 goldens unchanged).
- [x] 1.6 RED: admission — N=1 admitted, N≥2 classified admitted, unknown/mixed/ambiguous refused (spec `rdd-closure-disposition-execution` / "N-Node Admission for Closed Anomaly Classes", both scenarios).
- [x] 1.7 GREEN: relax `admitLeafDisposition` (`authority_disposition_execute.go:34`) to `len(SeedSet)==1 && len(Closure)>=1 && Closure[len-1]==SeedSet[0]`; rename export `AdmitAuthorityDispositionLeaf`→`AdmitAuthorityDispositionClosure`; update its 5 call sites (`authority_disposition_execute.go`, `compact_inspect.go`).
- [x] 1.8 RED: leaf regression — `rdd-leaf-disposition-execution` / "Single-node closure is admitted" scenario passes; the multi-node #2014-naming refusal scenario is deleted, not skipped.
- [x] 1.9 Threat matrix RED (Git repository selection, applicable): relative/absolute/foreign-repo `--cwd` still refuse or bind correctly through `ResolveRepositoryRoot`/`authorityRepairRoot` under the renamed export.
- [x] 1.10 REFACTOR/deletion (D9, scoped): delete `errAuthorityDispositionCardinality`, its `#1656`/`#2014`/"a future wave" text, and `TestAuthorityDispositionExecuteRefusesMultiNodeClosure`. Do not touch `ReconcileInvalidRecoveryEdge` — see Deletion Deferral below.
- [x] 1.11 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; write refusal-resolution notes for every refusal text changed or removed.

## Phase 2 (PR2 — S2): Ordered N-Node Transaction

- [x] 2.1 RED: crash after N-1 of N nodes — retained graph classifies cleanly, only the seed unmoved (spec: "Crash after N-1 of N nodes leaves a valid graph").
- [x] 2.2 GREEN: `lockedAuthorityDispositionMutation` (`authority_disposition_execute.go:124`) becomes an ordered loop over `plan.Closure`, reusing `quarantineCompactStoreEntry` per node unchanged.
- [x] 2.3 RED: CAS-all-N — all `ExpectedRevisions` checked before the first move; drift on any non-seed member refuses pre-move.
- [x] 2.4 GREEN: pre-loop CAS validation across the full closure.
- [x] 2.5 RED: partial closure never reports success (spec: "Atomic Visibility With Forward-Only Convergence").
- [x] 2.6 GREEN: gate `readBackAuthorityDisposition` behind last-node commit.
- [x] 2.7 RED: closure-manifest schema — `closure-manifest.json` written inside the seed node's quarantine dir, ordered closure + digest (D5, forensic-only).
- [x] 2.8 GREEN: manifest writer in `compact_reclaim.go` beside `residue/`.
- [x] 2.9 RED: unrelated-lineage byte-identical assertion helper, reusing the `requireDispositionWitnessBytesUnchanged` pattern.
- [x] 2.10 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; refusal-resolution notes.

## Phase 3 (PR3 — S3): Forward-Only Resume

- [x] 3.1 RED: exact replay resumes without a double move — committed skipped, prepared completes via `residue/` (spec scenario).
- [x] 3.2 GREEN: `discoverAuthorityDispositionRecord`/`resumeAuthorityDispositionRecord` invoked per node inside the ordered loop.
- [x] 3.3 RED: digest mismatch or non-re-deriving graph refuses, names the manifest path, escalates — no narrowing re-derivation attempted.
- [x] 3.4 GREEN: digest/re-derivation drift check ahead of resume.
- [x] 3.5 RED integration: crash-position matrix via `compactReclaimPhaseHook` at every ordered position of a 3-node closure (positions N, N-1, ..., 1); replay converges at each.
- [x] 3.6 GREEN: confirm loop coverage is sufficient at every position (expect no new production code beyond 3.2/3.4).
- [x] 3.7 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; refusal-resolution notes.

## Phase 4 (PR4 — S4/D7): Negotiated Transition — designated drop candidate

- [x] 4.1 RED: `next_transition` offers disposition `collect{disposition_authorization}` / `execute{review.repair, --plan-digest --inventory-revision --actor --reason --authorization}` for a repairable-classified graph with an authorized closure plan.
- [x] 4.2 GREEN: extend `reviewRepairTransition` (`review_next_transition.go:546`) with the disposition branch.
- [x] 4.3 RED: `compactStartInvalidGraphRefusal` names `CompactRecoveryEdgeExitRepair` again (restores W2 residue).
- [x] 4.4 GREEN: restore the exit in `compact_inspect.go`.
- [x] 4.5 Threat matrix RED (PR commands, applicable): emitted transition tokens contain no authorization bytes (`"provided"` sentinel) and run verbatim via `reviewTokenizedTransitionArguments`.
- [x] 4.6 `go test ./... -count=1`; `go run ./bench run --axis damaged-store`; deadcode ratchet; refusal-resolution notes.
- [x] 4.7 Overrun guard: if the chain overruns, defer this phase whole to the next wave/PR — it breaks no spec MUST (D4 tradeoff). Do not ship it partially. (Not triggered: the chain did not overrun on this phase and D7 shipped, verified COMPLIANT in verify-report cycle 3's Spec Compliance Matrix.)

## Phase 5 (PR5): ds09+ Bench Journeys — Exit Evidence

- [x] 5.1 `bench/axis_damaged_store.go`: `ds09-multi-chain-closure` — ≥2 chains, ≥3 nodes, classified and disposed end-to-end, byte-preserving.
- [x] 5.2 `ds10-cross-lineage-closure` — reachable chains quarantined, unrelated third lineage byte-identical (spec: "Cross-lineage closure disposes only reachable nodes").
- [x] 5.3 `ds11-crash-recovery-mid-closure` — interrupt at every ordered position, replay resumes, no double move, clean retained-graph revalidation at each position. (Superseded in shape, not substance, by Fix Cycle 2's F2.1: the single hand-authored journey was replaced by six genuinely-interrupting `ds11-crash-recovery-<phase>-<role>` journeys covering all 6 ordered positions.)
- [x] 5.4 `ds12-negotiated-transition-route` — `next_transition` disposition collect/execute journey. PR4/D7 was not deferred; ds12 shipped and is verified COMPLIANT in verify-report cycle 3.
- [x] 5.5 `go test ./... -count=1`; `go run ./bench run --axis damaged-store --journey ds09,ds10,ds11,ds12`; deadcode ratchet; refusal-resolution notes per journey.

## Fix Cycle 1 (Blocker Resolution — post sdd-verify FAIL)

sdd-verify found the S1-S5 chain FAILING with 2 CONFIRMED CRITICAL findings
(one a security hole) plus 3 warnings. Fixed on branch
`feat/rdd-wave6-f1-resume-authorization` (chained from S5 @ `40176a8f`),
commit `bba17974`.

- [x] F1.1 CRITICAL-1 (security, authorization bypass on resume): `validateAuthorityDispositionAuthorization` was gated inside `lockedAuthorityDispositionMutation`'s fresh-execution-only branch (its only non-test call site), so every resume executed unauthorized — a forged `--authorization` and a different actor were silently admitted. Fixed: authorization and CAS-all-N now validate on every execution path (fresh or resumed); only the plan re-derivation comparison stays fresh-only (the one part that legitimately cannot work mid-closure). RED: `TestAuthorityDispositionResumeRefusesForgedAuthorization` (Go, hook-interrupted resume + forged authorization + actor="attacker") and a new ds11 bench step driving the identical repro through the real binary (N=3 CLI garbage-authorization run).
- [x] F1.2 CRITICAL-2 (by-design refusals surfaced as defect reports): `reviewRepairOperationError.Error()` dropped its cause, and the type was unrecognized by the negotiated classification cascade, so every disposition-execution refusal (including a plain stale `--plan-digest`) fell through to `operation_outcome_unknown` and appended a saved-defect-report clause. Fixed: a refusal whose returned record proves nothing mutated in this call now routes through `reviewPreflightError`, restoring base bb3c22a9's classification; `reviewRepairOperationError.Error()` also stops dropping its cause for the rarer post-partial-mutation case. RED: `TestReviewRepairDispositionExecutionDigestMismatchRefusesWithoutDefectReport` (N=1 digest-mismatch, base-vs-tip comparison).
- [x] F1.3 SUGGESTION (folded in, cheap and directly adjacent to F1.1): `authorityDispositionResumeDigestMismatchRefusal` selected candidate quarantine directories by name-prefix only; now also checks the candidate's own decoded `LineageID` field, matching `discoverAuthorityDispositionRecord`'s existing pattern.
- [x] F1.4 WARNING (deliberately skipped in Fix Cycle 1, resolved in Fix Cycle 2): ds11 had no real crash-KILL semantics (hand-authored on-disk state, covered only 1 of 6 ordered positions). Resolved by F2.1's genuine six-position interruption through the real binary; per the tasks record, "F2.1 resolves WARNING-5's letter and spirit for the journey layer."
- [ ] F1.5 WARNING (deliberately deferred, not required for this blocker set): `TestAuthorityDispositionPlanDigestN1ByteStability` hand-constructs its plan and never calls `authorityDispositionClosure`, so it pins the digest function, not the derivation. A real fixture-driven regression test for this remains open; not added this cycle to keep scope to the two hard blockers plus directly-adjacent items.

Verification for this fix cycle: `go test ./... -count=1` (root, 63/63 packages ok) + bench module (ok); fresh binary bench runs — full damaged-store axis (71/71 completed, 0 failed) including ds06-ds12 explicitly, plus the full black-box core corpus (59/59 completed, 0 failed); gofmt/vet clean repo-wide; deadcode ratchet clean; refusal-resolution ratchet passes with no baseline change. Diff: 295 insertions / 20 deletions across 5 files, one commit. Tracker tip re-checked at `e599c679` (Wave 5 land, `feat(review): land RDD root simplification Wave 5 (receipt-only gate cutover)`) — zero file overlap with this fix cycle's touched files (`internal/reviewtransaction/authority_disposition_execute.go`, `internal/cli/review_repair.go`, `internal/cli/review_repair_test.go`, `internal/reviewtransaction/authority_disposition_resume_test.go`, `bench/axis_damaged_store_closure.go`); no rebase performed, chain stays on S5 per explicit fix-cycle instruction.

Both criticals close. This does not authorize archiving on its own — re-run sdd-verify next.

## Fix Cycle 2 (Blocker Resolution — wave's last, post sdd-verify FAIL cycle 2)

sdd-verify cycle 2 found zero criticals and zero blockers — both cycle-1
CRITICALs stayed CLOSED with fresh, independently reproduced evidence — but
kept the verdict `fail` because one spec scenario ("Multi-chain and
crash-recovery journeys pass") was still only PARTIAL: ds11 authored a
pre-broken on-disk state rather than genuinely interrupting, and covered only
1 of the 3-node closure's 6 ordered positions
(TestAuthorityDispositionResumeCrashPositionMatrix already proves 6/6 at the
Go layer). Coordinator decision: **extend the journey, do not amend the
spec** — this wave's own S5 found a real public-entrypoint defect Go-level
tests structurally could not see, so the spec's "interrupt at each ordered
position" is load-bearing at the journey layer, not ceremony. Fixed and
committed on branch `feat/rdd-wave6-f2-journey-positions` (chained from F1 @
`bba17974`), commit `e174bc2b`. This closes Wave 6.

- [x] F2.1 ds11 now genuinely interrupts (not authors) and resumes at all 6 ordered positions (prepared + committed x grandchild/child/seed), each converging byte-identically to its own pre-disposition store bytes. Mechanism: the exact deterministic phase-hook interruption `TestAuthorityDispositionResumeCrashPositionMatrix` already uses in-process (`compactReclaimPhaseHook`), made reachable through the real binary via a new build-tag-gated product hook (`internal/reviewtransaction/bench_fixture.go`, `-tags bench_fixture`, mirroring `internal/sddstatus/bench_fixture.go`'s established j57 pattern exactly): `GENTLE_AI_BENCH_CRASH_AT_PHASE="<phase>:<lineage>"` refuses right after that exact phase for that exact lineage, once per process. Six journeys generated (`ds11-crash-recovery-<phase>-<role>`), replacing the single hand-authored `ds11-crash-recovery-mid-closure`; the committed/grandchild variant carries forward fix cycle 1's forged-authorization-on-resume mutation proof (the position the old ds11 covered). Bench-side: `Sandbox.BenchCrashAtPhase` field (runner.go), `canonicalStoreDirectoryDigest` (pre/post-disposition byte comparison), `requireGenuineBenchFixtureCrash` (nonzero exit + the bench_fixture marker text, else reports the journey unsupported on a plain binary — j57's own graceful-degradation pattern). Verified: 6/6 positions complete against a `-tags bench_fixture` binary (0 failed); the same 6 correctly report `unsupported` (not `failed`) against a plain binary, matching j57. Now-orphaned hand-authoring helpers (`authorCommittedClosureMemberQuarantine`, `closureExpectedRevisions`, `authorCrashAfterFirstDescendantCommitted`, `requireClosureMemberAlreadyQuarantined`) deleted.
- [x] F2.2 WARNING-2 (doc comment inaccuracy): `validateAuthorityDispositionAuthorization`'s doc comment claimed it validates the CURRENT `authority_inventory_revision`; on resume it validates the plan's own FROZEN value against itself (a deliberate no-op for drift; CAS-all-N is the live guard). Comment rewritten to state the real per-path contract; parameter renamed `currentAuthorityInventoryRevision` -> `callerAuthorityInventoryRevision` to stop implying "always live."
- [x] F2.3 WARNING-4 (lost executable continuation): the digest-mismatch refusal in `repairAuthorityDispositionAtRepo` printed its cause (fix cycle 1) but, unlike base bb3c22a9, named no next action. Restored: "; run `gentle-ai review repair --preflight` again for the current values".
- Deliberately skipped per coordinator instruction: SUGGESTION-1 (mixed actor provenance across a resumed closure's members) needs design attention, not a code fix — left as a Wave 7 backlog item, noted here for that purpose. WARNING-3 (`record.LineageID == ""` heuristic misclassifies the rare mid-loop-after-skip digest mismatch), WARNING-5/6 (ds11 crash-kill semantics / N=1 digest pin test coverage, now folded into F2.1's genuine interruption and its own byte-identity proof respectively) were not separately re-opened — F2.1 resolves WARNING-5's letter and spirit for the journey layer.

Verification for fix cycle 2: `go test ./... -count=1` (root, all packages ok) + bench module (ok); fresh binaries — `-tags bench_fixture` binary: full damaged-store axis 76/76 completed 0 failed (incl. all 6 new ds11 positions + ds06-ds10/ds12 regression-clean), full corpus with every axis 89/89 completed 0 failed (j57 also completes, since bench_fixture supports its own seam too); plain binary: 70 completed + 6 unsupported (the 6 crash-position journeys, gracefully, not failed) + 0 failed on damaged-store, 82 completed + 7 unsupported (+j57) + 0 failed on the full corpus; gofmt/vet clean repo-wide; deadcode ratchet clean (orphaned hand-authoring helpers deleted, not left dead); refusal-resolution ratchet passes (one new by-design annotation on the bench-only injected-fault site in `bench_fixture.go`, `world-action`, since it is a deliberately-injected test fault with no genuine product-side resolution). Diff: 594 changed/added lines across 5 files (4 modified + 1 new), one commit. Tracker tip re-checked at `e599c679` (unchanged since fix cycle 1) — zero overlap with this cycle's touched files.

This closes Wave 6. Both fix cycles' blockers/warnings are resolved or explicitly deferred with rationale recorded above.

## Deletion Deferral (D9 / Wave 7)

- [x] D.1 PR1 (task 1.10) deletes only `errAuthorityDispositionCardinality`, its `#1656`/`#2014` refusal text, and `TestAuthorityDispositionExecuteRefusesMultiNodeClosure`.
- [x] D.2 Do NOT delete `ReconcileInvalidRecoveryEdge` (`compact_reconcile.go:233`) this wave. It has 4 live consumers confirmed by call-graph: `review_reconcile.go`, `review_reconcile_batch.go`, `compact_batch_reconcile_journal.go`, and bench journeys `ds01`/`ds02`/`ds04`. Record this evidence in the PR1 description; deletion is Wave 7's public-surface retirement with its own consumer-migration evidence, not this wave's admission relaxation. (Honored: `ReconcileInvalidRecoveryEdge` remained untouched across bb3c22a9..e174bc2b, per verify-report cycle 3's Coherence table, D9 row.)
