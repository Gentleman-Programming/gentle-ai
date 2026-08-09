# Tasks: Scoped Slice Verification (#2268)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | PR1 ~280, PR2 ~380, total ~660 |
| 400-line budget risk | Low per PR (PR2 close to ceiling; j82 growth triggers PR3 contingency) |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 (stacked-to-main); PR3 contingency if j82 grows >20 lines past headroom |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low

### Budget Reconciliation (concern 4)

Proposal estimated ~530 lines (250+280). Design refines to ~660 (280+380) after D10 `compactAcquireMatches` rewrite, full decision-table validator, and j82 journey. Authoritative per-PR estimates: **PR1 ~280**, **PR2 ~380**. Both under 400-line budget. PR2 headroom ~20 lines; if j82 grows beyond that, split commit (3) into PR3 (auto-chain permits).

### Suggested Work Units

| Unit | Goal | PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|----|----------------------|-----------------|-------------------|
| 1 | Ledger obligation ID binding + enforcement | PR1 | `go test ./internal/sddstatus/ -run 'Assignment|Rescope|SliceAssign|Legacy|Explicit'` | N/A (pure ledger logic, no CLI/runtime) | Revert commit (2) → inert fields; revert (1) → pristine |
| 2 | Slice admission validator + CLI + journey | PR2 | `go test ./internal/sddstatus/ ./internal/cli/ -run 'SliceVerify|RunSDDVerifyValidate|ResolveSlice|ResolveApply|j82'` | `go test ./bench/ -run TestDrivenMode/j82` | Revert commit (3) → no journey; revert (2) → exact current CLI |

---

## PR1: `feat(sdd): bind immutable obligation assignments on runtime work units` (~280 lines)

### Phase 1.1 — Foundation: fields + normalize + shape/digest (RED→GREEN)

- [x] **T1.1** RED: `internal/sddstatus/runtime_ledger_assignment_test.go` — `TestNormalizeRejectsNoneSentinelID`: comma-ban extended to reject literal `none` as obligation ID (concern 3). Spec: *Requirement: Immutable Obligation ID Assignment* / *Scenario: Assignment bound at begin*.
- [x] **T1.2** GREEN: `internal/sddstatus/runtime_ledger.go` — add 3-field block (`ObligationAssignmentExplicit`, `AssignedRequirementIDs`, `AssignedScenarioIDs`) to `BeginAttemptRequest`, `RuntimeObjective`, `RescopeObjectiveRequest`, `runtimeBeginEvent`, `runtimeRescopeEvent`. Extend `normalizeBeginAttemptRequest`/`normalizeRescopeObjectiveRequest`: `validateRuntimeText(id, 240)`, no commas, no duplicates, ≤4096, reject `none` sentinel, reject marker-false + non-empty lists.
- [x] **T1.3** GREEN: `internal/sddstatus/runtime_ledger.go` — `validateRuntimeBeginEvent`/`validateRuntimeRecordShape` rescope case: reconstruct request including all 3 fields for digest recompute; shape-validate IDs. Spec: *Requirement: Immutable Obligation ID Assignment* / *Scenario: Assignment bound at begin*.
- [x] **T1.4** RED→GREEN: `runtime_ledger_assignment_test.go` — `TestLegacyLedgerReplaysByteIdentical`: persisted records without new fields replay unchanged. Spec: *Requirement: Whole-Change Backward Compatibility* / *Scenario: Legacy ledger records replay unchanged*.
- [x] **T1.5** RED→GREEN: `runtime_ledger_assignment_test.go` — `TestExplicitEmptyVsAbsentAssignment`: marker-false + absent arrays rejected; marker-true + empty lists admitted (zero-obligation path). Spec: *Requirement: Zero-Obligation Work Units* / *Scenario: Explicit zero-obligation unit admitted* + *Scenario: Absent arrays are not zero-obligation*.

### Phase 1.2 — Enforcement: match checks, rescope rules, SliceAssignments

- [ ] **T1.6** RED: `runtime_ledger_assignment_test.go` — `TestContinuingAttemptAssignmentMatch`: identical assignment accepted, altered refused. Spec: *Requirement: Immutable Obligation ID Assignment* / *Scenario: Continuing attempt presents the identical assignment* + *Scenario: Report ID lists mismatched or assignment altered* (begin half).
- [ ] **T1.7** GREEN: `internal/sddstatus/runtime_ledger.go` — extend continuing-attempt match checks (runtime_ledger.go:764-766 and 789-791) with assignment equality → `runtimeObjectiveChangeRefusal` on mismatch.
- [ ] **T1.8** RED→GREEN: `runtime_ledger_assignment_test.go` — `TestRescopeCarryReassignWidenRefusal`: rescope carry-forward (equal), narrowing reassign (subset), widen/altered/bound→absent/unbound→present all refused. Spec: *Requirement: Immutable Obligation ID Assignment* / *Scenario: Rescope carries forward or reassigns*.
- [ ] **T1.9** GREEN: `internal/sddstatus/runtime_ledger.go` — `Rescope()` adds D4 subset checks against replayed objective next to existing widening checks.
- [ ] **T1.10** RED→GREEN: `runtime_ledger_assignment_test.go` — `TestSliceAssignmentsProjectionExcludesUnbound`: `SliceAssignments()` excludes objectives with `ObligationAssignmentExplicit=false` (concern 2). Spec: *Requirement: Provider-Owned Slice Identity* / *Scenario: Slice identity resolves solely from the provider-owned work unit*.
- [ ] **T1.11** GREEN: `internal/sddstatus/runtime_ledger.go` — `SliceAssignments()` projection with unbound-exclusion; supersede rule (rescope ancestors + reset predecessors excluded; advance predecessors retained).
- [ ] **T1.12** GREEN: `internal/sddstatus/runtime_compact.go` — `compactAcquireMatches` (line 356/361) rewritten field-wise with `slices.Equal` (D10 — `[]string` doesn't compile under `==`).
- [ ] **T1.13** RED→GREEN: `runtime_ledger_assignment_test.go` — `TestIdempotentAcquireReplayWithAssignment`: acquire replay with assignment fields idempotent.
- [ ] **T1.14** Extend: `internal/sddstatus/runtime_objective_owner_test.go` — `TestRuntimeObjectiveIsSoleWorkUnitScopeOwner` reflection guard covers new fields only on `BeginAttemptRequest`; compact inherits via embed. Spec: *Requirement: Provider-Owned Slice Identity*.
- [ ] **T1.15** Extend: `internal/sddstatus/runtime_objective_advance_test.go` — `TestRuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective` variant binds disjoint assignments on apply/verify objectives + replay idempotence.

### Phase 1.3 — Verification

- [ ] **T1.16** `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./internal/sddstatus/...` all green.

**PR1 commits** (work-unit boundaries):
1. `feat(sdd): add obligation assignment fields with normalize and digest validation` — T1.1–T1.5 (fields + normalize + shape/digest + legacy-replay + explicit-empty tests).
2. `feat(sdd): enforce immutable assignment through match checks, rescope rules, and SliceAssignments` — T1.6–T1.15 (enforcement + projection + compact fix + owner/advance extensions).

---

## PR2: `feat(sdd): admit slice-scoped verify reports under dual authority` (~380 lines)

### Phase 2.1 — Validator + envelope (RED→GREEN)

- [ ] **T2.1** RED: `internal/sddstatus/verification_test.go` — `TestValidateSliceVerifyReportAdmission`: full decision table (checks 1–17) incl. zero-obligation, overlap, altered, authority-only exit 125. Spec: *Requirement: Slice-Scoped Verify Report Admission* + *Requirement: Fail-Closed Rejection of Invalid Slice Claims* + *Requirement: Incomplete FAIL Slice Admission Preserved*.
- [ ] **T2.2** GREEN: `internal/sddstatus/verification.go` — add `verifyReportSliceFields` to `parseVerifyReport` allowed map; `VerifyReportContract` gains `SliceFields`; `parseSliceIDList` helper; `SliceAssignment` struct; `ValidateSliceVerifyReportAdmission` with full decision table.
- [ ] **T2.3** RED→GREEN: `verification_test.go` — whole-path byte-identity pin (concern 1): `TestWholePathAdmitsSliceFieldsInReport`: report containing slice metadata validated under whole scope falls through to totals gate (intentional deviation per D7; archive gating preserved). Spec: *Requirement: Whole-Change Backward Compatibility* / *Scenario: Default whole-change behavior byte-identical*.

### Phase 2.2 — CLI wiring

- [ ] **T2.4** RED: `internal/cli/sdd_verify_validate_test.go` — `TestRunSDDVerifyValidateSliceScope`: flag-matrix errors (invalid scope, `whole`+slice-only → error per D8, `slice`+empty `--slice-id` → error, `slice`+missing `--cwd/--change` → error); slice happy path + unknown identity + mismatched totals (git fixture). Spec: *Requirement: CLI Flag Authority* / all 3 scenarios.
- [ ] **T2.5** GREEN: `internal/cli/sdd_verify_validate.go` — flags `--scope` (default `whole`), `--slice-id`, `--cwd`, `--change`; matrix validation; slice path: `OpenRuntimeStore` → `SliceAssignments()` → lookup → validator; whole path: byte-identical dispatch.
- [ ] **T2.6** RED→GREEN: `internal/cli/sdd_attempt_assignment_test.go` — `TestSDDAttemptAssignmentFlags`: `--assigned-requirement-ids`/`--assigned-scenario-ids` on begin/acquire/rescope; exactly-one-present → error; both → `ObligationAssignmentExplicit=true`. Spec: *Requirement: CLI Flag Authority* / *Scenario: Assignment flags bind obligation IDs*.
- [ ] **T2.7** GREEN: `internal/cli/sdd_attempt.go` — assignment flags on begin/acquire/rescope; added to `sddAttemptOperationDefinitions`.

### Phase 2.3 — Archive isolation + journey

- [ ] **T2.8** RED→GREEN: `internal/sddstatus/bounded_review_test.go` — `TestResolveSlicePassDoesNotUnlockArchive`: passing slice report as verify-report.md → Verify≠AllDone, Archive blocked. Spec: *Requirement: Slice PASS Never Implies Whole-Change Completion* / *Scenario: Slice PASS does not unlock archive*.
- [ ] **T2.9** Extend: `internal/sddstatus/status_test.go` — `TestResolveApplyVerifyArchiveGates` case: slice-scoped passing report → archive blocked, verify ready. Spec: *Requirement: Slice PASS Never Implies Whole-Change Completion* / *Scenario: Archive opens only after final whole-change verification*.
- [ ] **T2.10** Extend: `internal/sddstatus/bounded_review_test.go` — `TestResolveFinalVerifyWaitsForAllTasks` slice variant. Spec: *Requirement: Chained Slice Lifecycle* / both scenarios.
- [ ] **T2.11** Create: `bench/journeys_sdd_slice_verify.go` — journey j82: fixture 2-req/4-scen spec; begin slice A with assignment flags → pass → `sdd-verify-validate --scope slice` admitted → proveRuntime evidence + `sdd-status` archive blocked while B pending → fail-closed denial (wrong `--slice-id`) → slice B admit → whole-change report (2/2, 4/4) → archive ready. Follows `journeys_sdd_chain.go` composite pattern. Spec: *Requirement: Chained Slice Lifecycle*.
- [ ] **T2.12** Modify: `bench/journeys.go`, `bench/journeys_id_collision_test.go`, `bench/journeys_sdd_test.go` — register j82; count pin 82→83.

### Phase 2.4 — Verification

- [ ] **T2.13** `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./internal/sddstatus/... ./internal/cli/...`, `go test ./bench/ -run TestDrivenMode/j82` all green.

**PR2 commits** (work-unit boundaries):
1. `feat(sdd): add ValidateSliceVerifyReportAdmission with dual-authority decision table` — T2.1–T2.3 (validator + envelope + whole-path pin; uncalled, green).
2. `feat(cli): wire --scope/--slice-id and assignment flags for slice verification` — T2.4–T2.7 (CLI wiring + tests).
3. `test(sdd): archive-isolation regressions and chained journey j82` — T2.8–T2.12 (archive tests + journey + registration). **Contingency**: if j82 grows >20 lines past PR2 headroom, split this commit into PR3.

---

## Fresh-Review Concern Resolutions

1. **Whole-path byte-identity** → T2.3: document as intentional deviation (D7 fields allowed but inert); test pins behavior; archive gating preserved via unchanged totals gate.
2. **SliceAssignments projection** → T1.10: explicit test that projection excludes `ObligationAssignmentExplicit=false` objectives.
3. **Sentinel collision** → T1.1: normalize rejects `none` as ID at bind time (fail-closed).
4. **Budget reconciliation** → stated above: PR1 ~280, PR2 ~380, both under 400; PR3 contingency if j82 grows.
