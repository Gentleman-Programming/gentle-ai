package qastage

import (
	"context"
	"errors"
)

// Approval records that a human (or authorized agent) reviewed a completed
// stage's output. Only "apply" requires one today — of its predecessor
// "spec" — matching v2's requires_approval intent, now actually enforced on
// every attempt to begin apply, not only when the ledger happens to already
// have prior history (baseline Escenario 2).
type Approval struct {
	Stage            string `json:"stage"`
	ArtifactRevision string `json:"artifact_revision"`
	Actor            string `json:"actor"`
	Reason           string `json:"reason"`
}

var (
	ErrApprovalRequired         = errors.New("qastage: apply requires a recorded approval of spec's current revision before it can begin")
	ErrNothingToApprove         = errors.New("qastage: stage has not completed yet, there is nothing to approve")
	ErrApprovalRevisionMismatch = errors.New("qastage: artifact_revision does not match the stage's actual current revision")
)

const stageRequiringApproval = "apply"
const approvedPredecessorStage = "spec"

// isApprovedForRevision reports whether stage has a recorded approval for
// exactly revision. A stage approved at an old revision is NOT approved for
// a newer one — this is what makes a spec amendment invalidate its prior
// approval (design decision 1.1 / contrato pendiente #5).
func isApprovedForRevision(state ledgerState, stage, revision string) bool {
	if revision == "" {
		return false
	}
	for _, approval := range state.Approvals {
		if approval.Stage == stage && approval.ArtifactRevision == revision {
			return true
		}
	}
	return false
}

func stageHasCompleted(state ledgerState, stage string) bool {
	for _, attempt := range state.Attempts {
		if attempt.Stage == stage && attempt.Outcome == OutcomePassed {
			return true
		}
	}
	return false
}

// Approve records an approval for stage, which must have already completed
// (Outcome == passed) at least once. It does not yet bind to a specific
// artifact_revision — that invalidation-on-change behavior is added in 3A.8.
func (m *QAStateMachine) Approve(ctx context.Context, change, stage, artifactRevision, actor, reason, requestID string) (Approval, error) {
	state, head, err := m.readState(ctx, change)
	if err != nil {
		return Approval{}, err
	}

	fingerprint := fingerprintParts(stage, artifactRevision, actor, reason)
	if existing, ok := findRequest(state, "approve", requestID); ok {
		if existing.Fingerprint != fingerprint {
			return Approval{}, ErrRequestConflict
		}
		return state.Approvals[existing.ResultIndex-1], nil
	}

	if !stageHasCompleted(state, stage) {
		return Approval{}, ErrNothingToApprove
	}
	if currentRevisionOf(state, stage) != artifactRevision {
		return Approval{}, ErrApprovalRevisionMismatch
	}

	approval := Approval{Stage: stage, ArtifactRevision: artifactRevision, Actor: actor, Reason: reason}
	state.Approvals = append(state.Approvals, approval)
	state.RequestLog = append(state.RequestLog, requestRecord{Operation: "approve", RequestID: requestID, Fingerprint: fingerprint, ResultIndex: len(state.Approvals)})

	if err := m.commit(ctx, change, head.Revision, state); err != nil {
		return Approval{}, err
	}
	return approval, nil
}
