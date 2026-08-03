package sddstatus

import (
	"context"
	"errors"
)

type CompactAttemptState string

const (
	CompactStateProceed  CompactAttemptState = "proceed"
	CompactStateBlocked  CompactAttemptState = "blocked"
	CompactStateComplete CompactAttemptState = "complete"
)

type CompactBlockReason string

const (
	CompactBlockActiveAttempt       CompactBlockReason = "active_attempt"
	CompactBlockMaintainerDecision  CompactBlockReason = "maintainer_decision"
	CompactBlockCorruptAuthority    CompactBlockReason = "corrupt_authority"
	CompactBlockInvalidContinuation CompactBlockReason = "invalid_continuation"
	CompactBlockAuthorityFailure    CompactBlockReason = "authority_failure"
)

// CompactAttemptResult is the bounded orchestration projection. RuntimeStatus
// remains available through the legacy diagnostic operations only.
type CompactAttemptResult struct {
	State  CompactAttemptState `json:"state"`
	Reason CompactBlockReason  `json:"reason,omitempty"`
	Token  string              `json:"token,omitempty"`
}

type CompactAcquireRequest struct {
	RequestID       string
	WorkUnit        string
	EvidenceGoal    string
	MaxAttempts     int
	MaxChangedLines int
}

type CompactSettleRequest struct {
	Token              string
	RequestID          string
	Outcome            AttemptOutcome
	EvidenceRevision   string
	Diagnosis          string
	HarnessDisposition HarnessDisposition
	CleanupEvidence    string
	ProcessEvidence    string
	SuccessorLineageID string

	RemediatesEvidenceRevision string
}

// Acquire claims one native attempt without exposing the growing runtime
// history. The returned token identifies that exact begin record for Settle.
func (store RuntimeStore) Acquire(ctx context.Context, request CompactAcquireRequest) (CompactAttemptResult, error) {
	begin, err := normalizeBeginAttemptRequest(BeginAttemptRequest{
		RequestID: request.RequestID, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
		MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
	})
	if err != nil {
		return CompactAttemptResult{}, err
	}

	replay, err := store.load()
	if err != nil {
		return compactBlocked(CompactBlockCorruptAuthority, ""), nil
	}
	if receipt, exists := replay.Requests[begin.RequestID]; exists {
		record, loadErr := store.loadRecord(receipt.Revision)
		if loadErr != nil {
			return compactBlocked(CompactBlockCorruptAuthority, ""), nil
		}
		begin.ExpectedRevision = record.PreviousRevision
		if !compactAcquireMatches(record, begin) {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		if _, err := store.Begin(ctx, begin); err != nil {
			return store.compactMutationFailure(err, false, begin), nil
		}
		current, loadErr := store.load()
		if loadErr != nil {
			return compactBlocked(CompactBlockCorruptAuthority, ""), nil
		}
		return compactAcquireResult(current, begin, receipt.Revision), nil
	}

	if result, terminal := compactAcquireBlock(replay, begin); terminal {
		return result, nil
	}
	begin.ExpectedRevision = replay.Status.Revision
	started, err := store.Begin(ctx, begin)
	if err != nil {
		return store.compactMutationFailure(err, false, begin), nil
	}
	return CompactAttemptResult{State: CompactStateProceed, Token: started.Revision}, nil
}

// Settle closes the attempt selected by Token through the ordinary Finish
// transition. Current binding and failed-evidence revisions are derived inside
// the authority; callers name a successor only when review approved a distinct
// lineage.
func (store RuntimeStore) Settle(ctx context.Context, request CompactSettleRequest) (CompactAttemptResult, error) {
	if err := normalizeCompactSettleRequest(request); err != nil {
		return CompactAttemptResult{}, err
	}
	replay, err := store.load()
	if err != nil {
		return compactBlocked(CompactBlockCorruptAuthority, ""), nil
	}
	if receipt, exists := replay.Requests[request.RequestID]; exists {
		record, loadErr := store.loadRecord(receipt.Revision)
		if loadErr != nil {
			return compactBlocked(CompactBlockCorruptAuthority, ""), nil
		}
		finish, ok := compactSettleReplayRequest(replay, record, request)
		if !ok {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		if _, err := store.Finish(ctx, finish); err != nil {
			return store.compactMutationFailure(err, true, BeginAttemptRequest{}), nil
		}
		return store.compactSettleResult()
	}

	status := replay.Status
	if status.Complete {
		return CompactAttemptResult{State: CompactStateComplete}, nil
	}
	if status.DecisionRequired {
		return compactBlocked(CompactBlockMaintainerDecision, ""), nil
	}
	if status.ActiveAttempt == nil {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	activeToken := replay.AttemptTokens[status.ActiveAttempt.Ordinal]
	if request.Token != activeToken {
		return compactBlocked(CompactBlockActiveAttempt, activeToken), nil
	}

	finish := FinishAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: request.RequestID, Outcome: request.Outcome,
		EvidenceRevision: request.EvidenceRevision, Diagnosis: request.Diagnosis,
		HarnessDisposition: request.HarnessDisposition, CleanupEvidence: request.CleanupEvidence,
		ProcessEvidence: request.ProcessEvidence,
	}
	explicitSuccessor := request.SuccessorLineageID != ""
	failedEvidence := status.EvidenceRevision
	if failedEvidence == "" {
		failedEvidence = request.RemediatesEvidenceRevision
	}
	if request.RemediatesEvidenceRevision != "" && failedEvidence != request.RemediatesEvidenceRevision {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	if request.Outcome == AttemptPassed && status.Binding != nil && failedEvidence != "" && (!store.ReviewDisabled || explicitSuccessor || request.RemediatesEvidenceRevision != "") {
		finish.ExpectedBindingRevision = status.Binding.Revision
		finish.SuccessorLineageID = request.SuccessorLineageID
		if finish.SuccessorLineageID == "" {
			finish.SuccessorLineageID = status.Binding.Lineage
		}
		finish.RemediatesEvidenceRevision = failedEvidence
	} else if explicitSuccessor || request.RemediatesEvidenceRevision != "" {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	if _, err := store.Finish(ctx, finish); err != nil {
		return store.compactMutationFailure(err, true, BeginAttemptRequest{}), nil
	}
	return store.compactSettleResult()
}

func normalizeCompactSettleRequest(request CompactSettleRequest) error {
	_, err := normalizeFinishAttemptRequest(FinishAttemptRequest{
		ExpectedRevision: request.Token, RequestID: request.RequestID, Outcome: request.Outcome,
		EvidenceRevision: request.EvidenceRevision, Diagnosis: request.Diagnosis,
		HarnessDisposition: request.HarnessDisposition, CleanupEvidence: request.CleanupEvidence,
		ProcessEvidence: request.ProcessEvidence,
	})
	if err != nil {
		return err
	}
	if request.SuccessorLineageID != "" && !validReviewBindingLineage(request.SuccessorLineageID) {
		return errors.New("successor_lineage_id must be a canonical lowercase lineage; rerun `gentle-ai sdd-attempt settle` with a lowercase --successor-lineage")
	}
	if request.RemediatesEvidenceRevision != "" && !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
		return errors.New("remediates_evidence_revision must be sha256; rerun `gentle-ai sdd-attempt settle` with --remediates-evidence-revision sha256:<64-lowercase-hex>")
	}
	return nil
}

func compactAcquireMatches(record runtimeRecord, request BeginAttemptRequest) bool {
	if (record.Operation != runtimeOperationBegin && record.Operation != runtimeOperationAdvance) || record.Begin == nil {
		return false
	}
	event := record.Begin
	return request == (BeginAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, WorkUnit: event.WorkUnit,
		EvidenceGoal: event.EvidenceGoal, MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
	})
}

func compactSettleReplayRequest(replay runtimeReplay, record runtimeRecord, request CompactSettleRequest) (FinishAttemptRequest, bool) {
	if record.Finish == nil || (record.Operation != runtimeOperationFinish && record.Operation != runtimeOperationFinishRemediation) {
		return FinishAttemptRequest{}, false
	}
	event := record.Finish
	finish := FinishAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Outcome: event.Outcome,
		EvidenceRevision: event.EvidenceRevision, Diagnosis: event.Diagnosis,
		HarnessDisposition: event.HarnessDisposition, CleanupEvidence: event.CleanupEvidence,
		ProcessEvidence: event.ProcessEvidence,
	}
	if record.Operation == runtimeOperationFinishRemediation {
		finish.ExpectedBindingRevision = record.Binding.ExpectedRevision
		finish.SuccessorLineageID = record.Binding.Current.Lineage
		finish.RemediatesEvidenceRevision = event.RemediatesEvidenceRevision
	}
	effectiveSuccessor := request.SuccessorLineageID
	if effectiveSuccessor == "" && record.Operation == runtimeOperationFinishRemediation {
		effectiveSuccessor = replay.Requests[record.RequestID].RemediationPredecessorLineage
	}
	matches := request.Token == replay.AttemptTokens[event.Ordinal] && request.RequestID == finish.RequestID &&
		request.Outcome == finish.Outcome && request.EvidenceRevision == finish.EvidenceRevision &&
		request.Diagnosis == finish.Diagnosis && request.HarnessDisposition == finish.HarnessDisposition &&
		request.CleanupEvidence == finish.CleanupEvidence && request.ProcessEvidence == finish.ProcessEvidence &&
		effectiveSuccessor == finish.SuccessorLineageID && request.RemediatesEvidenceRevision == finish.RemediatesEvidenceRevision
	return finish, matches
}

// compactAcquireBlock needs the request because completion is scoped to one
// objective: a passed apply is terminal for its own work unit while remaining an
// ordinary predecessor for the distinct verification the SDD graph still owes.
func compactAcquireBlock(replay runtimeReplay, request BeginAttemptRequest) (CompactAttemptResult, bool) {
	status := replay.Status
	switch {
	case status.Complete:
		if runtimeObjectiveAdvanceAdmissible(status, request) {
			return CompactAttemptResult{}, false
		}
		return CompactAttemptResult{State: CompactStateComplete}, true
	case status.DecisionRequired:
		return compactBlocked(CompactBlockMaintainerDecision, ""), true
	case status.ActiveAttempt != nil:
		return compactBlocked(CompactBlockActiveAttempt, replay.AttemptTokens[status.ActiveAttempt.Ordinal]), true
	default:
		return CompactAttemptResult{}, false
	}
}

func compactAcquireResult(replay runtimeReplay, request BeginAttemptRequest, ownedToken string) CompactAttemptResult {
	if result, terminal := compactAcquireBlock(replay, request); terminal {
		if result.Reason == CompactBlockActiveAttempt && result.Token == ownedToken {
			return CompactAttemptResult{State: CompactStateProceed, Token: ownedToken}
		}
		return result
	}
	return compactBlocked(CompactBlockInvalidContinuation, "")
}

func (store RuntimeStore) compactSettleResult(expected ...string) (CompactAttemptResult, error) {
	replay, err := store.load()
	if err != nil {
		return compactBlocked(CompactBlockCorruptAuthority, ""), nil
	}
	status := replay.Status
	if len(expected) == 1 && status.Revision != expected[0] {
		return compactBlocked(CompactBlockCorruptAuthority, ""), nil
	}
	switch {
	case status.Complete:
		return CompactAttemptResult{State: CompactStateComplete}, nil
	case status.DecisionRequired:
		return compactBlocked(CompactBlockMaintainerDecision, ""), nil
	case status.ActiveAttempt != nil:
		return compactBlocked(CompactBlockActiveAttempt, replay.AttemptTokens[status.ActiveAttempt.Ordinal]), nil
	default:
		return CompactAttemptResult{State: CompactStateProceed}, nil
	}
}

func (store RuntimeStore) compactMutationFailure(err error, settle bool, begin BeginAttemptRequest) CompactAttemptResult {
	var publication *RuntimePublicationError
	if errors.As(err, &publication) && publication.Committed {
		if settle {
			result, _ := store.compactSettleResult(publication.Revision)
			return result
		}
		replay, loadErr := store.load()
		if loadErr != nil {
			return compactBlocked(CompactBlockCorruptAuthority, "")
		}
		return compactAcquireResult(replay, begin, publication.Revision)
	}
	switch {
	case errors.Is(err, ErrRuntimeObjectiveDone):
		return CompactAttemptResult{State: CompactStateComplete}
	case errors.Is(err, ErrRuntimeBudgetExhausted), errors.Is(err, ErrRuntimeObjectiveChange):
		return compactBlocked(CompactBlockMaintainerDecision, "")
	case errors.Is(err, ErrRuntimeAttemptActive):
		return compactBlocked(CompactBlockActiveAttempt, "")
	case errors.Is(err, ErrRuntimeRevisionConflict), errors.Is(err, ErrRuntimeConcurrentUpdate),
		errors.Is(err, ErrRuntimeRequestConflict), errors.Is(err, ErrRuntimeNoActiveAttempt):
		return compactBlocked(CompactBlockInvalidContinuation, "")
	default:
		return compactBlocked(CompactBlockAuthorityFailure, "")
	}
}

func compactBlocked(reason CompactBlockReason, token string) CompactAttemptResult {
	return CompactAttemptResult{State: CompactStateBlocked, Reason: reason, Token: token}
}
