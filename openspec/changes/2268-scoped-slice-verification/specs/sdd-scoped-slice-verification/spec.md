# Delta for SDD Scoped Slice Verification

## ADDED Requirements

### Requirement: Provider-Owned Slice Identity

The provider-owned `RuntimeObjective` and its work-unit identity MUST be the sole slice identity and scope owner. The system MUST NOT introduce a free-form slice registry or any parallel source of truth for slice scope.

#### Scenario: Slice identity resolves solely from the provider-owned work unit

- GIVEN a work unit begun through `BeginAttemptRequest` with a bound obligation assignment
- WHEN slice identity is resolved for verify admission
- THEN identity and scope MUST resolve solely from the `RuntimeObjective` work-unit identity

#### Scenario: Assignments persist only on provider-owned structures

- GIVEN an obligation assignment for a slice
- WHEN the assignment is persisted
- THEN it MUST be stored only on the provider-owned objective and the requests that embed it, never in a separate registry

### Requirement: Immutable Obligation ID Assignment

Each candidate/revision MUST receive an immutable assignment of exact requirement IDs and scenario IDs plus derived totals, bound at begin time via `BeginAttemptRequest` (`AssignedRequirementIDs`/`AssignedScenarioIDs`, `omitempty` JSON) and propagated through begin/rescope events and replay validators. Derived totals MUST equal the assignment lengths. Rescope MUST carry the assignment forward or explicitly reassign it.

#### Scenario: Assignment bound at begin

- GIVEN a begin request carrying `AssignedRequirementIDs` and `AssignedScenarioIDs`
- WHEN the attempt begins
- THEN the objective MUST record the exact assignment with derived totals equal to its lengths

#### Scenario: Continuing attempt presents the identical assignment

- GIVEN an objective with a bound assignment
- WHEN a continuing attempt begins presenting the identical assignment
- THEN the attempt MUST be accepted

#### Scenario: Rescope carries forward or reassigns

- GIVEN an objective with a bound assignment
- WHEN a rescope is requested
- THEN the rescope MUST carry the identical assignment forward or explicitly reassign it, enforced by replay validation

### Requirement: Slice-Scoped Verify Report Admission

`ValidateSliceVerifyReportAdmission` MUST admit a slice report only when BOTH report metadata (`scope: slice`, non-empty `slice_id`, `requirement_ids`, `scenario_ids`) AND CLI authority (`--scope slice --slice-id <id>`) match the bound assignment. `--requirements`/`--scenarios` MUST be authoritative totals for the slice scope and MUST equal the assignment lengths. PASS and PASS_WITH_WARNINGS MUST require completed slice totals. `ValidateVerifyReportAdmission` (whole-change) MUST remain untouched.

#### Scenario: Complete slice PASS admitted under dual authority

- GIVEN a bound assignment of two requirements and three scenarios and a report with `scope: slice`, matching `slice_id`, matching ID lists, totals 2/2 and 3/3, verdict PASS
- WHEN admission runs with `--scope slice --slice-id <id> --requirements 2 --scenarios 3`
- THEN the report MUST be admitted

#### Scenario: CLI authority disagrees with the bound assignment

- GIVEN `--slice-id` differs from the bound slice identity, or `--requirements`/`--scenarios` differ from the assignment lengths
- WHEN admission runs
- THEN the report MUST be rejected

#### Scenario: Passing verdicts require completed slice totals

- GIVEN a slice report with verdict PASS or PASS_WITH_WARNINGS whose completed totals fall short of the assignment lengths
- WHEN admission runs
- THEN the report MUST be rejected

### Requirement: Fail-Closed Rejection of Invalid Slice Claims

Unknown identity, partial metadata, missing scope metadata, mismatched `slice_id`, mismatched ID lists, overlap with another work unit's assignment, duplicate coverage, and altered assignment MUST all be rejected fail-closed. Overlap and duplicate detection MUST use caller-supplied known assignments; the validator MUST remain pure.

#### Scenario: Report metadata unknown, missing, or partial

- GIVEN a report whose `slice_id` is unknown or mismatches the bound assignment, whose scope metadata is missing, or whose metadata is partial (empty `slice_id`, absent ID lists)
- WHEN admission runs
- THEN the report MUST be rejected fail-closed

#### Scenario: Report ID lists mismatched or assignment altered

- GIVEN a report whose `requirement_ids`/`scenario_ids` differ from the bound assignment, or a continuing attempt presenting an altered assignment
- WHEN admission or begin validation runs
- THEN the claim MUST be rejected fail-closed

#### Scenario: Overlap and duplicate coverage rejected

- GIVEN caller-supplied known assignments across work units
- WHEN a claimed assignment overlaps another work unit's assignment or duplicates coverage
- THEN the validator MUST reject fail-closed using only the supplied assignments

### Requirement: Incomplete FAIL Slice Admission Preserved

Incomplete FAIL slice reports MUST continue to be admitted under the existing contradiction rules, including the authority-only exit-code-125 rule.

#### Scenario: Incomplete FAIL slice report admitted

- GIVEN a slice report with verdict FAIL and totals below the assignment lengths
- WHEN slice admission runs
- THEN the report MUST be admitted under the existing contradiction rules

#### Scenario: Authority-only exit-code-125 rule preserved

- GIVEN an incomplete FAIL slice report produced under the authority-only exit-code-125 condition
- WHEN slice admission runs
- THEN the existing authority-only rule MUST apply unchanged

### Requirement: Slice PASS Never Implies Whole-Change Completion

A slice PASS MUST persist only work-unit evidence. It MUST NOT replace the whole-change verify report, MUST NOT set verify all_done, and MUST NOT unlock archive; `resolveDependencies` archive gating MUST remain unchanged.

#### Scenario: Slice PASS persists only work-unit evidence

- GIVEN an admitted passing slice report
- WHEN the result is persisted
- THEN it MUST persist as work-unit evidence only, without replacing the whole-change verify report

#### Scenario: Slice PASS does not unlock archive

- GIVEN a passing slice report while other obligations remain outstanding
- WHEN dependency resolution runs
- THEN verify MUST NOT become all_done and archive MUST remain blocked

### Requirement: Zero-Obligation Work Units

Zero-obligation work units MUST be allowed only when provider-owned metadata explicitly records `0/0` (explicitly empty JSON arrays, distinct from absent), includes a concrete evidence goal, and has non-empty passing execution evidence. They MUST receive no credit toward global obligations.

#### Scenario: Explicit zero-obligation unit admitted

- GIVEN provider-owned metadata with explicitly empty assignment arrays, a concrete evidence goal, and non-empty passing execution evidence
- WHEN the work unit is verified
- THEN the zero-obligation unit MUST be admitted

#### Scenario: Absent arrays are not zero-obligation

- GIVEN metadata whose assignment arrays are absent rather than explicitly empty
- WHEN zero-obligation eligibility is evaluated
- THEN the unit MUST be rejected fail-closed

#### Scenario: No credit toward global obligations

- GIVEN an admitted zero-obligation work unit
- WHEN global obligation totals are computed
- THEN it MUST contribute zero requirement and zero scenario credit

### Requirement: Whole-Change Backward Compatibility

Default whole-change CLI and report behavior MUST be preserved byte-identically. Legacy persisted ledger records without the new fields MUST replay unchanged. `--scope` MUST default to `whole`.

#### Scenario: Default whole-change behavior byte-identical

- GIVEN `sdd-verify-validate` invoked without slice flags
- WHEN admission runs
- THEN `--scope` MUST resolve to `whole` and behavior MUST be byte-identical to the untouched whole-change admission

#### Scenario: Legacy ledger records replay unchanged

- GIVEN persisted ledger records lacking assignment fields
- WHEN the ledger is replayed
- THEN replay MUST succeed with results identical to pre-change replay

### Requirement: CLI Flag Authority

`--scope slice` MUST require a non-empty `--slice-id`. With `--scope whole`, slice-only flags MUST be handled per house CLI conventions and MUST NOT alter whole-change admission. Assignment flags on `sdd-attempt begin/acquire/rescope` MUST bind the obligation IDs.

#### Scenario: Slice scope requires a non-empty slice id

- GIVEN `sdd-verify-validate --scope slice`
- WHEN `--slice-id` is empty or missing
- THEN the command MUST reject the invocation

#### Scenario: Whole scope is unaffected by slice-only flags

- GIVEN `--scope whole` (explicit or default)
- WHEN slice-only flags are present
- THEN they MUST be rejected or ignored per house CLI conventions, and whole-change admission MUST be unchanged

#### Scenario: Assignment flags bind obligation IDs

- GIVEN `sdd-attempt begin`, `acquire`, or `rescope` invoked with assignment flags
- WHEN the operation runs
- THEN the supplied requirement and scenario IDs MUST bind as the work unit's assignment

### Requirement: Chained Slice Lifecycle

A passing slice MUST persist its evidence while other slices remain pending. Archive MUST open only after the final whole-change verification passes.

#### Scenario: Slice A persists while slice B remains pending

- GIVEN a change split into slices A and B with disjoint bound assignments
- WHEN slice A's verify report passes and is admitted
- THEN slice A's work-unit evidence MUST persist while slice B remains pending

#### Scenario: Archive opens only after final whole-change verification

- GIVEN all slices have passed and a final whole-change verify report matching all spec counts passes
- WHEN dependency resolution runs
- THEN archive MUST become ready only after that final whole-change verification
