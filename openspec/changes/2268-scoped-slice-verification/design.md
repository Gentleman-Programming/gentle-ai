# Design: Scoped Slice Verification (#2268)

## Technical Approach

Approach 1 from explore.md, traced to code at `3c6a6341`: obligation ID assignment fields on the provider-owned `BeginAttemptRequest`/`RuntimeObjective` (Requirement: Provider-Owned Slice Identity), digest-bound through begin/rescope events and replay validators (Requirement: Immutable Obligation ID Assignment), a new pure `ValidateSliceVerifyReportAdmission` dual-authority validator (Requirement: Slice-Scoped Verify Report Admission, Requirement: Fail-Closed Rejection of Invalid Slice Claims), unchanged whole-change admission and dependency resolution (Requirement: Whole-Change Backward Compatibility, Requirement: Slice PASS Never Implies Whole-Change Completion), and a chain-projected `SliceAssignments` read for overlap input.

## Architecture Decisions

| # | Decision | Options → tradeoffs | Choice |
|---|----------|---------------------|--------|
| D1 | Explicit-empty vs absent arrays (`omitempty` serializes `nil` and `[]` identically) | (a) presence-marker bool — 1 field, matches `ChangedLineBudgetExceeded` precedent; (b) `*[]string` — distinguishes but awkward derefs, violates proposal's `[]string` shape; (c) non-omitempty — `null`/`[]` tri-state, violates approved `omitempty` | Marker bool `ObligationAssignmentExplicit` (`obligation_assignment_explicit,omitempty`). false/absent = no assignment bound → never zero-obligation (Scenario: Absent arrays are not zero-obligation); true + empty lists = explicit 0/0 (Scenario: Explicit zero-obligation unit admitted) |
| D2 | Slice identity | WorkUnit label (not unique across generations); free-form registry (maintainer-banned) | `slice_id` = `RuntimeObjective.ID` — the content-addressed work-unit identity (Requirement: Provider-Owned Slice Identity) |
| D3 | Envelope ID-list encoding | JSON array line (breaks `parseScalarFields` scalar contract); absent-when-zero (ambiguous with partial metadata) | Comma-separated scalar; sentinel `none` = empty list; strict parse (trim elements, reject empty elements/duplicates). Assignment IDs must not contain commas (enforced at normalize) so encoding is unambiguous |
| D4 | Rescope carry-forward | Implicit fill inside `mutate` (breaks digest: digest is computed pre-mutate; `validateRuntimeRecordShape` recomputes it from the event-reconstructed request) | Request always states assignment explicitly; CLI reads objective and re-passes it. Ledger validates: request ⊆ previous (equal = carry-forward, proper subset = narrowing reassignment); widen/altered → refuse; bound previous + absent flags → refuse (no silent un-bind); unbound previous + present flags → refuse (Scenario: Rescope carries forward or reassigns) |
| D5 | Validator signature | `expected SpecCounts` derived internally vs passed | Passed: CLI `--requirements/--scenarios` cross-checked inside against assignment lengths (Scenario: CLI authority disagrees with the bound assignment). No `scope` param — dispatch is scope; report `scope` field is checked |
| D6 | knownAssignments aggregation | Status projection (only live objective); new CLI subcommand | `RuntimeStore.SliceAssignments()` — projected during replay (`runtimeReplay.Assignments`), chain-validated. Supersede rule: rescope ancestors and reset predecessors excluded (no credit granted / same lineage); advance predecessors retained (earned credit is overlap-protected) |
| D7 | Whole path with new envelope fields | Reject slice fields in whole path (modifies `ValidateVerifyReportAdmission` — forbidden) | Fields allowed but inert on whole path; whole totals gate still rejects slice reports. `ValidateVerifyReportAdmission` byte-identical (Scenario: Default whole-change behavior byte-identical) |
| D8 | `--scope whole` + slice-only flags | Ignore silently | Reject with error — house convention per `sdd-attempt finish` remediation all-or-none refusal (Scenario: Whole scope is unaffected by slice-only flags) |
| D9 | Objective ID derivation | Include assignment in `runtimeObjectiveID` (signature change, legacy/V1 fallback churn) | Unchanged: immutability enforced by match checks + replay validators, not ID derivation |
| D10 | `compactAcquireMatches` (runtime_compact.go:361) | Keep `==` | Impossible: Go `==` does not compile on structs with `[]string` fields. Rewrite field-wise + `slices.Equal`. Corrects proposal's "runtime_compact.go unchanged" — the struct is unchanged via embed, this function is not |
| D11 | `RuntimeRescope` audit projection | Extend with assignment fields | Skip: live assignment is visible on `status.Objective`; the immutable chain is the source of truth; saves line budget |

## Data Model

Identical 3-field block appended to `BeginAttemptRequest`, `RuntimeObjective`, `RescopeObjectiveRequest`, `runtimeBeginEvent`, `runtimeRescopeEvent`:

```go
ObligationAssignmentExplicit bool     `json:"obligation_assignment_explicit,omitempty"`
AssignedRequirementIDs       []string `json:"assigned_requirement_ids,omitempty"`
AssignedScenarioIDs          []string `json:"assigned_scenario_ids,omitempty"`
```

Flow: CLI flags → request → `normalizeBeginAttemptRequest`/`normalizeRescopeObjectiveRequest` (each ID: `validateRuntimeText(id, 240)`, no commas, no duplicates, ≤ `maximumRuntimeAssignmentIDs` = 4096; marker false + non-empty lists → reject) → digest (`runtimeValueHash` over JSON: absent fields hash identically to legacy, so legacy records replay unchanged — Scenario: Legacy ledger records replay unchanged) → event → `validateRuntimeBeginEvent`/rescope case of `validateRuntimeRecordShape` (shape-validate IDs; include all 3 fields in the request reconstruction for digest recompute) → `applyRuntimeBeginEvent`/`applyRuntimeRescopeEvent` (carry fields into the reconstructed `RuntimeObjective`; continuing-objective branch adds assignment equality to its match check; rescope adds D4 subset check against the REPLAYED objective).

Write-time `Begin` extending both continuing-attempt match checks (runtime_ledger.go:764-766 and 789-791) with assignment equality → altered assignment hits `runtimeObjectiveChangeRefusal` (Scenario: Report ID lists mismatched or assignment altered, begin-validation half). `Rescope()` adds D4 checks next to the existing widening checks. `CompactAcquireRequest` inherits via embed; only `compactAcquireMatches` changes (D10). Derived totals = `len()` — no stored totals field (drift-free). Zero-obligation evidence goal and passing evidence are already enforced (`validateRuntimeText` rejects empty `EvidenceGoal`; Finish requires sha256 `EvidenceRevision`).

## Validator

```go
type SliceAssignment struct {
    SliceID        string
    RequirementIDs []string
    ScenarioIDs    []string
}

func ValidateSliceVerifyReportAdmission(
    text string,
    expected SpecCounts,          // CLI --requirements/--scenarios
    sliceID string,               // CLI --slice-id
    assigned SliceAssignment,     // bound assignment looked up by CLI (zero value if unknown)
    knownAssignments []SliceAssignment,
) VerifyReportAdmission
```

Envelope: new `verifyReportSliceFields = {"scope", "slice_id", "requirement_ids", "scenario_ids"}` added to `parseVerifyReport`'s allowed map (optional; not added to `verifyReportRequiredFields`); `VerifyReportContract` gains `SliceFields` for help. Decision table (first match wins; all rejections fail-closed):

| # | Check | Reason string | Spec class |
|---|-------|---------------|------------|
| 1 | `parseVerifyReport` failure | its reason | partial metadata |
| 2 | `scope` absent | `missing scope in verify result envelope` | missing scope |
| 3 | `scope != slice` | `slice admission requires scope: slice` | mismatched scope |
| 4 | `slice_id` absent | `missing slice_id in verify result envelope` | partial metadata |
| 5 | `slice_id != sliceID` | `slice_id does not match the slice authority` | mismatched slice_id |
| 6 | `assigned.SliceID != sliceID` | `slice identity is unknown to the bound assignment` | unknown identity |
| 7 | either ID list absent | `missing requirement_ids/scenario_ids in verify result envelope` | partial metadata |
| 8 | list parse fails (bad element/dup) | `invalid requirement_ids in verify result envelope` | partial metadata |
| 9 | list ≠ assignment (set equality) | `requirement_ids do not match the bound assignment` | mismatched ID lists |
| 10 | duplicate SliceID in known | `duplicate slice identity in known assignments` | duplicate coverage |
| 11 | known entry for sliceID ≠ assigned | `bound assignment was altered after binding` | altered assignment |
| 12 | non-empty intersection with other entry | `assignment overlaps work unit <id>` | overlap |
| 13 | `expected` ≠ assignment lengths | `CLI totals do not match the bound assignment` | CLI authority mismatch |
| 14 | report totals ≠ expected | existing `verify result total %d does not match actual ... count %d` | totals mismatch |
| 15 | pass/pass_with_warnings ∧ (nonzero exits/blockers/critical ∨ incomplete) | `passing verdict contradicts failing or incomplete evidence` | completion rule |
| 16-17 | fail: exit-125-without-extension; all-green contradiction | existing strings verbatim | Requirement: Incomplete FAIL Slice Admission Preserved (incl. authority-only exit 125) |

Zero-obligation needs no special branch: lengths 0 → expected 0/0 → report `0/0`, lists `none`; "no credit toward global obligations" holds because the status layer only ever compares whole-change `SpecCounts` (Scenario: No credit toward global obligations).

## CLI Plumbing

`sdd-verify-validate`: new flags `--scope` (default `whole`), `--slice-id`, `--cwd`, `--change` (last three slice-only). Matrix: invalid scope → error; `whole` + any slice-only flag → error (D8); `slice` + empty `--slice-id` → error (Scenario: Slice scope requires a non-empty slice id); `slice` + missing `--cwd/--change` → error. Slice path: `OpenRuntimeStore` → `SliceAssignments()` → lookup `assigned` → validator → same JSON output. Whole path: byte-identical dispatch to `ValidateVerifyReportAdmission`.

`sdd-attempt begin/acquire/rescope`: new optional pair `--assigned-requirement-ids`/`--assigned-scenario-ids` (comma-separated; empty value = explicit empty list). Exactly one present → error `assignment flags require --assigned-requirement-ids and --assigned-scenario-ids together`; both present → `ObligationAssignmentExplicit = true` on the request (Scenario: Assignment flags bind obligation IDs). Added to `sddAttemptOperationDefinitions` (single source for help/registration/validation).

## Data Flow

```
sdd-attempt begin/acquire/rescope --assigned-*-ids
   │ normalize (shape/bounds/no-comma/no-dup)
   ▼ digest-bound request
runtimeBeginEvent / runtimeRescopeEvent ──► immutable chain
   │ replay: digest recompute + match/subset guards
   ▼                                        │
RuntimeObjective (sole scope owner)   runtimeReplay.Assignments
   │                                        │ SliceAssignments()
   ▼                                        ▼
sdd-verify-validate --scope slice ──► ValidateSliceVerifyReportAdmission
   PASS ► work-unit evidence only ──► resolveDependencies UNCHANGED
                                      (whole-change SpecCounts gate archive)
```

## Archive Isolation Proof

No change to `resolveDependencies` (status.go:1837). `readVerifyResult` (review_gate.go:167) evaluates the verify-report artifact via `parseVerifyResult` against `readSpecCounts(artifactPaths.Specs)` (review_gate.go:155) — whole-change heading counts (status.go:474). A slice PASS (2/2, 3/3 vs whole 10/25) → totals mismatch → `Passing=false`, `Stale=true` → `verifyReportCurrent=false` (status.go:570) → `Verify` never reaches `DependencyAllDone` (status.go:1853) → `Archive` stays blocked (status.go:1858). Slice evidence persisted through `Finish`/`Settle` `EvidenceRevision` is never an input to `resolveDependencies`. Regression pins: new cases in `TestResolveApplyVerifyArchiveGates`, new `TestResolveSlicePassDoesNotUnlockArchive` beside `TestResolveFinalVerifyWaitsForAllTasks` (Scenario: Slice PASS does not unlock archive, Scenario: Archive opens only after final whole-change verification).

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/sddstatus/runtime_ledger.go` | Modify | Fields ×5 structs, normalize ×2, Begin match checks, Rescope rules, apply ×2, validate ×2, `SliceAssignments` |
| `internal/sddstatus/runtime_compact.go` | Modify | `compactAcquireMatches` field-wise compare (D10) |
| `internal/sddstatus/verification.go` | Modify | `SliceAssignment`, slice envelope fields, `parseSliceIDList`, `ValidateSliceVerifyReportAdmission` |
| `internal/cli/sdd_verify_validate.go` | Modify | Flags, matrix, dispatch, help |
| `internal/cli/sdd_attempt.go` | Modify | Assignment flags on begin/acquire/rescope |
| `internal/sddstatus/runtime_ledger_assignment_test.go` | Create | PR1 ledger tests |
| `internal/cli/sdd_attempt_assignment_test.go` | Create | CLI flag tests |
| `internal/sddstatus/verification_test.go`, `internal/cli/sdd_verify_validate_test.go`, `internal/sddstatus/status_test.go`, `internal/sddstatus/bounded_review_test.go`, `internal/sddstatus/runtime_objective_advance_test.go` | Modify | New cases per test strategy |
| `bench/journeys_sdd_slice_verify.go` | Create | Journey j82 |
| `bench/journeys.go`, `bench/journeys_id_collision_test.go`, `bench/journeys_sdd_test.go` | Modify | Register j82; count pin 82→83 |

## Testing Strategy (maintainer-named acceptance tests)

| Acceptance test | Today | Stays green because | Added |
|---|---|---|---|
| `ValidateVerifyReportAdmission` (verification_test.go:47) | 25-case table | Function untouched; `extra` still unknown | `TestValidateSliceVerifyReportAdmission`: full decision-table incl. zero-obligation, overlap, altered, authority-only 125 |
| `RunSDDVerifyValidate` (sdd_verify_validate_test.go:15) | Whole-path cases | Default `--scope whole` | Flag-matrix errors (no repo); slice happy path + unknown identity + mismatched totals (git fixture per `sdd_attempt_compact_test.go` pattern) |
| `RuntimeObjectiveIsSoleWorkUnitScopeOwner` (runtime_objective_owner_test.go:27) | Reflection guard | New fields only on `BeginAttemptRequest`; compact inherits via embed | None needed |
| `RuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective` (runtime_objective_advance_test.go:71) | Advance fixture | Additive fields | Variant binding disjoint assignments on apply/verify objectives + replay idempotence |
| `ResolveFinalVerifyWaitsForAllTasks` (bounded_review_test.go:569) | Tasks-pending pin | Unchanged | New `TestResolveSlicePassDoesNotUnlockArchive`: passing slice report as verify-report.md → Verify≠AllDone, Archive blocked |
| `ResolveApplyVerifyArchiveGates` (status_test.go:456) | Gate table | Unchanged | Case: slice-scoped passing report → archive blocked, verify ready |
| Chained journey | — | — | j82 in `bench/journeys_sdd_slice_verify.go`: fixture 2-req/4-scen spec; begin slice A with assignment flags → pass → `sdd-verify-validate --scope slice` admitted → proveRuntime evidence persisted + `sdd-status` archive blocked while B pending → fail-closed denial step (wrong `--slice-id`) → slice B admit → whole-change report (2/2, 4/4) → archive ready. Follows `journeys_sdd_chain.go` composite pattern; driven-mode proof per gentle-ai-bench |

PR1 unit tests additionally pin: assignment bound at begin with derived totals (Scenario: Assignment bound at begin), identical continuing assignment accepted, altered refused, legacy replay byte-identical, explicit-empty vs absent, rescope carry/reassign/widen/unbound, `SliceAssignments` supersede rule, idempotent acquire replay with assignment.

## PR Split (auto-chain, stacked-to-main, 400-line budget)

**PR1 `feat(sdd): bind immutable obligation assignments on runtime work units` (~280 lines)** — `runtime_ledger.go`, `runtime_compact.go`, `runtime_ledger_assignment_test.go`. Commits: (1) fields + normalize + shape/digest validation + legacy-replay tests (additive, no enforcement, main green); (2) enforcement: match checks, rescope rules, replay mirrors, `SliceAssignments` + tests. Rollback: revert (2) → inert fields; revert (1) → pristine.

**PR2 `feat(sdd): admit slice-scoped verify reports under dual authority` (~380 lines)** — `verification.go`, both CLI files, validator/CLI/status tests, journey + registration. Commits: (1) validator + envelope + unit tests (uncalled, green); (2) CLI wiring + CLI tests; (3) journey j82 + archive-isolation regression cases. Rollback: revert (2) restores exact current CLI (`--scope` default `whole`); fields stay additive/`omitempty`. Main green after each PR; no data migration.

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary changes. The change extends the product's own validator CLI flags and ledger structs; `resolveDependencies` routing is explicitly unchanged.

## Migration / Rollout

No migration required. `omitempty` fields replay legacy chains byte-identically; `--scope` defaults to `whole`.

## Risks & Open Questions

- PR2 lands near the 400-line budget; if the journey grows, split commit (3) into PR3 (delivery strategy auto-chain permits it).
- Comma-ban on obligation IDs rejects a spec heading containing a comma at bind time (fail-closed, documented in help).
- Design exceeds the 800-word skill budget by orchestrator mandate for implementation-level precision.
- Open: none blocking.
