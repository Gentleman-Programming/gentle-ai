package sddstatus

import (
	"context"
	"errors"
	"slices"
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
	// CompactBlockUndeclaredUntracked is the settlement-side untracked ruling
	// block (#3881): the attempt authority is intact and unmutated, and what
	// refused is the settlement's untracked declaration -- missing for files
	// the attempt itself created (#3806), stale, naming an ineligible path,
	// narrowing a begin selection, or offered to a pre-inventory legacy
	// record. Every exit is a corrected rerun of settle/finish, so a consumer
	// routing on this reason continues, where authority_failure would tell it
	// to stop.
	CompactBlockUndeclaredUntracked CompactBlockReason = "undeclared_untracked"
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
	// eventual failed-evidence settlement was already structurally unsatisfiable
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

	RemediatesEvidenceRevision string

	// The settle-time untracked declaration, forwarded verbatim to Finish.
	// Nil means the caller declared nothing.
	IntendedUntracked          *[]string
	ExpectedUntrackedInventory string
}

type CompactHandoffRequest struct {
	HandoffAttemptRequest
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
	activeToken := ""
	if in.Status.ActiveAttempt != nil {
		activeToken = in.AttemptTokens[in.Status.ActiveAttempt.Ordinal]
	}

	// Zero-mutation ownership check (#2291): a distinct call or process launched
	// by a parent that already holds a proceed-state acquire presents that exact
	// token to prove it continues the SAME attempt rather than colliding with
	// it. A non-matching token falls to the ordinary block naming the REAL
	// active token. An empty token leaves every other path unchanged.
	if in.PresentedToken != "" && in.Status.ActiveAttempt != nil {
		if in.PresentedToken == activeToken {
			return CompactAttemptResult{State: CompactStateProceed, Token: activeToken}, true
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
	case in.Status.ActiveAttempt != nil:
		return compactBlocked(CompactBlockActiveAttempt, activeToken), true
	default:
		return CompactAttemptResult{}, false
	}
}

// Acquire claims one native attempt without exposing the growing runtime
// history. The returned token identifies that exact begin record for Settle.
func (store RuntimeStore) Acquire(ctx context.Context, request CompactAcquireRequest) (CompactAttemptResult, error) {
	inheritIntendedUntracked := request.IntendedUntracked == nil
	begin, err := normalizeBeginAttemptRequest(request.BeginAttemptRequest)
	if err != nil {
		return CompactAttemptResult{}, err
	}
	if request.RemediatesEvidenceRevision != "" && !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
		return CompactAttemptResult{}, errors.New("remediates_evidence_revision must be sha256; rerun `gentle-ai sdd-attempt acquire` with --remediates-evidence-revision sha256:<64-lowercase-hex>")
	}

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
		if inheritIntendedUntracked && record.Begin != nil {
			begin.IntendedUntracked = nil
			if record.Begin.IntendedUntracked != nil {
				begin.IntendedUntracked = slices.Clone(*record.Begin.IntendedUntracked)
			}
		}
		if !compactAcquireMatches(record, begin, inheritIntendedUntracked) {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		if request.Token != "" && request.Token != receipt.Revision {
			return compactBlocked(CompactBlockInvalidContinuation, ""), nil
		}
		return compactAcquireResult(replay, begin, receipt.Revision), nil
	}
	begin = runtimeRescopeSuccessorRequest(replay.Status, begin, inheritIntendedUntracked)

	if result, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: replay.Status, AttemptTokens: replay.AttemptTokens,
		Request: begin, PresentedToken: request.Token,
	}); terminal {
		if result.State == CompactStateProceed {
			result.SettleObligation = runtimeSettleObligation(replay.Status)
		}
		return result, nil
	}
	// A declared correction must be structurally settleable before it spends an
	// attempt. Satisfiability is derived only from the immutable failed-evidence
	// chain, so an audited reset remains a legitimate predecessor.
	if request.RemediatesEvidenceRevision != "" &&
		!failedEvidenceRemediationSettleable(replay.Status, request.RemediatesEvidenceRevision) {
		return compactBlocked(CompactBlockRemediationUnsatisfiable, ""), nil
	}
	begin.ExpectedRevision = replay.Status.Revision
	started, err := store.Begin(ctx, begin)
	if err != nil {
		return store.compactMutationFailure(err, false, begin), nil
	}
	return CompactAttemptResult{
		State: CompactStateProceed, Token: started.Revision,
		// Derived from the PRE-mutation chain: the obligation this attempt
		// inherits is the one that existed when it was opened.
		SettleObligation: runtimeSettleObligation(replay.Status),
	}, nil
}

// Settle closes the attempt selected by Token through the ordinary Finish
// transition. Its only remediation authority is the immutable SDD failed-
// evidence chain; review bindings and successors do not participate.
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
		if _, err := store.Finish(ctx, finish); err != nil {
			return store.compactMutationFailure(err, true, BeginAttemptRequest{}), nil
		}
		return store.compactSettleResult()
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

	status := replay.Status
	finish := FinishAttemptRequest{
		ExpectedRevision: status.Revision, RequestID: request.RequestID, Outcome: request.Outcome,
		EvidenceRevision: request.EvidenceRevision, Diagnosis: request.Diagnosis,
		HarnessDisposition: request.HarnessDisposition, CleanupEvidence: request.CleanupEvidence,
		ProcessEvidence: request.ProcessEvidence, IntendedUntracked: request.IntendedUntracked,
		ExpectedUntrackedInventory: request.ExpectedUntrackedInventory,
	}
	failedEvidence, hasFailedEvidence := runtimeChainFailedEvidence(status.Attempts)
	if request.RemediatesEvidenceRevision != "" && (!hasFailedEvidence || failedEvidence != request.RemediatesEvidenceRevision) {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	if request.Outcome == AttemptPassed && hasFailedEvidence && request.RemediatesEvidenceRevision == "" {
		return compactBlocked(CompactBlockInvalidContinuation, ""), nil
	}
	finish.RemediatesEvidenceRevision = request.RemediatesEvidenceRevision
	if _, err := store.Finish(ctx, finish); err != nil {
		return store.compactMutationFailure(err, true, BeginAttemptRequest{}), nil
	}
	return store.compactSettleResult()
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

// failedEvidenceRemediationSettleable reports whether a settle carrying
// --remediates-evidence-revision can structurally succeed against this ledger
// state. The immutable attempt chain must still hold that exact failed evidence
// unremediated. A changed candidate and fresh distinct evidence remain
// settle-time facts and are not judged here; nor is "may this work proceed?",
// which stays runtimeReadiness's question alone -- this reads only the
// immutable attempt chain.
func failedEvidenceRemediationSettleable(status RuntimeStatus, failedEvidence string) bool {
	chainEvidence, chainHasFailedEvidence := runtimeChainFailedEvidence(status.Attempts)
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
		ProcessEvidence: request.ProcessEvidence, IntendedUntracked: request.IntendedUntracked,
		ExpectedUntrackedInventory: request.ExpectedUntrackedInventory,
	})
	if err != nil {
		return err
	}
	if request.RemediatesEvidenceRevision != "" && !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
		return errors.New("remediates_evidence_revision must be sha256; rerun `gentle-ai sdd-attempt settle` with --remediates-evidence-revision sha256:<64-lowercase-hex>")
	}
	return nil
}

func compactAcquireMatches(record runtimeRecord, request BeginAttemptRequest, inheritsIntendedUntracked bool) bool {
	if (record.Operation != runtimeOperationBegin && record.Operation != runtimeOperationAdvance) || record.Begin == nil {
		return false
	}
	event := record.Begin
	if !inheritsIntendedUntracked && event.IntendedUntracked == nil {
		return false
	}
	var intendedUntracked []string
	if event.IntendedUntracked != nil {
		intendedUntracked = *event.IntendedUntracked
	}
	return request.ExpectedRevision == record.PreviousRevision && request.RequestID == record.RequestID &&
		request.WorkUnit == event.WorkUnit && request.EvidenceGoal == event.EvidenceGoal &&
		request.MaxAttempts == event.MaxAttempts && request.MaxChangedLines == event.MaxChangedLines &&
		slices.Equal(request.IntendedUntracked, intendedUntracked)
}

func compactSettleReplayRequest(replay runtimeReplay, record runtimeRecord, request CompactSettleRequest) (FinishAttemptRequest, bool) {
	if record.Finish == nil || record.Operation != runtimeOperationFinish {
		return FinishAttemptRequest{}, false
	}
	event := record.Finish
	matches := request.Token == replay.AttemptTokens[event.Ordinal] && request.RequestID == record.RequestID &&
		request.Outcome == event.Outcome && request.EvidenceRevision == event.EvidenceRevision &&
		request.Diagnosis == event.Diagnosis && request.HarnessDisposition == event.HarnessDisposition &&
		request.CleanupEvidence == event.CleanupEvidence && request.ProcessEvidence == event.ProcessEvidence &&
		request.RemediatesEvidenceRevision == event.RemediatesEvidenceRevision &&
		request.ExpectedUntrackedInventory == event.DeclaredUntrackedInventory &&
		compactSettleDeclarationMatches(request.IntendedUntracked, event)
	return FinishAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Outcome: event.Outcome,
		EvidenceRevision: event.EvidenceRevision, Diagnosis: event.Diagnosis,
		HarnessDisposition: event.HarnessDisposition, CleanupEvidence: event.CleanupEvidence,
		ProcessEvidence: event.ProcessEvidence, RemediatesEvidenceRevision: event.RemediatesEvidenceRevision,
		IntendedUntracked: replayedSettleDeclaration(event), ExpectedUntrackedInventory: event.DeclaredUntrackedInventory,
	}, matches
}

// replayedSettleDeclaration recovers the selection the original request
// carried. The event always records the selection the settlement used, but the
// request carried one only when the caller declared, which the recorded digest
// is what says.
func replayedSettleDeclaration(event *runtimeFinishEvent) *[]string {
	if event.DeclaredUntrackedInventory == "" {
		return nil
	}
	return event.IntendedUntracked
}

func compactSettleDeclarationMatches(declared *[]string, event *runtimeFinishEvent) bool {
	replayed := replayedSettleDeclaration(event)
	if declared == nil || replayed == nil {
		return declared == nil && replayed == nil
	}
	return slices.Equal(*declared, *replayed)
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

func (store RuntimeStore) compactSettleResult(expected ...string) (CompactAttemptResult, error) {
	replay, err := store.load()
	if err != nil {
		return compactBlockedByUnreadableAuthority(err), nil
	}
	if len(expected) == 1 && replay.Status.Revision != expected[0] {
		return compactBlocked(CompactBlockCorruptAuthority, ""), nil
	}
	if result, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: replay.Status, AttemptTokens: replay.AttemptTokens,
	}); terminal {
		return result, nil
	}
	return CompactAttemptResult{State: CompactStateProceed}, nil
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
		errors.Is(err, ErrRuntimeRequestConflict), errors.Is(err, ErrRuntimeNoActiveAttempt):
		reason = CompactBlockInvalidContinuation
	// ErrRuntimeUndeclaredUntracked is the sentinel behind every
	// settlementUntrackedSelection refusal (#3881): the settlement needs an
	// untracked ruling the request does not carry. Caller-actionable -- the
	// wrapped text names the exact rerun -- so it must not report as
	// authority_failure, which the contract reserves for what its name says.
	case errors.Is(err, ErrRuntimeUndeclaredUntracked):
		reason = CompactBlockUndeclaredUntracked
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
// (runtimeChainFailedAttempt, #1974 slice 2 / #2565), so the notice cannot
// promise something the settle will not demand, or stay silent about
// something it will.
func runtimeSettleObligation(status RuntimeStatus) string {
	failed, ok := runtimeChainFailedAttempt(status.Attempts)
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
