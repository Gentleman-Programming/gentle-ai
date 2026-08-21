# Specification: Distinguish Unachievable Lens Attempts from Unattempted Slots

## Purpose

Define the formal requirement contracts, schema bindings, and state transitions for handling unachievable reviewer lens attempts (Issue #3442, PR #3520). The specification distinguishes unachievable reviewer attempts from unattempted slots via validated non-admitted attempt evidence, retry accounting, the truthful operational reason code `unachievable_reviewer_attempt`, and strict finalize/receipt blocking.

---

## 1. Admission and Invariant Contracts

### Requirement: Non-Admitted Unachievable Lens Attempt
The system MUST define `ArtifactAdmissionUnachievable` as a first-class admission decision (`"unachievable"`). `ArtifactAdmission.Validate(subject)` MUST fail closed on `ArtifactAdmissionUnachievable` by requiring `admission.Decision == ArtifactAdmissionCompleted`. An unachievable attempt record MUST NOT be admitted as a completed reviewer result.

#### Scenario: Unachievable attempt fails validation for completed result
- GIVEN an artifact admission payload with `Decision: "unachievable"`
- WHEN `ArtifactAdmission.Validate(subject)` is evaluated
- THEN it returns an error and denies admission as a completed result
- AND the completed reviewer result slot remains vacant

#### Scenario: Valid completed result admits successfully
- GIVEN an artifact admission payload with `Decision: "completed"` matching the artifact subject and manifest
- WHEN `ArtifactAdmission.Validate(subject)` is evaluated
- THEN it returns nil and admits the result into the canonical result slot

---

## 2. Data Structures and Storage Contract

### Requirement: Durable Reviewer Attempt Record
The system MUST persist unachievable reviewer attempts in a dedicated `ReviewerAttemptRecord` bound to `(LineageID, TargetIdentity, AuthorityRevision, Lens, SelectedOrder, SubjectHash)`. Attempt records MUST be stored under `<storeDir>/reviewer-attempts/%02d-%s-%02d.json` and MUST NOT collide with or mutate completed reviewer result slots (`<storeDir>/reviewer-results/%02d-%s.json`).

#### Schema: `gentle-ai.review-attempt-record/v1`
```json
{
  "schema": "gentle-ai.review-attempt-record/v1",
  "lineage_id": "string",
  "target_identity": "string",
  "authority_revision": "string",
  "lens": "string",
  "selected_order": 0,
  "subject_hash": "string",
  "attempt_index": 1,
  "admission": {
    "schema": "gentle-ai.review-artifact-admission/v1",
    "decision": "unachievable",
    "subject_hash": "string",
    "raw_sha256": "string",
    "canonical_sha256": "string",
    "diagnostic": "string"
  },
  "raw_sha256": "string",
  "canonical_sha256": "string"
}
```

#### Scenario: Attempt record stored separately from completed results
- GIVEN a reviewer provider emits an unachievable attempt for lens `reliability` at order `0`
- WHEN attempt capture executes
- THEN the attempt is written to `reviewer-attempts/00-reliability-01.json`
- AND `reviewer-results/00-reliability.json` is not created or modified

#### Scenario: Readback discovers attempt without modifying completed semantics
- GIVEN persisted attempt records for an uncompleted lens slot
- WHEN `DiscoverReviewerAttempts` is invoked
- THEN all validated attempt records for `(SelectedOrder, Lens)` are returned
- AND `discoverCapturedReviewerArtifacts` reports the slot as uncompleted

---

## 3. Retry Accounting Contract

### Requirement: Per-Slot Retry Accounting
The system MUST enforce a bounded retry threshold per lens slot (`maxReviewerAttemptsPerSlot = 3`). While attempt count is below the threshold, STATUS MUST continue to re-offer collection for that uncompleted slot. Once attempt count reaches the threshold, STATUS MUST transition to a terminal stop state.

#### Scenario: Attempt count below threshold prompts re-offer
- GIVEN lens slot `0` has `1` or `2` recorded unachievable attempts
- WHEN `newReviewNextTransition` evaluates reviewing state
- THEN it emits `Kind: "collect"` with `ReasonCode: "reviewer_results_required"`
- AND the collect input targets the unachieved lens slot

#### Scenario: Attempt count reaches threshold triggers terminal stop
- GIVEN lens slot `0` has `3` recorded unachievable attempts without a completed result
- WHEN `newReviewNextTransition` evaluates reviewing state
- THEN it emits `Kind: "stop"` with `ReasonCode: "unachievable_reviewer_attempt"`

---

## 4. Next Transition Routing Contract

### Requirement: Truthful Reason Code and Continuation Routing
The system MUST emit `unachievable_reviewer_attempt` for exhausted lens attempts. It MUST NOT emit `captured_artifacts_unverifiable` for honest provider refusals or unachievable attempts. `captured_artifacts_unverifiable` SHALL remain strictly reserved for data tampering, hash mismatches, or storage corruption of existing artifacts.

#### Scenario: Truthful stop reason on provider unachievability
- GIVEN a reviewer lens that exhausted retry attempts due to provider refusal
- WHEN transition routing computes the next step
- THEN `head.Kind` is `"stop"`
- AND `head.ReasonCode` is `"unachievable_reviewer_attempt"`
- AND `head.ReasonCode` is NOT `"captured_artifacts_unverifiable"`

#### Scenario: Corruption triggers captured_artifacts_unverifiable
- GIVEN a captured artifact whose SHA256 digest does not match storage bytes
- WHEN transition routing evaluates the artifact
- THEN `head.Kind` is `"stop"`
- AND `head.ReasonCode` is `"captured_artifacts_unverifiable"`

---

## 5. Invariant Classification and Narration Contract

### Requirement: Invariant Table and Documentation Agreement
The system MUST register `unachievable_reviewer_attempt` in `reviewStopInvariantClassification`, `review_narration.go`, `docs/review-integration.md`, and `review-ledger-contract.md`. The reason code MUST be classified as `Terminal: true` with `ToolFault: false` (user/environment decision).

#### Invariant Classification Entry
| Reason Code | Terminal | ToolFault | Justification |
|-------------|----------|-----------|---------------|
| `unachievable_reviewer_attempt` | `true` | `false` | Reviewer lens could not achieve a completed result after bounded attempts; provider refusals or environmental obstacles require operator investigation or scope reduction. |

#### Scenario: Invariant tests pass with new reason code
- GIVEN `reviewStopInvariantClassification` contains `unachievable_reviewer_attempt`
- WHEN `TestReviewStopInvariantReasonCodesAreClassified`, `TestReviewStopInvariantTerminalClassificationAgreesWithDocs`, and `TestReviewStopInvariantTerminalClassificationAgreesWithShippedContract` run
- THEN all tests pass without errors

---

## 6. Finalize and Receipt Blocking Contract

### Requirement: Strict Finalize Blocking on Unachieved Slots
The system MUST reject `review.finalize` and MUST NOT issue a terminal receipt if any selected lens is not admitted as `ArtifactAdmissionCompleted`. Unachievable attempt records MUST NOT satisfy finalize preconditions.

#### Scenario: Finalize refused on unachieved lens slot
- GIVEN a review where 3 of 4 lenses are completed and 1 lens has unachievable attempt records
- WHEN `gentle-ai review finalize` is executed
- THEN finalization is refused with a typed preflight error
- AND no receipt is issued
- AND transaction state does not transition to `approved`

#### Scenario: Finalize succeeds only when all lenses are completed
- GIVEN all selected lenses have valid `ArtifactAdmissionCompleted` artifacts
- WHEN `gentle-ai review finalize` is executed
- THEN finalization succeeds and exactly one terminal receipt is written

---

## 7. Acceptance Test Contract

| ID | Test Name | Focus Area | Expected Outcome |
|----|-----------|------------|------------------|
| T1 | `TestAdmitArtifactUnachievableNonAdmission` | `artifact_admission.go` | `ArtifactAdmissionUnachievable` fails `Validate(subject)` |
| T2 | `TestReviewerAttemptRecordPersistence` | `compact_reviewer_capture.go` | Attempt written to attempt directory, completed slot untouched |
| T3 | `TestReviewerSlotRetryAccounting` | `review_next_transition.go` | Collect offered for attempts < 3; stop emitted at attempt 3 |
| T4 | `TestTruthfulStopReasonCode` | `review_next_transition.go` | Emits `unachievable_reviewer_attempt`, avoids `captured_artifacts_unverifiable` |
| T5 | `TestFinalizeBlockedOnUnachievedSlot` | `review_facade.go` | `review.finalize` fails closed without receipt |
| T6 | `TestReviewStopInvariantTableCoverage` | `review_stop_invariant_test.go` | All invariant table checks and docs cross-checks pass |
