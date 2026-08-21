# Tasks: Distinguish Unachievable Lens Attempts from Unattempted Slots

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 280 - 360 lines |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR (PR #3520) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: stacked-to-main
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Attempt data structures and transaction store persistence | PR 1 | `go test ./internal/reviewtransaction -run 'Test(AdmitArtifactUnachievableNonAdmission\|ReviewerAttemptRecordPersistence\|AttemptPathEscapesStoreDir)'` | N/A: internal storage engine unit tests | `internal/reviewtransaction/artifact_admission.go`, `internal/reviewtransaction/compact_reviewer_capture.go`, `internal/reviewtransaction/compact_store.go` |
| 2 | Artifact capture and unachieved slot discovery | PR 1 | `go test ./internal/cli -run 'TestReviewCaptureResultUnachievableAttempt'` | `gentle-ai review capture-result --lineage ... --lens ...` | `internal/cli/review_artifact.go` |
| 3 | Retry accounting, stop routing, and finalize blocking | PR 1 | `go test ./internal/cli -run 'Test(ReviewerSlotRetryAccounting\|TruthfulStopReasonCode\|FinalizeBlockedOnUnachievedSlot)'` | `gentle-ai review status --next-transition` / `gentle-ai review finalize` | `internal/cli/review_next_transition.go`, `internal/cli/review_facade.go` |
| 4 | Invariants, narration, docs, and regression suite | PR 1 | `go test ./internal/cli -run 'Test(ReviewStopInvariant\|ReviewNarration\|UnachievableLensSlotRegression)'` | End-to-end simulated CLI review session | `internal/cli/review_narration.go`, `internal/cli/review_stop_invariant_test.go`, `internal/cli/review_unachievable_lens_slot_test.go`, docs |

## Phase 1: Transaction Foundation & Attempt Storage

- [x] 1.1 Add RED unit tests in `internal/reviewtransaction/artifact_admission_test.go` and `compact_reviewer_capture_test.go` for `ArtifactAdmissionUnachievable` non-admission and `TestAttemptPathEscapesStoreDir`.
- [x] 1.2 Define `ReviewerAttemptRecord`, `CaptureReviewerAttemptRequest`, and `CompactReviewerAttemptsDir` in `internal/reviewtransaction/compact_store.go` and `compact_reviewer_capture.go`.
- [x] 1.3 Implement `CaptureUnachievableReviewerAttempt`, `ReadCompactReviewerAttempts`, and `DiscoverReviewerSlotAttempts` in `internal/reviewtransaction/compact_reviewer_capture.go`.
- [x] 1.4 Enforce fail-closed validation on `ArtifactAdmissionUnachievable` in `ArtifactAdmission.Validate()` in `internal/reviewtransaction/artifact_admission.go`.

## Phase 2: Artifact Capture & Slot Discovery

- [x] 2.1 Add RED test in `internal/cli/review_artifact_test.go` for unachievable provider capture and slot attempt discovery.
- [x] 2.2 Update `RunReviewCaptureResult` in `internal/cli/review_artifact.go` to capture provider refusal / failure evidence via `CaptureUnachievableReviewerAttempt`.
- [x] 2.3 Update `discoverCapturedReviewerArtifacts` in `internal/cli/review_artifact.go` to discover attempt records and report unachieved slots as `ArtifactAdmissionUnachievable`.

## Phase 3: Retry Accounting & Transition Routing

- [x] 3.1 Add RED unit tests `TestReviewerSlotRetryAccounting` and `TestFinalizeBlockedOnUnachievedSlot` in `internal/cli/review_next_transition_test.go` and `internal/cli/review_facade_test.go`.
- [x] 3.2 Implement bounded retry accounting (`maxReviewerAttemptsPerSlot = 3`) in `newReviewNextTransition` and `reviewFinalizeNextTransition` in `internal/cli/review_next_transition.go`.
- [x] 3.3 Emit `reviewCollectTransition` for uncompleted slots when attempt count < 3.
- [x] 3.4 Emit `reviewStopTransition("unachievable_reviewer_attempt")` when any slot reaches 3 attempts without a completed result.
- [x] 3.5 Block `review.finalize` and refuse receipt issuance when any lens slot is unachieved.

## Phase 4: Narration, Invariant Tables & Regressions

- [x] 4.1 Add Tier C narration for `stop:unachievable_reviewer_attempt` in `internal/cli/review_narration.go`.
- [x] 4.2 Register `unachievable_reviewer_attempt` (`Terminal: true`, `ToolFault: false`) in `reviewStopInvariantClassification` in `internal/cli/review_stop_invariant_test.go`.
- [x] 4.3 Update stop reason continuation table in `docs/review-integration.md`.
- [x] 4.4 Update stop reason continuation table in `internal/assets/skills/_shared/review-ledger-contract.md`.
- [x] 4.5 Add unit test `TestTruthfulStopReasonCode` verifying `unachievable_reviewer_attempt` is emitted and `captured_artifacts_unverifiable` is not emitted on provider refusals.

## Phase 5: Documentation & Contract Alignment

- [x] 5.1 Update stop reason continuation table in `docs/review-integration.md`.
- [x] 5.2 Update stop reason continuation table in `internal/assets/skills/_shared/review-ledger-contract.md`.
- [x] 5.3 Verify all tests pass with `go test ./internal/reviewtransaction/... ./internal/cli/...`.
