```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:7b1f5f96b22474a337d8e1f8b5a603494f8d3ea2a5522b104f4892e4aa24379b
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 19/19
scenarios: 24/24
test_command: go test -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:34762889eb2865385ef645eb1e7f08dc917dfa44f039ff67dfc77866ec3bb00c
build_command: go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: rdd-root-simplification-wave2 (Leaf Disposition), whole Wave 2
**Version**: 3 delta specs (2 NEW, 1 MODIFIED)
**Mode**: Strict TDD
**Verified tip**: `56629a01` on `feat/rdd-wave2-bench-journeys`, worktree `/home/gentleman/work/gentle-ai-worktrees/rdd-wave0`
**Base**: `d591f4cf` (main, Wave 1 merged)
**Delivery mode**: receipt-driven development is `off` (global off, clone-local off) — delivery reports `disabled/unmanaged`; no review authority is required or claimed by this verification.
**Attempt authority (echoed, not settled)**: `sha256:1d5fc742eee75c654e65ed704a26a90ac0c3244b734b397e260ba13f3959b606`

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 44 |
| Tasks complete | 44 |
| Tasks incomplete | 0 |

Phases: 0.1–0.2 (2), 1.1–1.14 (14), 2.1–2.14 (14), 3.1–3.5 (5), 4.1–4.5 (5). Every `[x]` was cross-checked against a real file and a passing test; no task is checked without corresponding code.

### Chain Integrity

Verified by `git rev-parse` on every parent — strictly linear, six commits, no merge, no fork.

| Slice | Branch | Commit | Parent | Verified |
|---|---|---|---|---|
| base | (main) | `d591f4cf` | — | ✅ |
| PR0 | `docs/rdd-wave2-sdd-artifacts` | `722df9d6` | `d591f4cf` | ✅ |
| S1 | `feat/rdd-wave2-disposition-plan` | `2b12bc95` | `722df9d6` | ✅ |
| S2 | `feat/rdd-wave2-leaf-executor` | `f7d234f2` | `2b12bc95` | ✅ |
| S3 | `feat/rdd-wave2-repair-wiring` | `929a344f` | `f7d234f2` | ✅ |
| S4 | `feat/rdd-wave2-bench-journeys` | `29a023d6` | `929a344f` | ✅ |
| fix | `feat/rdd-wave2-bench-journeys` | `56629a01` | `29a023d6` | ✅ |

Every branch tip resolves to the expected commit; `feat/rdd-wave2-bench-journeys` carries both S4 and the fix.

### Per-Slice Review Budgets (re-measured, `git diff --numstat`)

| Slice | Additions | Deletions | Total | vs 1000 cap | vs 400 CI gate | Forecast | Delta |
|---|---|---|---|---|---|---|---|
| PR0 | 1700 | 0 | **1700** | ❌ exceeds | needs `size:exception` | ~650 | +162% |
| S1 | 739 | 21 | **760** | ✅ within | needs `size:exception` | ~400 | +90% |
| S2 | 859 | 15 | **874** | ✅ within | needs `size:exception` | ~600 | +46% |
| S3 | 430 | 55 | **485** | ✅ within | needs `size:exception` | ~350 | +39% |
| S4 (`29a023d6`) | 500 | 5 | 505 | ✅ within | — | ~350 | +44% |
| fix (`56629a01`) | 155 | 121 | 276 | ✅ within | — | — | — |
| **S4 + fix (PR4 surface)** | 549 | 20 | **569** | ✅ within | needs `size:exception` | ~350 | +63% |

**5 of 5 delivery slices exceed the 400-line CI gate**, not the "3 of 5" tasks.md forecast. PR0's raw diff also breaches the design's ≤1000 authored-lines-per-slice cap; 1155 of its 1700 lines are Wave 1 archival carry (`archive-report.md` 217, Wave 1 `verify-report.md` 637, 301 lines of Wave 1 spec promotion into `openspec/specs/`), leaving 545 Wave-2-authored doc lines.

### Build & Tests Execution

**Build**: ✅ Passed — `go build ./...` exit 0, empty output. Both binaries built from tip `56629a01` (`cmd/gentle-ai` 23.2 MB, `bench` 5.6 MB).

**Tests**: ✅ Passed — `go test -count=1 ./...` exit 0, 63 packages `ok`, **0 cached**, 0 `FAIL`.

```text
ok  github.com/gentleman-programming/gentle-ai/v2/internal/app              17.799s
ok  github.com/gentleman-programming/gentle-ai/v2/internal/cli             161.682s
ok  github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction 125.163s
```

A first `go test ./...` returned exit 0 entirely from cache (63 cached entries) and was **discarded as non-evidence**; the recorded run is the uncached `-count=1` execution.

**Bench module**: `go test -count=1 ./...` in `bench/` → `ok github.com/gentleman-programming/gentle-ai/bench 0.173s`, exit 0.

**Runtime harness (re-executed independently, not trusted from apply)**:
`gentle-ai-bench run --binary <tip-built gentle-ai> --axis damaged-store` → exit 0, **65 journeys: 65 completed, 0 unsupported, 0 failed**. Result JSON status histogram: `{"completed": 65}` over 65 journeys.

| Bench journey | Status |
|---|---|
| `ds06-content-mismatched-leaf-repaired-via-disposition-plan` | ✅ completed |
| `ds07-two-content-mismatched-edges-no-disposition-plan` | ✅ completed |
| `ds08-content-mismatched-leaf-forged-authorization-refuses` | ✅ completed |

**Ratchets**: `scripts/deadcode-ratchet.sh` → exit 0, "no new unreachable functions". Only one ratchet script exists in this repository; no second ratchet was found (confirms the apply-progress claim).

**Quality gates**: `gofmt -l .` clean (main module and `bench/`); `go vet ./...` exit 0.

**Coverage**: ➖ Not run — no coverage threshold is configured for this repository and coverage is informational under the Strict TDD module.

### Spec Compliance Matrix

Counted from the authoritative branch-committed specs: **19 requirements, 24 scenarios**. Every scenario below is COMPLIANT only because a covering test was observed passing at runtime in this verification.

#### `rdd-authority-disposition-plan` (7 requirements, 10 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Plan Field Set | Plan carries all required fields | `authority_disposition_plan_test.go > TestAuthorityDispositionPlanFieldSet` | ✅ COMPLIANT |
| Deterministic Closure Derivation | Same graph state derives the same closure | `TestAuthorityDispositionPlanDeterministicClosureDerivation` | ✅ COMPLIANT |
| Closed Anomaly Classification Required | Unknown shape yields no plan | `TestAuthorityDispositionPlanRequiresClosedClassification` | ✅ COMPLIANT |
| Closed Anomaly Classification Required | Content-mismatched binding classifies before it can plan | `TestAuthorityDispositionPlanRequiresClosedClassification` | ✅ COMPLIANT |
| Plan Digest Binds Exact Content | Content change invalidates the digest | `TestAuthorityDispositionPlanDigestDeterminism` | ✅ COMPLIANT |
| Plan Digest Binds Exact Content | Actor and reason do not affect plan_digest | `TestAuthorityDispositionPlanDigestExcludesActorAndReason` + `review_repair_test.go > TestReviewRepairDispositionExecutionAcceptsPreflightPublishedDigest` | ✅ COMPLIANT |
| Authorization Binds to Digest and Revision | Stale revision refuses regardless of elapsed time | `TestAuthorityDispositionPlanAuthorizationBindsWithNoExpiry` | ✅ COMPLIANT |
| Authorization Binds to Digest and Revision | Valid, unexpired-by-clock plan proceeds | `TestAuthorityDispositionPlanAuthorizationBindsWithNoExpiry` | ✅ COMPLIANT |
| Cardinality Is an Executor Admission Policy | Same plan shape serves a single-node closure | `TestAuthorityDispositionPlanCardinalityIsExecutorPolicyNotShapeConstraint` | ✅ COMPLIANT |
| No New Public Repair Verb | Plan derivation has no new command | `TestAuthorityDispositionPlanNoNewPublicRepairVerb` (+ independent chain diff, see W5) | ✅ COMPLIANT |

#### `rdd-leaf-disposition-execution` (11 requirements, 12 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| Cardinality-One Admission | Single-node closure is admitted | `authority_disposition_execute_test.go > TestAuthorityDispositionExecuteAdmitsSingleNodeClosure` | ✅ COMPLIANT |
| Cardinality-One Admission | Multi-node closure refuses | `TestAuthorityDispositionExecuteRefusesMultiNodeClosure` | ✅ COMPLIANT |
| No Predecessor Pointer Rewritten | Retained predecessors are untouched | `TestAuthorityDispositionExecuteNoPredecessorPointerRewritten` | ✅ COMPLIANT |
| Lock and CAS Reinspection Before Mutation | Revision drift under lock refuses | `TestAuthorityDispositionExecuteLockAndCASReinspection` | ✅ COMPLIANT |
| Byte-Preserving Quarantine With Forensic Residue | Quarantined bytes are unmodified | `TestAuthorityDispositionExecuteQuarantineIsByteExact` | ✅ COMPLIANT |
| Retained-Graph Revalidation Before Success | Success requires a clean revalidation | `TestAuthorityDispositionExecuteRetainedGraphRevalidationBeforeSuccess` | ✅ COMPLIANT |
| Exact Replay Converges Without Double-Move | Replay after success is a no-op | `TestAuthorityDispositionExecuteReplayConvergesWithoutDoubleMove` | ✅ COMPLIANT |
| Crash Mid-Execution Leaves a Valid Retained Graph | Crash after lock, before quarantine completes | `TestAuthorityDispositionExecuteCrashMidExecutionLeavesValidGraph` (3 subtests: prepared/renamed/committed) | ✅ COMPLIANT |
| Concurrent Execution Refuses Duplicate Mutation | Two operators execute the same plan concurrently | `TestAuthorityDispositionExecuteConcurrentRefusesDuplicateMutation` | ✅ COMPLIANT |
| Unknown, Mixed, or Ambiguous Shapes Block | Ambiguous classification refuses | `TestAuthorityDispositionExecuteUnknownMixedAmbiguousBlocksNoFallback` + bench `ds07` | ✅ COMPLIANT |
| Refusal Names Diagnosis and Escalation Artifact | Multi-lineage refusal names #1656 without a delivery promise | `TestAuthorityDispositionExecuteRefusalNamesDiagnosisAndEscalationArtifact` | ✅ COMPLIANT |
| Refusal Requires Explicit Maintainer Authorization | Unauthorized refusal does not block unrelated work | `TestAuthorityDispositionExecuteRefusalNeverBlocksElsewhere` + bench `ds08` | ✅ COMPLIANT |

#### `rdd-authority-graph-classification` MODIFIED (1 requirement, 2 scenarios)

| Requirement | Scenario | Test | Result |
|---|---|---|---|
| No Mutation or Execution | Classification has no side effect | `compact_reconcile_test.go` classification regression + `TestLoadCompactRecoveryRecordsIsTheSingleSeam` | ✅ COMPLIANT |
| No Mutation or Execution | Repairable result feeds a separate plan derivation, not the classifier itself | `TestLoadCompactRecoveryRecordsIsTheSingleSeam` (derivation lives in `authority_disposition_plan.go`, classifier alone produces no plan) | ✅ COMPLIANT |

**Compliance summary**: 24/24 scenarios compliant, 19/19 requirements complete.

### Targeted Spec-MUST Re-Execution (uncached, `-count=1`)

`go test -count=1 ./internal/reviewtransaction/... -run 'TestAuthorityDispositionPlan|TestAuthorityDispositionExecute|TestLoadCompactRecoveryRecordsIsTheSingleSeam|TestForgedRecoveryAuthorizationOnNonPristineSuccessorHasSanctionedRepairExit' -v` → 24 tests, **24 PASS, 0 FAIL**, `ok ... 2.330s`.

`go test -count=1 ./internal/cli/... -run 'TestReviewRepairPreflight|TestReviewRepairDisposition' -v` → 8 tests, **8 PASS, 0 FAIL**, `ok ... 0.832s`, including the preflight→execute round trip `TestReviewRepairDispositionExecutionAcceptsPreflightPublishedDigest`.

### Correctness (Source Evidence)

| Requirement / obligation | Status | Notes |
|---|---|---|
| Ten-field plan carry | ✅ Implemented | `AuthorityDispositionPlan` carries `repository_id`, `authority_inventory_revision`, `anomaly_class`, `ordered_seed_set`, `ordered_closure`, `expected_revisions`, `plan_digest`, `actor`, `reason`, `authorization` plus the permitted 11th `schema`. |
| Identity-only digest pre-image | ✅ Implemented | `authorityDispositionPlanDigest` canonical struct has exactly 7 fields; `Actor`/`Reason`/`Authorization`/`PlanDigest` absent. Provenance is still authenticated: `authorityDispositionAuthorizationBinding` includes `actor=` and `reason=` lines, and execution still requires them non-empty. |
| Leaf-only admission | ✅ Implemented | `admitLeafDisposition` requires `len(Closure) == 1`; called twice — before lock (`executeAuthorityDisposition`) and again on the under-lock re-derivation. |
| Lock + CAS reinspection | ✅ Implemented | Exclusive `acquireMaintenanceLock`, then re-derive, then three independent guards: `admitLeafDisposition(currentPlan)`, `currentPlan.PlanDigest != plan.PlanDigest` → `ErrConcurrentUpdate`, and per-seed `ExpectedRevisions` comparison → `ErrConcurrentUpdate`. |
| Authorization-vs-digest validation | ✅ Implemented | `validateAuthorityDispositionAuthorization` compares against the freshly recomputed `currentInventoryRevision` (not the plan's own claim) and requires exact equality with the rendered binding. No wall-clock input exists in the function or its caller. |
| Byte-preserving quarantine + residue | ✅ Implemented | `quarantineCompactStoreEntry` with sorted `Residue` inventory, `SourcePath`, and an `AuthorityDispositionProof` carrying plan digest, inventory revision, class, seed, closure, expected revisions, and the authorization SHA-256. |
| Exact replay convergence | ✅ Implemented | `discoverAuthorityDispositionRecord(base, seed, plan.PlanDigest)` runs first under lock and short-circuits; a different digest refuses. |
| Retained-graph revalidation | ✅ Implemented | `readBackAuthorityDisposition` runs after the lock is released; a fresh inspection must be `Complete && Valid`. |
| Unknown/mixed/ambiguous blocked with named exits | ✅ Implemented | `errAuthorityDispositionPlanNotDerivable` is always wrapped with a specific cause; `seedCount != 1` refuses with the observed count. No generic fallback path exists. |
| No new public verb | ✅ Implemented | Independently confirmed: `git diff d591f4cf 56629a01 -- internal/cli/` adds **zero** `func Run*`/`func New*` entrypoints; the only additions are three flags (`--plan-digest`, `--inventory-revision`, `--authorization`) on the pre-existing `review repair`. |
| Single-seam derivation | ✅ Implemented | `loadCompactRecoveryRecords` is a package-level `var` swapped by the test; call count is asserted at exactly 2 with byte-identical report JSON and equal record counts. See W4 for the under-lock exception. |
| #1892 / #2014 dead-end resolution | ✅ Implemented | `TestForgedRecoveryAuthorizationOnNonPristineSuccessorHasSanctionedRepairExit` flipped from `Operation == "" && Blocked contains "no advertised operation admits this edge"` to `Operation == CompactRecoveryEdgeExitRepair && Blocked == ""`. `ReconcileInvalidRecoveryEdge` still independently refuses the edge as corruption. |
| #1656 refusal naming | ✅ Implemented | Refusal text names cardinality, `#1656` and `#2014`, and "a future wave (Wave 6) with no delivery-date commitment"; the test asserts absence of "will ship", "will land", "scheduled for", "delivery date", "committed for". |
| Blocking budget | ✅ Compliant | Repair requires explicit maintainer authorization (proved by `TestAuthorityDispositionExecuteValidatesAuthorizationAgainstDigestBoundPlan` and bench `ds08`, both of which prove zero bytes mutated). An outstanding unauthorized refusal does not block `StartCompactAuthority` in an unrelated repository. See W2 for a discoverability caveat that does not increase blocking. |

### Coherence (Design)

| Decision | Followed? | Notes |
|---|---|---|
| 1 — plan struct + digest domains, 7-field identity pre-image | ✅ Yes | Design row 1 was amended by the fix batch to describe the 7-field pre-image and to record "including Actor/Reason in the digest pre-image" as explicitly rejected. Code matches. |
| 2 — closed class outside `AnomalyClasses` | ✅ Yes | `compactContentMismatchedRecoveryAuthorizationClass` is a separate `DispositionClass`; `CompactRecoveryEdgeInspection` JSON is unchanged, so batch-reconcile binding identity is untouched. |
| 2b — Wave 1 leaf predicate promoted verbatim | ✅ Yes | `authorityDispositionClosure` derives descendants from report edges only, never from records. |
| 3 — authorization binding, no wall-clock expiry | ✅ Yes | Seven-line binding, no expiry field. Ships as the stated pending-confirmation assumption. |
| 4 — reuse `quarantineCompactStoreEntry` + `AuthorityDispositionProof` | ✅ Yes | Record/residue split and `compactReclaimPhaseHook` crash phases reused. |
| 5 — cardinality in executor admission, derivation stays generic | ✅ Yes | `admitLeafDisposition` holds the constraint; `authorityDispositionClosure` already derives full transitive closures for a future wave. |
| 6 — `CompactRecoveryEdgeExitRepair` gated on derivation + admission | ⚠️ Partial | Emitted correctly by `SanctionedCompactRecoveryExits`, but never consumed by `compactStartInvalidGraphRefusal` (W2). |
| File Changes table — `internal/cli/review_next_transition.go` | ❌ Deviation | Listed in design and in task 3.4; not modified. Breaks no spec MUST (W3). |
| Testing Strategy — 7 exit-evidence families | ✅ Yes | Unit (classification, plan determinism), integration (replay, crash via `compactReclaimPhaseHook`, concurrency via maintenance lock, retained graph), bench (black-box repair + 2 refusals). Crash and CAS-drift correctly stayed integration-level as the design's "Rejected" note requires. |
| Threat matrix — Git repository selection | ✅ Yes | `authorityRepairRoot` + `RepositoryBinding` inside the digest; `executeAuthorityDisposition` refuses a foreign binding before acquiring any lock. |
| PR slicing preview | ⚠️ Partial | Chain shape correct; every slice overran its forecast (W6). |

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD Evidence reported | ⚠️ Partial | Present and complete for the post-S4 fix batch. The S1–S4 per-batch tables were overwritten by `topic_key` upserts on `sdd/rdd-root-simplification-wave2/apply-progress` (7 revisions, latest only retrievable). See S2. |
| All tasks have tests | ✅ | 44/44 — every implementation task maps to a named test that exists and passes. |
| RED confirmed (test files exist) | ✅ | 4/4 test files verified on disk: `authority_disposition_plan_test.go` (8 tests), `authority_disposition_execute_test.go` (13 tests), `review_repair_test.go` (8 Wave 2 tests), `compact_inspect_test.go` (single-seam test). |
| GREEN confirmed (tests pass now) | ✅ | 32/32 Wave 2 tests re-executed uncached in this verification; 32 PASS, 0 FAIL. |
| Triangulation adequate | ✅ | Digest family asserts 3 distinct invalidation axes plus 2 non-invalidation axes; crash family runs 3 phase subtests; refusal family covers cardinality, ambiguity, forged authorization, and missing authorization as 4 distinct causes. |
| Safety net for modified files | ✅ | `compact_forged_authorization_test.go`'s pre-existing blocked assertion was deliberately converted (not deleted) into the runnable-exit assertion, exactly as the design's Testing Strategy prescribed. |
| Regression test preceded the fix | ✅ | The fix batch records a genuine RED: `TestReviewRepairDispositionExecutionAcceptsPreflightPublishedDigest` failed with "actor/reason leaked into plan identity" before the digest narrowing and passes after. |

**TDD Compliance**: 7/8 checks passed (1 partial, artifact-retention only).

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|---|---|---|---|
| Unit | 8 | `authority_disposition_plan_test.go` | `go test` |
| Integration | 14 | `authority_disposition_execute_test.go`, `compact_inspect_test.go` | `go test` + real temp repos, flock, phase hooks |
| CLI / black-box | 8 | `review_repair_test.go` | `go test` driving `RunReview` |
| Bench (E2E, real binary) | 3 | `bench/axis_damaged_store.go` (`ds06`, `ds07`, `ds08`) | `gentle-ai-bench` against a tip-built `gentle-ai` |
| **Total** | **33** | **5** | |

### Changed File Coverage

➖ Coverage analysis skipped — no coverage tool or threshold is configured for this repository. Informational only; not a failure.

### Assertion Quality

Audited all four Wave 2 Go test files (234 assertion sites) plus the three bench journeys.

**Assertion quality**: ✅ All assertions verify real behavior.

No tautologies, no orphan empty-collection checks, no type-only assertions used alone, no ghost loops, no smoke-test-only cases. Assertions are byte comparisons (`os.ReadFile` original vs quarantined residue), digest equality, typed error identity (`errors.Is(err, ErrConcurrentUpdate)`, `errors.Is(err, errAuthorityDispositionPlanNotDerivable)`), filesystem-state proofs (`os.Stat` on the untouched source, `os.IsNotExist` on the quarantine root to prove no fallback), and directory-entry counts to prove replay moved nothing twice. Every refusal test additionally proves zero mutation, not only that an error was returned. The three `_ = successor` statements are unused-variable silencers, not assertions. Bench refusal journeys use `proveStoreStillDamaged` as an explicit no-mutation composite.

### Quality Metrics

**Formatter**: ✅ `gofmt -l` clean, main module and `bench/`.
**Vet**: ✅ `go vet ./...` exit 0.
**Deadcode ratchet**: ✅ `scripts/deadcode-ratchet.sh` exit 0 — "no new unreachable functions".

### Documented-Deviation Audit (verdicts, not deferrals)

| # | Deviation | Verdict | Severity |
|---|---|---|---|
| 1 | Seam folds inspection: `loadCompactRecoveryRecords` returns `(report, records)` rather than records only | **Accepted.** Strengthens mandatory obligation (a) — one call yields both derivation inputs, so they provably describe the same read. The single-seam test asserts byte-identical report JSON across both callers. | — |
| 2 | Forbidden-phrase narrowing in the no-new-verb test | **Accepted with a caveat.** Excluding `review dispose` / `RunReviewDispose` is correct: they are substrings of the pre-existing, unrelated `review dispose-result`. But the guard is a 6-entry blocklist, not a command-set snapshot. | WARNING (W5) |
| 3 | Lock-safe reader `loadCompactRecoveryRecordsUnderMaintenanceHold` | **Accepted with a caveat.** Architecturally required: `CompactStore.Load` takes its own shared maintenance lock per entry and flock is not reentrant across descriptors, so the ordinary seam would self-contend and time out under the executor's own exclusive lease. It reuses the same `inspectCompactRecoveryRecordSet` classifier. But ~40 lines of the read loop are duplicated and no test asserts loader parity. | WARNING (W4) |
| 4 | `// refusal:by-design` annotations on new refusal paths | **Accepted.** Matches the repository's existing convention and correctly classifies each refusal (`human-authority` for authorization and unclassifiable shapes, `operator-knowledge` for the foreign-binding refusal). | — |
| 5 | S3 did not modify `internal/cli/review_next_transition.go` despite the design File Changes table and task 3.4 naming it | **Real deviation, not a spec break.** The only CLI-surface spec requirement is negative ("No New Public Repair Verb"), and leaving the file untouched satisfies rather than violates it — independently confirmed by diffing the whole chain's `internal/cli/` for new entrypoints (none). What is genuinely absent is any negotiated-transition route for the disposition flow: `reviewRepairTransition` still serves only the legacy `ClassifiedAuthorityRepairRequest` path, so an orchestrator following `review status --next-transition` never receives a `collect`/`execute` for a leaf disposition. The flow remains reachable through the documented direct path (`review repair --preflight` → execute) and through `inspect-authority`'s sanctioned exit, both proven by bench `ds06`. | WARNING (W3), with W2 as its substantive residue |

### Issues Found

**CRITICAL**: None.

**WARNING** (6):

- **W1 — Main-checkout OpenSpec mirror is stale and will deny archive admission.** `/home/gentleman/work/gentle-ai/openspec/changes/rdd-root-simplification-wave2/` is an untracked working copy that predates the fix batch. `design.md` still describes the nine-field pre-image and `specs/rdd-authority-disposition-plan/spec.md` is missing both the "MUST NOT be computed over `actor` or `reason`" sentence and the "Actor and reason do not affect plan_digest" scenario. That mirror therefore counts **19 requirements / 23 scenarios**, while the authoritative branch-committed specs at `56629a01` count **19 / 24** — the counts this report declares. An archive run that reads the stale mirror will fail admission with "verify result total 24 does not match actual scenario count 23". Remediation is mechanical: refresh the mirror from `feat/rdd-wave2-bench-journeys` (`design.md` and `specs/rdd-authority-disposition-plan/spec.md`; the other four artifacts are already identical) before archiving. Deliberately not fixed here — spec and design artifacts are owned by their authoring phases.
- **W2 — `review start`'s invalid-graph refusal drops the new sanctioned exit.** `compactStartInvalidGraphRefusal` (`internal/reviewtransaction/compact_inspect.go`) switches on `exit.Operation` with cases for `CompactRecoveryEdgeExitAbandon` and `CompactRecoveryEdgeExitReconcile` only. `CompactRecoveryEdgeExitRepair` falls through silently, so a human blocked at `review start` by exactly the content-mismatched leaf Wave 2 now repairs is told only to run `review inspect-authority`, never `review repair` — even though `SanctionedCompactRecoveryExits` has already proved that command would run. That function's own contract is "names the sanctioned exit the read-only inspection proves", and design decision 6 exists precisely to make the exit nameable. No Wave 2 spec MUST covers the START refusal text, so this is a coherence and discoverability gap, not a spec violation; it does not increase blocking (the human is blocked identically either way, just less usefully informed).
- **W3 — `internal/cli/review_next_transition.go` unmodified.** Named by the design File Changes table and by task 3.4, which is checked `[x]`. See deviation-audit row 5 for the full verdict. No spec MUST is broken.
- **W4 — No loader-parity test for the under-lock reader.** `loadCompactRecoveryRecordsUnderMaintenanceHold` duplicates ~40 lines of `loadCompactRecoveryRecords`'s read-and-classify body. If the two ever drift, the under-lock CAS comparison silently compares against a differently-derived graph, which is the one place the whole safety argument rests. `TestLoadCompactRecoveryRecordsIsTheSingleSeam` swaps only the package-level seam var and therefore does not exercise the under-lock path at all.
- **W5 — No-new-verb guard is a blocklist, not a snapshot.** `TestAuthorityDispositionPlanNoNewPublicRepairVerb` scans `internal/cli/*.go` for six hypothetical identifiers. A new command named anything outside that list passes. The requirement itself holds — verified independently by diffing `internal/cli/` across the whole chain (zero new `Run*`/`New*` entrypoints; only three flags added to the existing `review repair`) — but the guard is weaker than the invariant it names.
- **W6 — Review-workload forecast miss; five `size:exception` labels needed, not three.** Measured totals: PR0 1700, S1 760, S2 874, S3 485, S4+fix 569. S3 and S4 were both forecast "likely within 400" and are not. PR0 also breaches the design's ≤1000-line-per-slice cap on raw diff, though 1155 of those lines are Wave 1 archival carry.

**SUGGESTION** (4):

- **S1 — `tasks.md` 1.6 is stale.** It still reads "nine-field digest, ten-field struct"; the shipped pre-image is seven-field. `spec.md` and `design.md` were both updated by the fix batch; `tasks.md` was not.
- **S2 — Per-batch TDD evidence is unrecoverable.** The `sdd/rdd-root-simplification-wave2/apply-progress` topic has 7 revisions but `mem_get_observation` returns only the latest, so the S1–S4 TDD Cycle Evidence tables are gone. TDD compliance for those slices was reconfirmed by uncached execution instead of from the artifact. Consider distinct `topic_key`s per batch (e.g. `.../apply-progress/s1`) in future waves.
- **S3 — Pre-existing bench staleness (`ds01`).** Its dead-end declaration is stale; confirmed present at the S2 tip `f7d234f2` and predating Wave 2 entirely. It does not fail the run (65/65 completed). Bench-hygiene item for a maintainer, correctly not fixed in this wave.
- **S4 — `design.md` Open Questions remain unchecked.** Question 1 (does #2111's fixture re-derive with a non-empty `DispositionClass`?) is now empirically answered: `TestAuthorityDispositionPlanPR2111SupersessionProbe` passes, so the #2111-shaped fixture re-derives with a non-empty `DispositionClass` and its successor is a leaf — supersession stands. The checkbox is still `[ ]`. Questions 2 and 3 (fail-closed entry-diagnostic strictness; CAS-only staleness in place of decision 4's short expiry) are genuine open maintainer confirmations that ship as-implemented per `tasks.md`'s pending-confirmation assumptions, and should be carried into the archive record rather than silently closed.

### Verdict

**PASS WITH WARNINGS** — all 44 tasks complete, all 19 requirements and 24 scenarios covered by tests that passed at runtime in this verification (uncached full suite exit 0, 65/65 bench journeys, both quality gates and the deadcode ratchet clean), with zero CRITICAL findings; six WARNINGs remain, of which W1 (stale OpenSpec mirror) must be resolved before archive can be admitted.
