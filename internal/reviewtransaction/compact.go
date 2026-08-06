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
	"strconv"
	"strings"
	"time"
)

const CompactStateSchema = "gentle-ai.review-state/v2"
const CompactReceiptSchema = "gentle-ai.review-receipt/v2"
const NativeLowRiskVerificationDomain = "gentle-ai.native-low-risk-verification/v1"
const CompactRecoveredEvidenceSchema = "gentle-ai.review-recovered-evidence/v1"

const (
	StateCorrectionRequired             State = "correction_required"
	StateValidating                     State = "validating"
	MaxCompactCorrectionAttempts              = 1
	historicalCompactCorrectionAttempts       = 3
)

var ErrCompactCorrectionConsumed = errors.New("ordinary compact correction already consumed")

// CompactSemanticStateError identifies a CompactState.Validate() failure with
// its lineage and state, distinguishing a semantic-validation failure from a
// structural one (JSON decode, schema mismatch, or checksum error). Only
// parseCompactRecord's call to Validate() (compact_store.go) constructs this
// type, so errors.As matching it is exact: never a checksum/IO/parse failure
// (issue-1813).
type CompactSemanticStateError struct {
	LineageID string
	State     State
	Problem   string
}

func (err *CompactSemanticStateError) Error() string {
	return fmt.Sprintf("compact lineage %q semantic state %q is invalid: %s", err.LineageID, err.State, err.Problem)
}

// compactLineageQuarantinable reports whether a store-discovery load failure
// is eligible to exclude ONE TERMINAL-for-lineage lineage from enumeration
// (issue-1813) instead of poisoning the entire store. It is true only when
// errors.As matches *CompactSemanticStateError (never a checksum/IO/parse
// failure) AND the failed state is one of {Approved, Escalated, Invalidated}
// — the terminal-for-lineage set. This deliberately differs from
// facadeTerminalState (review_facade.go), which excludes Invalidated: 1813's
// reachable shape is exactly an Invalidated lineage failing semantic
// validation.
func compactLineageQuarantinable(err error) (*CompactSemanticStateError, bool) {
	var semantic *CompactSemanticStateError
	if !errors.As(err, &semantic) {
		return nil, false
	}
	switch semantic.State {
	case StateApproved, StateEscalated, StateInvalidated:
		return semantic, true
	default:
		return nil, false
	}
}

type CompactState struct {
	Schema                       string                       `json:"schema"`
	LineageID                    string                       `json:"lineage_id"`
	Generation                   int                          `json:"generation"`
	State                        State                        `json:"state"`
	InitialSnapshot              Snapshot                     `json:"initial_snapshot"`
	CurrentSnapshot              Snapshot                     `json:"current_snapshot"`
	GenesisPaths                 []string                     `json:"genesis_paths"`
	PolicyHash                   string                       `json:"policy_hash"`
	RiskLevel                    RiskLevel                    `json:"risk_level"`
	SelectedLenses               []string                     `json:"selected_lenses"`
	OriginalChangedLines         int                          `json:"original_changed_lines"`
	CorrectionBudget             int                          `json:"correction_budget"`
	LensResults                  []LensResult                 `json:"lens_results"`
	Findings                     []Finding                    `json:"findings"`
	Classifications              map[string]FindingEvidence   `json:"classifications"`
	Outcomes                     map[string]EvidenceOutcome   `json:"outcomes"`
	FixFindingIDs                []string                     `json:"fix_finding_ids"`
	FollowUps                    []FollowUp                   `json:"follow_ups"`
	ProposedCorrectionLines      *int                         `json:"proposed_correction_lines,omitempty"`
	ActualCorrectionLines        *int                         `json:"actual_correction_lines,omitempty"`
	FixDeltaHash                 string                       `json:"fix_delta_hash"`
	OriginalCriteria             *ValidationCheck             `json:"original_criteria,omitempty"`
	CorrectionRegression         *ValidationCheck             `json:"correction_regression,omitempty"`
	EvidenceHash                 string                       `json:"evidence_hash,omitempty"`
	EvidenceRecordDigest         string                       `json:"evidence_record_digest,omitempty"`
	EvidenceOutcome              VerificationOutcome          `json:"evidence_outcome,omitempty"`
	EvidenceTargetIdentity       string                       `json:"evidence_target_identity,omitempty"`
	EvidenceAuthorityRevision    string                       `json:"evidence_authority_revision,omitempty"`
	CorrectionVerificationTarget *Snapshot                    `json:"correction_verification_target,omitempty"`
	InvalidationReason           string                       `json:"invalidation_reason,omitempty"`
	InvalidationEvidence         *CompactInvalidationEvidence `json:"invalidation_evidence,omitempty"`
	Recovery                     *CompactRecoveryProvenance   `json:"recovery,omitempty"`
	CorrectionAttempts           []CompactCorrectionAttempt   `json:"correction_attempts,omitempty"`
	CumulativeCorrectionLines    int                          `json:"cumulative_correction_lines,omitempty"`
	ResultDispositions           []CompactResultDisposition   `json:"result_dispositions,omitempty"`
	ResultReopens                []CompactResultReopen        `json:"result_reopens,omitempty"`
	ReviewerContextLevel         ReviewerContextLevel         `json:"reviewer_context_level,omitempty"`
}

// CorrectionBudgetExceededError identifies a repository-derived correction
// whose changed lines exceed the authority's remaining correction budget.
type CorrectionBudgetExceededError struct {
	Actual    int
	Remaining int
}

func (err *CorrectionBudgetExceededError) Error() string {
	return fmt.Sprintf("actual correction is %d changed lines, exceeding the remaining budget of %d", err.Actual, err.Remaining)
}

func IsCorrectionBudgetExceeded(err error) bool {
	var budgetErr *CorrectionBudgetExceededError
	return errors.As(err, &budgetErr)
}

// CompactResultReopenSlot binds one selected lens artifact at the exact
// validating revision from which results were reopened. Quarantined slots may
// be historical unadmitted artifacts and therefore omit subject metadata;
// retained slots must carry the admitted provider subject.
type CompactResultReopenSlot struct {
	Lens              string `json:"lens"`
	SelectedOrder     int    `json:"selected_order"`
	ArtifactDigest    string `json:"artifact_digest"`
	ResultHash        string `json:"result_hash"`
	SubjectHash       string `json:"subject_hash,omitempty"`
	AuthorityRevision string `json:"authority_revision,omitempty"`
}

// CompactResultReopen is the durable audit record for one exact-revision
// same-lineage reviewer-result repair back to reviewing. It preserves all
// original scope and budget inputs while identifying the unusable slots and
// every admitted slot retained. AuthorizedLenses names the admitted slots the
// maintainer explicitly overrode: those results were structurally valid and
// provider-admitted, and only this recorded authorization — never native
// detection — moved them into quarantine. An observer therefore always sees
// which quarantined results a maintainer discarded by decision rather than
// which ones the store proved unusable.
type CompactResultReopen struct {
	PreviousRevision        string                    `json:"previous_revision"`
	TargetIdentity          string                    `json:"target_identity"`
	Quarantined             []CompactResultReopenSlot `json:"quarantined"`
	Retained                []CompactResultReopenSlot `json:"retained"`
	AuthorizedLenses        []string                  `json:"authorized_lenses,omitempty"`
	Reason                  string                    `json:"reason"`
	Actor                   string                    `json:"actor"`
	ReopenedAt              time.Time                 `json:"reopened_at"`
	MaintainerAuthorization string                    `json:"maintainer_authorization"`
}

// ResultDispositionClass names which class of failure makes one preserved
// reviewer result inapplicable to the frozen candidate. The two classes are
// deliberately distinct: a transport or syntax failure says the payload never
// decoded, while a wrong-target failure says a decodable payload described a
// candidate that is not the frozen one. Both are recorded verbatim so an
// auditor can tell which claim was actually proven.
type ResultDispositionClass string

const (
	ResultDispositionTransportSyntax ResultDispositionClass = "transport_syntax"
	ResultDispositionWrongTarget     ResultDispositionClass = "wrong_target"
)

// ResultIncidentClass names which extraction-failure shape a preserved raw
// reviewer envelope was classified as at the plugin boundary. This is a
// distinct type from ResultDispositionClass on purpose: disposition classes
// judge candidate-inapplicability of a decodable payload, while incident
// classes describe why the plugin could never extract a payload at all.
type ResultIncidentClass string

const (
	ResultIncidentEmptyResult    ResultIncidentClass = "empty_result"
	ResultIncidentNestedEnvelope ResultIncidentClass = "nested_envelope"
)

// ValidResultIncidentClass reports whether c is a known incident class or the
// empty string (backward-compatible: omitting --class remains valid).
func ValidResultIncidentClass(c ResultIncidentClass) bool {
	switch c {
	case "", ResultIncidentEmptyResult, ResultIncidentNestedEnvelope:
		return true
	default:
		return false
	}
}

// CompactResultDisposition records one audited refusal of a preserved reviewer
// result as candidate-inapplicable. It binds the exact lens, selected order,
// frozen target identity, and preserved-artifact digest it dispositions, and
// it never carries findings, evidence, or any other admissible review content:
// a disposition terminally escalates a lineage, it never contributes to one.
type CompactResultDisposition struct {
	Lens           string                 `json:"lens"`
	SelectedOrder  int                    `json:"selected_order"`
	TargetIdentity string                 `json:"target_identity"`
	ArtifactDigest string                 `json:"artifact_digest"`
	Class          ResultDispositionClass `json:"class"`
	// PayloadDecodable records the decodability the disposition actually
	// observed in the preserved bytes. It is what makes the two classes
	// mutually exclusive in persisted shape: transport_syntax may only be
	// recorded for a payload that did not decode, and wrong_target only for one
	// that did, so no stored record can claim the stronger semantic class over
	// a payload that never decoded at all.
	PayloadDecodable        bool      `json:"payload_decodable,omitempty"`
	Diagnostic              string    `json:"diagnostic"`
	AbsentPaths             []string  `json:"absent_paths,omitempty"`
	Reason                  string    `json:"reason"`
	Actor                   string    `json:"actor"`
	DisposedAt              time.Time `json:"disposed_at"`
	MaintainerAuthorization string    `json:"maintainer_authorization"`
}

type CompactCorrectionAttempt struct {
	Snapshot                      Snapshot        `json:"snapshot"`
	ProposedLines                 int             `json:"proposed_lines"`
	ActualLines                   int             `json:"actual_lines"`
	FixDeltaHash                  string          `json:"fix_delta_hash"`
	OriginalCriteria              ValidationCheck `json:"original_criteria"`
	CorrectionRegression          ValidationCheck `json:"correction_regression"`
	TargetedValidationRequestHash string          `json:"targeted_validation_request_hash,omitempty"`
	CorrectionTargetIdentity      string          `json:"correction_target_identity,omitempty"`
}

type CompactInvalidationEvidence struct {
	Gate    GateKind    `json:"gate"`
	Reason  string      `json:"reason"`
	Context GateContext `json:"context"`
}

type RecoveryDisposition string

const (
	RecoveryScopeChanged           RecoveryDisposition = "scope_changed"
	RecoveryInvalidated            RecoveryDisposition = "invalidated"
	RecoveryEscalated              RecoveryDisposition = "escalated"
	RecoveryFinalVerificationRetry RecoveryDisposition = "final_verification_retry"
)

type CompactRecoveryProvenance struct {
	PredecessorLineageID       string                              `json:"predecessor_lineage_id"`
	PredecessorRevision        string                              `json:"predecessor_revision"`
	Disposition                RecoveryDisposition                 `json:"disposition"`
	Reason                     string                              `json:"reason"`
	Actor                      string                              `json:"actor"`
	RecoveredAt                time.Time                           `json:"recovered_at"`
	MaintainerAuthorization    string                              `json:"maintainer_authorization,omitempty"`
	ConsumedCorrectionAttempts int                                 `json:"consumed_correction_attempts,omitempty"`
	ConsumedCorrectionLines    int                                 `json:"consumed_correction_lines,omitempty"`
	Evidence                   *CompactRecoveredEvidence           `json:"evidence,omitempty"`
	FinalVerificationRetry     *CompactFinalVerificationRetryProof `json:"final_verification_retry,omitempty"`
}

// CompactRecoveredEvidence is the self-contained provenance for the only
// recovery that may reuse review/correction evidence: an accounting-only
// escalated predecessor whose corrected bytes are exactly the successor
// target. The source attempt remains byte-for-byte visible while
// NativeCorrectionLines records the repository-derived correction size used by
// the successor.
type CompactRecoveredEvidence struct {
	Schema                    string                    `json:"schema"`
	Relation                  string                    `json:"relation"`
	PathRelation              string                    `json:"path_relation"`
	SuccessorTargetIdentity   string                    `json:"successor_target_identity"`
	ReviewEvidenceHash        string                    `json:"review_evidence_hash"`
	SourceCorrectionAttempt   CompactCorrectionAttempt  `json:"source_correction_attempt"`
	NativeCorrectionLines     int                       `json:"native_correction_lines"`
	TargetedValidationRequest TargetedValidationRequest `json:"targeted_validation_request"`
}

type CompactReceipt struct {
	Schema                    string              `json:"schema"`
	LineageID                 string              `json:"lineage_id"`
	Projection                Projection          `json:"projection,omitempty"`
	Generation                int                 `json:"generation"`
	BaseTree                  string              `json:"base_tree"`
	InitialReviewTree         string              `json:"initial_review_tree"`
	FinalCandidateTree        string              `json:"final_candidate_tree"`
	PathsDigest               string              `json:"paths_digest"`
	FixDeltaHash              string              `json:"fix_delta_hash"`
	PolicyHash                string              `json:"policy_hash"`
	EvidenceHash              string              `json:"evidence_hash"`
	EvidenceRecordDigest      string              `json:"evidence_record_digest,omitempty"`
	EvidenceOutcome           VerificationOutcome `json:"evidence_outcome,omitempty"`
	EvidenceTargetIdentity    string              `json:"evidence_target_identity,omitempty"`
	EvidenceAuthorityRevision string              `json:"evidence_authority_revision,omitempty"`
	RiskLevel                 RiskLevel           `json:"risk_level"`
	SelectedLenses            []string            `json:"selected_lenses"`
	ResolvedFindingIDs        []string            `json:"resolved_finding_ids"`
	TerminalState             TerminalState       `json:"terminal_state"`
	// ReviewerContextLevel records the mechanism that put the immutable
	// candidate evidence in front of this review's reviewers. It is recorded
	// only, never gated on, and never compared to another level: whether any
	// delivery gate ever requires a particular mechanism is a separate
	// decision, and this field exists now because it cannot be backfilled — a
	// receipt issued without it can never be classified later.
	//
	// It is omitempty, and absence means "not recorded" rather than any
	// particular mechanism. That keeps every receipt issued before this field
	// existed re-derivable byte-identically, which its immutable publication
	// requires.
	ReviewerContextLevel ReviewerContextLevel `json:"reviewer_context_level,omitempty"`
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
	if err := validateCompactSnapshot(start.Snapshot); err != nil {
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
		case RecoveryFinalVerificationRetry:
			if recovery.FinalVerificationRetry == nil || recovery.MaintainerAuthorization == "" {
				return errors.New("final-verification retry recovery requires exact source proof and maintainer authorization")
			}
			if err := validateCompactFinalVerificationRetryProofShape(state, *recovery); err != nil {
				return err
			}
		default:
			return errors.New("compact recovery disposition is invalid")
		}
		if recovery.Evidence != nil && recovery.Disposition != RecoveryEscalated {
			return errors.New("only escalated recovery may carry predecessor evidence")
		}
		if recovery.FinalVerificationRetry != nil && recovery.Disposition != RecoveryFinalVerificationRetry {
			return errors.New("only final-verification retry recovery may carry final-verification source proof")
		}
		if recovery.ConsumedCorrectionAttempts < 0 || recovery.ConsumedCorrectionAttempts > MaxCompactCorrectionAttempts ||
			recovery.ConsumedCorrectionLines < 0 || recovery.ConsumedCorrectionLines > state.CorrectionBudget ||
			recovery.ConsumedCorrectionAttempts == 0 && recovery.ConsumedCorrectionLines != 0 {
			return errors.New("compact recovery correction accounting is invalid") // refusal:by-design world-action: malformed persisted accounting cannot be repaired without replacing its provider-owned authority
		}
		if recovery.ConsumedCorrectionAttempts > 0 && recovery.Disposition != RecoveryScopeChanged {
			return errors.New("only scope-changed recovery may preserve consumed correction accounting") // refusal:by-design world-action: contradictory persisted recovery provenance requires code or storage repair
		}
	}
	if err := validateCompactResultDispositions(state); err != nil {
		return err
	}
	if err := validateCompactResultReopens(state); err != nil {
		return err
	}
	if state.State != StateInvalidated && state.InvalidationEvidence != nil {
		return errors.New("only an invalidated compact state may contain invalidation evidence")
	}
	if state.State == StateInvalidated && strings.TrimSpace(state.InvalidationReason) != "" && state.InvalidationEvidence != nil {
		evidence := state.InvalidationEvidence
		payload, err := json.Marshal(evidence.Context)
		parsed, parseErr := ParseGateContext(payload)
		if err != nil || parseErr != nil || !reflect.DeepEqual(parsed, evidence.Context) || evidence.Gate != evidence.Context.Gate ||
			evidence.Reason != state.InvalidationReason || evidence.Context.LineageID != state.LineageID || evidence.Context.Generation != state.Generation {
			return errors.New("approved compact invalidation evidence is incomplete or invalid")
		}
		approved := state
		approved.State, approved.InvalidationReason, approved.InvalidationEvidence = StateApproved, "", nil
		approvedRecord, _, recordErr := makeCompactRecord(approved)
		if approved.Validate() == nil && recordErr == nil && evidence.Context.StoreRevision == approvedRecord.Revision {
			return nil
		}
		return errors.New("approved compact invalidation evidence does not bind its predecessor revision")
	}
	if err := validateCompactSnapshot(state.InitialSnapshot); err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	if err := validateCompactSnapshot(state.CurrentSnapshot); err != nil {
		return fmt.Errorf("current snapshot: %w", err)
	}
	if err := validateCompactSnapshotMetadata(state.InitialSnapshot); err != nil {
		return fmt.Errorf("initial snapshot: %w", err)
	}
	if err := validateCompactSnapshotMetadata(state.CurrentSnapshot); err != nil {
		return fmt.Errorf("current snapshot: %w", err)
	}
	if state.CurrentSnapshot.Projection != state.InitialSnapshot.Projection {
		return errors.New("compact current snapshot must retain the initial projection")
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
	preservedRecoveryBudget := state.Recovery != nil && state.Recovery.ConsumedCorrectionAttempts > 0
	if err != nil || state.CorrectionBudget != wantBudget && !preservedRecoveryBudget {
		return errors.New("compact correction budget does not match original changed lines")
	}
	if state.LensResults == nil || state.Findings == nil || state.Classifications == nil || state.Outcomes == nil || state.FixFindingIDs == nil || state.FollowUps == nil {
		return errors.New("compact review collections must be explicit arrays or objects")
	}
	if len(state.LensResults) > len(state.SelectedLenses) {
		return errors.New("compact review has more results than selected lenses")
	}
	for index, result := range state.LensResults {
		canonical, canonicalErr := CanonicalCompactLensResult(result)
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
	if err := validateCompactVerificationEvidence(state); err != nil {
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

func validateCompactVerificationEvidence(state CompactState) error {
	hasRecordBinding := state.EvidenceRecordDigest != "" || state.EvidenceOutcome != "" ||
		state.EvidenceTargetIdentity != "" || state.EvidenceAuthorityRevision != ""
	if !hasRecordBinding {
		return nil
	}
	expectedTarget := state.CurrentSnapshot.Identity
	if state.CorrectionVerificationTarget != nil {
		expectedTarget = state.CorrectionVerificationTarget.Identity
	} else if len(state.CorrectionAttempts) > 0 {
		expectedTarget = state.CorrectionAttempts[len(state.CorrectionAttempts)-1].Snapshot.Identity
	}
	if !validSHA256(state.EvidenceHash) || !validSHA256(state.EvidenceRecordDigest) ||
		!validSHA256(state.EvidenceTargetIdentity) || !validSHA256(state.EvidenceAuthorityRevision) ||
		state.EvidenceTargetIdentity != expectedTarget || !validVerificationOutcome(state.EvidenceOutcome) {
		return errors.New("compact verification evidence record binding is incomplete or invalid") // refusal:by-design world-action: persisted authority metadata is corrupt and cannot be reconstructed from an untrusted partial binding
	}
	switch state.EvidenceOutcome {
	case VerificationOutcomePassed:
		if state.State != StateApproved {
			return errors.New("passed compact verification evidence requires approved authority") // refusal:by-design world-action: persisted outcome and lifecycle state contradict each other and require code or storage repair
		}
	case VerificationOutcomeFailed, VerificationOutcomeProceduralFailure:
		if state.State != StateEscalated {
			return errors.New("failed compact verification evidence requires escalated authority") // refusal:by-design world-action: persisted outcome and lifecycle state contradict each other and require code or storage repair
		}
	}
	return nil
}

// LedgerHash derives the canonical findings-ledger binding of the
// authoritative compact record. Compact authority never persists a separate
// ledger artifact: the frozen findings themselves are the ledger, validated by
// Validate as the exact concatenation of the completed lens results. When at
// least one finding was frozen, the binding is the SHA-256 of the canonical
// gentle-ai.review-ledger/v1 bytes for exactly those findings, so auditors can
// reconstruct and verify it from the persisted state. A pristine lineage — one
// whose completed review froze no findings at all — has no ledger content to
// bind and keeps the honest empty-input hash (SHA-256 of zero bytes); it never
// fabricates a canonical empty-ledger artifact that was not persisted.
func (state CompactState) LedgerHash() string {
	if len(state.Findings) == 0 {
		return EmptyFixDeltaHash
	}
	// CanonicalLedger only fails for a nil findings array, which the length
	// guard above already excludes.
	ledger, err := CanonicalLedger(state.Findings)
	if err != nil {
		return EmptyFixDeltaHash
	}
	sum := sha256.Sum256(ledger)
	return "sha256:" + hex.EncodeToString(sum[:])
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
	wantIdentity := snapshotIdentityForProjection(snapshot.Kind, snapshot.Projection, snapshot.BaseTree, snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	if snapshot.Identity != wantIdentity {
		return errors.New("compact snapshot identity does not match its metadata")
	}
	return nil
}

func validateCompactFindings(state CompactState) error {
	if state.State == StateReviewing || state.State == StateInvalidated {
		return nil
	}
	// A lineage terminally escalated by an audited reviewer-result disposition
	// never completed its review, so by construction it holds no lens results
	// to require. The exemption is exactly as narrow as that shape: it demands
	// that no review content was frozen at all, so it can never excuse a
	// partially completed review from the ordinary every-lens requirement.
	if state.State == StateEscalated && len(state.ResultDispositions) > 0 {
		if len(state.LensResults) != 0 || len(state.Findings) != 0 || len(state.Classifications) != 0 ||
			len(state.Outcomes) != 0 || len(state.FixFindingIDs) != 0 || state.EvidenceHash != "" {
			return errors.New("a reviewer-result-dispositioned compact state must hold no frozen review content")
		}
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
		if err := validateLensFinding(finding, true); err != nil {
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
	if state.Recovery != nil && state.Recovery.Evidence != nil {
		return validateCompactRecoveredCorrection(state, *state.Recovery.Evidence)
	}
	if len(state.CorrectionAttempts) == 0 && state.CumulativeCorrectionLines != 0 {
		return errors.New("compact cumulative correction lines require persisted attempts")
	}
	if len(state.CorrectionAttempts) > 0 {
		base, cumulative := state.InitialSnapshot.CandidateTree, 0
		for _, attempt := range state.CorrectionAttempts {
			if attempt.ProposedLines <= 0 || attempt.ActualLines < 0 || attempt.Snapshot.Kind != TargetFixDiff || attempt.Snapshot.Projection != state.InitialSnapshot.Projection || attempt.Snapshot.BaseTree != base ||
				!equalStrings(attempt.Snapshot.LedgerIDs, state.FixFindingIDs) || pathsAreSubset(attempt.Snapshot.Paths, state.GenesisPaths) != nil ||
				validateCompactSnapshot(attempt.Snapshot) != nil || validateCompactSnapshotMetadata(attempt.Snapshot) != nil ||
				attempt.FixDeltaHash != FixDeltaHashForSnapshot(attempt.Snapshot) {
				return errors.New("compact correction attempt is outside frozen scope")
			}
			result := ScopedValidationResult{
				OriginalCriteria: attempt.OriginalCriteria, CorrectionRegression: attempt.CorrectionRegression,
				TargetedValidationRequestHash: attempt.TargetedValidationRequestHash, CorrectionTargetIdentity: attempt.CorrectionTargetIdentity,
			}
			if err := validateTargetedValidation(result, attempt.FixDeltaHash); err != nil {
				return err
			}
			if attempt.CorrectionTargetIdentity != "" && attempt.CorrectionTargetIdentity != attempt.Snapshot.Identity {
				return errors.New("compact correction attempt targets a different immutable correction")
			}
			base, cumulative = attempt.Snapshot.CandidateTree, cumulative+attempt.ActualLines
		}
		last := state.CorrectionAttempts[len(state.CorrectionAttempts)-1].Snapshot
		if cumulative != state.CumulativeCorrectionLines || cumulative > state.CorrectionBudget && state.State != StateEscalated ||
			!snapshotsEqual(state.CurrentSnapshot, last) && validateCompactCorrectedCandidate(state, last) != nil {
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
		if state.State == StateEscalated && len(state.CorrectionAttempts) >= historicalCompactCorrectionAttempts &&
			state.ProposedCorrectionLines == nil && state.ActualCorrectionLines == nil && state.FixDeltaHash == EmptyFixDeltaHash &&
			state.OriginalCriteria == nil && state.CorrectionRegression == nil && state.EvidenceHash == "" {
			return nil
		}
	}
	if state.CorrectionVerificationTarget != nil {
		target := state.CorrectionVerificationTarget
		if state.State != StateEscalated || state.EvidenceOutcome != VerificationOutcomeProceduralFailure ||
			len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 || state.ProposedCorrectionLines == nil ||
			*state.ProposedCorrectionLines > state.CorrectionBudget || state.ActualCorrectionLines != nil ||
			state.OriginalCriteria != nil || state.CorrectionRegression != nil || state.FixDeltaHash != EmptyFixDeltaHash ||
			target.Kind != TargetFixDiff || target.Projection != state.InitialSnapshot.Projection ||
			target.BaseTree != state.CurrentSnapshot.CandidateTree || !equalStrings(target.LedgerIDs, state.FixFindingIDs) ||
			pathsAreSubset(target.Paths, state.GenesisPaths) != nil {
			return errors.New("procedural correction verification target is invalid") // refusal:by-design world-action: persisted terminal evidence targets an invalid correction snapshot and cannot authorize continuation
		}
		if err := validateCompactSnapshot(*target); err != nil {
			return err
		}
		return nil
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
	correction := state.CurrentSnapshot
	if len(state.CorrectionAttempts) > 0 {
		correction = state.CorrectionAttempts[len(state.CorrectionAttempts)-1].Snapshot
	}
	if correction.Kind != TargetFixDiff || len(state.CorrectionAttempts) == 0 && correction.BaseTree != state.InitialSnapshot.CandidateTree ||
		!equalStrings(correction.LedgerIDs, state.FixFindingIDs) ||
		!equalStrings(correction.IntendedUntracked, state.InitialSnapshot.IntendedUntracked) {
		return errors.New("completed compact correction snapshot is not bound to the original candidate and causal findings")
	}
	if state.FixDeltaHash != FixDeltaHashForSnapshot(correction) {
		return errors.New("compact fix delta hash does not match the correction snapshot")
	}
	if state.OriginalCriteria == nil || state.CorrectionRegression == nil {
		return errors.New("completed compact correction requires both targeted validation checks")
	}
	result := ScopedValidationResult{OriginalCriteria: *state.OriginalCriteria, CorrectionRegression: *state.CorrectionRegression}
	if len(state.CorrectionAttempts) > 0 {
		last := state.CorrectionAttempts[len(state.CorrectionAttempts)-1]
		if *state.ProposedCorrectionLines != last.ProposedLines || *state.ActualCorrectionLines != last.ActualLines ||
			state.FixDeltaHash != last.FixDeltaHash || *state.OriginalCriteria != last.OriginalCriteria ||
			*state.CorrectionRegression != last.CorrectionRegression {
			return errors.New("completed compact correction does not match its latest attempt")
		}
		result.TargetedValidationRequestHash = last.TargetedValidationRequestHash
		result.CorrectionTargetIdentity = last.CorrectionTargetIdentity
	}
	if err := validateTargetedValidation(result, state.FixDeltaHash); err != nil {
		return err
	}
	if (state.State == StateValidating || state.State == StateApproved) && (!state.OriginalCriteria.Passed || !state.CorrectionRegression.Passed) {
		return errors.New("compact correction checks must both pass before validation or approval")
	}
	return nil
}

func validateCompactCorrectedCandidate(state CompactState, correction Snapshot) error {
	current, initial := state.CurrentSnapshot, state.InitialSnapshot
	if current.Kind != initial.Kind || current.Projection != initial.Projection || current.UnbornHead != correction.UnbornHead ||
		current.BaseTree != initial.BaseTree || current.CandidateTree != correction.CandidateTree ||
		current.IntendedUntrackedProof != correction.IntendedUntrackedProof ||
		!equalStrings(current.IntendedUntracked, initial.IntendedUntracked) || !equalStrings(current.LedgerIDs, initial.LedgerIDs) ||
		pathsAreSubset(current.Paths, state.GenesisPaths) != nil {
		return errors.New("terminal correction authority does not preserve the complete reviewed candidate") // refusal:by-design world-action: contradictory persisted authority requires code or storage repair
	}
	return nil
}

func validateCompactRecoveredCorrection(state CompactState, evidence CompactRecoveredEvidence) error {
	if evidence.Schema != CompactRecoveredEvidenceSchema ||
		evidence.Relation != string(compactTargetChangedScope) || evidence.PathRelation != string(compactPathsSame) ||
		evidence.SuccessorTargetIdentity != state.InitialSnapshot.Identity || !validSHA256(evidence.ReviewEvidenceHash) ||
		evidence.NativeCorrectionLines <= 0 {
		return errors.New("recovered correction evidence binding is incomplete")
	}
	if state.State != StateValidating && state.State != StateApproved && state.State != StateEscalated ||
		!snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) || len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 ||
		state.ProposedCorrectionLines == nil || state.ActualCorrectionLines == nil ||
		*state.ProposedCorrectionLines != evidence.SourceCorrectionAttempt.ProposedLines ||
		*state.ActualCorrectionLines != evidence.NativeCorrectionLines || len(state.FixFindingIDs) == 0 {
		return errors.New("recovered correction state does not preserve the imported evidence shape")
	}
	attempt := evidence.SourceCorrectionAttempt
	if attempt.ProposedLines <= 0 || attempt.ActualLines <= evidence.NativeCorrectionLines ||
		evidence.NativeCorrectionLines > attempt.ProposedLines || attempt.Snapshot.Kind != TargetFixDiff ||
		attempt.Snapshot.CandidateTree != state.InitialSnapshot.CandidateTree ||
		attempt.Snapshot.Projection != state.InitialSnapshot.Projection ||
		!equalStrings(attempt.Snapshot.LedgerIDs, state.FixFindingIDs) ||
		pathsAreSubset(attempt.Snapshot.Paths, state.GenesisPaths) != nil ||
		attempt.FixDeltaHash != FixDeltaHashForSnapshot(attempt.Snapshot) || state.FixDeltaHash != attempt.FixDeltaHash ||
		state.OriginalCriteria == nil || state.CorrectionRegression == nil ||
		*state.OriginalCriteria != attempt.OriginalCriteria || *state.CorrectionRegression != attempt.CorrectionRegression ||
		!attempt.OriginalCriteria.Passed || !attempt.CorrectionRegression.Passed {
		return errors.New("recovered correction does not match the accounting-only source attempt")
	}
	if err := validateCompactSnapshot(attempt.Snapshot); err != nil {
		return fmt.Errorf("recovered correction snapshot: %w", err)
	}
	if err := validateCompactSnapshotMetadata(attempt.Snapshot); err != nil {
		return fmt.Errorf("recovered correction snapshot: %w", err)
	}
	validation := ScopedValidationResult{
		OriginalCriteria: attempt.OriginalCriteria, CorrectionRegression: attempt.CorrectionRegression,
		TargetedValidationRequestHash: attempt.TargetedValidationRequestHash, CorrectionTargetIdentity: attempt.CorrectionTargetIdentity,
	}
	if err := validateTargetedValidation(validation, attempt.FixDeltaHash); err != nil ||
		attempt.CorrectionTargetIdentity != "" && attempt.CorrectionTargetIdentity != attempt.Snapshot.Identity {
		return errors.New("recovered source validator binding is invalid")
	}
	request := evidence.TargetedValidationRequest
	if err := ValidateTargetedValidationRequest(request); err != nil ||
		request.LineageID != state.Recovery.PredecessorLineageID || request.ExpectedRevision != state.Recovery.PredecessorRevision ||
		request.CorrectionCandidateTree != attempt.Snapshot.CandidateTree || request.CorrectionTargetIdentity != attempt.Snapshot.Identity ||
		!equalStrings(request.CorrectionPaths, attempt.Snapshot.Paths) || request.CorrectionPathsDigest != attempt.Snapshot.PathsDigest ||
		!equalStrings(request.FixFindingIDs, state.FixFindingIDs) {
		return errors.New("recovered targeted validation request does not bind the source correction")
	}
	if compactReviewEvidenceHash(state) != evidence.ReviewEvidenceHash {
		return errors.New("recovered review evidence does not match the imported authority")
	}
	if state.State == StateValidating && state.EvidenceHash != "" ||
		(state.State == StateApproved || state.State == StateEscalated) && !validSHA256(state.EvidenceHash) {
		return errors.New("recovered correction has invalid final verification evidence")
	}
	return nil
}

func compactReviewEvidenceHash(state CompactState) string {
	payload, _ := json.Marshal(struct {
		LensResults     []LensResult               `json:"lens_results"`
		Findings        []Finding                  `json:"findings"`
		Classifications map[string]FindingEvidence `json:"classifications"`
		Outcomes        map[string]EvidenceOutcome `json:"outcomes"`
		FixFindingIDs   []string                   `json:"fix_finding_ids"`
		FollowUps       []FollowUp                 `json:"follow_ups"`
	}{
		LensResults: state.LensResults, Findings: state.Findings, Classifications: state.Classifications,
		Outcomes: state.Outcomes, FixFindingIDs: state.FixFindingIDs, FollowUps: state.FollowUps,
	})
	sum := sha256.Sum256(append([]byte(CompactRecoveredEvidenceSchema+"/review\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (state *CompactState) CompleteReview(input CompactReviewInput) error {
	if state.State != StateReviewing {
		return fmt.Errorf("cannot complete review from compact state %q", state.State)
	}
	if len(input.LensResults) != len(state.SelectedLenses) {
		return fmt.Errorf("compact review requires all %d selected lens results", len(state.SelectedLenses))
	}
	// Historical validating authority may contain evidence from a provider
	// that never inspected the candidate. Keep that state parseable so the
	// explicit reopen transition can quarantine it, but never admit the same
	// evidence through a new review completion.
	for index, result := range input.LensResults {
		for _, evidence := range result.Evidence {
			if evidenceReportsUnavailableInspection(evidence) {
				return fmt.Errorf("lens result %d reports unavailable candidate inspection", index+1)
			}
		}
	}
	state.LensResults = []LensResult{}
	state.Findings = []Finding{}
	for index, result := range input.LensResults {
		result.Lens = state.SelectedLenses[index]
		canonical, err := CanonicalCompactLensResult(result)
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
		switch item.Causality {
		case CausalIntroduced, CausalBehaviorActivated, CausalWorsened:
			if !findingLocationInGenesis(finding.Location, state.GenesisPaths) {
				item.Causality = CausalUnknown
			}
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

func findingLocationInGenesis(location string, genesisPaths []string) bool {
	logicalPath, _, err := parseFindingLocation(location)
	return err == nil && stringIndex(genesisPaths, logicalPath) >= 0
}

// ErrInvalidFindingLocation identifies reviewer locations that cannot be used
// as repository line evidence.
var ErrInvalidFindingLocation = errors.New("invalid reviewer finding location; correct it to repository/path:<positive-line> before running gentle-ai review capture-result again")

// FindingLocationErrorReason is a stable machine-readable validation reason.
type FindingLocationErrorReason string

const (
	FindingLocationExpectedPathAndLine FindingLocationErrorReason = "expected_path_and_line"
	FindingLocationLineNotInteger      FindingLocationErrorReason = "line_suffix_not_integer"
	FindingLocationLineNotPositive     FindingLocationErrorReason = "line_must_be_positive"
	FindingLocationPathNotRelative     FindingLocationErrorReason = "path_must_be_repository_relative"
	FindingLocationPathNotCanonical    FindingLocationErrorReason = "path_must_be_canonical"
)

// FindingLocationError describes why a reviewer location is invalid.
type FindingLocationError struct {
	Location string
	Reason   FindingLocationErrorReason
}

func (err *FindingLocationError) Error() string {
	return fmt.Sprintf("%v: %s", ErrInvalidFindingLocation, err.Reason)
}

func (err *FindingLocationError) Unwrap() error { return ErrInvalidFindingLocation }

func parseFindingLocation(location string) (string, int, error) {
	separator := strings.LastIndexByte(location, ':')
	if separator <= 0 || separator == len(location)-1 {
		return "", 0, &FindingLocationError{Location: location, Reason: FindingLocationExpectedPathAndLine}
	}
	lineSuffix := location[separator+1:]
	line, err := strconv.Atoi(lineSuffix)
	if err != nil {
		return "", 0, &FindingLocationError{Location: location, Reason: FindingLocationLineNotInteger}
	}
	for index := range lineSuffix {
		if lineSuffix[index] < '0' || lineSuffix[index] > '9' {
			reason := FindingLocationLineNotInteger
			if line <= 0 {
				reason = FindingLocationLineNotPositive
			}
			return "", 0, &FindingLocationError{Location: location, Reason: reason}
		}
	}
	if line <= 0 {
		return "", 0, &FindingLocationError{Location: location, Reason: FindingLocationLineNotPositive}
	}
	logicalPath := location[:separator]
	if len(logicalPath) >= 3 && logicalPath[1] == ':' && logicalPath[2] == '/' &&
		((logicalPath[0] >= 'A' && logicalPath[0] <= 'Z') || (logicalPath[0] >= 'a' && logicalPath[0] <= 'z')) {
		return "", 0, &FindingLocationError{Location: location, Reason: FindingLocationPathNotRelative}
	}
	if _, pathErr := normalizeLogicalPath(strings.ReplaceAll(logicalPath, ":", "/")); pathErr != nil {
		return "", 0, &FindingLocationError{Location: location, Reason: FindingLocationPathNotCanonical}
	}
	canonical, pathErr := normalizeLogicalPath(logicalPath)
	if pathErr != nil || canonical != logicalPath {
		return "", 0, &FindingLocationError{Location: location, Reason: FindingLocationPathNotCanonical}
	}
	return canonical, line, nil
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

// validateCompactResultDispositions enforces the persisted shape of audited
// reviewer-result dispositions. Only a terminally escalated authority may
// carry them, each binds a distinct selected lens/order pair on the frozen
// target, and each records the class it actually proved.
func validateCompactResultDispositions(state CompactState) error {
	if len(state.ResultDispositions) == 0 {
		return nil
	}
	if state.State != StateEscalated {
		return errors.New("only a terminally escalated compact state may record reviewer result dispositions")
	}
	orders := make(map[int]struct{}, len(state.ResultDispositions))
	for _, disposition := range state.ResultDispositions {
		if disposition.SelectedOrder < 0 || disposition.SelectedOrder >= len(state.SelectedLenses) ||
			state.SelectedLenses[disposition.SelectedOrder] != disposition.Lens {
			return errors.New("reviewer result disposition does not bind a selected lens and order")
		}
		if _, duplicate := orders[disposition.SelectedOrder]; duplicate {
			return errors.New("reviewer result disposition order is recorded twice")
		}
		orders[disposition.SelectedOrder] = struct{}{}
		if disposition.TargetIdentity != state.InitialSnapshot.Identity || !validSHA256(disposition.ArtifactDigest) {
			return errors.New("reviewer result disposition does not bind the frozen target and preserved artifact digest")
		}
		if strings.TrimSpace(disposition.Diagnostic) == "" || strings.TrimSpace(disposition.Reason) == "" ||
			strings.TrimSpace(disposition.Actor) == "" || strings.TrimSpace(disposition.MaintainerAuthorization) == "" ||
			disposition.DisposedAt.IsZero() {
			return errors.New("reviewer result disposition requires a diagnostic, reason, actor, authorization, and timestamp")
		}
		switch disposition.Class {
		case ResultDispositionTransportSyntax:
			if len(disposition.AbsentPaths) != 0 {
				return errors.New("transport/syntax reviewer result disposition carries no wrong-target path evidence")
			}
			if disposition.PayloadDecodable {
				return errors.New("transport/syntax reviewer result disposition must record a payload that did not decode")
			}
		case ResultDispositionWrongTarget:
			absent, err := canonicalPaths(disposition.AbsentPaths)
			if err != nil || len(absent) == 0 || !equalStrings(absent, disposition.AbsentPaths) {
				return errors.New("wrong-target reviewer result disposition requires canonical absent-path evidence")
			}
			for _, path := range absent {
				for _, candidate := range state.InitialSnapshot.Paths {
					if candidate == path {
						return errors.New("wrong-target reviewer result disposition cites a path inside the frozen candidate")
					}
				}
			}
			if !disposition.PayloadDecodable {
				return errors.New("wrong-target reviewer result disposition must record a payload that actually decoded")
			}
		default:
			return errors.New("invalid reviewer result disposition class")
		}
	}
	return nil
}

func validateCompactResultReopens(state CompactState) error {
	for _, reopen := range state.ResultReopens {
		if !validSHA256(reopen.PreviousRevision) || reopen.TargetIdentity != state.InitialSnapshot.Identity ||
			strings.TrimSpace(reopen.Reason) == "" || strings.TrimSpace(reopen.Actor) == "" ||
			strings.TrimSpace(reopen.MaintainerAuthorization) == "" || reopen.ReopenedAt.IsZero() || len(reopen.Quarantined) == 0 {
			return errors.New("reviewer result reopen audit record is incomplete")
		}
		seen := make(map[int]struct{}, len(state.SelectedLenses))
		validateSlots := func(slots []CompactResultReopenSlot, retained bool) error {
			for _, slot := range slots {
				if slot.SelectedOrder < 0 || slot.SelectedOrder >= len(state.SelectedLenses) ||
					state.SelectedLenses[slot.SelectedOrder] != slot.Lens || !validSHA256(slot.ArtifactDigest) || !validSHA256(slot.ResultHash) {
					return errors.New("reviewer result reopen slot does not bind the frozen selected lens")
				}
				if _, duplicate := seen[slot.SelectedOrder]; duplicate {
					return errors.New("reviewer result reopen slot is recorded twice")
				}
				seen[slot.SelectedOrder] = struct{}{}
				if retained {
					if !validSHA256(slot.SubjectHash) || !validSHA256(slot.AuthorityRevision) {
						return errors.New("retained reviewer result reopen slot lacks an admitted subject")
					}
				} else if (slot.SubjectHash == "") != (slot.AuthorityRevision == "") ||
					slot.SubjectHash != "" && (!validSHA256(slot.SubjectHash) || !validSHA256(slot.AuthorityRevision)) {
					return errors.New("quarantined reviewer result reopen slot has partial subject metadata")
				}
			}
			return nil
		}
		if err := validateSlots(reopen.Quarantined, false); err != nil {
			return err
		}
		if err := validateSlots(reopen.Retained, true); err != nil {
			return err
		}
		if len(seen) != len(state.SelectedLenses) {
			return errors.New("reviewer result reopen audit record must classify every selected lens artifact")
		}
		authorized := make(map[string]struct{}, len(reopen.AuthorizedLenses))
		for _, lens := range reopen.AuthorizedLenses {
			if stringIndex(state.SelectedLenses, lens) < 0 {
				return errors.New("reviewer result reopen authorization names a lens outside the frozen selection")
			}
			if _, duplicate := authorized[lens]; duplicate {
				return errors.New("reviewer result reopen authorization names a lens twice")
			}
			authorized[lens] = struct{}{}
			quarantinedLens := false
			for _, slot := range reopen.Quarantined {
				quarantinedLens = quarantinedLens || slot.Lens == lens
			}
			if !quarantinedLens {
				return errors.New("reviewer result reopen authorization names a lens whose result was not quarantined")
			}
		}
	}
	return nil
}

func compactPristineReviewing(state CompactState) bool {
	return state.State == StateReviewing && len(state.ResultDispositions) == 0 && snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) &&
		len(state.LensResults) == 0 && len(state.Findings) == 0 && len(state.Classifications) == 0 && len(state.Outcomes) == 0 &&
		len(state.FixFindingIDs) == 0 && len(state.FollowUps) == 0 && state.ProposedCorrectionLines == nil && state.ActualCorrectionLines == nil &&
		state.FixDeltaHash == EmptyFixDeltaHash && state.OriginalCriteria == nil && state.CorrectionRegression == nil && state.EvidenceHash == "" &&
		state.EvidenceRecordDigest == "" && state.EvidenceOutcome == "" && state.EvidenceTargetIdentity == "" && state.EvidenceAuthorityRevision == "" && state.CorrectionVerificationTarget == nil && state.InvalidationReason == "" &&
		len(state.CorrectionAttempts) == 0 && state.CumulativeCorrectionLines == 0
}

// CorrectionAttemptConsumed reports whether current policy permits no further
// ordinary correction append. Historical records may remain readable without
// regaining permission to mutate their predecessor authority.
func (state CompactState) CorrectionAttemptConsumed() bool {
	consumed := len(state.CorrectionAttempts)
	if state.Recovery != nil {
		consumed += state.Recovery.ConsumedCorrectionAttempts
	}
	return consumed >= MaxCompactCorrectionAttempts
}

func (state *CompactState) BeginCorrection(proposed int) error {
	if state.CorrectionAttemptConsumed() {
		return ErrCompactCorrectionConsumed
	}
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
	if state.CorrectionAttemptConsumed() {
		return ErrCompactCorrectionConsumed
	}
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil {
		return fmt.Errorf("cannot complete correction from compact state %q", state.State)
	}
	if snapshot.Kind != TargetFixDiff || snapshot.Projection != state.InitialSnapshot.Projection || snapshot.BaseTree != state.CurrentSnapshot.CandidateTree || !equalStrings(snapshot.LedgerIDs, state.FixFindingIDs) {
		return errors.New("compact correction snapshot is not bound to the reviewed candidate, projection, and causal findings")
	}
	if snapshot.CandidateTree == snapshot.BaseTree {
		return errors.New("compact correction has an unchanged candidate tree")
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
	if !validSHA256(validation.TargetedValidationRequestHash) || validation.CorrectionTargetIdentity != snapshot.Identity {
		return errors.New("compact targeted validation does not bind the provider-owned correction request")
	}
	attempt := CompactCorrectionAttempt{Snapshot: snapshot, ProposedLines: *state.ProposedCorrectionLines, ActualLines: actual, FixDeltaHash: fixHash,
		OriginalCriteria: validation.OriginalCriteria, CorrectionRegression: validation.CorrectionRegression,
		TargetedValidationRequestHash: validation.TargetedValidationRequestHash, CorrectionTargetIdentity: validation.CorrectionTargetIdentity}
	state.CorrectionAttempts = append(state.CorrectionAttempts, attempt)
	state.CumulativeCorrectionLines += actual
	state.CurrentSnapshot = snapshot
	state.FollowUps = append(state.FollowUps, validation.FollowUps...)
	state.FixDeltaHash, state.ActualCorrectionLines = fixHash, &actual
	original, regression := validation.OriginalCriteria, validation.CorrectionRegression
	state.OriginalCriteria, state.CorrectionRegression = &original, &regression
	if state.CumulativeCorrectionLines > state.CorrectionBudget || !original.Passed || !regression.Passed {
		state.State = StateEscalated
	} else {
		state.State = StateValidating
	}
	return state.Validate()
}

// CompleteCorrectionVerification crosses the correction and repository
// verification boundary in one state value. No correction accounting becomes
// authoritative unless both candidate-bound checks pass.
func (state *CompactState) CompleteCorrectionVerification(snapshot Snapshot, actual int, validation ScopedValidationResult, record VerificationEvidenceRecord, payload []byte, complete ...Snapshot) error {
	if len(complete) > 1 {
		return errors.New("compact correction accepts at most one complete candidate snapshot") // refusal:by-design world-action: provider code must submit one exact terminal authority
	}
	if record.Outcome != VerificationOutcomePassed {
		return errors.New("compact correction repository verification must pass before acceptance") // refusal:by-design operator-knowledge: the caller must retain the open correction and submit only a passed candidate-bound verification bundle
	}
	if err := record.ValidatePayload(payload); err != nil {
		return err
	}
	if err := record.ValidateBinding(state.LineageID, record.AuthorityRevision, snapshot); err != nil {
		return err
	}
	next, err := cloneCompactStateValue(*state)
	if err != nil {
		return err
	}
	if err := next.CompleteCorrection(snapshot, actual, validation); err != nil {
		return err
	}
	if next.State != StateValidating {
		return errors.New("compact correction checks and budget must pass before repository verification acceptance") // refusal:by-design operator-knowledge: the atomic caller must adjust the candidate or validation result without consuming the open correction
	}
	if err := next.CompleteVerificationRecord(record, payload); err != nil {
		return err
	}
	if len(complete) == 1 {
		next.CurrentSnapshot = complete[0]
		if err := next.Validate(); err != nil {
			return err
		}
	}
	*state = next
	return nil
}

func (state *CompactState) EscalateCorrectionVerification(snapshot Snapshot, record VerificationEvidenceRecord, payload []byte) error {
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil || record.Outcome != VerificationOutcomeProceduralFailure {
		return errors.New("procedural correction verification escalation requires one open correction transaction") // refusal:by-design operator-knowledge: this edge accepts only tooling-failure evidence for the current forecasted correction
	}
	if snapshot.Kind != TargetFixDiff || snapshot.Projection != state.InitialSnapshot.Projection ||
		snapshot.BaseTree != state.CurrentSnapshot.CandidateTree || !equalStrings(snapshot.LedgerIDs, state.FixFindingIDs) ||
		pathsAreSubset(snapshot.Paths, state.GenesisPaths) != nil {
		return errors.New("procedural correction verification target is outside the open correction transaction") // refusal:by-design world-action: candidate mutation invalidated the captured correction target and requires a fresh stable evidence capture
	}
	if err := record.ValidatePayload(payload); err != nil {
		return err
	}
	if err := record.ValidateBinding(state.LineageID, record.AuthorityRevision, snapshot); err != nil {
		return err
	}
	next, err := cloneCompactStateValue(*state)
	if err != nil {
		return err
	}
	target := snapshot
	next.State = StateEscalated
	next.CorrectionVerificationTarget = &target
	next.EvidenceHash = record.RawPayloadSHA256
	next.EvidenceRecordDigest = record.RecordDigest
	next.EvidenceOutcome = record.Outcome
	next.EvidenceTargetIdentity = record.TargetIdentity
	next.EvidenceAuthorityRevision = record.AuthorityRevision
	if err := next.Validate(); err != nil {
		return err
	}
	*state = next
	return nil
}

// CompactEscalationAccounting labels are deliberately distinct so no consumer
// ever confuses a remaining-budget value with the frozen total, mirroring the
// sddstatus.RemediationState precedent (CorrectionBudgetRemaining/Total).
const (
	CompactEscalationCauseBudgetExceeded             = "budget_exceeded"
	CompactEscalationCauseOriginalCriteriaFailed     = "original_criteria_failed"
	CompactEscalationCauseCorrectionRegressionFailed = "correction_regression_failed"
)

// CompactEscalationAccounting derives the spent/remaining/total correction
// budget bookkeeping for a compact authority, instead of persisting it.
// Spent and Total are already-persisted fields (CumulativeCorrectionLines,
// CorrectionBudget); Remaining is their difference, clamped at 0 so an
// over-budget escalation never renders a negative value on a visible
// surface.
type CompactEscalationAccounting struct {
	// Cause names which of the three compact.go BindCorrection escalation
	// conditions triggered, in the same precedence the source checks them:
	// budget exceeded first, then a failed original-criteria check, then a
	// failed correction-regression check. Empty when the state is not
	// escalated.
	Cause     string
	Spent     int
	Remaining int
	Total     int
}

// EscalationAccounting reports the correction budget bookkeeping behind an
// escalation, deriving it from already-persisted fields so no new schema or
// Validate() invariant is required.
func (state CompactState) EscalationAccounting() CompactEscalationAccounting {
	spent := state.CumulativeCorrectionLines
	if state.Recovery != nil {
		spent += state.Recovery.ConsumedCorrectionLines
	}
	// A correction forecast that never ran still crossed the budget:
	// BeginCorrection escalates on CumulativeCorrectionLines+proposed and
	// leaves ActualCorrectionLines nil, so for that shape the lines that
	// crossed live in ProposedCorrectionLines. Reading the cumulative alone
	// would report "spent 0" with no derivable cause for precisely the
	// over-budget escalation every visible surface is expected to explain.
	if state.State == StateEscalated && state.ActualCorrectionLines == nil && state.ProposedCorrectionLines != nil &&
		state.CumulativeCorrectionLines+*state.ProposedCorrectionLines > state.CorrectionBudget {
		spent = state.CumulativeCorrectionLines + *state.ProposedCorrectionLines
	}
	remaining := state.CorrectionBudget - spent
	if remaining < 0 {
		remaining = 0
	}
	accounting := CompactEscalationAccounting{
		Spent: spent, Remaining: remaining, Total: state.CorrectionBudget,
	}
	if state.State != StateEscalated {
		return accounting
	}
	switch {
	case spent > state.CorrectionBudget:
		accounting.Cause = CompactEscalationCauseBudgetExceeded
	case state.OriginalCriteria != nil && !state.OriginalCriteria.Passed:
		accounting.Cause = CompactEscalationCauseOriginalCriteriaFailed
	case state.CorrectionRegression != nil && !state.CorrectionRegression.Passed:
		accounting.Cause = CompactEscalationCauseCorrectionRegressionFailed
	}
	return accounting
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

func (state *CompactState) CompleteVerificationRecord(record VerificationEvidenceRecord, payload []byte) error {
	if state.State != StateValidating {
		return fmt.Errorf("cannot complete verification from compact state %q", state.State)
	}
	if err := record.ValidatePayload(payload); err != nil {
		return err
	}
	if err := record.ValidateBinding(state.LineageID, record.AuthorityRevision, state.CurrentSnapshot); err != nil {
		return err
	}
	state.EvidenceHash = record.RawPayloadSHA256
	state.EvidenceRecordDigest = record.RecordDigest
	state.EvidenceOutcome = record.Outcome
	state.EvidenceTargetIdentity = record.TargetIdentity
	state.EvidenceAuthorityRevision = record.AuthorityRevision
	if record.Outcome == VerificationOutcomePassed {
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
	pathsDigest := state.CurrentSnapshot.PathsDigest
	if state.CurrentSnapshot.Kind == TargetFixDiff {
		pathsDigest = state.InitialSnapshot.PathsDigest
	}
	receipt := CompactReceipt{
		Schema: CompactReceiptSchema, LineageID: state.LineageID, Generation: state.Generation,
		Projection: state.InitialSnapshot.Projection,
		BaseTree:   state.InitialSnapshot.BaseTree, InitialReviewTree: state.InitialSnapshot.CandidateTree,
		FinalCandidateTree: state.CurrentSnapshot.CandidateTree, PathsDigest: pathsDigest,
		FixDeltaHash: state.FixDeltaHash, PolicyHash: state.PolicyHash, EvidenceHash: evidence,
		EvidenceRecordDigest: state.EvidenceRecordDigest, EvidenceOutcome: state.EvidenceOutcome,
		EvidenceTargetIdentity: state.EvidenceTargetIdentity, EvidenceAuthorityRevision: state.EvidenceAuthorityRevision,
		RiskLevel: state.RiskLevel, SelectedLenses: append([]string{}, state.SelectedLenses...),
		ResolvedFindingIDs: append([]string(nil), state.FixFindingIDs...), TerminalState: terminal,
		ReviewerContextLevel: state.ReviewerContextLevel,
	}
	if err := receipt.Validate(); err != nil {
		return CompactReceipt{}, err
	}
	return receipt, nil
}

// NativeLowRiskVerificationEvidence returns the canonical structural evidence
// used only for a genuine low-risk, zero-lens, uncorrected compact review. The
// state machine still completes review before final verification; this preimage
// merely removes the need for a caller-created evidence file when native Git
// and risk evidence already prove the exact frozen target.
func NativeLowRiskVerificationEvidence(state CompactState, assessment RiskAssessment) ([]byte, error) {
	if state.State != StateReviewing && state.State != StateValidating && state.State != StateApproved {
		return nil, fmt.Errorf("native low-risk verification cannot run from compact state %q", state.State)
	}
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("validate native low-risk authority: %w", err)
	}
	if state.RiskLevel != RiskLow || assessment.Level != RiskLow || len(state.SelectedLenses) != 0 ||
		len(state.LensResults) != 0 || len(state.Findings) != 0 || len(state.FixFindingIDs) != 0 ||
		state.ProposedCorrectionLines != nil || state.ActualCorrectionLines != nil ||
		len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 ||
		state.FixDeltaHash != EmptyFixDeltaHash || !snapshotsEqual(state.InitialSnapshot, state.CurrentSnapshot) {
		return nil, errors.New("native low-risk verification requires an uncorrected zero-lens low-risk authority")
	}
	if assessment.ChangedLines != state.OriginalChangedLines {
		return nil, errors.New("native low-risk verification changed-line count does not match frozen authority")
	}
	preimage := struct {
		Schema               string         `json:"schema"`
		LineageID            string         `json:"lineage_id"`
		Generation           int            `json:"generation"`
		Snapshot             Snapshot       `json:"snapshot"`
		PolicyHash           string         `json:"policy_hash"`
		Risk                 RiskAssessment `json:"risk"`
		SelectedLenses       []string       `json:"selected_lenses"`
		CorrectionBudget     int            `json:"correction_budget"`
		OriginalChangedLines int            `json:"original_changed_lines"`
		FixDeltaHash         string         `json:"fix_delta_hash"`
	}{
		Schema: NativeLowRiskVerificationDomain, LineageID: state.LineageID, Generation: state.Generation,
		Snapshot: state.InitialSnapshot, PolicyHash: state.PolicyHash, Risk: assessment,
		SelectedLenses: []string{}, CorrectionBudget: state.CorrectionBudget,
		OriginalChangedLines: state.OriginalChangedLines, FixDeltaHash: state.FixDeltaHash,
	}
	payload, err := json.Marshal(preimage)
	if err != nil {
		return nil, fmt.Errorf("marshal native low-risk verification evidence: %w", err)
	}
	return append([]byte(NativeLowRiskVerificationDomain+"\x00"), payload...), nil
}

func (receipt CompactReceipt) Validate() error {
	projection, err := canonicalProjection(receipt.Projection)
	if err != nil || projection != receipt.Projection {
		return errors.New("compact receipt projection is unsupported or non-canonical")
	}
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
	// Shape only, deliberately never membership: an unknown level recorded by a
	// later release must stay readable here. See ReviewerContextLevel.
	if receipt.ReviewerContextLevel != "" && !ReviewerContextLevelWellFormed(receipt.ReviewerContextLevel) {
		return errors.New("compact receipt reviewer context level is malformed") // refusal:by-design world-action: a persisted receipt carrying unreadable bytes in an audit field requires storage repair, not an operator command
	}
	hasRecordBinding := receipt.EvidenceRecordDigest != "" || receipt.EvidenceOutcome != "" ||
		receipt.EvidenceTargetIdentity != "" || receipt.EvidenceAuthorityRevision != ""
	if hasRecordBinding {
		if !validSHA256(receipt.EvidenceRecordDigest) || !validSHA256(receipt.EvidenceTargetIdentity) ||
			!validSHA256(receipt.EvidenceAuthorityRevision) || !validVerificationOutcome(receipt.EvidenceOutcome) ||
			receipt.EvidenceOutcome == VerificationOutcomePassed && receipt.TerminalState != TerminalApproved ||
			receipt.EvidenceOutcome != VerificationOutcomePassed && receipt.TerminalState != TerminalEscalated {
			return errors.New("compact receipt verification evidence binding is invalid") // refusal:by-design world-action: a contradictory persisted receipt cannot safely authorize delivery and requires code or storage repair
		}
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
	if err := receipt.Validate(); err != nil {
		return CompactReceipt{}, err
	}
	normalizeCompactReceipt(&receipt)
	return receipt, nil
}

func WriteCompactReceiptAtomic(path string, receipt CompactReceipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	return publishImmutable(path, append(payload, '\n'), 0o644)
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
	normalizeCompactReceipt(&left)
	normalizeCompactReceipt(&right)
	return reflect.DeepEqual(left, right)
}
func CompactReceiptEqual(left, right CompactReceipt) bool { return compactReceiptEqual(left, right) }

func normalizeCompactReceipt(receipt *CompactReceipt) {
	if len(receipt.SelectedLenses) == 0 {
		receipt.SelectedLenses = []string{}
	}
	if len(receipt.ResolvedFindingIDs) == 0 {
		receipt.ResolvedFindingIDs = nil
	}
}

func CompactReceiptSchemaOf(payload []byte) string {
	var header struct {
		Schema string `json:"schema"`
	}
	_ = json.Unmarshal(payload, &header)
	return strings.TrimSpace(header.Schema)
}
