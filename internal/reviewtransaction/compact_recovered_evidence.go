package reviewtransaction

import (
	"context"
	"errors"
	"reflect"
)

// deriveCompactRecoveredEvidence recognizes the one predecessor topology that
// may advance without a second review budget: review and targeted validation
// both succeeded, but historical correction accounting alone escalated the
// authority. The corrected candidate must be the exact recovery target.
func deriveCompactRecoveredEvidence(ctx context.Context, repo string, predecessorStore CompactStore, predecessor CompactRecord, successor CompactState) (CompactRecoveredEvidence, bool, error) {
	if !compactAccountingOnlyEscalation(predecessor.State) || !compactRecoveredEvidenceScopeMatches(predecessor.State, successor) {
		return CompactRecoveredEvidence{}, false, nil
	}
	attempt := predecessor.State.CorrectionAttempts[len(predecessor.State.CorrectionAttempts)-1]
	relation := classifyCompactTargetRelation(
		predecessor.State.InitialSnapshot, successor.InitialSnapshot, predecessor.State.GenesisPaths,
		compactTargetRelationEvidence{ExplicitScopeChange: true},
	)
	if relation.Kind != compactTargetChangedScope || relation.Paths != compactPathsSame {
		return CompactRecoveredEvidence{}, false, nil
	}
	if !compactRecoveryReceiptBound(predecessorStore, predecessor) {
		return CompactRecoveredEvidence{}, false, errors.New("accounting-only recovery predecessor receipt does not match authority")
	}
	builder := SnapshotBuilder{Repo: repo}
	for _, snapshot := range []Snapshot{predecessor.State.InitialSnapshot, attempt.Snapshot, successor.InitialSnapshot} {
		if err := builder.ValidateEvidence(ctx, snapshot); err != nil {
			return CompactRecoveredEvidence{}, false, errors.New("accounting-only recovery evidence is not repository-derived")
		}
	}
	nativeLines, err := builder.ChangedLines(ctx, attempt.Snapshot)
	if err != nil {
		return CompactRecoveredEvidence{}, false, err
	}
	if nativeLines <= 0 || nativeLines > predecessor.State.CorrectionBudget || nativeLines > attempt.ProposedLines || nativeLines >= attempt.ActualLines {
		return CompactRecoveredEvidence{}, false, errors.New("escalated correction is not proven to be an accounting-only failure")
	}
	request, err := targetedValidationRequestForCorrection(predecessor.State, predecessor.Revision, attempt.Snapshot)
	if err != nil {
		return CompactRecoveredEvidence{}, false, err
	}
	evidence := CompactRecoveredEvidence{
		Schema: CompactRecoveredEvidenceSchema, Relation: string(relation.Kind), PathRelation: string(relation.Paths),
		SuccessorTargetIdentity: successor.InitialSnapshot.Identity,
		ReviewEvidenceHash:      compactReviewEvidenceHash(predecessor.State),
		SourceCorrectionAttempt: attempt, NativeCorrectionLines: nativeLines, TargetedValidationRequest: request,
	}
	return evidence, true, nil
}

func compactAccountingOnlyEscalation(state CompactState) bool {
	if state.State != StateEscalated || state.EvidenceHash != "" || len(state.CorrectionAttempts) != 1 ||
		state.CumulativeCorrectionLines <= state.CorrectionBudget || state.ProposedCorrectionLines == nil || state.ActualCorrectionLines == nil ||
		state.OriginalCriteria == nil || state.CorrectionRegression == nil || len(state.FixFindingIDs) == 0 || len(state.ResultDispositions) != 0 {
		return false
	}
	attempt := state.CorrectionAttempts[0]
	return attempt.ProposedLines == *state.ProposedCorrectionLines && attempt.ActualLines == *state.ActualCorrectionLines &&
		attempt.ActualLines == state.CumulativeCorrectionLines && attempt.FixDeltaHash == state.FixDeltaHash &&
		attempt.OriginalCriteria == *state.OriginalCriteria && attempt.CorrectionRegression == *state.CorrectionRegression &&
		attempt.OriginalCriteria.Passed && attempt.CorrectionRegression.Passed && snapshotsEqual(attempt.Snapshot, state.CurrentSnapshot)
}

func compactRecoveredEvidenceScopeMatches(predecessor, successor CompactState) bool {
	return predecessor.PolicyHash == successor.PolicyHash && predecessor.RiskLevel == successor.RiskLevel &&
		equalStrings(predecessor.SelectedLenses, successor.SelectedLenses) &&
		predecessor.CurrentSnapshot.CandidateTree == successor.InitialSnapshot.CandidateTree &&
		equalStrings(predecessor.GenesisPaths, successor.GenesisPaths) &&
		equalStrings(predecessor.InitialSnapshot.IntendedUntracked, successor.InitialSnapshot.IntendedUntracked) &&
		predecessor.InitialSnapshot.Projection == successor.InitialSnapshot.Projection
}

func importCompactRecoveredEvidence(successor *CompactState, predecessor CompactState, evidence CompactRecoveredEvidence) {
	successor.State = StateValidating
	successor.CurrentSnapshot = successor.InitialSnapshot
	successor.LensResults = cloneCompactLensResults(predecessor.LensResults)
	successor.Findings = cloneCompactFindings(predecessor.Findings)
	successor.Classifications = make(map[string]FindingEvidence, len(predecessor.Classifications))
	for id, classification := range predecessor.Classifications {
		successor.Classifications[id] = classification
	}
	successor.Outcomes = make(map[string]EvidenceOutcome, len(predecessor.Outcomes))
	for id, outcome := range predecessor.Outcomes {
		successor.Outcomes[id] = outcome
	}
	successor.FixFindingIDs = append([]string(nil), predecessor.FixFindingIDs...)
	successor.FollowUps = cloneCompactFollowUps(predecessor.FollowUps)
	proposed, actual := evidence.SourceCorrectionAttempt.ProposedLines, evidence.NativeCorrectionLines
	successor.ProposedCorrectionLines, successor.ActualCorrectionLines = &proposed, &actual
	successor.FixDeltaHash = evidence.SourceCorrectionAttempt.FixDeltaHash
	original, regression := evidence.SourceCorrectionAttempt.OriginalCriteria, evidence.SourceCorrectionAttempt.CorrectionRegression
	successor.OriginalCriteria, successor.CorrectionRegression = &original, &regression
	successor.EvidenceHash = ""
	successor.CorrectionAttempts = nil
	successor.CumulativeCorrectionLines = 0
	successor.Recovery.Evidence = &evidence
}

func validateCompactRecoveredEvidenceEdge(predecessor CompactRecord, successor CompactState) error {
	if successor.Recovery == nil || successor.Recovery.Evidence == nil || !compactAccountingOnlyEscalation(predecessor.State) ||
		!compactRecoveredEvidenceScopeMatches(predecessor.State, successor) {
		return errors.New("recovered evidence is not eligible for this predecessor and successor")
	}
	evidence := *successor.Recovery.Evidence
	attempt := predecessor.State.CorrectionAttempts[len(predecessor.State.CorrectionAttempts)-1]
	relation := classifyCompactTargetRelation(
		predecessor.State.InitialSnapshot, successor.InitialSnapshot, predecessor.State.GenesisPaths,
		compactTargetRelationEvidence{ExplicitScopeChange: true},
	)
	if relation.Kind != compactTargetChangedScope || relation.Paths != compactPathsSame ||
		evidence.Relation != string(relation.Kind) || evidence.PathRelation != string(relation.Paths) ||
		evidence.SuccessorTargetIdentity != successor.InitialSnapshot.Identity ||
		evidence.ReviewEvidenceHash != compactReviewEvidenceHash(predecessor.State) ||
		evidence.ReviewEvidenceHash != compactReviewEvidenceHash(successor) ||
		!reflect.DeepEqual(evidence.SourceCorrectionAttempt, attempt) ||
		evidence.NativeCorrectionLines <= 0 || evidence.NativeCorrectionLines > predecessor.State.CorrectionBudget ||
		evidence.NativeCorrectionLines > attempt.ProposedLines || evidence.NativeCorrectionLines >= attempt.ActualLines {
		return errors.New("recovered evidence does not exactly bind the accounting-only predecessor")
	}
	request := evidence.TargetedValidationRequest
	if request.LineageID != predecessor.State.LineageID || request.ExpectedRevision != predecessor.Revision ||
		request.TargetIdentity != predecessor.State.InitialSnapshot.Identity || request.RequestHash != targetedValidationRequestHash(request) ||
		request.CorrectionTargetIdentity != attempt.Snapshot.Identity || request.CorrectionCandidateTree != attempt.Snapshot.CandidateTree ||
		!equalStrings(request.CorrectionPaths, attempt.Snapshot.Paths) || !equalStrings(request.FixFindingIDs, predecessor.State.FixFindingIDs) {
		return errors.New("recovered evidence targeted validation request is stale")
	}
	return nil
}

func cloneCompactLensResults(values []LensResult) []LensResult {
	cloned := make([]LensResult, len(values))
	for index, value := range values {
		value.Findings = cloneCompactFindings(value.Findings)
		value.Evidence = append([]string(nil), value.Evidence...)
		cloned[index] = value
	}
	return cloned
}

func cloneCompactFindings(values []Finding) []Finding {
	cloned := make([]Finding, len(values))
	for index, value := range values {
		value.ProofRefs = append([]string(nil), value.ProofRefs...)
		cloned[index] = value
	}
	return cloned
}

func cloneCompactFollowUps(values []FollowUp) []FollowUp {
	cloned := make([]FollowUp, len(values))
	for index, value := range values {
		value.ProofRefs = append([]string(nil), value.ProofRefs...)
		cloned[index] = value
	}
	return cloned
}
