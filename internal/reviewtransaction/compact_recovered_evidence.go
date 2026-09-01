package reviewtransaction

import (
	"context"
	"errors"
	"reflect"
	"strconv"
)

// deriveCompactRecoveredEvidence recognizes the one predecessor topology that
// may advance without a second review budget: review and targeted validation
// both succeeded, but historical correction accounting alone escalated the
// authority. The corrected candidate must be the exact recovery target.
func deriveCompactRecoveredEvidence(ctx context.Context, repo string, _ CompactStore, predecessor CompactRecord, successor CompactState) (CompactRecoveredEvidence, bool, error) {
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
	// Escalations are intentionally receiptless. Their validated compact state
	// and correction evidence are the recovery authority; requiring an absent
	// approval receipt would turn the accounting-only recovery edge into a
	// terminal dead end.
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
	evidence := CompactRecoveredEvidence{
		Schema:                    CompactRecoveredEvidenceSchema,
		Relation:                  string(relation.Kind),
		PathRelation:              string(relation.Paths),
		PredecessorTargetIdentity: predecessor.State.InitialSnapshot.Identity,
		NativeCorrectionLines:     nativeLines,
		AdmittedRoleReferences:    compactRecoveredEvidenceReferences(predecessor.State),
	}
	if _, err := rebuildCompactRecoveredTargetedValidationRequest(predecessor.State, evidence); err != nil {
		return CompactRecoveredEvidence{}, false, err
	}
	return evidence, true, nil
}

func compactAccountingOnlyEscalation(state CompactState) bool {
	if state.State != StateEscalated || state.EvidenceHash != "" || len(state.CorrectionAttempts) != 1 ||
		state.CumulativeCorrectionLines <= state.CorrectionBudget || state.ProposedCorrectionLines == nil || state.ActualCorrectionLines == nil ||
		state.OriginalCriteria == nil || state.CorrectionRegression == nil || len(state.FixFindingIDs) == 0 {
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

func compactRecoveredEvidenceReferences(state CompactState) []CompactRecoveredEvidenceReference {
	references := make([]CompactRecoveredEvidenceReference, len(state.AdmittedRoleResults))
	for index, admitted := range state.AdmittedRoleResults {
		references[index] = CompactRecoveredEvidenceReference{
			Role: admitted.Role, Lens: admitted.Lens, SelectedOrder: admitted.SelectedOrder,
			TargetIdentity: admitted.TargetIdentity, CapturePhaseRevision: admitted.CapturePhaseRevision,
			RequestHash: admitted.RequestHash, ArtifactDigest: admitted.ArtifactDigest,
		}
	}
	return references
}

// compactAdmittedRoleResultIsAccountingOnly recognizes the exact tuple and
// digest references that recovery retains solely for accounting. Those values are
// canonical evidence, not current capture slots or materialization inputs.
func compactAdmittedRoleResultIsAccountingOnly(state CompactState, admitted CompactAdmittedRoleResult) bool {
	if state.Recovery == nil || state.Recovery.Evidence == nil {
		return false
	}
	for _, reference := range state.Recovery.Evidence.AdmittedRoleReferences {
		if compactRecoveredEvidenceReferenceMatchesAdmittedRoleResult(reference, admitted) {
			return true
		}
	}
	return false
}

// IsAccountingOnlyAdmittedRoleResult exposes the compact authority distinction
// to provider materialization without exposing recovery payloads or a second
// role-result owner.
func (state CompactState) IsAccountingOnlyAdmittedRoleResult(admitted CompactAdmittedRoleResult) bool {
	return compactAdmittedRoleResultIsAccountingOnly(state, admitted)
}

func compactAdmittedRoleResultCanSatisfyActiveCapture(state CompactState, admitted CompactAdmittedRoleResult) bool {
	return !compactAdmittedRoleResultIsAccountingOnly(state, admitted)
}

func compactRecoveredEvidenceReferenceMatchesAdmittedRoleResult(reference CompactRecoveredEvidenceReference, admitted CompactAdmittedRoleResult) bool {
	return reference.Role == admitted.Role && reference.Lens == admitted.Lens &&
		reference.SelectedOrder == admitted.SelectedOrder && reference.TargetIdentity == admitted.TargetIdentity &&
		reference.CapturePhaseRevision == admitted.CapturePhaseRevision && reference.RequestHash == admitted.RequestHash &&
		reference.ArtifactDigest == admitted.ArtifactDigest
}

func validateCompactRecoveredEvidenceReferences(predecessor CompactState, evidence CompactRecoveredEvidence) error {
	if err := predecessor.Validate(); err != nil {
		return errors.New("recovered evidence predecessor is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if evidence.PredecessorTargetIdentity != predecessor.InitialSnapshot.Identity ||
		len(evidence.AdmittedRoleReferences) != len(predecessor.AdmittedRoleResults) ||
		len(evidence.AdmittedRoleReferences) == 0 {
		return errors.New("recovered evidence does not reference every admitted predecessor result") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := validateCompactRecoveredEvidenceReferenceShape(predecessor.SelectedLenses, evidence.AdmittedRoleReferences); err != nil {
		return err
	}
	validatorCount := 0
	for index, admitted := range predecessor.AdmittedRoleResults {
		if !compactRecoveredEvidenceReferenceMatchesAdmittedRoleResult(evidence.AdmittedRoleReferences[index], admitted) {
			return errors.New("recovered evidence reference is stale, unordered, duplicated, or unknown") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if admitted.Role == CompactRoleTargetedValidator {
			validatorCount++
		}
	}
	if validatorCount != 1 {
		return errors.New("recovered evidence requires exactly one admitted targeted validator") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	return nil
}

func validateCompactRecoveredEvidenceReferenceShape(selectedLenses []string, references []CompactRecoveredEvidenceReference) error {
	previousRole, previousLensOrder := -1, -1
	seenLensOrders := make(map[int]bool, len(references))
	seenTuples := make(map[string]bool, len(references))
	validatorCount := 0
	for _, reference := range references {
		roleOrder := -1
		switch reference.Role {
		case CompactRoleLens:
			roleOrder = 0
		case CompactRoleRefuter:
			roleOrder = 1
		case CompactRoleTargetedValidator:
			roleOrder = 2
			validatorCount++
		default:
			return errors.New("recovered evidence reference has an unsupported role") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if roleOrder < previousRole || roleOrder == previousRole && reference.Role == CompactRoleLens && reference.SelectedOrder <= previousLensOrder {
			return errors.New("recovered evidence references are not canonically ordered") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		previousRole = roleOrder
		if reference.Role == CompactRoleLens {
			previousLensOrder = reference.SelectedOrder
		}
		if !validSHA256(reference.TargetIdentity) || !validSHA256(reference.CapturePhaseRevision) || !validSHA256(reference.ArtifactDigest) ||
			reference.RequestHash != "" && !validSHA256(reference.RequestHash) {
			return errors.New("recovered evidence reference binding is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		switch reference.Role {
		case CompactRoleLens:
			if reference.RequestHash != "" || reference.SelectedOrder < 0 || reference.SelectedOrder >= len(selectedLenses) ||
				reference.Lens != selectedLenses[reference.SelectedOrder] || seenLensOrders[reference.SelectedOrder] {
				return errors.New("recovered evidence lens reference is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
			seenLensOrders[reference.SelectedOrder] = true
		case CompactRoleRefuter, CompactRoleTargetedValidator:
			if reference.Lens != "" || reference.SelectedOrder != 0 || reference.RequestHash == "" {
				return errors.New("recovered evidence non-lens reference is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
		}
		tuple := string(reference.Role) + "\x00" + reference.Lens + "\x00" + strconv.Itoa(reference.SelectedOrder) + "\x00" +
			reference.TargetIdentity + "\x00" + reference.CapturePhaseRevision + "\x00" + reference.RequestHash
		if seenTuples[tuple] {
			return errors.New("recovered evidence reference repeats an admitted role tuple") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		seenTuples[tuple] = true
	}
	if len(seenLensOrders) != len(selectedLenses) || validatorCount != 1 {
		return errors.New("recovered evidence references do not cover the review and targeted validator") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	return nil
}

// rebuildCompactRecoveredTargetedValidationRequest derives the provider request
// from predecessor facts and the canonical validator reference. It never reads
// Recovery.PredecessorRevision because that is live Rn provenance, not Pn.
func rebuildCompactRecoveredTargetedValidationRequest(predecessor CompactState, evidence CompactRecoveredEvidence) (TargetedValidationRequest, error) {
	if err := validateCompactRecoveredEvidenceReferences(predecessor, evidence); err != nil {
		return TargetedValidationRequest{}, err
	}
	attempt := predecessor.CorrectionAttempts[len(predecessor.CorrectionAttempts)-1]
	var validator *CompactAdmittedRoleResult
	for index := range predecessor.AdmittedRoleResults {
		admitted := &predecessor.AdmittedRoleResults[index]
		if admitted.Role == CompactRoleTargetedValidator {
			validator = admitted
			break
		}
	}
	if validator == nil || validator.TargetIdentity != attempt.Snapshot.Identity ||
		validator.CapturePhaseRevision != predecessor.CapturePhaseRevision {
		return TargetedValidationRequest{}, errors.New("recovered evidence validator reference is stale") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	request, err := targetedValidationRequestForCorrection(predecessor, validator.CapturePhaseRevision, attempt.Snapshot)
	if err != nil {
		return TargetedValidationRequest{}, err
	}
	if request.RequestHash != validator.RequestHash || request.RequestHash != attempt.TargetedValidationRequestHash ||
		request.ExpectedRevision != predecessor.CapturePhaseRevision || request.TargetIdentity != predecessor.InitialSnapshot.Identity ||
		request.CorrectionTargetIdentity != attempt.Snapshot.Identity {
		return TargetedValidationRequest{}, errors.New("recovered evidence targeted validation request is stale")
	}
	return request, nil
}

func importCompactRecoveredEvidence(successor *CompactState, predecessor CompactState, evidence CompactRecoveredEvidence) {
	attempt := predecessor.CorrectionAttempts[len(predecessor.CorrectionAttempts)-1]
	successor.State = StateValidating
	successor.CurrentSnapshot = successor.InitialSnapshot
	// The admitted collection is the only successor owner of recovered role
	// values. Evidence references address these exact entries by tuple and
	// ArtifactDigest; no result projection or nested recovery payload survives.
	successor.AdmittedRoleResults = cloneCompactAdmittedRoleResults(predecessor.AdmittedRoleResults)
	successor.FixFindingIDs = append([]string(nil), predecessor.FixFindingIDs...)
	proposed, actual := attempt.ProposedLines, evidence.NativeCorrectionLines
	successor.ProposedCorrectionLines, successor.ActualCorrectionLines = &proposed, &actual
	successor.FixDeltaHash = attempt.FixDeltaHash
	original, regression := attempt.OriginalCriteria, attempt.CorrectionRegression
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
		evidence.Schema != CompactRecoveredEvidenceSchema || evidence.Relation != string(relation.Kind) ||
		evidence.PathRelation != string(relation.Paths) || evidence.PredecessorTargetIdentity != predecessor.State.InitialSnapshot.Identity ||
		evidence.NativeCorrectionLines <= 0 || evidence.NativeCorrectionLines > predecessor.State.CorrectionBudget ||
		evidence.NativeCorrectionLines > attempt.ProposedLines || evidence.NativeCorrectionLines >= attempt.ActualLines {
		return errors.New("recovered evidence does not exactly bind the accounting-only predecessor")
	}
	if err := validateCompactRecoveredEvidenceReferences(predecessor.State, evidence); err != nil {
		return err
	}
	if !reflect.DeepEqual(successor.AdmittedRoleResults, predecessor.State.AdmittedRoleResults) {
		return errors.New("recovered evidence does not retain the canonical admitted predecessor entries exactly once") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := validateCompactRecoveredEvidenceReferencesInSuccessor(successor, evidence); err != nil {
		return err
	}
	request, err := rebuildCompactRecoveredTargetedValidationRequest(predecessor.State, evidence)
	if err != nil {
		return err
	}
	if successor.State != StateValidating || successor.ProposedCorrectionLines == nil || successor.ActualCorrectionLines == nil ||
		*successor.ProposedCorrectionLines != attempt.ProposedLines || *successor.ActualCorrectionLines != evidence.NativeCorrectionLines ||
		successor.FixDeltaHash != attempt.FixDeltaHash || successor.OriginalCriteria == nil || successor.CorrectionRegression == nil ||
		*successor.OriginalCriteria != attempt.OriginalCriteria || *successor.CorrectionRegression != attempt.CorrectionRegression ||
		!equalStrings(successor.FixFindingIDs, predecessor.State.FixFindingIDs) ||
		request.LineageID != predecessor.State.LineageID || request.ExpectedRevision != predecessor.State.CapturePhaseRevision ||
		request.CorrectionCandidateTree != attempt.Snapshot.CandidateTree || request.CorrectionTargetIdentity != attempt.Snapshot.Identity ||
		!equalStrings(request.CorrectionPaths, attempt.Snapshot.Paths) || request.CorrectionPathsDigest != attempt.Snapshot.PathsDigest {
		return errors.New("recovered evidence does not preserve the exact accounting-only predecessor") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	return nil
}

func validateCompactRecoveredEvidenceReferencesInSuccessor(successor CompactState, evidence CompactRecoveredEvidence) error {
	if len(successor.AdmittedRoleResults) != len(evidence.AdmittedRoleReferences) {
		return errors.New("recovered evidence does not retain exactly one admitted entry per reference") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	for index, admitted := range successor.AdmittedRoleResults {
		if !compactRecoveredEvidenceReferenceMatchesAdmittedRoleResult(evidence.AdmittedRoleReferences[index], admitted) ||
			!compactAdmittedRoleResultIsAccountingOnly(successor, admitted) {
			return errors.New("recovered evidence reference does not resolve its canonical admitted entry") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
	}
	return nil
}
