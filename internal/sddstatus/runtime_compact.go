package sddstatus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	runtimeAcquireRetryTimeout = 2 * time.Second
	runtimeAcquireRetryPoll    = 25 * time.Millisecond
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
	CompactBlockWorktreeMismatch    CompactBlockReason = "worktree_mismatch"
	CompactBlockAuthorityFailure    CompactBlockReason = "authority_failure"
	// CompactBlockCandidateUnavailable is the repository-side block: the
	// attempt authority is intact and unmutated, and what refused is the Git
	// capture of the candidate the mutation must record (#2114). It is
	// deliberately distinct from authority_failure, which now means only what
	// its name says.
	CompactBlockCandidateUnavailable CompactBlockReason = "candidate_unavailable"
	// CompactBlockRemediationUnsatisfiable is #2564's acquire-time fail-fast:
	// the caller declared a correction for failed evidence the immutable
	// attempt chain does not hold unremediated (nothing failed, the failure
	// was already corrected by a passed settlement, or a different revision
	// was declared), so the settlement that declaration promises is
	// structurally impossible and no token may be issued for it.
	CompactBlockRemediationUnsatisfiable CompactBlockReason = "remediation_unsatisfiable"
)

// CompactAttemptResult is the bounded orchestration projection. RuntimeStatus
// remains available through the legacy diagnostic operations only.
//
// Exit and Detail carry a wrapped mutation refusal's message text through to
// this compact boundary (#2249): compactMutationFailure populates both
// whenever it classifies a real error, so a well-constructed refusal that
// names a runnable continuation — like runtimeRemediationExitRefusal's — is
// never silently reduced to a bare Reason. Both stay empty on every happy
// path (proceed, complete-with-no-error).
type CompactAttemptResult struct {
	State  CompactAttemptState `json:"state"`
	Reason CompactBlockReason  `json:"reason,omitempty"`
	Token  string              `json:"token,omitempty"`
	Exit   string              `json:"exit,omitempty"`
	Detail string              `json:"detail,omitempty"`
	// SettleObligation names what this attempt's passing settle will already
	// be bound to, at the moment the token is issued rather than after the
	// work is done (#2912). An attempt is a bounded, spendable resource, and
	// every report in this class paid one to discover a demand acquire could
	// have named up front. It is never a block: the state stays proceed and
	// the token is real. Empty whenever the chain holds nothing, because a
	// field that is always populated is noise.
	SettleObligation string `json:"settle_obligation,omitempty"`
	// ItemSettlement is present only after a passed, item-bound settle reaches
	// this request's immutable terminal record.
	ItemSettlement *ItemSettlement `json:"item_settlement,omitempty"`
}

// ItemSettlement is the immutable authority for a coordinator to project one
// completed item into shared artifacts. It is not an artifact write or a Done
// claim: only the coordinator may perform and verify that projection.
type ItemSettlement struct {
	ItemID              string `json:"item_id"`
	WorkUnit            string `json:"work_unit"`
	ObjectiveID         string `json:"objective_id"`
	ObjectiveGeneration int    `json:"objective_generation"`
	AttemptOrdinal      int    `json:"attempt_ordinal"`
	EvidenceRevision    string `json:"evidence_revision"`
	SettlementRequestID string `json:"settlement_request_id"`
}

// CompactAcquireRequest is the bounded orchestration projection of
// BeginAttemptRequest. Work-unit scope (WorkUnit, EvidenceGoal, MaxAttempts,
// MaxChangedLines) is owned exactly once, by the embedded BeginAttemptRequest
// (decision 9 / CON-08: RuntimeObjective's BeginAttemptRequest is the single
// work-unit-scope owner). ExpectedRevision is inert here — Acquire always
// derives it from ledger replay state before use — but embedding rather than
// re-declaring keeps the projection from drifting out of sync with its one
// source of truth the way the pre-Wave-4 parallel struct did (#2133/#2151).
type CompactAcquireRequest struct {
	BeginAttemptRequest

	// Token is optional ownership proof (#2291): a distinct call/process
	// (e.g. an actor launched by a parent that already holds a proceed-state
	// acquire) presents the token that acquire already returned to prove it
	// is continuing that SAME attempt rather than colliding with it. A Token
	// matching the ledger's currently active attempt short-circuits to
	// proceed with zero mutation — no store.Begin, no ledger chain touched.
	// A non-matching Token falls through to the ordinary active_attempt
	// block, naming the REAL active token. An empty Token leaves every
	// existing acquire/collide path byte-for-byte unchanged.
	Token string

	// RemediatesEvidenceRevision declares at acquire the same correction
	// intent Settle expresses through --remediates-evidence-revision (#2564).
	// Before this, remediation intent was settle-only: an acquire whose
	// eventual unmanaged settlement was already structurally unsatisfiable
	// (no unremediated failed evidence in the immutable attempt chain, or a
	// different revision declared) still returned proceed, and the refusal
	// only arrived after the correction work was done. Empty leaves every
	// existing acquire path unchanged.
	RemediatesEvidenceRevision string
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

type CompactHandoffRequest struct {
	HandoffAttemptRequest
}

func runtimeAttemptTargetForToken(replay runtimeReplay, token string) (runtimeAttemptTarget, RuntimeAttempt, bool) {
	ordinal, found := 0, false
	for candidate, candidateToken := range replay.AttemptTokens {
		if candidateToken == token {
			if found {
				return runtimeAttemptTarget{}, RuntimeAttempt{}, false
			}
			ordinal, found = candidate, true
		}
	}
	if !found {
		return runtimeAttemptTarget{}, RuntimeAttempt{}, false
	}
	var resolved *RuntimeAttempt
	for index := range replay.Status.Attempts {
		attempt := &replay.Status.Attempts[index]
		if attempt.Ordinal == ordinal {
			if resolved != nil {
				return runtimeAttemptTarget{}, RuntimeAttempt{}, false
			}
			owner := replay.Status.ownership.objectives[attempt.ObjectiveID]
			if replay.Status.ownership.objectives != nil && (owner == nil || owner.objective.Generation != attempt.ObjectiveGeneration) {
				return runtimeAttemptTarget{}, RuntimeAttempt{}, false
			}
			resolved = attempt
		}
	}
	if resolved == nil {
		return runtimeAttemptTarget{}, RuntimeAttempt{}, false
	}
	return runtimeAttemptTarget{ObjectiveID: resolved.ObjectiveID, ObjectiveGeneration: resolved.ObjectiveGeneration, Ordinal: ordinal, Token: token}, *resolved, true
}

func runtimeActiveSettleTarget(replay runtimeReplay, token string) (runtimeAttemptTarget, RuntimeAttempt, bool) {
	target, attempt, ok := runtimeAttemptTargetForToken(replay, token)
	active := replay.Status.runtimeActiveAttemptForOrdinal(target.Ordinal)
	return target, attempt, ok && active != nil && active.Ordinal == target.Ordinal && active.ObjectiveID == target.ObjectiveID && active.ObjectiveGeneration == target.ObjectiveGeneration
}

// runtimeReadinessInput is everything the one readiness predicate reads. It
// carries the whole AttemptTokens map rather than a pre-resolved token so the
// predicate stays the only code that inspects the readiness triple; a caller
// that had to resolve the active attempt's ordinal first would be deciding a
// little bit of the answer on its own, which is the drift this collapses.
//
// Request and PresentedToken are optional. A caller that names neither (status,
// and Settle's post-mutation projection) gets the request-blind, token-blind
// answer, which is exactly what it is entitled to state.
type runtimeReadinessInput struct {
	Status         RuntimeStatus
	AttemptTokens  map[int]string
	Request        BeginAttemptRequest
	PresentedToken string
}

// runtimeReadiness answers "may this work proceed?" exactly once, for every
// consumer. It reports the compact result plus whether that result is terminal;
// a non-terminal answer means nothing blocks, and each caller then does its own
// thing with that permission (Acquire begins an attempt and mints a token,
// Settle reports proceed, status leaves routing to the artifacts).
//
// Before this, three call sites derived the same verdict separately and
// disagreed: compactAcquireBlock (request-aware), compactSettleResult
// (request-blind), and status's applyNativeRuntimeRouting, which was
// request-blind AND token-blind and asserted acquire's answer in a hand-written
// string it never checked. #2463 is that string being wrong: acquire returned
// proceed and handed back a token, and status reported the very same attempt as
// an active blocker whose "external execution" could only be settled, for an
// execution the caller was about to launch.
//
// The ordering is the ledger's own. applyRuntimeFinishEvent sets exactly one of
// Complete or DecisionRequired and clears ActiveAttempt in both branches, so
// checking Complete first is not a precedence choice among reachable states.
func runtimeReadiness(in runtimeReadinessInput) (CompactAttemptResult, bool) {
	if in.Status.validateRuntimeLineage() != nil {
		return compactBlocked(CompactBlockCorruptAuthority, ""), true
	}
	active := in.Status.runtimeActiveAttempt()
	activeToken := ""
	if active != nil {
		activeToken = in.AttemptTokens[active.Ordinal]
	}

	// Zero-mutation ownership check (#2291): a distinct call or process launched
	// by a parent that already holds a proceed-state acquire presents that exact
	// token to prove it continues the SAME attempt rather than colliding with
	// it. A non-matching token falls to the ordinary block naming the REAL
	// active token. An empty token leaves every other path unchanged.
	if in.PresentedToken != "" {
		for ordinal, token := range in.AttemptTokens {
			if token == in.PresentedToken && in.Status.runtimeActiveAttemptForOrdinal(ordinal) != nil {
				return CompactAttemptResult{State: CompactStateProceed, Token: token}, true
			}
		}
		return compactForeignAcquireToken(activeToken), true
	}

	switch {
	case in.Status.Complete:
		// Completion is scoped to one objective: a passed apply is terminal for
		// its own work unit while remaining an ordinary predecessor for the
		// distinct verification the SDD graph still owes. A caller that names no
		// work unit has named no successor scope, so completion stays terminal
		// for it.
		if in.Request.WorkUnit != "" && runtimeObjectiveAdvanceAdmissible(in.Status, in.Request) {
			return CompactAttemptResult{}, false
		}
		return CompactAttemptResult{State: CompactStateComplete}, true
	case in.Status.DecisionRequired:
		return compactBlocked(CompactBlockMaintainerDecision, ""), true
	case in.Status.runtimeActiveCount() > 0:
		if active == nil {
			_, active = in.Status.runtimeCompatibilityActive()
			activeToken = in.AttemptTokens[active.Ordinal]
		}
		return compactBlocked(CompactBlockActiveAttempt, activeToken), true
	default:
		return CompactAttemptResult{}, false
	}
}

// Acquire claims one native attempt without exposing the growing runtime
// history. The returned token identifies that exact begin record for Settle.
func (store RuntimeStore) Acquire(ctx context.Context, request CompactAcquireRequest) (CompactAttemptResult, error) {
	begin, err := normalizeBeginAttemptRequest(request.BeginAttemptRequest)
	if err != nil {
		return CompactAttemptResult{}, err
	}
	if request.RemediatesEvidenceRevision != "" && !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
		return CompactAttemptResult{}, errors.New("remediates_evidence_revision must be sha256; rerun `gentle-ai sdd-attempt acquire` with --remediates-evidence-revision sha256:<64-lowercase-hex>")
	}
	timeout, poll := store.runtimeAcquireRetryTimeout(), store.runtimeAcquireRetryPoll()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		result, err := store.acquireOnce(ctx, request, begin)
		if store.acquireAttemptObserved != nil {
			store.acquireAttemptObserved(err)
		}
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, reviewtransaction.ErrStoreLockContended) {
			return store.compactMutationFailure(err, false, begin), nil
		}
		select {
		case <-ctx.Done():
			return CompactAttemptResult{}, ctx.Err()
		case <-deadline.C:
			return store.compactMutationFailure(fmt.Errorf("%w: acquire retry timed out after %s: %w", ErrRuntimeConcurrentUpdate, timeout, reviewtransaction.ErrStoreLockContended), false, begin), nil
		case <-ticker.C:
		}
	}
}

// acquireOnce starts from a new replay on every call. A retry can therefore
// never reuse a stale revision, candidate snapshot, or concurrent admission.
func (store RuntimeStore) acquireOnce(ctx context.Context, request CompactAcquireRequest, begin BeginAttemptRequest) (CompactAttemptResult, error) {
	replay, err := store.load()
	if err != nil {
		return compactBlockedByUnreadableAuthority(err), nil
	}
	if receipt, exists := replay.Requests[begin.RequestID]; exists {
		record, loadErr := store.loadRecord(receipt.Revision)
		if loadErr != nil {
			return compactBlockedByUnreadableAuthority(loadErr), nil
		}
		begin.ExpectedRevision = record.PreviousRevision
		if !compactAcquireMatches(record, begin) {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		if _, err := store.Begin(ctx, begin); err != nil {
			return CompactAttemptResult{}, err
		}
		current, loadErr := store.load()
		if loadErr != nil {
			return compactBlockedByUnreadableAuthority(loadErr), nil
		}
		return compactAcquireResult(current, begin, receipt.Revision), nil
	}

	if result, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: replay.Status, AttemptTokens: replay.AttemptTokens,
		Request: begin, PresentedToken: request.Token,
	}); terminal {
		if result.Reason == CompactBlockActiveAttempt && runtimeConcurrentItemAdmissible(replay, begin) {
			terminal = false
		} else {
			if result.State == CompactStateProceed {
				result.SettleObligation = runtimeSettleObligation(replay.Status, store.ReviewDisabled)
			}
			return result, nil
		}
	}
	// #2564 fail-fast: a declared unmanaged correction whose settlement is
	// already structurally unsatisfiable earns its typed refusal HERE, before
	// any token is issued and before any correction work runs. Satisfiability
	// follows the chain-derived binding (#2565): the refusal fires only when
	// the immutable attempt chain holds no unremediated failed evidence
	// matching the declaration, so an acquire after an audited reset stays
	// legitimate while the chain still binds. Scoped to the unmanaged regime
	// (review disabled, no binding): with review enabled or a binding present
	// the settle routes through managed remediation, whose authority can
	// still materialize during the attempt.
	if request.RemediatesEvidenceRevision != "" && store.ReviewDisabled && replay.Status.Binding == nil &&
		!unmanagedRemediationSettleable(replay.Status, request.RemediatesEvidenceRevision) {
		return compactBlocked(CompactBlockRemediationUnsatisfiable, ""), nil
	}
	begin.ExpectedRevision = replay.Status.Revision
	started, err := store.Begin(ctx, begin)
	if err != nil {
		return CompactAttemptResult{}, err
	}
	return CompactAttemptResult{
		State: CompactStateProceed, Token: started.Revision,
		// Derived from the PRE-mutation chain: the obligation this attempt
		// inherits is the one that existed when it was opened.
		SettleObligation: runtimeSettleObligation(replay.Status, store.ReviewDisabled),
	}, nil
}

func (store RuntimeStore) runtimeAcquireRetryTimeout() time.Duration {
	if store.acquireRetryTimeout > 0 {
		return store.acquireRetryTimeout
	}
	return runtimeAcquireRetryTimeout
}

func (store RuntimeStore) runtimeAcquireRetryPoll() time.Duration {
	if store.acquireRetryPoll > 0 {
		return store.acquireRetryPoll
	}
	return runtimeAcquireRetryPoll
}

// Settle closes the attempt selected by Token through the ordinary Finish
// transition. Current binding and failed-evidence revisions are derived inside
// the authority; callers name a successor only when review approved a distinct
// lineage.
func (store RuntimeStore) Settle(ctx context.Context, request CompactSettleRequest) (CompactAttemptResult, error) {
	replay, err := store.load()
	if err != nil {
		return compactBlockedByUnreadableAuthority(err), nil
	}
	if receipt, exists := replay.Requests[request.RequestID]; exists {
		record, loadErr := store.loadRecord(receipt.Revision)
		if loadErr != nil {
			return compactBlockedByUnreadableAuthority(loadErr), nil
		}
		finish, ok := compactSettleReplayRequest(replay, record, request)
		if !ok {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		target, _, targetOK := runtimeAttemptTargetForToken(replay, request.Token)
		if !targetOK {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		if _, err := store.finishTarget(ctx, finish, target); err != nil {
			return store.compactMutationFailure(err, true, BeginAttemptRequest{}), nil
		}
		return store.compactSettleResult(&request)
	}
	if err := normalizeCompactSettleRequest(request); err != nil {
		return CompactAttemptResult{}, err
	}

	// Settle asks the same predicate the same question and interprets the same
	// answer for its own purpose: a proceed means this token owns the live
	// attempt and may close it, and a non-terminal answer means there is no
	// active attempt to close at all.
	readiness, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: replay.Status, AttemptTokens: replay.AttemptTokens, PresentedToken: request.Token,
	})
	if !terminal {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	if readiness.State != CompactStateProceed {
		return readiness, nil
	}
	target, _, ok := runtimeActiveSettleTarget(replay, request.Token)
	if !ok {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}

	status := replay.Status
	finish := FinishAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: request.RequestID, Outcome: request.Outcome,
		EvidenceRevision: request.EvidenceRevision, Diagnosis: request.Diagnosis,
		HarnessDisposition: request.HarnessDisposition, CleanupEvidence: request.CleanupEvidence,
		ProcessEvidence: request.ProcessEvidence,
	}
	explicitSuccessor := request.SuccessorLineageID != ""
	failedEvidence, _ := runtimeLineageFailedEvidence(status)
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
	} else if store.ReviewDisabled && !explicitSuccessor && request.RemediatesEvidenceRevision != "" {
		finish.RemediatesEvidenceRevision = request.RemediatesEvidenceRevision
	} else if explicitSuccessor || request.RemediatesEvidenceRevision != "" {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	if _, err := store.finishTarget(ctx, finish, target); err != nil {
		result := store.compactMutationFailure(err, true, BeginAttemptRequest{})
		if result.State == CompactStateComplete {
			return store.compactSettleResult(&request)
		}
		return result, nil
	}
	return store.compactSettleResult(&request)
}

func (store RuntimeStore) HandoffCompact(ctx context.Context, request CompactHandoffRequest) (CompactAttemptResult, error) {
	_, err := store.Handoff(ctx, request.HandoffAttemptRequest)
	if err == nil {
		return CompactAttemptResult{State: CompactStateProceed}, nil
	}
	var publication *RuntimePublicationError
	if errors.As(err, &publication) && publication.Committed {
		return CompactAttemptResult{State: CompactStateProceed}, nil
	}
	return store.compactMutationFailure(err, false, BeginAttemptRequest{}), nil
}

// unmanagedRemediationSettleable reports whether a settle carrying
// --remediates-evidence-revision failedEvidence can structurally succeed
// against this ledger state: the immutable attempt chain must still hold that
// exact failed evidence unremediated, per runtimeLineageFailedEvidence, the
// same chain-derived binding Finish's unmanaged guard enforces (#1974 slice
// 2). A changed candidate and fresh distinct evidence remain settle-time
// facts and are not judged here; nor is "may this work proceed?", which
// stays runtimeReadiness's question alone -- this reads only the immutable
// attempt chain.
func unmanagedRemediationSettleable(status RuntimeStatus, failedEvidence string) bool {
	chainEvidence, chainHasFailedEvidence := runtimeLineageFailedEvidence(status)
	return chainHasFailedEvidence && failedEvidence != "" && chainEvidence == failedEvidence
}

func normalizeCompactSettleRequest(request CompactSettleRequest) error {
	if request.Outcome == AttemptInterrupted && request.EvidenceRevision != "" {
		return errors.New("interrupted evidence_revision must be empty; rerun `gentle-ai sdd-attempt settle` without --evidence-revision")
	}
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
	expected := BeginAttemptRequest{ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, WorkUnit: event.WorkUnit,
		EvidenceGoal: event.EvidenceGoal, MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
		ItemID: event.ItemID, ItemEditRoots: event.ItemEditRoots}
	return request.ExpectedRevision == expected.ExpectedRevision && request.RequestID == expected.RequestID &&
		request.WorkUnit == expected.WorkUnit && request.EvidenceGoal == expected.EvidenceGoal &&
		request.MaxAttempts == expected.MaxAttempts && request.MaxChangedLines == expected.MaxChangedLines &&
		runtimeItemBindingEqual(request.ItemID, request.ItemEditRoots, expected.ItemID, expected.ItemEditRoots) &&
		((request.itemPlan == nil && event.ItemPlanDigest == "") ||
			(request.itemPlan != nil && request.itemPlan.Plan.Digest == event.ItemPlanDigest))
}

func compactSettleReplayRequest(replay runtimeReplay, record runtimeRecord, request CompactSettleRequest) (FinishAttemptRequest, bool) {
	if record.Finish == nil || (record.Operation != runtimeOperationFinish && record.Operation != runtimeOperationFinishRemediation) {
		return FinishAttemptRequest{}, false
	}
	event := record.Finish
	target, attempt, targetOK := runtimeAttemptTargetForToken(replay, request.Token)
	if !targetOK || target.Ordinal != event.Ordinal || attempt.ObjectiveID != target.ObjectiveID || attempt.ObjectiveGeneration != target.ObjectiveGeneration {
		return FinishAttemptRequest{}, false
	}
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
	matches := request.Token == target.Token && request.RequestID == finish.RequestID &&
		request.Outcome == finish.Outcome && request.EvidenceRevision == finish.EvidenceRevision &&
		request.Diagnosis == finish.Diagnosis && request.HarnessDisposition == finish.HarnessDisposition &&
		request.CleanupEvidence == finish.CleanupEvidence && request.ProcessEvidence == finish.ProcessEvidence &&
		effectiveSuccessor == finish.SuccessorLineageID && request.RemediatesEvidenceRevision == finish.RemediatesEvidenceRevision
	return finish, matches
}

// compactAcquireResult reconciles a committed begin whose publication the
// caller could not observe. The already-committed record's revision IS the
// caller's ownership proof, so it presents that token to the same predicate
// rather than re-deriving the active-attempt comparison here.
func compactAcquireResult(replay runtimeReplay, request BeginAttemptRequest, ownedToken string) CompactAttemptResult {
	if result, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: replay.Status, AttemptTokens: replay.AttemptTokens,
		Request: request, PresentedToken: ownedToken,
	}); terminal {
		return result
	}
	return compactBlocked(CompactBlockInvalidContinuation, "")
}

func (store RuntimeStore) compactSettleResult(request *CompactSettleRequest, expected ...string) (CompactAttemptResult, error) {
	replay, err := store.load()
	if err != nil {
		return compactBlockedByUnreadableAuthority(err), nil
	}
	if len(expected) == 1 && replay.Status.Revision != expected[0] {
		return compactBlocked(CompactBlockCorruptAuthority, ""), nil
	}
	var settlement *ItemSettlement
	if request != nil {
		settlement = compactItemSettlement(replay, *request)
	}
	if result, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: replay.Status, AttemptTokens: replay.AttemptTokens,
	}); terminal {
		if settlement != nil {
			// A v2 retained plan is globally joined only after the coordinator
			// projects every settlement into tasks. Compact Settle cannot observe
			// that write, so it must never claim terminal completion here.
			if replay.itemPlan != nil && replay.itemPlan.Version == itemPlanVersionV2 {
				return CompactAttemptResult{State: CompactStateProceed, ItemSettlement: settlement}, nil
			}
			if result.State == CompactStateComplete {
				result.ItemSettlement = settlement
				return result, nil
			}
			return CompactAttemptResult{State: CompactStateProceed, ItemSettlement: settlement}, nil
		}
		return result, nil
	}
	if settlement != nil {
		return CompactAttemptResult{State: CompactStateProceed, ItemSettlement: settlement}, nil
	}
	return CompactAttemptResult{State: CompactStateProceed}, nil
}

func compactItemSettlement(replay runtimeReplay, request CompactSettleRequest) *ItemSettlement {
	status := replay.Status
	if request.Outcome != AttemptPassed || len(status.Attempts) == 0 {
		return nil
	}
	target, attempt, ok := runtimeAttemptTargetForToken(replay, request.Token)
	if !ok {
		return nil
	}
	objective := status.runtimeObjectiveForAttempt(&attempt)
	if objective == nil || attempt.Outcome != AttemptPassed || attempt.ItemID == "" || attempt.EvidenceRevision != request.EvidenceRevision ||
		attempt.ObjectiveID != objective.ID || attempt.ObjectiveGeneration != objective.Generation || attempt.WorkUnit != objective.WorkUnit ||
		attempt.ItemID != objective.ItemID || target.ObjectiveID != objective.ID || target.ObjectiveGeneration != objective.Generation {
		return nil
	}
	return &ItemSettlement{ItemID: attempt.ItemID, WorkUnit: attempt.WorkUnit, ObjectiveID: attempt.ObjectiveID,
		ObjectiveGeneration: attempt.ObjectiveGeneration, AttemptOrdinal: attempt.Ordinal, EvidenceRevision: attempt.EvidenceRevision,
		SettlementRequestID: request.RequestID}
}

func (store RuntimeStore) compactMutationFailure(err error, settle bool, begin BeginAttemptRequest) CompactAttemptResult {
	var publication *RuntimePublicationError
	if errors.As(err, &publication) && publication.Committed {
		if settle {
			result, _ := store.compactSettleResult(nil, publication.Revision)
			return result
		}
		replay, loadErr := store.load()
		if loadErr != nil {
			return compactBlockedByUnreadableAuthority(loadErr)
		}
		return compactAcquireResult(replay, begin, publication.Revision)
	}
	// Every branch below wraps a real error, so `detail` always has message
	// text to carry through to the compact JSON boundary instead of being
	// thrown away behind a bare classification (#2249). `exit` reuses the
	// same text: these refusals are constructed specifically to name a
	// runnable continuation inline (see runtimeRemediationExitRefusal), and
	// there is no reliable field-agnostic way to shorten that further.
	detail := err.Error()
	reason := CompactBlockAuthorityFailure
	switch {
	case errors.Is(err, ErrRuntimeObjectiveDone):
		return CompactAttemptResult{State: CompactStateComplete, Exit: detail, Detail: detail}
	case errors.Is(err, ErrRuntimeBudgetExhausted), errors.Is(err, ErrRuntimeObjectiveChange):
		reason = CompactBlockMaintainerDecision
	case errors.Is(err, ErrRuntimeAttemptActive):
		reason = CompactBlockActiveAttempt
	case errors.Is(err, ErrRuntimeRevisionConflict), errors.Is(err, ErrRuntimeConcurrentUpdate),
		errors.Is(err, ErrRuntimeRequestConflict), errors.Is(err, ErrRuntimeNoActiveAttempt),
		errors.Is(err, ErrBindingRevisionConflict):
		reason = CompactBlockInvalidContinuation
	// ErrRuntimeWorktreeMismatch is the sentinel behind
	// runtimeWorktreeMismatchRefusal (#2296 part 1): Finish is running from a
	// different linked worktree than the one Begin recorded. Left
	// unclassified it would fall through to the opaque default
	// authority_failure and lose the exact --cwd the refusal names.
	case errors.Is(err, ErrRuntimeWorktreeMismatch):
		reason = CompactBlockWorktreeMismatch
	// ErrRuntimeCandidateUnavailable is the sentinel behind both of
	// Begin/Finish's candidate captures (#2114), reachable on the very first
	// acquire of a brand-new change, where authority_failure is doubly wrong:
	// nothing about the authority failed, and the consumer had no prior state
	// to inspect and no continuation to run.
	case errors.Is(err, ErrRuntimeCandidateUnavailable):
		reason = CompactBlockCandidateUnavailable
	case errors.Is(err, ErrRuntimeHandoffSource):
		reason = CompactBlockWorktreeMismatch
	case errors.Is(err, ErrRuntimeHandoffDestination), errors.Is(err, ErrRuntimeHandoffAlreadyPerformed):
		reason = CompactBlockInvalidContinuation
	}
	return CompactAttemptResult{State: CompactStateBlocked, Reason: reason, Exit: detail, Detail: detail}
}

// compactBlockedReviewModeDisableExit is gone with its last caller (#2913).
// It offered `review mode disable --scope clone` as the way out of SDD attempt
// blockers. It is a real delivery fallback, but it is not an exit from any of
// these: every reason below blocks the SDD attempt ledger, and receipt-driven
// review has no authority over whether a work unit may open. A reporter ran it,
// watched it succeed, and found the block untouched.

// compactBlockedExitText names the runnable continuation for every reason
// compactBlocked itself is ever called with (exit-naming audit fix #2):
// before this, all 20 of this file's compactBlocked call sites shipped a
// bare {"state":"blocked","reason":"<code>"} with nothing behind it — no
// Exit, no Detail, and (unlike next_transition's stop reason codes) no
// stderr narration or docs mirror either.
func compactBlockedExitText(reason CompactBlockReason, token string) string {
	switch reason {
	case CompactBlockCorruptAuthority:
		return "the attempt ledger for this work unit cannot be read as valid authority; run " +
			"`gentle-ai sdd-attempt status --cwd <repo> --change <change>` to see what is readable, " +
			"then ask a maintainer to inspect the SDD runtime authority under the Git common directory"
	case CompactBlockInvalidContinuation:
		return "this call does not continue the attempt currently on record; run " +
			"`gentle-ai sdd-attempt status --cwd <repo> --change <change>` to see the live attempt and its " +
			"current revision, then reissue this call against that state"
	case CompactBlockMaintainerDecision:
		// #2530: this said "rescope or reset", and rescope is structurally
		// refused for exactly this state — runtimeObjectiveRescopeStructurally
		// Permitted returns false whenever DecisionRequired is set, which is
		// the only way this block is reached. Reset is the admitted one, and
		// the ledger's own next_action has said so all along.
		//
		// #2913: this also offered `review mode disable --scope clone` as an
		// exit. A reporter ran it, it SUCCEEDED, effective mode went to off,
		// and this block was exactly where they left it — then status told
		// them to run it again. The command could never have cleared this.
		// Receipt-driven review governs DELIVERY of a finished change; it has
		// no authority over whether an SDD work unit may open, so turning it
		// off cannot open one. Reset is the whole exit, and it is named here
		// as a complete command instead of as advice.
		return "this work unit's attempt or changed-line budget needs a maintainer decision; run " +
			"`gentle-ai sdd-attempt status --cwd <repo> --change <change>` for the accounting, then have a " +
			"maintainer reset the objective with `gentle-ai sdd-attempt reset --cwd <repo> --change <change> " +
			"--expected-revision <the revision that status prints> --request-id \"<unique-request-id>\" " +
			"--reason \"<why-the-objective-is-being-reset>\" --actor \"<actor>\"`; turning receipt-driven " +
			"review off does not clear this, because review governs delivery of a finished change, not " +
			"whether a work unit may open"
	case CompactBlockActiveAttempt:
		// Adversarial finding F2: the bare `sdd-attempt acquire --token <t>`
		// / `settle --token <t>` forms are not complete commands -- each
		// needs five more required flags (verified by execution: acquire
		// additionally requires --cwd, --change, --request-id, --work-unit,
		// --evidence-goal; settle additionally requires --cwd, --change,
		// --request-id, --outcome, --evidence-revision, --diagnosis,
		// --harness-disposition, --cleanup-evidence, --process-evidence).
		// Only `gentle-ai sdd-attempt status --cwd <repo> --change <change>`
		// is named as a complete command; the token is described as an
		// addition to the caller's own already-in-flight acquire/settle
		// call, never as a standalone invocation.
		return "a distinct attempt token " + token + " is already active for this work unit; run " +
			"`gentle-ai sdd-attempt status --cwd <repo> --change <change>` to see it, then add `--token " + token +
			"` to your own `sdd-attempt acquire` call to continue that exact attempt, or to your " +
			"`sdd-attempt settle` call to close it before starting a new one"
	case CompactBlockRemediationUnsatisfiable:
		return "this acquire declares a correction for failed evidence the attempt chain does not hold unremediated (nothing failed, the failure was already corrected by a passed settlement, or the declared revision differs from the chain's), so its settle could never succeed and no token is issued; run " +
			"`gentle-ai sdd-attempt status --cwd <repo> --change <change>` to read the attempt chain and its most recent unremediated failed evidence, then either reissue this acquire declaring that exact revision, or drop --remediates-evidence-revision and continue through a fresh verification objective whose own failed settlement records new evidence a bounded correction can name"
	default:
		return ""
	}
}

// compactBlockedByUnreadableAuthority is compactBlocked for the one reason
// whose typed cause was being discarded at every site.
//
// #2829 / root 8's shape on a surface root 8 did not cover: seven call sites
// wrote `compactBlocked(CompactBlockCorruptAuthority, "")`, throwing away the
// error `store.load()` had just returned. The operator, and anyone triaging
// their report, then saw "the attempt ledger cannot be read as valid
// authority" with no way to tell an invalid HEAD encoding from a broken chain
// link from a permissions refusal. Those need different repairs and looked
// identical.
//
// Exit text is unchanged: this appends the cause to Detail only, so the
// runnable continuation a caller routes on keeps its exact bytes.
func compactBlockedByUnreadableAuthority(cause error) CompactAttemptResult {
	result := compactBlocked(CompactBlockCorruptAuthority, "")
	var repair *runtimeConsecutiveRescopeRepairRequiredError
	if errors.As(cause, &repair) {
		result.Exit = repair.Error()
		result.Detail = repair.Error()
		return result
	}
	if cause != nil {
		result.Detail = result.Detail + " (cause: " + cause.Error() + ")"
	}
	return result
}

// runtimeSettleObligation is the ONE derivation of "what will this attempt's
// passing settle already owe". Acquire and the read-only admission surface
// both call it; neither computes it alongside the other. That is #2114's
// lesson applied before the fact — two derivations of one truth drift, and the
// surface that speaks earliest is the one that ends up lying.
//
// It reads the same chain-derived binding the settle itself enforces
// (runtimeLineageFailedAttempt, #1974 slice 2 / #2565), so the notice cannot
// promise something the settle will not demand, or stay silent about
// something it will.
func runtimeSettleObligation(status RuntimeStatus, reviewDisabled bool) string {
	if !reviewDisabled || status.Binding != nil {
		return ""
	}
	failed, ok := runtimeLineageFailedAttempt(status)
	if !ok || failed.EvidenceRevision == "" {
		return ""
	}
	return "this attempt's passing settle is already bound to the chain's unremediated failed verification " +
		failed.EvidenceRevision + ": settle it passed with `--remediates-evidence-revision \"" + failed.EvidenceRevision +
		"\"`, and with verification evidence distinct from it, over a correction candidate that no longer matches the state that failed. " +
		"An audited reset or an interrupted settlement between that failure and this correction does not release the " +
		"binding — only a passing settlement that names it does."
}

func compactBlocked(reason CompactBlockReason, token string) CompactAttemptResult {
	exit := compactBlockedExitText(reason, token)
	return CompactAttemptResult{State: CompactStateBlocked, Reason: reason, Token: token, Exit: exit, Detail: exit}
}

// compactForeignAcquireToken names the exact continuation for a losing
// ownership check (#2291): the caller presented a token, but it is not the
// ledger's live active attempt. It always carries the REAL active token —
// never the foreign one the caller supplied — through Token, Exit, and
// Detail alike, so a legible refusal (slice 1) also names how to proceed.
func compactForeignAcquireToken(activeToken string) CompactAttemptResult {
	return compactBlocked(CompactBlockActiveAttempt, activeToken)
}
