# Exploration: Scoped Slice Verification (Issue #2268)

## Current State

The SDD verify admission contract (`ValidateVerifyReportAdmission` in `internal/sddstatus/verification.go:152`) requires the report's `requirements` and `scenarios` totals to exactly match the whole-change spec heading counts. A passing verdict additionally requires all requirements and scenarios completed, zero blockers, zero critical findings, and zero test/build exit codes. There is no concept of scope, slice identity, or partial obligation coverage.

The runtime ledger (`internal/sddstatus/runtime_ledger.go`) owns work-unit scope through `RuntimeObjective` (line 197) and `BeginAttemptRequest` (line 339). These structs carry `WorkUnit`, `EvidenceGoal`, `MaxAttempts`, `MaxChangedLines`, and candidate identity fields — but NO requirement/scenario ID assignment fields. `CompactAcquireRequest` (runtime_compact.go:60) embeds `BeginAttemptRequest` per decision 9's "single work-unit-scope owner" rule, so any field added to `BeginAttemptRequest` propagates to the compact path automatically.

The verify report envelope (`verifyReportRequiredFields`, verification.go:28) is a closed set of 12 fields. `parseScalarFields` (line 293) rejects unknown fields. There is no `scope`, `slice_id`, `requirement_ids`, or `scenario_ids` field.

The CLI (`internal/cli/sdd_verify_validate.go`) exposes only `--input`, `--requirements`, `--scenarios`. No `--scope` or `--slice-id` flags exist.

Archive gating (`resolveDependencies` in status.go:1837) requires `Verify == DependencyAllDone && taskProgress.AllComplete` for `Archive == DependencyReady`. `Verify` becomes `DependencyAllDone` only when `verifyReportCurrent && coreReady && taskProgress.AllComplete && verifyReportPassing` — which requires the report to match whole-change spec counts. A slice-scoped passing report cannot satisfy this unless it happens to cover all requirements and scenarios.

The consecutive-rescope repair series (commits 6c9ac09f through 74fb2cf6) added `RuntimeRescope`, `RepairConsecutiveRescope`, and associated replay validators. Rescope carries `CumulativeAttempts`/`CumulativeChangedLines` forward unchanged and creates a new objective with new `WorkUnit`/`EvidenceGoal`. It is the closest existing mechanism to "scope change" but operates on budget narrowing, not obligation ID assignment.

## Affected Areas

- `internal/sddstatus/runtime_ledger.go` — RuntimeObjective, BeginAttemptRequest, runtimeBeginEvent, runtimeRescopeEvent, applyRuntimeBeginEvent, applyRuntimeRescopeEvent, validateRuntimeBeginEvent, validateRuntimeRecordShape, normalizeBeginAttemptRequest, normalizeRescopeObjectiveRequest. New fields for obligation ID assignment.
- `internal/sddstatus/verification.go` — Verify report envelope (new optional fields: scope, slice_id, requirement_ids, scenario_ids), new `ValidateSliceVerifyReportAdmission` function, overlap/duplicate/altered-assignment detection helpers.
- `internal/cli/sdd_verify_validate.go` — New `--scope` and `--slice-id` flags, flag validation, dispatch to new validator.
- `internal/cli/sdd_attempt.go` — New `--assigned-requirement-ids` and `--assigned-scenario-ids` flags on `begin`, `acquire`, and `rescope` operations.
- `internal/sddstatus/status.go` — `resolveDependencies` must NOT promote slice PASS to `Verify == DependencyAllDone`. Archive gating isolation.
- `internal/sddstatus/runtime_compact.go` — `CompactAcquireRequest` inherits new fields via embed (no direct change needed).
- `bench/journeys_sdd_chain.go` or new file — Chained SDD journey proving slice A persists while B remains pending and archive opens only after final whole-change verification.
- Test files: `verification_test.go`, `bounded_review_test.go`, `runtime_objective_owner_test.go`, `runtime_objective_advance_test.go`, `sdd_verify_validate_test.go`.

## Approaches

### Approach 1: Add obligation ID fields to BeginAttemptRequest + RuntimeObjective (RECOMMENDED)

Add `AssignedRequirementIDs []string` and `AssignedScenarioIDs []string` (JSON: `assigned_requirement_ids`, `assigned_scenario_ids`, both `omitempty`) to `BeginAttemptRequest`. These propagate through the begin event, rescope event, and into `RuntimeObjective`. The fields are immutable once set — Begin validates they match the existing objective on continuing attempts, and Rescope carries them forward or reassigns them for the narrower scope.

Admission: new `ValidateSliceVerifyReportAdmission(text string, expected SpecCounts, scope string, sliceID string, assignedReqIDs, assignedScenarioIDs []string)` function. The verify report envelope gains optional `scope`, `slice_id`, `requirement_ids`, `scenario_ids` fields. When `scope == "slice"`, the report's ID lists must exactly match the assignment, and the totals must match the assignment lengths (not the whole-change counts). Overlap detection: the caller supplies all known assignments for the change, and the function checks for duplicates/overlap across work units.

- **Pros**: Uses the provider-owned structure as the sole scope owner (per maintainer directive). Backward-compatible (`omitempty` means legacy records replay byte-identically). Non-breaking Go API (new exported function, old one untouched). CompactAcquireRequest inherits via embed.
- **Cons**: Adds ~4 fields to 2 structs + event structs + replay validators. The overlap/duplicate detection needs a caller-supplied list of all assignments, which means the CLI or status layer must aggregate them from the ledger.
- **Effort**: Medium-High. Estimated ~350-450 changed lines across production code + tests.

### Approach 2: Separate slice registry (REJECTED by maintainer)

Create a parallel slice registry keyed by slice_id that maps to requirement/scenario ID lists. The verify validator consults both the runtime ledger and the slice registry.

- **Pros**: Clean separation of concerns.
- **Cons**: Maintainer explicitly rejected this: "Do not add a free-form slice registry or parallel source of truth." Violates the "sole scope owner" directive.
- **Effort**: N/A (rejected).

### Approach 3: Totals-only slice verification (REJECTED by gate)

Interpret `--requirements` and `--scenarios` as authoritative slice totals without tracking individual IDs. The report's totals must match the CLI-supplied totals.

- **Pros**: Simpler implementation, no ID tracking.
- **Cons**: Cannot detect overlap, duplicate coverage, or altered assignment. The maintainer's acceptance criteria explicitly require "immutable assignment of exact requirement and scenario IDs plus derived totals" and "overlap, duplicate coverage, or altered assignment must fail closed." Attempt 2 failed the gate for exactly this reinterpretation.
- **Effort**: N/A (rejected).

## Recommendation

**Approach 1** is the only design that satisfies all maintainer constraints:

1. **Sole scope owner**: `BeginAttemptRequest` / `RuntimeObjective` remain the single source of truth. No parallel registry.
2. **Immutable ID assignment**: Fields are set at Begin time, carried forward by Rescope, and validated on every continuing attempt. The immutable chain records bind them content-addressed.
3. **Non-breaking Go API**: New `ValidateSliceVerifyReportAdmission` function leaves `ValidateVerifyReportAdmission` untouched. Existing callers (`RunSDDVerifyValidate`, tests) are unaffected.
4. **Backward-compatible persistence**: `omitempty` JSON tags mean legacy records (without the new fields) replay byte-identically. New records with empty slices also serialize identically to legacy.
5. **Archive isolation**: `resolveDependencies` is unchanged — a slice-scoped report does NOT set `Verify == DependencyAllDone` because the status layer's `readVerifyResult` continues to compare against whole-change `SpecCounts`. A slice PASS persists as work-unit evidence only.
6. **Zero-obligation path**: A `RuntimeObjective` with `AssignedRequirementIDs: []` and `AssignedScenarioIDs: []` (explicitly empty, not absent) + non-empty `EvidenceGoal` + non-empty passing execution evidence is the zero-obligation work unit. It receives no credit toward global obligations.

## Risks

1. **Line budget**: The implementation plausibly exceeds 400 changed lines. Estimated breakdown:
   - `runtime_ledger.go`: +80 lines (4 new fields across 2 structs + 2 event structs, replay validation, normalize updates)
   - `verification.go`: +120 lines (new envelope fields, new `ValidateSliceVerifyReportAdmission`, overlap/duplicate/altered-assignment helpers)
   - `sdd_verify_validate.go`: +40 lines (new flags, dispatch)
   - `sdd_attempt.go`: +30 lines (new flags on begin/acquire/rescope)
   - Tests: +200 lines (6 new test functions + table cases)
   - Journey: +60 lines (chained SDD journey)
   - **Total: ~530 lines** — exceeds 400-line budget.
   - **Decision needed before apply: Yes**. Suggest splitting into 2 chained PRs:
     - PR1: RuntimeObjective/BeginAttemptRequest ID assignment fields + replay + tests (~250 lines)
     - PR2: Slice verify admission + CLI flags + journey (~280 lines)

2. **Engram availability**: Engram MCP tools are available in this session. The artifact will be persisted to topic_key `sdd/2268-scoped-slice-verification/explore`.

3. **Overlap detection input**: The overlap/duplicate-coverage check requires knowing ALL assignments for the change. The runtime ledger's `Attempts` list carries `WorkUnit` but not the assignments of OTHER objectives. Two options:
   - (a) The CLI caller aggregates assignments from all objectives in the chain (requires a new `sdd-attempt assignments` subcommand or extends `status` output).
   - (b) The validator accepts a `knownAssignments []SliceAssignment` parameter and the caller supplies it.
   - Recommendation: (b) — keeps the validator pure and testable. The CLI/status layer aggregates.

4. **Altered assignment detection**: A continuing attempt must present the same IDs as the objective. This is already enforced by Begin's existing "request fields must match objective" check (line 764-766 of runtime_ledger.go) — the new ID fields are added to that comparison.

5. **Rescope interaction**: Rescope creates a new objective with potentially different IDs. The rescope request must carry the new assignment, and the replay validator must verify it. This is a natural extension of the existing rescope flow.

## Design Details

### New fields on BeginAttemptRequest

```go
type BeginAttemptRequest struct {
    ExpectedRevision     string   `json:"expected_revision"`
    RequestID            string   `json:"request_id"`
    WorkUnit             string   `json:"work_unit"`
    EvidenceGoal         string   `json:"evidence_goal"`
    MaxAttempts          int      `json:"max_attempts"`
    MaxChangedLines      int      `json:"max_changed_lines"`
    AssignedRequirementIDs []string `json:"assigned_requirement_ids,omitempty"`
    AssignedScenarioIDs    []string `json:"assigned_scenario_ids,omitempty"`
}
```

### New fields on RuntimeObjective

```go
type RuntimeObjective struct {
    ID                       string   `json:"id"`
    Generation               int      `json:"generation"`
    WorkUnit                 string   `json:"work_unit"`
    EvidenceGoal             string   `json:"evidence_goal"`
    InitialCandidateIdentity string   `json:"initial_candidate_identity"`
    InitialCandidateTree     string   `json:"initial_candidate_tree"`
    MaxAttempts              int      `json:"max_attempts"`
    MaxChangedLines          int      `json:"max_changed_lines"`
    AssignedRequirementIDs   []string `json:"assigned_requirement_ids,omitempty"`
    AssignedScenarioIDs      []string `json:"assigned_scenario_ids,omitempty"`
}
```

### New verify report envelope fields (optional)

```
scope: slice | whole (default: whole)
slice_id: <non-empty when scope=slice>
requirement_ids: REQ-001,REQ-002 (comma-separated, required when scope=slice)
scenario_ids: S1,S2,S3 (comma-separated, required when scope=slice)
```

### New CLI flags

```
--scope <whole|slice>     (default: whole; backward-compatible)
--slice-id <id>           (required when scope=slice)
```

The existing `--requirements` and `--scenarios` flags remain. When `--scope slice`, they are interpreted as the slice's authoritative totals (must match the report's totals AND the assignment lengths).

### New exported function

```go
func ValidateSliceVerifyReportAdmission(
    text string,
    expected SpecCounts,
    scope string,
    sliceID string,
    assignedReqIDs, assignedScenarioIDs []string,
    knownAssignments []SliceAssignment,
) VerifyReportAdmission
```

Where `SliceAssignment` is:

```go
type SliceAssignment struct {
    SliceID          string
    RequirementIDs   []string
    ScenarioIDs      []string
}
```

### Archive isolation proof

`resolveDependencies` (status.go:1837) computes `Verify == DependencyAllDone` only when `verifyReportPassing` is true. `verifyReportPassing` comes from `parseVerifyResult(report, specCounts)` where `specCounts` is the whole-change count from `readSpecCounts(artifactPaths.Specs)`. A slice-scoped report with `requirements: 2/2` and `scenarios: 3/3` would be internally complete, but `parseVerifyResult` compares against the whole-change counts (e.g., 5 requirements, 10 scenarios) and finds a totals mismatch → `Stale: true`, `Passing: false`. Therefore `Verify` stays `DependencyReady` (not `DependencyAllDone`), and `Archive` stays `DependencyBlocked`.

The slice PASS persists as work-unit evidence in the runtime ledger (the objective's `EvidenceRevision` is set), but it does NOT unlock archive. Only a final whole-change verify report that matches all spec counts can set `Verify == DependencyAllDone` and unlock archive.

### Zero-obligation work unit

A `RuntimeObjective` with `AssignedRequirementIDs: []` and `AssignedScenarioIDs: []` (explicitly empty JSON arrays, not absent) + non-empty `EvidenceGoal` + non-empty passing `EvidenceRevision` is the zero-obligation work unit. It receives no credit toward global obligations because its assignment lengths are 0, and the status layer's whole-change comparison never sees it as passing.

## Acceptance Tests (per maintainer)

1. `ValidateVerifyReportAdmission` — existing tests remain green (backward compatibility).
2. `RunSDDVerifyValidate` — existing tests remain green; new tests for `--scope slice --slice-id` flags.
3. `RuntimeObjectiveIsSoleWorkUnitScopeOwner` — remains green (new fields are on BeginAttemptRequest, CompactAcquireRequest inherits via embed).
4. `RuntimeLedgerAdvancesDistinctWorkUnitAfterPassedObjective` — remains green; new test variant with ID assignments.
5. `ResolveFinalVerifyWaitsForAllTasks` — remains green; new test proving slice PASS does NOT unlock archive.
6. `ResolveApplyVerifyArchiveGates` — remains green; new test proving whole-change verify is still required.
7. New chained SDD journey: slice A persists while B remains pending, archive opens only after final whole-change verification.

## Ready for Proposal

**Yes.** The design is fully traced against the current codebase at SHA 3c6a6341. All affected files, structs, functions, and test locations are identified. The maintainer's contract is satisfied. The line budget exceeds 400 lines and needs a chained-PR split decision before apply begins.
