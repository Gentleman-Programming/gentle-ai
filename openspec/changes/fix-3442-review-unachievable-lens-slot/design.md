# Design: Distinguish Unachievable Lens Attempts from Unattempted Slots

## Technical Approach

We introduce non-admitted reviewer attempt records stored separately under `reviewer-attempts/`, preserving cryptographic binding to `(LineageID, TargetIdentity, AuthorityRevision, Lens, SelectedOrder, SubjectHash)`. Provider refusals and unachievable results persist attempt evidence without occupying `reviewer-results/`. A retry threshold (`maxReviewerAttemptsPerSlot = 3`) governs re-offers; upon exhaustion, transition routing truthfully emits `unachievable_reviewer_attempt` instead of misleading corruption codes (`captured_artifacts_unverifiable`) or infinite loops. Finalize strictly blocks receipt issuance on any unachieved slot.

## Architecture Decisions

| Decision | Choice | Alternatives Considered | Rationale |
|---|---|---|---|
| **Attempt Storage** | Isolated `reviewer-attempts/%02d-%s-%02d.json` | Reusing `reviewer-results/` with error flag; ephemeral memory | Isolates completed result slots from non-admitted attempts; prevents false-positive finalization while preserving immutable evidence on disk. |
| **Retry Limit** | Bounded constant (`maxReviewerAttemptsPerSlot = 3`) | Unlimited re-offers; immediate 1-shot stop | Mitigates transient provider failures while preventing infinite collect re-offer loops (Issue #3442). |
| **Reason Code** | `unachievable_reviewer_attempt` (`Terminal: true`, `ToolFault: false`) | `captured_artifacts_unverifiable`; `reviewer_results_required` | Truthfully distinguishes provider inability/refusal from data corruption or pending initial collection. |
| **Finalize Guard** | Strict precondition: all lenses must be `ArtifactAdmissionCompleted` | Partial receipts; soft warnings | Prevents invalid approvals and receipt signing when review requirements are unmet. |

## Data Flow

```
Provider Invocation / Capture
       │
   [Success?]
  ┌────┴────────────────────────┐
[Yes]                         [No / Refusal]
  │                             │
AdmitCompleted                AdmitUnachievable
  │                             │
Write reviewer-results/       Write reviewer-attempts/
  │                             │
  │                           [Attempt Count >= 3?]
  │                          ┌──┴───────────────┐
  │                        [No]               [Yes]
  │                          │                  │
  ▼                          ▼                  ▼
Execute review.finalize    Collect Re-offer   Stop: unachievable_reviewer_attempt
```

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/reviewtransaction/artifact_admission.go` | Modify | Validate `ArtifactAdmissionUnachievable` fails closed for completed result admission. |
| `internal/reviewtransaction/compact_reviewer_capture.go` | Modify | Implement `ReviewerAttemptRecord`, `CaptureUnachievableReviewerAttempt`, and slot attempt discovery. |
| `internal/reviewtransaction/compact_store.go` | Modify | Define `CompactReviewerAttemptsDir = "reviewer-attempts"`. |
| `internal/cli/review_artifact.go` | Modify | Capture unachievable attempts on provider refusal/failure; discover attempts in `discoverCapturedReviewerArtifacts`. |
| `internal/cli/review_next_transition.go` | Modify | Enforce retry accounting (`maxReviewerAttemptsPerSlot = 3`) and route exhausted attempts to `unachievable_reviewer_attempt`. |
| `internal/cli/review_narration.go` | Modify | Register Tier C narration for `unachievable_reviewer_attempt`. |
| `internal/cli/review_facade.go` | Modify | Block finalize and receipt issuance when unachieved slots exist. |
| `internal/cli/review_stop_invariant_test.go` | Modify | Register reason code in invariant classification table. |
| `docs/review-integration.md` | Modify | Document operational reason code and continuation semantics. |

## Interfaces / Contracts

```go
// ReviewerAttemptRecord represents non-admitted attempt evidence.
type ReviewerAttemptRecord struct {
	Schema            string                    `json:"schema"` // "gentle-ai.review-attempt-record/v1"
	LineageID         string                    `json:"lineage_id"`
	TargetIdentity    string                    `json:"target_identity"`
	AuthorityRevision string                    `json:"authority_revision"`
	Lens              string                    `json:"lens"`
	SelectedOrder     int                       `json:"selected_order"`
	SubjectHash       string                    `json:"subject_hash"`
	AttemptIndex      int                       `json:"attempt_index"`
	Admission         ArtifactAdmission         `json:"admission"`
	RawSHA256         string                    `json:"raw_sha256"`
	CanonicalSHA256   string                    `json:"canonical_sha256"`
}

type CaptureReviewerAttemptRequest struct {
	StoreDir                  string
	LineageID                 string
	TargetIdentity            string
	AuthorityRevision         string
	Lens                      string
	SelectedOrder             int
	SubjectHash               string
	Admission                 ArtifactAdmission
	RawPayload                []byte
	CanonicalPayload          []byte
}

func (store CompactStore) CaptureUnachievableReviewerAttempt(ctx context.Context, req CaptureReviewerAttemptRequest) (ReviewerAttemptRecord, error)
func ReadCompactReviewerAttempts(storeDir string, order int, lens string) ([]ReviewerAttemptRecord, error)
func DiscoverReviewerSlotAttempts(storeDir string, state CompactState, revision string) (map[int][]ReviewerAttemptRecord, error)
```

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | `ArtifactAdmission.Validate` non-admission | Verify `ArtifactAdmissionUnachievable` returns error on validation. |
| Unit | Attempt isolation & persistence | Capture unachievable attempt; verify record in `reviewer-attempts/` and vacancy in `reviewer-results/`. |
| Integration | Retry accounting & truthful stop | Verify `collect` re-offered for attempts 1 & 2; verify `stop` with `unachievable_reviewer_attempt` at attempt 3. |
| Integration | Finalize blocking | Attempt finalize with 1 unachieved slot; verify refusal and receipt prevention. |
| Integration | Invariant table agreement | Execute `review_stop_invariant_test.go` verifying docs and shipped contract agreement. |
| E2E | Black-box host/provider regression | End-to-end simulation of provider refusal lifecycle and truthful terminal exit. |

## Threat Matrix

| Boundary | Minimum adversarial cases | Applicability | Design response | Planned RED tests |
|---|---|---|---|---|
| Documentation-like paths | `requirements.txt`, MDX | N/A | No path classification or executable routing changed. | N/A |
| Git repository selection | `git -C`, relative paths | Applicable | Attempt storage bound strictly under validated `store.Dir`. | `TestAttemptPathEscapesStoreDir` |
| Commit state | staged, empty index | N/A | No commit state mutation. | N/A |
| Push state | tracking branch | N/A | No remote push integration changed. | N/A |
| PR commands | `--lineage`, `--agent` | Applicable | Finalize preflight strictly checks slot completion before receipt issuance. | `TestFinalizeBlockedOnUnachievedSlot` |

## Migration / Rollout

No migration required. Legacy reviews without `reviewer-attempts/` evaluate attempts as zero and remain fully backwards-compatible.

## Open Questions

None.
