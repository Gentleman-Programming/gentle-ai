package sddstatus

import (
	"context"
	"errors"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// runtimeBeginAdmissionResult is everything Begin needs from the admission
// predicate once it has passed: which objective scope the attempt lands in and
// the exact candidate it will record.
type runtimeBeginAdmissionResult struct {
	Advancing  bool
	Generation int
	Snapshot   reviewtransaction.Snapshot
}

// runtimeRescopeSuccessorIntendedUntracked recovers only the exact selection
// whose zero-drift candidate opened a fresh rescope successor. The successor
// has no attempt of its own yet, so its predecessor's recorded selection is
// the sole inventory-validated scope that can reproduce InitialCandidate*.
func runtimeRescopeSuccessorIntendedUntracked(status RuntimeStatus) ([]string, bool) {
	if status.Objective == nil || status.ActiveAttempt != nil || status.LastRescope == nil ||
		status.LastRescope.ObjectiveID != status.Objective.ID || runtimeObjectiveHasRecordedAttempt(status) ||
		len(status.Attempts) == 0 {
		return nil, false
	}
	predecessor := status.Attempts[len(status.Attempts)-1]
	if predecessor.ObjectiveID != status.LastRescope.PreviousObjectiveID ||
		predecessor.ObjectiveGeneration != status.LastRescope.PreviousGeneration ||
		predecessor.Outcome == AttemptRunning {
		return nil, false
	}
	return slices.Clone(predecessor.IntendedUntracked), true
}

func runtimeRescopeSuccessorRequest(status RuntimeStatus, request BeginAttemptRequest, inherit bool) BeginAttemptRequest {
	if !inherit {
		return request
	}
	if intended, ok := runtimeRescopeSuccessorIntendedUntracked(status); ok {
		request.IntendedUntracked = intended
	}
	return request
}

// runtimeBeginAdmission is the ONE evaluator of every precondition Begin must
// satisfy before it can record an attempt, ledger-side and repository-side
// alike. It mutates nothing: it reads the replayed status the caller hands it
// and the repository, and answers "would Begin be admitted, and if not, why".
//
// runtimeReadiness already did this for the LEDGER half, and #2471's root 13
// counted that as the fix for "two authorities disagree". The reports kept
// arriving because the REPOSITORY half lived inline in Begin's mutation
// closure, where no read-only surface could reach it. Status asked
// runtimeReadiness, got "nothing blocks", and printed next_action: "begin"
// while acquire, running the very same request through this code a moment
// later, blocked on a candidate that could not be captured or one that had
// drifted out from under the objective (#2114, reported three times).
//
// Extracting it is what makes the disagreement structurally impossible rather
// than merely fixed for today's causes: Begin cannot grow a new
// repository-side precondition without every read-only surface inheriting it,
// because there is only one place to put it.
func (store RuntimeStore) runtimeBeginAdmission(
	ctx context.Context, status RuntimeStatus, request BeginAttemptRequest,
) (runtimeBeginAdmissionResult, error) {
	if status.ActiveAttempt != nil {
		return runtimeBeginAdmissionResult{}, ErrRuntimeAttemptActive
	}
	// A passed objective terminates its own scope, not the change. When the
	// request names a distinct work unit, the ordinary continuation is the
	// successor objective, not a maintainer reset that would discard the
	// completed apply authority along with its evidence.
	advancing := false
	if status.Complete {
		if !runtimeObjectiveAdvanceAdmissible(status, request) {
			return runtimeBeginAdmissionResult{}, store.runtimeObjectiveCompleteRefusal(status)
		}
		advancing = true
	}
	if status.DecisionRequired {
		return runtimeBeginAdmissionResult{}, ErrRuntimeBudgetExhausted
	}

	generation := status.ObjectiveGeneration + 1
	var snapshot reviewtransaction.Snapshot
	var err error
	switch {
	case status.Objective != nil && !advancing && runtimeObjectiveHasRecordedAttempt(status):
		// The ordinary continuing-objective path: at least one attempt is
		// already recorded under this exact objective ID, so its terminal
		// Finish is the candidate provenance to chase.
		generation = status.Objective.Generation
		if runtimeObjectiveScopeChanged(status, request) {
			return runtimeBeginAdmissionResult{}, store.runtimeObjectiveChangeRefusal(ctx, status)
		}
		last := status.Attempts[len(status.Attempts)-1]
		if last.Outcome == AttemptRunning || last.FinishCandidateIdentity == "" || last.FinishCandidateTree == "" {
			return runtimeBeginAdmissionResult{}, errors.New("SDD runtime objective has invalid terminal candidate provenance")
		}
		// #3842: the terminal capture replays the RECORDED selection, which
		// the user may have legitimately committed since the last settle, so
		// reconcile it against the current index first — a committed path then
		// replays as zero drift or as ordinary candidate drift, never as a
		// capture failure. The request-vs-recorded comparison below
		// deliberately stays against the recorded list: what the caller must
		// re-request is the scope the ledger holds, exactly as acquired.
		var intended []string
		intended, err = runtimeReplayedIntendedUntracked(ctx, store.Repo, last.IntendedUntracked)
		if err == nil {
			snapshot, err = captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree, intended)
		}
		if err == nil && (snapshot.Identity != last.FinishCandidateIdentity || snapshot.CandidateTree != last.FinishCandidateTree) {
			return runtimeBeginAdmissionResult{}, store.runtimeObjectiveChangeRefusal(ctx, status)
		}
		if err == nil && !slices.Equal(request.IntendedUntracked, last.IntendedUntracked) {
			return runtimeBeginAdmissionResult{}, store.runtimeObjectiveChangeRefusal(ctx, status)
		}
	case status.Objective != nil && !advancing:
		// A freshly opened objective (Rescope) with no attempt recorded
		// under its own ID yet: there is no terminal Finish belonging to
		// THIS objective to chase, so capture a fresh candidate the same
		// way a brand-new objective's first attempt does, and validate it
		// against the candidate the objective itself opened with
		// (RuntimeObjective.InitialCandidate*, set by Rescope to the exact
		// zero-drift candidate it verified) instead of a prior attempt's
		// Finish record. Chasing the LAST recorded attempt here would find
		// one that belongs to the objective THIS one superseded (#2298,
		// #2296 part 2's landmine) and wrongly refuse.
		generation = status.Objective.Generation
		if runtimeObjectiveScopeChanged(status, request) {
			return runtimeBeginAdmissionResult{}, store.runtimeObjectiveChangeRefusal(ctx, status)
		}
		snapshot, err = captureRuntimeCandidate(ctx, store.Repo, request.IntendedUntracked)
		if err == nil && (snapshot.Identity != status.Objective.InitialCandidateIdentity || snapshot.CandidateTree != status.Objective.InitialCandidateTree) {
			return runtimeBeginAdmissionResult{}, store.runtimeObjectiveChangeRefusal(ctx, status)
		}
	default:
		snapshot, err = captureRuntimeCandidate(ctx, store.Repo, request.IntendedUntracked)
	}
	if err != nil {
		return runtimeBeginAdmissionResult{}, wrapRuntimeCandidateUnavailable("before launch", err)
	}
	// The successor opens a fresh per-objective budget, so the charges the
	// completed scope accrued cannot exhaust it before its first attempt.
	if !advancing && (status.CumulativeAttempts >= request.MaxAttempts || status.CumulativeChangedLines >= request.MaxChangedLines) {
		return runtimeBeginAdmissionResult{}, ErrRuntimeBudgetExhausted
	}
	return runtimeBeginAdmissionResult{Advancing: advancing, Generation: generation, Snapshot: snapshot}, nil
}

// runtimeObjectiveScopeChanged is the single work-unit-scope comparison both
// continuing-objective branches make. It was written out twice, which is how a
// scope field could be added to one branch and forgotten in the other.
func runtimeObjectiveScopeChanged(status RuntimeStatus, request BeginAttemptRequest) bool {
	return request.WorkUnit != status.Objective.WorkUnit ||
		request.EvidenceGoal != status.Objective.EvidenceGoal ||
		request.MaxAttempts != status.Objective.MaxAttempts ||
		request.MaxChangedLines != status.Objective.MaxChangedLines
}

// AdmissionStatus is the read-only surface's answer to the question consumers
// were actually asking when they ran `sdd-attempt status` before an acquire:
// not "what does the ledger hold" but "may this work proceed, and if not, what
// do I run". It is the ledger projection Status() already returned, plus the
// verdict acquire would reach for this exact request, derived by running the
// same admission predicate acquire runs, so the two cannot disagree.
//
// It mutates nothing and consumes no budget. A blocked verdict fills
// BlockedReason and BlockedExit; the ledger fields, next_action included, are
// what Status() reports, because the ledger's own next transition does not
// change just because the repository is not ready for it yet.
func (store RuntimeStore) AdmissionStatus(ctx context.Context, request BeginAttemptRequest) (RuntimeStatus, error) {
	replay, err := store.load()
	if err != nil {
		return RuntimeStatus{}, err
	}
	status := replay.Status
	// The obligation is a property of the immutable chain, not of whatever is
	// blocking right now, so it is set before any early return. An active
	// attempt or an exhausted budget does not make the chain owe less, and a
	// surface that goes quiet under those states would disagree with acquire
	// exactly when the operator is looking hardest.
	status.SettleObligation = runtimeSettleObligation(status)

	inheritIntendedUntracked := request.IntendedUntracked == nil
	normalized, err := normalizeBeginAttemptRequest(request)
	if err != nil {
		status.BlockedReason = CompactBlockInvalidContinuation
		status.BlockedExit = err.Error()
		return status, nil
	}
	normalized = runtimeRescopeSuccessorRequest(status, normalized, inheritIntendedUntracked)
	if result, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: status, AttemptTokens: replay.AttemptTokens, Request: normalized,
	}); terminal && result.State == CompactStateBlocked {
		status.BlockedReason, status.BlockedExit = result.Reason, result.Exit
		// An exhausted budget is a decision, so it asks instead of ending the
		// conversation. The grant is the reset the ledger already admits at
		// decision-required, offered as a runnable choice rather than as prose
		// the human has to assemble from six flags.
		if result.Reason == CompactBlockMaintainerDecision {
			if consent, consentErr := BudgetConsentEnvelope(store.budgetConsentInput(status)); consentErr == nil {
				status.Consent = &consent
			}
		}
		return status, nil
	}
	if _, admissionErr := store.runtimeBeginAdmission(ctx, status, normalized); admissionErr != nil {
		if blocked := store.compactMutationFailure(admissionErr, false, normalized); blocked.State == CompactStateBlocked {
			status.BlockedReason, status.BlockedExit = blocked.Reason, blocked.Exit
		}
	}
	return status, nil
}

// budgetConsentInput reads the question's facts off the ledger. HarnessFailures
// is derived from HarnessDisposition, which the settle contract already
// carries: an attempt the actor settled as `invalidated` is one whose harness
// could not be used, so it produced no evidence about the candidate. Reusing
// the declared field beats inventing a second way to say the same thing.
func (store RuntimeStore) budgetConsentInput(status RuntimeStatus) BudgetConsentInput {
	in := BudgetConsentInput{
		Repo: store.Workspace, Change: store.Change, Revision: status.Revision,
		CumulativeAttempts: status.CumulativeAttempts, CumulativeLines: status.CumulativeChangedLines,
	}
	if status.Objective != nil {
		in.MaxAttempts, in.MaxChangedLines = status.Objective.MaxAttempts, status.Objective.MaxChangedLines
		for _, attempt := range status.Attempts {
			if attempt.ObjectiveID == status.Objective.ID &&
				attempt.Outcome != AttemptPassed && attempt.HarnessDisposition == HarnessInvalidated {
				in.HarnessFailures++
			}
		}
	}
	return in
}
