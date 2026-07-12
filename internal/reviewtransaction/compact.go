package reviewtransaction

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"
)

const CompactStateSchema = "gentle-ai.review-state/v2"
const CompactReceiptSchema = "gentle-ai.review-receipt/v2"

const (
	StateCorrectionRequired      State = "correction_required"
	StateValidating              State = "validating"
	MaxCompactCorrectionAttempts       = 3
)

type CompactState struct {
	Schema                    string                     `json:"schema"`
	LineageID                 string                     `json:"lineage_id"`
	Generation                int                        `json:"generation"`
	State                     State                      `json:"state"`
	InitialSnapshot           Snapshot                   `json:"initial_snapshot"`
	CurrentSnapshot           Snapshot                   `json:"current_snapshot"`
	GenesisPaths              []string                   `json:"genesis_paths"`
	PolicyHash                string                     `json:"policy_hash"`
	RiskLevel                 RiskLevel                  `json:"risk_level"`
	SelectedLenses            []string                   `json:"selected_lenses"`
	OriginalChangedLines      int                        `json:"original_changed_lines"`
	CorrectionBudget          int                        `json:"correction_budget"`
	LensResults               []LensResult               `json:"lens_results"`
	Findings                  []Finding                  `json:"findings"`
	Classifications           map[string]FindingEvidence `json:"classifications"`
	Outcomes                  map[string]EvidenceOutcome `json:"outcomes"`
	FixFindingIDs             []string                   `json:"fix_finding_ids"`
	FollowUps                 []FollowUp                 `json:"follow_ups"`
	ProposedCorrectionLines   *int                       `json:"proposed_correction_lines,omitempty"`
	ActualCorrectionLines     *int                       `json:"actual_correction_lines,omitempty"`
	FixDeltaHash              string                     `json:"fix_delta_hash"`
	OriginalCriteria          *ValidationCheck           `json:"original_criteria,omitempty"`
	CorrectionRegression      *ValidationCheck           `json:"correction_regression,omitempty"`
	EvidenceHash              string                     `json:"evidence_hash,omitempty"`
	InvalidationReason        string                     `json:"invalidation_reason,omitempty"`
	Recovery                  *CompactRecoveryProvenance `json:"recovery,omitempty"`
	CorrectionAttempts        []CompactCorrectionAttempt `json:"correction_attempts,omitempty"`
	CumulativeCorrectionLines int                        `json:"cumulative_correction_lines,omitempty"`
}

type CompactCorrectionAttempt struct {
	Snapshot             Snapshot        `json:"snapshot"`
	ProposedLines        int             `json:"proposed_lines"`
	ActualLines          int             `json:"actual_lines"`
	FixDeltaHash         string          `json:"fix_delta_hash"`
	OriginalCriteria     ValidationCheck `json:"original_criteria"`
	CorrectionRegression ValidationCheck `json:"correction_regression"`
}

type RecoveryDisposition string

const (
	RecoveryScopeChanged RecoveryDisposition = "scope_changed"
	RecoveryInvalidated  RecoveryDisposition = "invalidated"
	RecoveryEscalated    RecoveryDisposition = "escalated"
)

type CompactRecoveryProvenance struct {
	PredecessorLineageID    string              `json:"predecessor_lineage_id"`
	PredecessorRevision     string              `json:"predecessor_revision"`
	Disposition             RecoveryDisposition `json:"disposition"`
	Reason                  string              `json:"reason"`
	Actor                   string              `json:"actor"`
	RecoveredAt             time.Time           `json:"recovered_at"`
	MaintainerAuthorization string              `json:"maintainer_authorization,omitempty"`
}

type CompactReceipt struct {
	Schema             string        `json:"schema"`
	LineageID          string        `json:"lineage_id"`
	Generation         int           `json:"generation"`
	BaseTree           string        `json:"base_tree"`
	InitialReviewTree  string        `json:"initial_review_tree"`
	FinalCandidateTree string        `json:"final_candidate_tree"`
	PathsDigest        string        `json:"paths_digest"`
	FixDeltaHash       string        `json:"fix_delta_hash"`
	PolicyHash         string        `json:"policy_hash"`
	EvidenceHash       string        `json:"evidence_hash"`
	RiskLevel          RiskLevel     `json:"risk_level"`
	SelectedLenses     []string      `json:"selected_lenses"`
	ResolvedFindingIDs []string      `json:"resolved_finding_ids"`
	TerminalState      TerminalState `json:"terminal_state"`
}

type CompactReviewInput struct {
	LensResults     []LensResult
	Classifications []FindingEvidence
	RefuterOutcomes []EvidenceResult
}

func NewCompactState(start Start) (CompactState, error) {
	if start.Mode != ModeOrdinaryBounded {
		return CompactState{}, errors.New("compact reviews require ordinary_bounded mode")
	}
	if start.OriginalChangedLines == nil {
		return CompactState{}, errors.New("compact reviews require repository-derived original changed lines")
	}
	if err := validateLineageID(start.LineageID); err != nil {
		return CompactState{}, err
	}
	if start.Generation < 1 {
		return CompactState{}, errors.New("generation must be positive")
	}
	if err := validateSnapshot(start.Snapshot); err != nil {
		return CompactState{}, err
	}
	if !validSHA256(start.PolicyHash) {
		return CompactState{}, errors.New("policy_hash must be a lowercase SHA-256 identity")
	}
	lenses, err := validateSelectedLenses(start.Mode, start.RiskLevel, start.SelectedLenses)
	if err != nil {
		return CompactState{}, err
	}
	budget, err := CorrectionBudget(*start.OriginalChangedLines)
	if err != nil {
		return CompactState{}, err
	}
	state := CompactState{
		Schema: CompactStateSchema, LineageID: start.LineageID, Generation: start.Generation,
		State: StateReviewing, InitialSnapshot: start.Snapshot, CurrentSnapshot: start.Snapshot,
		GenesisPaths: append([]string(nil), start.Snapshot.Paths...), PolicyHash: start.PolicyHash,
		RiskLevel: start.RiskLevel, SelectedLenses: lenses, OriginalChangedLines: *start.OriginalChangedLines,
		CorrectionBudget: budget, LensResults: []LensResult{}, Findings: []Finding{},
		Classifications: map[string]FindingEvidence{}, Outcomes: map[string]EvidenceOutcome{},
		FixFindingIDs: []string{}, FollowUps: []FollowUp{}, FixDeltaHash: EmptyFixDeltaHash,
	}
	return state, state.Validate()
}

func (state CompactState) Validate() error {
	if state.Schema != CompactStateSchema {
		return errors.New("unsupported compact review state schema")
	}
	if err := validateLineageID(state.LineageID); err != nil {
		return err
	}
	if state.Generation < 1 {
		return errors.New("compact review state requires a positive generation")
	}
	if state.Recovery != nil {
		recovery := state.Recovery
		if validateLineageID(recovery.PredecessorLineageID) != nil || recovery.PredecessorLineageID == state.LineageID ||
			!validSHA256(recovery.PredecessorRevision) || strings.TrimSpace(recovery.Reason) == "" || strings.TrimSpace(recovery.Actor) == "" || recovery.RecoveredAt.IsZero() {
			return errors.New("compact recovery provenance is incomplete or invalid")
		}
		switch recovery.Disposition {
		case RecoveryScopeChanged, RecoveryInvalidated:
		case RecoveryEscalated:
			if strings.TrimSpace(recovery.MaintainerAuthorization) == "" {
				return errors.New("escalated recovery requires maintainer authorization")
			}
		default:
			return errors.New("compact recovery disposition is invalid")
		}
	}
	if err := validateSnapshot(state.InitialSnapshot); err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	if err := validateSnapshot(state.CurrentSnapshot); err != nil {
		return fmt.Errorf("current snapshot: %w", err)
	}
	if err := validateCompactSnapshotMetadata(state.InitialSnapshot); err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	if err := validateCompactSnapshotMetadata(state.CurrentSnapshot); err != nil {
		return fmt.Errorf("current snapshot: %w", err)
	}
	if state.CurrentSnapshot.BaseTree != state.InitialSnapshot.BaseTree && state.CurrentSnapshot.Kind != TargetFixDiff {
		return errors.New("compact current snapshot must retain the original base or be a fix diff")
	}
	paths, err := canonicalPaths(state.GenesisPaths)
	if err != nil || !equalStrings(paths, state.GenesisPaths) || !equalStrings(state.GenesisPaths, state.InitialSnapshot.Paths) {
		return errors.New("compact genesis paths must exactly match the canonical initial scope")
	}
	if err := pathsAreSubset(state.CurrentSnapshot.Paths, state.GenesisPaths); err != nil {
		return err
	}
	if !validSHA256(state.PolicyHash) || !validSHA256(state.FixDeltaHash) {
		return errors.New("compact policy and fix delta hashes must be lowercase SHA-256 identities")
	}
	selected, err := validateSelectedLenses(ModeOrdinaryBounded, state.RiskLevel, state.SelectedLenses)
	if err != nil || !equalStrings(selected, state.SelectedLenses) {
		return errors.New("compact selected lenses are invalid")
	}
	wantBudget, err := CorrectionBudget(state.OriginalChangedLines)
	if err != nil || state.CorrectionBudget != wantBudget {
		return errors.New("compact correction budget does not match original changed lines")
	}
	if state.LensResults == nil || state.Findings == nil || state.Classifications == nil || state.Outcomes == nil || state.FixFindingIDs == nil || state.FollowUps == nil {
		return errors.New("compact review collections must be explicit arrays or objects")
	}
	if len(state.LensResults) > len(state.SelectedLenses) {
		return errors.New("compact review has more results than selected lenses")
	}
	for index, result := range state.LensResults {
		canonical, canonicalErr := CanonicalLensResult(result)
		if canonicalErr != nil || result.Lens != state.SelectedLenses[index] || !reflect.DeepEqual(result, canonical) {
			return errors.New("compact lens results must be complete and canonically ordered")
		}
	}
	if err := validateCompactFindings(state); err != nil {
		return err
	}
	if state.ProposedCorrectionLines != nil && *state.ProposedCorrectionLines <= 0 {
		return errors.New("compact correction forecast must be positive")
	}
	if state.ProposedCorrectionLines != nil && *state.ProposedCorrectionLines > state.CorrectionBudget && (state.State != StateEscalated || state.ActualCorrectionLines != nil) {
		return errors.New("only a terminally escalated compact state may retain an over-budget forecast")
	}
	if state.ActualCorrectionLines != nil && (*state.ActualCorrectionLines < 0 || *state.ActualCorrectionLines > state.CorrectionBudget && state.State != StateEscalated) {
		return errors.New("compact actual correction lines must be within the frozen budget")
	}
	if err := validateCompactCorrection(state); err != nil {
		return err
	}
	switch state.State {
	case StateReviewing:
		if len(state.Findings) != 0 || len(state.Classifications) != 0 || len(state.Outcomes) != 0 || len(state.FixFindingIDs) != 0 || state.ProposedCorrectionLines != nil || state.ActualCorrectionLines != nil || state.EvidenceHash != "" {
			return errors.New("reviewing compact state contains post-review data")
		}
		if state.InvalidationReason != "" {
			return errors.New("reviewing compact state cannot contain an invalidation reason")
		}
	case StateInvalidated:
		reviewing := state
		reviewing.State, reviewing.InvalidationReason = StateReviewing, ""
		if strings.TrimSpace(state.InvalidationReason) == "" || !compactPristineReviewing(reviewing) {
			return errors.New("invalidated compact state must retain only a pristine reviewing authority and reason")
		}
	case StateCorrectionRequired:
		if len(state.LensResults) != len(state.SelectedLenses) || len(state.FixFindingIDs) == 0 || state.EvidenceHash != "" {
			return errors.New("correction-required compact state is incomplete")
		}
	case StateValidating:
		if len(state.LensResults) != len(state.SelectedLenses) || state.EvidenceHash != "" {
			return errors.New("validating compact state is incomplete")
		}
	case StateApproved:
		if !validSHA256(state.EvidenceHash) {
			return errors.New("approved compact state requires verification evidence")
		}
	case StateEscalated:
	default:
		return fmt.Errorf("invalid compact review state %q", state.State)
	}
	return nil
}

func validateCompactSnapshotMetadata(snapshot Snapshot) error {
	paths, err := canonicalPaths(snapshot.Paths)
	if err != nil || !equalStrings(paths, snapshot.Paths) || snapshot.PathsDigest != digestPaths(paths) {
		return errors.New("compact snapshot paths and digest are inconsistent")
	}
	intended, err := canonicalPaths(snapshot.IntendedUntracked)
	if err != nil || !equalStrings(intended, snapshot.IntendedUntracked) {
		return errors.New("compact snapshot intended-untracked paths are not canonical")
	}
	ledgerIDs, err := canonicalStrings(snapshot.LedgerIDs, "ledger id")
	if err != nil || !equalStrings(ledgerIDs, snapshot.LedgerIDs) {
		return errors.New("compact snapshot ledger IDs are not canonical")
	}
	wantIdentity := snapshotIdentity(snapshot.Kind, snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	if snapshot.Identity != wantIdentity {
		return errors.New("compact snapshot identity does not match its metadata")
	}
	return nil
}

func validateCompactFindings(state CompactState) error {
	if state.State == StateReviewing || state.State == StateInvalidated {
		return nil
	}
	if len(state.LensResults) != len(state.SelectedLenses) {
		return errors.New("post-review compact state requires every selected lens result")
	}
	canonicalFindings := make([]Finding, 0, len(state.Findings))
	for _, result := range state.LensResults {
		canonicalFindings = append(canonicalFindings, result.Findings...)
	}
	if !reflect.DeepEqual(canonicalFindings, state.Findings) {
		return errors.New("compact findings must exactly match canonical lens result concatenation")
	}
	seen := make(map[string]Finding, len(state.Findings))
	for _, finding := range state.Findings {
		if err := validateStructuredFinding(finding); err != nil {
			return err
		}
		if _, exists := seen[finding.ID]; exists {
			return fmt.Errorf("duplicate compact finding %q", finding.ID)
		}
		seen[finding.ID] = finding
	}
	fixIDs, err := canonicalStrings(state.FixFindingIDs, "fix finding id")
	if err != nil || !equalStrings(fixIDs, state.FixFindingIDs) {
		return errors.New("compact fix finding IDs must be canonical")
	}
	expectedFixIDs := []string{}
	unresolved := false
	for _, finding := range state.Findings {
		classification, classified := state.Classifications[finding.ID]
		outcome, hasOutcome := state.Outcomes[finding.ID]
		if !isSevereSeverity(finding.Severity) {
			if classified || !hasOutcome || outcome != OutcomeInfo || stringIndex(state.FixFindingIDs, finding.ID) >= 0 {
				return fmt.Errorf("non-severe compact finding %q must be informational only", finding.ID)
			}
			continue
		}
		if !classified || classification.FindingID != finding.ID || !isConcreteEvidence(classification.Proof) {
			return fmt.Errorf("severe compact finding %q requires exactly one concrete classification", finding.ID)
		}
		switch classification.Class {
		case EvidenceDeterministic, EvidenceInferential, EvidenceInsufficient:
		default:
			return fmt.Errorf("compact finding %q has unsupported evidence class %q", finding.ID, classification.Class)
		}
		if !isSupportedCausalDisposition(classification.Causality) || !hasOutcome {
			return fmt.Errorf("compact finding %q has incomplete causal routing", finding.ID)
		}
		if classification.Class == EvidenceInsufficient {
			if outcome != OutcomeInconclusive {
				return fmt.Errorf("insufficient compact finding %q must be inconclusive", finding.ID)
			}
			unresolved = true
			continue
		}
		switch classification.Causality {
		case CausalPreExisting, CausalBaseOnly:
			if outcome != OutcomeInfo || !hasFollowUp(state.FollowUps, causalFollowUp(finding, classification.Proof)) {
				return fmt.Errorf("non-candidate compact finding %q must route to an informational follow-up", finding.ID)
			}
		case CausalUnknown:
			if outcome != OutcomeInconclusive {
				return fmt.Errorf("unknown-causality compact finding %q must be inconclusive", finding.ID)
			}
			unresolved = true
		case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
			switch classification.Class {
			case EvidenceDeterministic:
				if outcome != OutcomeCorroborated {
					return fmt.Errorf("deterministic candidate-causal finding %q must be corroborated", finding.ID)
				}
				expectedFixIDs = append(expectedFixIDs, finding.ID)
			case EvidenceInferential:
				switch outcome {
				case OutcomeCorroborated:
					expectedFixIDs = append(expectedFixIDs, finding.ID)
				case OutcomeRefuted:
				case OutcomeInconclusive:
					unresolved = true
				default:
					return fmt.Errorf("inferential compact finding %q has unsupported outcome %q", finding.ID, outcome)
				}
			}
		}
	}
	if len(state.Classifications) != compactSevereFindingCount(state.Findings) || len(state.Outcomes) != len(state.Findings) {
		return errors.New("compact finding routing contains missing or extra classifications or outcomes")
	}
	sort.Strings(expectedFixIDs)
	if !equalStrings(expectedFixIDs, state.FixFindingIDs) {
		return errors.New("compact fix finding IDs must exactly match candidate-causal corroborated findings")
	}
	if unresolved && state.State != StateEscalated {
		return errors.New("unresolved compact finding routing must be terminally escalated")
	}
	for id := range state.Classifications {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("compact classification %q does not name a finding", id)
		}
	}
	for id := range state.Outcomes {
		if _, exists := seen[id]; !exists {
			return fmt.Errorf("compact outcome %q does not name a finding", id)
		}
	}
	return validateFollowUps(state.FollowUps)
}

func compactSevereFindingCount(findings []Finding) int {
	count := 0
	for _, finding := range findings {
		if isSevereSeverity(finding.Severity) {
			count++
		}
	}
	return count
}

func validateCompactCorrection(state CompactState) error {
	if len(state.CorrectionAttempts) == 0 && state.CumulativeCorrectionLines != 0 {
		return errors.New("compact cumulative correction lines require persisted attempts")
	}
	if len(state.CorrectionAttempts) > 0 {
		base, cumulative := state.InitialSnapshot.CandidateTree, 0
		for _, attempt := range state.CorrectionAttempts {
			if attempt.ProposedLines <= 0 || attempt.ActualLines < 0 || attempt.Snapshot.Kind != TargetFixDiff || attempt.Snapshot.BaseTree != base ||
				!equalStrings(attempt.Snapshot.LedgerIDs, state.FixFindingIDs) || pathsAreSubset(attempt.Snapshot.Paths, state.GenesisPaths) != nil ||
				attempt.FixDeltaHash != FixDeltaHashForSnapshot(attempt.Snapshot) {
				return errors.New("compact correction attempt is outside frozen scope")
			}
			result := ScopedValidationResult{OriginalCriteria: attempt.OriginalCriteria, CorrectionRegression: attempt.CorrectionRegression}
			if err := validateTargetedValidation(result, attempt.FixDeltaHash); err != nil {
				return err
			}
			base, cumulative = attempt.Snapshot.CandidateTree, cumulative+attempt.ActualLines
		}
		if cumulative != state.CumulativeCorrectionLines || cumulative > state.CorrectionBudget && state.State != StateEscalated || !snapshotsEqual(state.CurrentSnapshot, state.CorrectionAttempts[len(state.CorrectionAttempts)-1].Snapshot) {
			return errors.New("compact cumulative correction accounting is invalid")
		}
		if state.State == StateCorrectionRequired {
			if state.ProposedCorrectionLines != nil && state.CumulativeCorrectionLines+*state.ProposedCorrectionLines > state.CorrectionBudget {
				return errors.New("compact correction forecast exceeds the remaining budget")
			}
			if state.ActualCorrectionLines != nil || state.OriginalCriteria != nil || state.CorrectionRegression != nil || state.FixDeltaHash != EmptyFixDeltaHash {
				return errors.New("failed compact correction retained completed attempt state")
			}
			return nil
		}
		if state.State == StateEscalated && state.ProposedCorrectionLines != nil && state.CumulativeCorrectionLines+*state.ProposedCorrectionLines > state.CorrectionBudget && state.ActualCorrectionLines == nil {
			return nil
		}
		if state.State == StateEscalated && len(state.CorrectionAttempts) >= MaxCompactCorrectionAttempts && state.ActualCorrectionLines == nil {
			return nil
		}
	}
	corrected := !snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) || state.FixDeltaHash != EmptyFixDeltaHash || state.ActualCorrectionLines != nil || state.OriginalCriteria != nil || state.CorrectionRegression != nil
	if !corrected {
		if !snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) || state.FixDeltaHash != EmptyFixDeltaHash || state.ActualCorrectionLines != nil || state.OriginalCriteria != nil || state.CorrectionRegression != nil {
			return errors.New("uncorrected compact state contains correction output")
		}
		if state.ProposedCorrectionLines != nil {
			if len(state.FixFindingIDs) == 0 || state.State != StateCorrectionRequired && state.State != StateEscalated {
				return errors.New("compact correction forecast requires pending causal correction")
			}
			if state.State == StateEscalated && *state.ProposedCorrectionLines <= state.CorrectionBudget {
				return errors.New("uncorrected escalated compact forecast must exceed the frozen budget")
			}
		}
		if len(state.FixFindingIDs) > 0 && state.State != StateCorrectionRequired && state.State != StateEscalated {
			return errors.New("candidate-causal compact findings cannot bypass correction")
		}
		return nil
	}
	if len(state.FixFindingIDs) == 0 || state.State == StateReviewing || state.State == StateCorrectionRequired {
		return errors.New("completed compact correction requires causal findings and a post-correction state")
	}
	if state.ProposedCorrectionLines == nil || *state.ProposedCorrectionLines > state.CorrectionBudget || state.ActualCorrectionLines == nil {
		return errors.New("completed compact correction requires in-budget forecast and actual size")
	}
	if state.CurrentSnapshot.Kind != TargetFixDiff || len(state.CorrectionAttempts) == 0 && state.CurrentSnapshot.BaseTree != state.InitialSnapshot.CandidateTree ||
		!equalStrings(state.CurrentSnapshot.LedgerIDs, state.FixFindingIDs) ||
		!equalStrings(state.CurrentSnapshot.IntendedUntracked, state.InitialSnapshot.IntendedUntracked) {
		return errors.New("completed compact correction snapshot is not bound to the original candidate and causal findings")
	}
	if state.FixDeltaHash != FixDeltaHashForSnapshot(state.CurrentSnapshot) {
		return errors.New("compact fix delta hash does not match the correction snapshot")
	}
	if state.OriginalCriteria == nil || state.CorrectionRegression == nil {
		return errors.New("completed compact correction requires both targeted validation checks")
	}
	result := ScopedValidationResult{OriginalCriteria: *state.OriginalCriteria, CorrectionRegression: *state.CorrectionRegression}
	if err := validateTargetedValidation(result, state.FixDeltaHash); err != nil {
		return err
	}
	if (state.State == StateValidating || state.State == StateApproved) && (!state.OriginalCriteria.Passed || !state.CorrectionRegression.Passed) {
		return errors.New("compact correction checks must both pass before validation or approval")
	}
	return nil
}

func (state *CompactState) CompleteReview(input CompactReviewInput) error {
	if state.State != StateReviewing {
		return fmt.Errorf("cannot complete review from compact state %q", state.State)
	}
	if len(input.LensResults) != len(state.SelectedLenses) {
		return fmt.Errorf("compact review requires all %d selected lens results", len(state.SelectedLenses))
	}
	state.LensResults = []LensResult{}
	state.Findings = []Finding{}
	for index, result := range input.LensResults {
		result.Lens = state.SelectedLenses[index]
		canonical, err := CanonicalLensResult(result)
		if err != nil {
			return fmt.Errorf("lens result %d: %w", index+1, err)
		}
		state.LensResults = append(state.LensResults, canonical)
		state.Findings = append(state.Findings, canonical.Findings...)
	}
	severe := map[string]Finding{}
	for _, finding := range state.Findings {
		if isSevereSeverity(finding.Severity) {
			severe[finding.ID] = finding
		} else {
			state.Outcomes[finding.ID] = OutcomeInfo
		}
	}
	classifications := map[string]FindingEvidence{}
	for _, item := range input.Classifications {
		if _, exists := classifications[item.FindingID]; exists {
			return fmt.Errorf("duplicate evidence for finding %q", item.FindingID)
		}
		if _, exists := severe[item.FindingID]; !exists || !isSupportedCausalDisposition(item.Causality) || !isConcreteEvidence(item.Proof) {
			return fmt.Errorf("finding %q requires valid causal evidence", item.FindingID)
		}
		classifications[item.FindingID] = item
	}
	if len(classifications) != len(severe) {
		return errors.New("compact evidence classification must cover every severe finding")
	}
	refuted := map[string]EvidenceResult{}
	for _, result := range input.RefuterOutcomes {
		if _, exists := refuted[result.FindingID]; exists || !isConcreteEvidence(result.Proof) {
			return fmt.Errorf("refuter result %q is invalid", result.FindingID)
		}
		refuted[result.FindingID] = result
	}
	escalate := false
	for _, finding := range state.Findings {
		item, severeFinding := classifications[finding.ID]
		if !severeFinding {
			continue
		}
		state.Classifications[finding.ID] = item
		if item.Class == EvidenceInsufficient {
			state.Outcomes[finding.ID] = OutcomeInconclusive
			escalate = true
			continue
		}
		switch item.Causality {
		case CausalPreExisting, CausalBaseOnly:
			state.Outcomes[finding.ID] = OutcomeInfo
			state.FollowUps = append(state.FollowUps, causalFollowUp(finding, item.Proof))
			continue
		case CausalUnknown:
			state.Outcomes[finding.ID] = OutcomeInconclusive
			escalate = true
			continue
		}
		switch item.Class {
		case EvidenceDeterministic:
			state.Outcomes[finding.ID] = OutcomeCorroborated
			state.FixFindingIDs = append(state.FixFindingIDs, finding.ID)
		case EvidenceInferential:
			result, ok := refuted[finding.ID]
			if !ok {
				return fmt.Errorf("inferential finding %q requires one refuter outcome", finding.ID)
			}
			switch result.Outcome {
			case OutcomeCorroborated:
				state.Outcomes[finding.ID] = result.Outcome
				state.FixFindingIDs = append(state.FixFindingIDs, finding.ID)
			case OutcomeRefuted:
				state.Outcomes[finding.ID] = result.Outcome
			case OutcomeInconclusive:
				state.Outcomes[finding.ID] = result.Outcome
				escalate = true
			default:
				return fmt.Errorf("unsupported refuter outcome %q", result.Outcome)
			}
		default:
			return fmt.Errorf("unsupported evidence class %q", item.Class)
		}
	}
	sort.Strings(state.FixFindingIDs)
	if escalate {
		state.State = StateEscalated
	} else if len(state.FixFindingIDs) > 0 {
		state.State = StateCorrectionRequired
	} else {
		state.State = StateValidating
	}
	return state.Validate()
}

func (state *CompactState) Invalidate(reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("invalidation reason is required")
	}
	if !compactPristineReviewing(*state) {
		return errors.New("only a pristine reviewing compact authority may be invalidated")
	}
	state.State, state.InvalidationReason = StateInvalidated, reason
	return nil
}

func compactPristineReviewing(state CompactState) bool {
	return state.State == StateReviewing && snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) &&
		len(state.LensResults) == 0 && len(state.Findings) == 0 && len(state.Classifications) == 0 && len(state.Outcomes) == 0 &&
		len(state.FixFindingIDs) == 0 && len(state.FollowUps) == 0 && state.ProposedCorrectionLines == nil && state.ActualCorrectionLines == nil &&
		state.FixDeltaHash == EmptyFixDeltaHash && state.OriginalCriteria == nil && state.CorrectionRegression == nil && state.EvidenceHash == "" && state.InvalidationReason == ""
}

func (state *CompactState) BeginCorrection(proposed int) error {
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines != nil {
		return fmt.Errorf("cannot begin correction from compact state %q", state.State)
	}
	if proposed <= 0 {
		return errors.New("compact correction requires a positive changed-line forecast")
	}
	value := proposed
	state.ProposedCorrectionLines = &value
	if state.CumulativeCorrectionLines+proposed > state.CorrectionBudget {
		state.State = StateEscalated
	}
	return state.Validate()
}

func (state *CompactState) CompleteCorrection(snapshot Snapshot, actual int, validation ScopedValidationResult) error {
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil {
		return fmt.Errorf("cannot complete correction from compact state %q", state.State)
	}
	if snapshot.Kind != TargetFixDiff || snapshot.BaseTree != state.CurrentSnapshot.CandidateTree || !equalStrings(snapshot.LedgerIDs, state.FixFindingIDs) {
		return errors.New("compact correction snapshot is not bound to the reviewed candidate and causal findings")
	}
	if err := pathsAreSubset(snapshot.Paths, state.GenesisPaths); err != nil {
		return err
	}
	if actual < 0 {
		return fmt.Errorf("actual correction is %d changed lines, exceeding the frozen budget of %d", actual, state.CorrectionBudget)
	}
	fixHash := FixDeltaHashForSnapshot(snapshot)
	if !equalStrings(validation.LedgerIDs, state.FixFindingIDs) || len(validation.FixCausedFindings) != 0 || validation.FollowUps == nil {
		return errors.New("compact targeted validation must cover the causal finding set without expanding correction scope")
	}
	if err := validateTargetedValidation(validation, fixHash); err != nil {
		return err
	}
	attempt := CompactCorrectionAttempt{Snapshot: snapshot, ProposedLines: *state.ProposedCorrectionLines, ActualLines: actual, FixDeltaHash: fixHash,
		OriginalCriteria: validation.OriginalCriteria, CorrectionRegression: validation.CorrectionRegression}
	state.CorrectionAttempts = append(state.CorrectionAttempts, attempt)
	state.CumulativeCorrectionLines += actual
	state.CurrentSnapshot = snapshot
	state.FollowUps = append(state.FollowUps, validation.FollowUps...)
	if state.CumulativeCorrectionLines > state.CorrectionBudget {
		state.FixDeltaHash, state.ActualCorrectionLines = fixHash, &actual
		original, regression := validation.OriginalCriteria, validation.CorrectionRegression
		state.OriginalCriteria, state.CorrectionRegression = &original, &regression
		state.State = StateEscalated
	} else if validation.OriginalCriteria.Passed && validation.CorrectionRegression.Passed {
		state.FixDeltaHash, state.ActualCorrectionLines = fixHash, &actual
		original, regression := validation.OriginalCriteria, validation.CorrectionRegression
		state.OriginalCriteria, state.CorrectionRegression = &original, &regression
		state.State = StateValidating
	} else {
		state.State = StateCorrectionRequired
		if len(state.CorrectionAttempts) >= MaxCompactCorrectionAttempts {
			state.State = StateEscalated
		}
		state.ProposedCorrectionLines, state.ActualCorrectionLines = nil, nil
		state.FixDeltaHash = EmptyFixDeltaHash
		state.OriginalCriteria, state.CorrectionRegression = nil, nil
	}
	return state.Validate()
}

func (state *CompactState) CompleteVerification(evidence []byte, approved bool) error {
	if state.State != StateValidating {
		return fmt.Errorf("cannot complete verification from compact state %q", state.State)
	}
	if len(evidence) == 0 {
		return errors.New("compact final verification evidence is required")
	}
	sum := sha256.Sum256(evidence)
	state.EvidenceHash = "sha256:" + hex.EncodeToString(sum[:])
	if approved {
		state.State = StateApproved
	} else {
		state.State = StateEscalated
	}
	return state.Validate()
}

func (state CompactState) Receipt() (CompactReceipt, error) {
	var terminal TerminalState
	switch state.State {
	case StateApproved:
		terminal = TerminalApproved
	case StateEscalated:
		terminal = TerminalEscalated
	default:
		return CompactReceipt{}, errors.New("compact receipt requires a terminal state")
	}
	evidence := state.EvidenceHash
	if evidence == "" {
		evidence = EmptyFixDeltaHash
	}
	receipt := CompactReceipt{
		Schema: CompactReceiptSchema, LineageID: state.LineageID, Generation: state.Generation,
		BaseTree: state.InitialSnapshot.BaseTree, InitialReviewTree: state.InitialSnapshot.CandidateTree,
		FinalCandidateTree: state.CurrentSnapshot.CandidateTree, PathsDigest: state.InitialSnapshot.PathsDigest,
		FixDeltaHash: state.FixDeltaHash, PolicyHash: state.PolicyHash, EvidenceHash: evidence,
		RiskLevel: state.RiskLevel, SelectedLenses: append([]string(nil), state.SelectedLenses...),
		ResolvedFindingIDs: append([]string(nil), state.FixFindingIDs...), TerminalState: terminal,
	}
	return receipt, receipt.Validate()
}

func (receipt CompactReceipt) Validate() error {
	if receipt.Schema != CompactReceiptSchema || validateLineageID(receipt.LineageID) != nil || receipt.Generation < 1 {
		return errors.New("invalid compact review receipt identity")
	}
	for _, tree := range []string{receipt.BaseTree, receipt.InitialReviewTree, receipt.FinalCandidateTree} {
		if !validGitTree(tree) {
			return errors.New("compact receipt tree identities are invalid")
		}
	}
	for _, identity := range []string{receipt.PathsDigest, receipt.FixDeltaHash, receipt.PolicyHash, receipt.EvidenceHash} {
		if !validSHA256(identity) {
			return errors.New("compact receipt hashes are invalid")
		}
	}
	if _, err := validateSelectedLenses(ModeOrdinaryBounded, receipt.RiskLevel, receipt.SelectedLenses); err != nil {
		return err
	}
	ids, err := canonicalStrings(receipt.ResolvedFindingIDs, "resolved finding id")
	if err != nil || !equalStrings(ids, receipt.ResolvedFindingIDs) {
		return errors.New("compact receipt resolved finding IDs must be canonical")
	}
	if receipt.TerminalState != TerminalApproved && receipt.TerminalState != TerminalEscalated {
		return errors.New("compact receipt terminal state is invalid")
	}
	return nil
}

func ParseCompactReceipt(payload []byte) (CompactReceipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var receipt CompactReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return CompactReceipt{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CompactReceipt{}, errors.New("multiple JSON values in compact review receipt")
	}
	return receipt, receipt.Validate()
}

func WriteCompactReceiptAtomic(path string, receipt CompactReceipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(payload, '\n'), 0o644)
}

func compactStateEqual(left, right CompactState) bool {
	normalizeCompactState(&left)
	normalizeCompactState(&right)
	return reflect.DeepEqual(left, right)
}

func normalizeCompactState(state *CompactState) {
	normalizeSnapshot := func(snapshot *Snapshot) {
		if snapshot.IntendedUntracked == nil {
			snapshot.IntendedUntracked = []string{}
		}
		if snapshot.LedgerIDs == nil {
			snapshot.LedgerIDs = []string{}
		}
		if snapshot.Paths == nil {
			snapshot.Paths = []string{}
		}
	}
	normalizeSnapshot(&state.InitialSnapshot)
	normalizeSnapshot(&state.CurrentSnapshot)
	if state.GenesisPaths == nil {
		state.GenesisPaths = []string{}
	}
	if state.SelectedLenses == nil {
		state.SelectedLenses = []string{}
	}
	if state.LensResults == nil {
		state.LensResults = []LensResult{}
	}
	for index := range state.LensResults {
		if state.LensResults[index].Findings == nil {
			state.LensResults[index].Findings = []Finding{}
		}
		if state.LensResults[index].Evidence == nil {
			state.LensResults[index].Evidence = []string{}
		}
	}
	if state.Findings == nil {
		state.Findings = []Finding{}
	}
	if state.Classifications == nil {
		state.Classifications = map[string]FindingEvidence{}
	}
	if state.Outcomes == nil {
		state.Outcomes = map[string]EvidenceOutcome{}
	}
	if state.FixFindingIDs == nil {
		state.FixFindingIDs = []string{}
	}
	if state.FollowUps == nil {
		state.FollowUps = []FollowUp{}
	}
}

func compactReceiptEqual(left, right CompactReceipt) bool {
	return reflect.DeepEqual(left, right)
}

func CompactReceiptSchemaOf(payload []byte) string {
	var header struct {
		Schema string `json:"schema"`
	}
	_ = json.Unmarshal(payload, &header)
	return strings.TrimSpace(header.Schema)
}
