package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

const CompactStateSchema = "gentle-ai.review-state/v2"
const NativeLowRiskVerificationDomain = "gentle-ai.native-low-risk-verification/v1"
const CompactRecoveredEvidenceSchema = "gentle-ai.review-recovered-evidence/v1"

const (
	StateCorrectionRequired             State = "correction_required"
	StateValidating                     State = "validating"
	MaxCompactCorrectionAttempts              = 1
	historicalCompactCorrectionAttempts       = 3
)

// CorrectionBudgetPolicy names the persisted budget formula a compact state
// uses. New states persist CorrectionBudgetPolicyFloorTwo so every positive
// budget admits one atomic line replacement; historical states omit the field
// and retain the legacy CorrectionBudget formula byte-for-byte (issue #2247).
const (
	CorrectionBudgetPolicyFloorTwo = "floor_two"
)

var ErrCompactCorrectionConsumed = errors.New("ordinary compact correction already consumed")

// ErrCompactTargetedValidatorAttemptsExhausted refuses a fourth distinct
// non-verdict for one frozen targeted-validator request without changing the
// active authority.
var ErrCompactTargetedValidatorAttemptsExhausted = errors.New("targeted validator exhausted its three inconclusive attempts") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it

// CompactSemanticStateError identifies a CompactState.Validate() failure with
// its lineage and state, distinguishing a semantic-validation failure from a
// structural one (JSON decode, schema mismatch, or checksum error). Only
// parseCompactRecord's call to Validate() (compact_store.go) constructs this
// type, so errors.As matching it is exact: never a checksum/IO/parse failure
// (issue-1813).
// CompactFrozenPolicyUnavailableError denies a targeted validator request when
// legacy compact authority retained only the policy hash. Reconstructing policy
// material from a mutable source would change the provider-bound semantics.
type CompactFrozenPolicyUnavailableError struct {
	LineageID  string
	PolicyHash string
}

func (err *CompactFrozenPolicyUnavailableError) Error() string {
	return fmt.Sprintf("compact lineage %q has policy hash %q but no frozen policy content; start a new review authority before targeted validation", err.LineageID, err.PolicyHash)
}

// CompactFrozenPolicyIntegrityError denies use of frozen content that does not
// match the existing compact policy hash.
type CompactFrozenPolicyIntegrityError struct {
	LineageID  string
	PolicyHash string
}

func (err *CompactFrozenPolicyIntegrityError) Error() string {
	return fmt.Sprintf("compact lineage %q frozen policy content does not match policy hash %q", err.LineageID, err.PolicyHash)
}

type CompactSemanticStateError struct {
	LineageID string
	State     State
	Problem   string
	// OutdatedIdentity marks the one semantic failure a retired
	// snapshot-identity formula leaves behind (#2743): the record's bytes
	// parse and its structure is intact, but the identity it froze was
	// computed by an earlier release's formula, so recomputation under the
	// current formula no longer matches. Outdated means gate-invalid, not
	// damaged: diagnostics classify it historical instead of malformed, and
	// no scoped walk lets it block another lineage's operation.
	OutdatedIdentity bool
	// PriorSchemaPredecessorLineageID names the recovery predecessor an
	// OutdatedIdentity record froze, recovered through the same read-only
	// forensic parse that proved the prior-schema classification. It lets a
	// scoped ancestry audit keep walking past inert prior-schema history; it
	// is forensic classification only and never live authority.
	PriorSchemaPredecessorLineageID string
}

func (err *CompactSemanticStateError) Error() string {
	return fmt.Sprintf("compact lineage %q semantic state %q is invalid: %s", err.LineageID, err.State, err.Problem)
}

// compactLineageQuarantinable reports whether a store-discovery load failure
// is eligible to exclude one semantically corrupt lineage from selector-free
// enumeration (issue-1813). Semantic corruption is quarantined regardless of
// the failed state: the error already proves this exact authority cannot be
// used, while unrelated lineages remain independently discoverable. Checksum,
// I/O, and parse failures are never semantic quarantine candidates.
func compactLineageQuarantinable(err error) (*CompactSemanticStateError, bool) {
	var semantic *CompactSemanticStateError
	if !errors.As(err, &semantic) {
		return nil, false
	}
	return semantic, true
}

type CompactState struct {
	Schema               string   `json:"schema"`
	LineageID            string   `json:"lineage_id"`
	Generation           int      `json:"generation"`
	State                State    `json:"state"`
	InitialSnapshot      Snapshot `json:"initial_snapshot"`
	CurrentSnapshot      Snapshot `json:"current_snapshot"`
	GenesisPaths         []string `json:"genesis_paths"`
	CorrectionAddedPaths []string `json:"correction_added_paths,omitempty"`
	PolicyHash           string   `json:"policy_hash"`
	// FrozenPolicyContent is the exact policy text START read before it derived
	// PolicyHash. A non-nil empty string intentionally represents an empty policy;
	// nil remains readable historical authority and fails closed only when a
	// targeted validator needs this semantic context.
	FrozenPolicyContent    *string   `json:"frozen_policy_content,omitempty"`
	RiskLevel              RiskLevel `json:"risk_level"`
	SelectedLenses         []string  `json:"selected_lenses"`
	OriginalChangedLines   int       `json:"original_changed_lines"`
	CorrectionBudget       int       `json:"correction_budget"`
	CorrectionBudgetPolicy string    `json:"correction_budget_policy,omitempty"`
	// Historical review projections remain decode-only so released records can
	// be classified as outdated and quarantined without restoring them as active
	// authority. CompactReviewView derives all live review semantics from
	// AdmittedRoleResults instead.
	HistoricalLensResults     *compactHistoricalEmptyArray  `json:"lens_results,omitempty"`
	HistoricalFindings        *compactHistoricalEmptyArray  `json:"findings,omitempty"`
	HistoricalClassifications *compactHistoricalEmptyObject `json:"classifications,omitempty"`
	HistoricalOutcomes        *compactHistoricalEmptyObject `json:"outcomes,omitempty"`
	FixFindingIDs             []string                      `json:"fix_finding_ids"`
	HistoricalFollowUps       *compactHistoricalEmptyArray  `json:"follow_ups,omitempty"`
	ProposedCorrectionLines   *int                          `json:"proposed_correction_lines,omitempty"`
	ActualCorrectionLines     *int                          `json:"actual_correction_lines,omitempty"`
	FixDeltaHash              string                        `json:"fix_delta_hash"`
	OriginalCriteria          *ValidationCheck              `json:"original_criteria,omitempty"`
	CorrectionRegression      *ValidationCheck              `json:"correction_regression,omitempty"`
	EvidenceHash              string                        `json:"evidence_hash,omitempty"`
	InvalidationReason        string                        `json:"invalidation_reason,omitempty"`
	InvalidationEvidence      *CompactInvalidationEvidence  `json:"invalidation_evidence,omitempty"`
	Recovery                  *CompactRecoveryProvenance    `json:"recovery,omitempty"`
	CorrectionAttempts        []CompactCorrectionAttempt    `json:"correction_attempts,omitempty"`
	CumulativeCorrectionLines int                           `json:"cumulative_correction_lines,omitempty"`
	ResultReopens             []CompactResultReopen         `json:"result_reopens,omitempty"`
	ReviewerContextLevel      ReviewerContextLevel          `json:"reviewer_context_level,omitempty"`
	// CapturePhaseRevision is the stable capture binding for the frozen review
	// phase. Unlike CompactRecord.Revision, it does not advance when sibling
	// captures merge their admitted values under the CAS lock.
	CapturePhaseRevision string `json:"capture_phase_revision,omitempty"`
	// CapturePhaseEpoch advances only at a lifecycle phase seam. It prevents a
	// reopened reviewing authority from reissuing the invalidated phase binding.
	CapturePhaseEpoch int `json:"capture_phase_epoch,omitempty"`
	// AdmittedRoleResults is the canonical, ordered in-record home for admitted
	// provider role values. The initial migration populates lens entries; later
	// milestones move refuter and targeted-validator callers to the same owner.
	AdmittedRoleResults []CompactAdmittedRoleResult `json:"admitted_role_results,omitempty"`
	// TargetedValidatorAttempts is a fixed, hash-only retry ledger for the
	// current correction phase. It intentionally excludes rejected output bytes,
	// evidence bodies, artifact paths, and role values.
	TargetedValidatorAttempts []CompactTargetedValidatorAttempt `json:"targeted_validator_attempts,omitempty"`
	// ApprovedAckToken is the one bounded opaque 256-bit acknowledgement token.
	// It is present only on an active approved authority and is cleared by burn.
	ApprovedAckToken string `json:"approved_ack_token,omitempty"`
	// InitialAtomicStart is the optional immutable binding written only by the
	// exact worktree-bound atomic START API. Its absence keeps historical compact
	// records readable, but makes them ineligible for atomic START replay.
	InitialAtomicStart *CompactAtomicStartBinding `json:"initial_atomic_start,omitempty"`
	// RuntimeAgent is the runtime identity START froze this lineage to. STATUS
	// without --agent resolves the validator role from it (#3805); a record
	// that predates the field keeps the manual route. Internal state only.
	RuntimeAgent string `json:"runtime_agent,omitempty"`
}

// compactHistoricalEmptyArray preserves an empty retired projection only long
// enough for a released record's checksum and outdated-identity proof to be
// reconstructed. A non-empty projection follows the existing historical parser,
// which removes it and marks the authority read-only.
type compactHistoricalEmptyArray struct{ raw json.RawMessage }

func (projection *compactHistoricalEmptyArray) UnmarshalJSON(payload []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(payload, &values); err != nil || len(values) != 0 {
		return errors.New(`json: unknown field "lens_results"`) // refusal:by-design world-action: a non-empty retired projection must take the existing read-only historical compatibility path instead of becoming active authority
	}
	projection.raw = append(projection.raw[:0], payload...)
	return nil
}

func (projection compactHistoricalEmptyArray) MarshalJSON() ([]byte, error) {
	if projection.raw == nil {
		return []byte("[]"), nil
	}
	return projection.raw, nil
}

// compactHistoricalEmptyObject is the map counterpart for retired findings
// routing projections. It preserves only the empty release shape.
type compactHistoricalEmptyObject struct{ raw json.RawMessage }

func (projection *compactHistoricalEmptyObject) UnmarshalJSON(payload []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(payload, &values); err != nil || len(values) != 0 {
		return errors.New(`json: unknown field "classifications"`) // refusal:by-design world-action: a non-empty retired projection must take the existing read-only historical compatibility path instead of becoming active authority
	}
	projection.raw = append(projection.raw[:0], payload...)
	return nil
}

func (projection compactHistoricalEmptyObject) MarshalJSON() ([]byte, error) {
	if projection.raw == nil {
		return []byte("{}"), nil
	}
	return projection.raw, nil
}

// CompactAtomicStartBinding is the immutable compact-v2 START identity. It
// binds one exact worktree and frozen snapshot to the policy, tier, lenses, and
// bounded-correction inputs that created the authority.
type CompactRole string

const (
	CompactRoleLens              CompactRole = "lens"
	CompactRoleRefuter           CompactRole = "refuter"
	CompactRoleTargetedValidator CompactRole = "targeted_validator"
)

// CompactAdmittedRoleResult binds one canonical admitted value to the frozen
// tuple that authorized its capture. Value is canonical JSON; the persisted
// artifact digest is derived from its exact role bytes, not a caller supplied
// projection.
type CompactAdmittedRoleResult struct {
	Role                 CompactRole     `json:"role"`
	Lens                 string          `json:"lens,omitempty"`
	SelectedOrder        int             `json:"selected_order,omitempty"`
	TargetIdentity       string          `json:"target_identity"`
	CapturePhaseRevision string          `json:"capture_phase_revision"`
	RequestHash          string          `json:"request_hash,omitempty"`
	ArtifactDigest       string          `json:"artifact_digest"`
	ResultHash           string          `json:"result_hash,omitempty"`
	Value                json.RawMessage `json:"value"`
}

type CompactTargetedValidatorAttempt struct {
	CapturePhaseRevision string `json:"capture_phase_revision"`
	TargetIdentity       string `json:"target_identity"`
	RequestHash          string `json:"request_hash"`
	AttemptDigest        string `json:"attempt_digest"`
	Outcome              string `json:"outcome"`
}

const compactTargetedValidatorAttemptInconclusive = "inconclusive"

const maxCompactTargetedValidatorAttempts = 3

type CompactAtomicStartBinding struct {
	LineageID              string    `json:"lineage_id"`
	WorktreeIdentity       string    `json:"worktree_identity"`
	TargetIdentity         string    `json:"target_identity"`
	Selector               Target    `json:"selector"`
	PolicyHash             string    `json:"policy_hash"`
	Tier                   RiskLevel `json:"tier"`
	SelectedLenses         []string  `json:"selected_lenses"`
	OriginalChangedLines   int       `json:"original_changed_lines"`
	CorrectionBudget       int       `json:"correction_budget"`
	CorrectionBudgetPolicy string    `json:"correction_budget_policy"`
}

// Validate rejects a structurally non-canonical binding. Its equality to the
// compact state is checked separately so a conflicting replay can report the
// exact immutable field without rewriting authority.
func (binding CompactAtomicStartBinding) Validate() error {
	if err := validateLineageID(binding.LineageID); err != nil {
		return err
	}
	if !validSHA256(binding.WorktreeIdentity) {
		return errors.New("compact atomic START requires a canonical worktree identity") // refusal:by-design world-action: a malformed provider-built worktree binding must be rebuilt before it can create authority
	}
	if !validSHA256(binding.TargetIdentity) {
		return errors.New("compact atomic START requires a canonical target identity") // refusal:by-design world-action: a malformed provider-built target binding must be rebuilt before it can create authority
	}
	if !validSHA256(binding.PolicyHash) {
		return errors.New("compact atomic START requires a canonical policy hash") // refusal:by-design world-action: a malformed provider-built policy binding must be rebuilt before it can create authority
	}
	selector, err := canonicalCompactAtomicStartSelector(binding.Selector)
	if err != nil {
		return err
	}
	if !equalCompactAtomicStartSelector(selector, binding.Selector) {
		return errors.New("compact atomic START selector must be canonical") // refusal:by-design world-action: a provider-built selector must be canonicalized before it can create authority
	}
	switch binding.Tier {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return errors.New("compact atomic START tier must be a native risk classification") // refusal:by-design world-action: an unsupported provider-built risk tier requires a code fix before it can create authority
	}
	seen := make(map[string]bool, len(binding.SelectedLenses))
	for _, lens := range binding.SelectedLenses {
		if strings.TrimSpace(lens) != lens || !isSupportedLens(lens) || seen[lens] {
			return errors.New("compact atomic START selected lenses must be canonical and unique") // refusal:by-design world-action: a malformed provider-built lens selection must be rebuilt before it can create authority
		}
		seen[lens] = true
	}
	if binding.OriginalChangedLines < 0 || binding.CorrectionBudget < 0 {
		return errors.New("compact atomic START changed lines and correction budget cannot be negative") // refusal:by-design world-action: invalid provider-built budget inputs must be recalculated before they can create authority
	}
	if _, err := CompactExpectedBudget(binding.OriginalChangedLines, binding.CorrectionBudgetPolicy); err != nil {
		return errors.New("compact atomic START correction budget policy is unsupported") // refusal:by-design world-action: an unsupported provider-built budget policy requires a code fix before it can create authority
	}
	return nil
}

func canonicalCompactAtomicStartSelector(selector Target) (Target, error) {
	switch selector.Kind {
	case TargetCurrentChanges, TargetBaseDiff, TargetBaseWorkspaceOverlay, TargetExactRevision, TargetFixDiff:
	default:
		return Target{}, fmt.Errorf("unsupported compact atomic START target kind %q", selector.Kind) // refusal:by-design world-action: an unsupported provider-built target kind requires a code fix before it can create authority
	}
	selector = CanonicalTarget(selector)
	projection, err := canonicalProjection(selector.Projection)
	if err != nil {
		return Target{}, err
	}
	selector.Projection = projection
	intended, err := canonicalPaths(selector.IntendedUntracked)
	if err != nil {
		return Target{}, err
	}
	ledgerIDs, err := canonicalStrings(selector.LedgerIDs, "ledger id")
	if err != nil {
		return Target{}, err
	}
	selector.IntendedUntracked, selector.LedgerIDs = intended, ledgerIDs
	for _, value := range []string{selector.BaseRef, selector.Revision} {
		if strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
			return Target{}, errors.New("compact atomic START selector values must be canonical") // refusal:by-design world-action: malformed provider-built selector values must be canonicalized before they can create authority
		}
	}
	return selector, nil
}

func equalCompactAtomicStartSelector(left, right Target) bool {
	return left.Kind == right.Kind && left.Projection == right.Projection && left.BaseRef == right.BaseRef &&
		left.Revision == right.Revision && equalStrings(left.IntendedUntracked, right.IntendedUntracked) &&
		equalStrings(left.LedgerIDs, right.LedgerIDs)
}

func (binding CompactAtomicStartBinding) mismatchState(state CompactState) string {
	snapshot := state.InitialSnapshot
	switch {
	case binding.LineageID != state.LineageID:
		return "lineage_id"
	case binding.TargetIdentity != snapshot.Identity:
		return "target_identity"
	case binding.Selector.Kind != snapshot.Kind || binding.Selector.Projection != snapshot.Projection ||
		!equalStrings(binding.Selector.IntendedUntracked, snapshot.IntendedUntracked) ||
		!equalStrings(binding.Selector.LedgerIDs, snapshot.LedgerIDs):
		return "selector"
	case binding.PolicyHash != state.PolicyHash:
		return "policy_hash"
	case binding.Tier != state.RiskLevel:
		return "tier"
	case !equalStrings(binding.SelectedLenses, state.SelectedLenses):
		return "selected_lenses"
	case binding.OriginalChangedLines != state.OriginalChangedLines:
		return "original_changed_lines"
	case binding.CorrectionBudget != state.CorrectionBudget:
		return "correction_budget"
	case binding.CorrectionBudgetPolicy != state.CorrectionBudgetPolicy:
		return "correction_budget_policy"
	default:
		return ""
	}
}

func cloneCompactAtomicStartBinding(binding CompactAtomicStartBinding) CompactAtomicStartBinding {
	binding.Selector.IntendedUntracked = append([]string(nil), binding.Selector.IntendedUntracked...)
	binding.Selector.LedgerIDs = append([]string(nil), binding.Selector.LedgerIDs...)
	binding.SelectedLenses = append([]string(nil), binding.SelectedLenses...)
	return binding
}

func equalCompactAtomicStartBinding(left, right *CompactAtomicStartBinding) bool {
	if left == nil || right == nil {
		return left == right
	}
	return compactAtomicStartMismatch(*left, *right) == ""
}

func compactAtomicStartMismatch(existing, requested CompactAtomicStartBinding) string {
	switch {
	case existing.LineageID != requested.LineageID:
		return "lineage_id"
	case existing.WorktreeIdentity != requested.WorktreeIdentity:
		return "worktree_identity"
	case existing.TargetIdentity != requested.TargetIdentity:
		return "target_identity"
	case !equalCompactAtomicStartSelector(existing.Selector, requested.Selector):
		return "selector"
	case existing.PolicyHash != requested.PolicyHash:
		return "policy_hash"
	case existing.Tier != requested.Tier:
		return "tier"
	case !equalStrings(existing.SelectedLenses, requested.SelectedLenses):
		return "selected_lenses"
	case existing.OriginalChangedLines != requested.OriginalChangedLines:
		return "original_changed_lines"
	case existing.CorrectionBudget != requested.CorrectionBudget:
		return "correction_budget"
	case existing.CorrectionBudgetPolicy != requested.CorrectionBudgetPolicy:
		return "correction_budget_policy"
	default:
		return ""
	}
}

func cloneCompactStateInitialAtomicStart(state CompactState) CompactState {
	if state.InitialAtomicStart != nil {
		binding := cloneCompactAtomicStartBinding(*state.InitialAtomicStart)
		state.InitialAtomicStart = &binding
	}
	state.AdmittedRoleResults = cloneCompactAdmittedRoleResults(state.AdmittedRoleResults)
	return state
}

func cloneCompactAdmittedRoleResults(values []CompactAdmittedRoleResult) []CompactAdmittedRoleResult {
	if values == nil {
		return nil
	}
	cloned := make([]CompactAdmittedRoleResult, len(values))
	copy(cloned, values)
	for index := range cloned {
		cloned[index].Value = append(json.RawMessage(nil), cloned[index].Value...)
	}
	return cloned
}

func compactPreservedPayloadDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// deriveCompactCapturePhaseRevision creates the stable Pn binding before a
// record exists. Its preimage is deliberately record-free: including a record
// revision (or an admitted result) would make Pn advance with Rn and break
// parallel capture admission.
func deriveCompactCapturePhaseRevision(state CompactState) (string, error) {
	preimage := struct {
		Schema           string    `json:"schema"`
		LineageID        string    `json:"lineage_id"`
		Generation       int       `json:"generation"`
		TargetIdentity   string    `json:"target_identity"`
		BaseTree         string    `json:"base_tree"`
		CandidateTree    string    `json:"candidate_tree"`
		PathsDigest      string    `json:"paths_digest"`
		PolicyHash       string    `json:"policy_hash"`
		RiskLevel        RiskLevel `json:"risk_level"`
		SelectedLenses   []string  `json:"selected_lenses"`
		GenesisPaths     []string  `json:"genesis_paths"`
		CorrectionBudget int       `json:"correction_budget"`
		CorrectionPolicy string    `json:"correction_budget_policy"`
		WorktreeIdentity string    `json:"worktree_identity,omitempty"`
		PhaseState       State     `json:"phase_state"`
		PhaseEpoch       int       `json:"phase_epoch"`
		CurrentTarget    string    `json:"current_target"`
		FixFindingIDs    []string  `json:"fix_finding_ids"`
		ProposedLines    *int      `json:"proposed_correction_lines,omitempty"`
		AdmittedDigests  []string  `json:"admitted_digests"`
	}{
		Schema:           state.Schema,
		LineageID:        state.LineageID,
		Generation:       state.Generation,
		TargetIdentity:   state.InitialSnapshot.Identity,
		BaseTree:         state.InitialSnapshot.BaseTree,
		CandidateTree:    state.InitialSnapshot.CandidateTree,
		PathsDigest:      state.InitialSnapshot.PathsDigest,
		PolicyHash:       state.PolicyHash,
		RiskLevel:        state.RiskLevel,
		SelectedLenses:   append([]string(nil), state.SelectedLenses...),
		GenesisPaths:     append([]string(nil), state.GenesisPaths...),
		CorrectionBudget: state.CorrectionBudget,
		CorrectionPolicy: state.CorrectionBudgetPolicy,
		WorktreeIdentity: compactCapturePhaseWorktreeIdentity(state),
		PhaseState:       state.State,
		PhaseEpoch:       state.CapturePhaseEpoch,
		CurrentTarget:    state.CurrentSnapshot.Identity,
		FixFindingIDs:    append([]string(nil), state.FixFindingIDs...),
		ProposedLines:    state.ProposedCorrectionLines,
		AdmittedDigests:  compactCapturePhaseAdmittedDigests(state),
	}
	payload, err := json.Marshal(preimage)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("gentle-ai.review-capture-phase/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// compactCapturePhaseWorktreeIdentity includes the frozen worktree only once
// atomic START has supplied it. NewCompactState remains usable by historical and
// test-only callers; CreateOrReplayAtomicStart always derives P0 again after
// attaching its validated immutable binding.
func compactCapturePhaseWorktreeIdentity(state CompactState) string {
	if state.InitialAtomicStart == nil {
		return ""
	}
	return state.InitialAtomicStart.WorktreeIdentity
}

func compactCapturePhaseAdmittedDigests(state CompactState) []string {
	if state.State == StateReviewing {
		return []string{}
	}
	values := make([]string, 0, len(state.AdmittedRoleResults))
	for _, entry := range state.AdmittedRoleResults {
		if compactAdmittedRoleResultCanSatisfyActiveCapture(state, entry) {
			values = append(values, string(entry.Role)+":"+entry.ArtifactDigest)
		}
	}
	return values
}

func (state *CompactState) advanceCapturePhase() error {
	state.CapturePhaseEpoch++
	phase, err := deriveCompactCapturePhaseRevision(*state)
	if err != nil {
		return err
	}
	state.CapturePhaseRevision = phase
	return nil
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

// CompactResultReopen is payload-free audit metadata for one lock-owned
// quarantine set reopening. Removed references are not slots, paths, or a second
// role-value source; they cannot be read back into active capture.
type CompactResultReopen struct {
	PreviousRevision string `json:"previous_revision"`
	TargetIdentity   string `json:"target_identity"`
	// SelectedLens is the historical singular audit field. New audit records use
	// QuarantineLenses, while this remains readable for persisted authority.
	SelectedLens            string                         `json:"selected_lens,omitempty"`
	QuarantineLenses        []string                       `json:"quarantine_lenses,omitempty"`
	Removed                 []CompactResultReopenReference `json:"removed"`
	Reason                  string                         `json:"reason"`
	Actor                   string                         `json:"actor"`
	ReopenedAt              time.Time                      `json:"reopened_at"`
	MaintainerAuthorization string                         `json:"maintainer_authorization"`
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
	RecoveryScopeChanged RecoveryDisposition = "scope_changed"
	RecoveryInvalidated  RecoveryDisposition = "invalidated"
	RecoveryEscalated    RecoveryDisposition = "escalated"
)

type CompactRecoveryProvenance struct {
	PredecessorLineageID       string                    `json:"predecessor_lineage_id"`
	PredecessorRevision        string                    `json:"predecessor_revision"`
	Disposition                RecoveryDisposition       `json:"disposition"`
	Reason                     string                    `json:"reason"`
	Actor                      string                    `json:"actor"`
	RecoveredAt                time.Time                 `json:"recovered_at"`
	MaintainerAuthorization    string                    `json:"maintainer_authorization,omitempty"`
	ConsumedCorrectionAttempts int                       `json:"consumed_correction_attempts,omitempty"`
	ConsumedCorrectionLines    int                       `json:"consumed_correction_lines,omitempty"`
	Evidence                   *CompactRecoveredEvidence `json:"evidence,omitempty"`
}

// CompactRecoveredEvidence is accounting-only provenance for the one recovery
// that may reuse predecessor review/correction evidence. It owns no role value,
// correction attempt, request, successor target, or role-derived digest: ordered
// references bind the exact canonical predecessor entries instead.
type CompactRecoveredEvidence struct {
	Schema                    string                              `json:"schema"`
	Relation                  string                              `json:"relation"`
	PathRelation              string                              `json:"path_relation"`
	PredecessorTargetIdentity string                              `json:"predecessor_target_identity"`
	NativeCorrectionLines     int                                 `json:"native_correction_lines"`
	AdmittedRoleReferences    []CompactRecoveredEvidenceReference `json:"admitted_role_references"`
}

// CompactRecoveredEvidenceReference identifies exactly one canonical admitted
// predecessor role value. It is intentionally payload-free and has only the
// persisted artifact digest; a reference cannot become another result owner.
type CompactRecoveredEvidenceReference struct {
	Role                 CompactRole `json:"role"`
	Lens                 string      `json:"lens,omitempty"`
	SelectedOrder        int         `json:"selected_order,omitempty"`
	TargetIdentity       string      `json:"target_identity"`
	CapturePhaseRevision string      `json:"capture_phase_revision"`
	RequestHash          string      `json:"request_hash,omitempty"`
	ArtifactDigest       string      `json:"artifact_digest"`
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
	var frozenPolicy *string
	if start.PolicyContent != nil {
		content := *start.PolicyContent
		if compactPolicyContentHash(content) != start.PolicyHash {
			return CompactState{}, errors.New("frozen policy content does not match policy_hash") // refusal:by-design world-action: frozen policy content and its immutable hash disagree, so safe repair requires replacing the authority
		}
		frozenPolicy = &content
	}
	lenses, err := validateSelectedLenses(start.Mode, start.RiskLevel, start.SelectedLenses)
	if err != nil {
		return CompactState{}, err
	}
	budget, err := CompactCorrectionBudget(*start.OriginalChangedLines)
	if err != nil {
		return CompactState{}, err
	}
	state := CompactState{
		Schema: CompactStateSchema, LineageID: start.LineageID, Generation: start.Generation,
		State: StateReviewing, InitialSnapshot: start.Snapshot, CurrentSnapshot: start.Snapshot,
		GenesisPaths: append([]string(nil), start.Snapshot.Paths...), PolicyHash: start.PolicyHash,
		FrozenPolicyContent: frozenPolicy, RiskLevel: start.RiskLevel, SelectedLenses: lenses, OriginalChangedLines: *start.OriginalChangedLines,
		CorrectionBudget: budget, CorrectionBudgetPolicy: CorrectionBudgetPolicyFloorTwo,
		AdmittedRoleResults: []CompactAdmittedRoleResult{}, FixFindingIDs: []string{}, FixDeltaHash: EmptyFixDeltaHash,
		RuntimeAgent: start.RuntimeAgent,
	}
	phase, err := deriveCompactCapturePhaseRevision(state)
	if err != nil {
		return CompactState{}, err
	}
	state.CapturePhaseRevision = phase
	return state, state.Validate()
}

// CompactExpectedBudget derives the budget a compact state should carry under
// its persisted policy. Floor-two states use CompactCorrectionBudget;
// historical states with no policy field retain the legacy CorrectionBudget
// formula byte-for-byte. An unrecognized policy is rejected (issue #2247).
func CompactExpectedBudget(originalChangedLines int, policy string) (int, error) {
	switch policy {
	case CorrectionBudgetPolicyFloorTwo:
		return CompactCorrectionBudget(originalChangedLines)
	case "":
		return CorrectionBudget(originalChangedLines)
	default:
		// refusal:by-design world-action: a persisted policy outside the closed contract cannot be repaired safely without provider-owned authority
		return 0, errors.New("compact correction budget policy is unrecognized")
	}
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
		if recovery.Evidence != nil && recovery.Disposition != RecoveryEscalated {
			return errors.New("only escalated recovery may carry predecessor evidence")
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
	if err := validateCompactCorrectionAddedPaths(state); err != nil {
		return err
	}
	candidateScope, err := compactCorrectionCandidateScope(state)
	if err != nil {
		return err
	}
	if err := pathsAreSubset(state.CurrentSnapshot.Paths, candidateScope); err != nil {
		return err
	}
	if !validSHA256(state.PolicyHash) || !validSHA256(state.FixDeltaHash) {
		return errors.New("compact policy and fix delta hashes must be lowercase SHA-256 identities")
	}
	if state.FrozenPolicyContent != nil && compactPolicyContentHash(*state.FrozenPolicyContent) != state.PolicyHash {
		return errors.New("compact frozen policy content does not match policy_hash") // refusal:by-design world-action: frozen policy content and its immutable hash disagree, so safe repair requires replacing the authority
	}
	selected, err := validateSelectedLenses(ModeOrdinaryBounded, state.RiskLevel, state.SelectedLenses)
	if err != nil || !equalStrings(selected, state.SelectedLenses) {
		return errors.New("compact selected lenses are invalid")
	}
	wantBudget, err := CompactExpectedBudget(state.OriginalChangedLines, state.CorrectionBudgetPolicy)
	preservedRecoveryBudget := state.Recovery != nil && state.Recovery.ConsumedCorrectionAttempts > 0
	if err != nil {
		return err
	}
	if state.CorrectionBudget != wantBudget && !preservedRecoveryBudget {
		return errors.New("compact correction budget does not match original changed lines")
	}
	// Pn is recomputed only when a new capture phase is created. Existing
	// successor transitions deliberately retain the current Pn until the later
	// correction/reopen/recovery seams advance it; re-deriving it from mutable
	// lifecycle fields here would make a sibling capture stale when only Rn moved.
	if state.CapturePhaseRevision != "" && !validSHA256(state.CapturePhaseRevision) {
		return errors.New("compact capture phase revision is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if state.CapturePhaseEpoch < 0 {
		return errors.New("compact capture phase epoch is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if err := validateCompactAdmittedRoleResults(state); err != nil {
		return err
	}
	if err := validateCompactTargetedValidatorAttempts(state); err != nil {
		return err
	}
	if state.InitialAtomicStart != nil {
		if err := state.InitialAtomicStart.Validate(); err != nil {
			return fmt.Errorf("compact initial atomic START binding: %w", err)
		}
		if field := state.InitialAtomicStart.mismatchState(state); field != "" {
			return fmt.Errorf("compact initial atomic START binding does not match state at %s", field) // refusal:by-design world-action: contradictory persisted atomic authority requires code or storage repair
		}
	}
	if state.FixFindingIDs == nil {
		return errors.New("compact fix finding IDs must be explicit")
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return err
	}
	if err := validateCompactReviewLifecycle(state, view); err != nil {
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
	if state.State != StateApproved && state.ApprovedAckToken != "" {
		return errors.New("compact acknowledgement token requires approved authority state") // refusal:by-design world-action: a pending acknowledgement is valid only for the active approved authority that owns it
	}
	if state.State == StateApproved && state.ApprovedAckToken != "" && !validCompactAcknowledgementToken(state.ApprovedAckToken) {
		return errors.New("approved compact acknowledgement token is malformed") // refusal:by-design world-action: only the exact opaque token returned by the provider can acknowledge this authority
	}
	return nil
}

func validateCompactTargetedValidatorAttempts(state CompactState) error {
	if len(state.TargetedValidatorAttempts) > maxCompactTargetedValidatorAttempts {
		return errors.New("compact targeted validator has more than three attempts") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	if len(state.TargetedValidatorAttempts) == 0 {
		return nil
	}
	if state.State != StateCorrectionRequired || state.ProposedCorrectionLines == nil {
		return errors.New("compact targeted validator attempts require an open correction phase") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	seen := make(map[string]bool, len(state.TargetedValidatorAttempts))
	for _, attempt := range state.TargetedValidatorAttempts {
		if attempt.CapturePhaseRevision != state.CapturePhaseRevision || !validSHA256(attempt.TargetIdentity) ||
			!validSHA256(attempt.RequestHash) || !validSHA256(attempt.AttemptDigest) ||
			attempt.Outcome != compactTargetedValidatorAttemptInconclusive {
			return errors.New("compact targeted validator attempt is not bound to the current correction phase") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if seen[attempt.AttemptDigest] {
			return errors.New("compact targeted validator attempt digest repeats") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		seen[attempt.AttemptDigest] = true
	}
	return nil
}

func validateCompactAdmittedRoleResults(state CompactState) error {
	if err := validateCompactRoleResultBounds(state.AdmittedRoleResults); err != nil {
		return err
	}
	previousRole, previousLensOrder := -1, -1
	seenLensOrders := make(map[int]bool, len(state.AdmittedRoleResults))
	seenTuples := make(map[string]bool, len(state.AdmittedRoleResults))
	for _, entry := range state.AdmittedRoleResults {
		roleOrder := -1
		switch entry.Role {
		case CompactRoleLens:
			roleOrder = 0
		case CompactRoleRefuter:
			roleOrder = 1
		case CompactRoleTargetedValidator:
			roleOrder = 2
		default:
			return errors.New("compact admitted role result has an unsupported role") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if roleOrder < previousRole || roleOrder == previousRole && entry.Role == CompactRoleLens && entry.SelectedOrder <= previousLensOrder {
			return errors.New("compact admitted role results are not canonically ordered") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		previousRole = roleOrder
		if entry.Role == CompactRoleLens {
			previousLensOrder = entry.SelectedOrder
		}
		if !validSHA256(entry.TargetIdentity) || !validSHA256(entry.CapturePhaseRevision) ||
			!validSHA256(entry.ArtifactDigest) {
			return errors.New("compact admitted role result has an invalid authority binding") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if entry.RequestHash != "" && !validSHA256(entry.RequestHash) {
			return errors.New("compact admitted role result request hash is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if entry.ResultHash != "" && !validSHA256(entry.ResultHash) {
			return errors.New("compact admitted role result hash is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		value, err := canonicalCompactRoleValue(entry.Value)
		if err != nil || compactPreservedPayloadDigest(append(append([]byte(nil), value...), '\n')) != entry.ArtifactDigest {
			return errors.New("compact admitted role result value does not match its artifact digest") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if compactAdmittedRoleResultIsAccountingOnly(state, entry) {
			continue
		}
		switch entry.Role {
		case CompactRoleLens:
			if entry.TargetIdentity != state.InitialSnapshot.Identity || entry.RequestHash != "" || entry.ResultHash == "" || entry.SelectedOrder < 0 ||
				entry.SelectedOrder >= len(state.SelectedLenses) || state.SelectedLenses[entry.SelectedOrder] != entry.Lens ||
				seenLensOrders[entry.SelectedOrder] {
				return errors.New("compact admitted lens result does not match a unique selected lens") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
			seenLensOrders[entry.SelectedOrder] = true
		case CompactRoleRefuter:
			if entry.TargetIdentity != state.InitialSnapshot.Identity || entry.Lens != "" || entry.SelectedOrder != 0 || entry.RequestHash == "" {
				return errors.New("compact admitted refuter result has an invalid role tuple") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
		case CompactRoleTargetedValidator:
			if entry.Lens != "" || entry.SelectedOrder != 0 || entry.RequestHash == "" {
				return errors.New("compact admitted targeted validator result has an invalid role tuple") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
		}
		tuple := string(entry.Role) + "\x00" + entry.Lens + "\x00" + strconv.Itoa(entry.SelectedOrder) + "\x00" + entry.TargetIdentity + "\x00" + entry.CapturePhaseRevision + "\x00" + entry.RequestHash
		if seenTuples[tuple] {
			return errors.New("compact admitted role result repeats its role request tuple") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		seenTuples[tuple] = true
	}
	return nil
}

// AdmittedRoleResult returns the canonical in-record value for one exact role
// tuple. Callers must still decode and admit the returned bytes at their own
// semantic boundary; this accessor never falls back to a role sidecar.
func (state CompactState) AdmittedRoleResult(role CompactRole, phase, targetIdentity, requestHash string) (json.RawMessage, bool) {
	for _, entry := range state.AdmittedRoleResults {
		if compactAdmittedRoleResultCanSatisfyActiveCapture(state, entry) && entry.Role == role && entry.CapturePhaseRevision == phase && entry.TargetIdentity == targetIdentity && entry.RequestHash == requestHash {
			return append(json.RawMessage(nil), entry.Value...), true
		}
	}
	return nil, false
}

// CompactReviewView is the deterministic semantic interpretation of active
// admitted role values. Persisted result projections remain compatibility data
// until every consumer moves, but never contribute to this derived view.
type CompactReviewView struct {
	LensResults              []LensResult
	Findings                 []Finding
	Classifications          map[string]FindingEvidence
	Outcomes                 map[string]EvidenceOutcome
	FixFindingIDs            []string
	FollowUps                []FollowUp
	RefuterOutcomes          []EvidenceResult
	TargetedValidatorOutcome string
}

func invalidCompactReviewView(format string, args ...any) error {
	return fmt.Errorf("compact review view: "+format, args...)
}

// CompactReviewView derives all review semantics from active admitted values.
// It intentionally does not consult the persisted result projections while the
// migration retains them for old consumers.
func (state CompactState) CompactReviewView() (CompactReviewView, error) {
	view := CompactReviewView{
		LensResults: []LensResult{}, Findings: []Finding{}, Classifications: map[string]FindingEvidence{},
		Outcomes: map[string]EvidenceOutcome{}, FixFindingIDs: []string{}, FollowUps: []FollowUp{},
		RefuterOutcomes: []EvidenceResult{},
	}
	seenLensOrders := map[int]bool{}
	var refuter []EvidenceResult
	previousRole, previousOrder := -1, -1
	candidateCausal := map[string]bool{}
	seenFindings := map[string]bool{}
	for _, entry := range state.AdmittedRoleResults {
		if state.IsAccountingOnlyAdmittedRoleResult(entry) {
			continue
		}
		value, err := canonicalCompactRoleValue(entry.Value)
		if err != nil || !validSHA256(entry.TargetIdentity) || !validSHA256(entry.CapturePhaseRevision) ||
			!validSHA256(entry.ArtifactDigest) || entry.RequestHash != "" && !validSHA256(entry.RequestHash) ||
			entry.ResultHash != "" && !validSHA256(entry.ResultHash) || compactPreservedPayloadDigest(append(value, '\n')) != entry.ArtifactDigest {
			return CompactReviewView{}, invalidCompactReviewView("role value does not match its authority binding")
		}
		roleOrder := map[CompactRole]int{CompactRoleLens: 0, CompactRoleRefuter: 1, CompactRoleTargetedValidator: 2}[entry.Role]
		if entry.Role != CompactRoleLens && entry.Role != CompactRoleRefuter && entry.Role != CompactRoleTargetedValidator || roleOrder < previousRole ||
			roleOrder == previousRole && entry.Role == CompactRoleLens && entry.SelectedOrder <= previousOrder {
			return CompactReviewView{}, invalidCompactReviewView("active roles are not canonically ordered")
		}
		previousRole = roleOrder
		if entry.Role == CompactRoleLens {
			previousOrder = entry.SelectedOrder
		}
		switch entry.Role {
		case CompactRoleLens:
			if entry.TargetIdentity != state.InitialSnapshot.Identity || entry.RequestHash != "" || entry.SelectedOrder < 0 ||
				entry.SelectedOrder >= len(state.SelectedLenses) || entry.Lens != state.SelectedLenses[entry.SelectedOrder] || entry.ResultHash == "" ||
				!compactAdmittedLensCapturePhaseIsKnown(state, entry.CapturePhaseRevision) {
				return CompactReviewView{}, invalidCompactReviewView("lens tuple does not match the active authority")
			}
			envelope, err := decodeCompactAdmittedReviewerValue(value)
			if err != nil || envelope.Schema != admittedReviewerResultSchemaForSubject(envelope.Subject) ||
				envelope.Subject.LineageID != state.LineageID || envelope.Subject.AuthorityRevision != entry.CapturePhaseRevision ||
				envelope.Subject.TargetIdentity != entry.TargetIdentity || envelope.Subject.Lens != entry.Lens ||
				envelope.Subject.SelectedOrder != entry.SelectedOrder || envelope.Subject.CorrectionTargetIdentity != "" ||
				envelope.Admission.Validate(envelope.Subject) != nil {
				return CompactReviewView{}, invalidCompactReviewView("lens envelope does not bind its active tuple")
			}
			var provider compactProviderReviewerResult
			if err := decodeCompactAdmittedRoleValue(envelope.Result, &provider); err != nil || provider.SubjectHash != envelope.Subject.SubjectHash || provider.Lens != entry.Lens {
				return CompactReviewView{}, invalidCompactReviewView("lens provider value does not bind its admitted envelope")
			}
			canonicalPayload, err := json.Marshal(provider)
			if err != nil || envelope.Admission.CanonicalSHA256 != payloadSHA256(append(canonicalPayload, '\n')) {
				return CompactReviewView{}, invalidCompactReviewView("lens envelope canonical digest is invalid")
			}
			result, err := canonicalReviewerResult(LensResult{Lens: provider.Lens, Findings: provider.Findings, Evidence: provider.Evidence}, entry.Lens)
			if err != nil || !reflect.DeepEqual(provider.Findings, result.Findings) || !reflect.DeepEqual(provider.Evidence, result.Evidence) ||
				envelope.Admission.ResultHash != result.ResultHash || entry.ResultHash != result.ResultHash {
				return CompactReviewView{}, invalidCompactReviewView("lens result is not the canonical admitted result")
			}
			if duplicate := seenLensOrders[entry.SelectedOrder]; duplicate {
				return CompactReviewView{}, invalidCompactReviewView("lens slot is ambiguous")
			}
			for _, id := range envelope.Admission.CandidateCausalFindingIDs {
				if candidateCausal[id] {
					return CompactReviewView{}, invalidCompactReviewView("candidate-causal finding ID is admitted more than once")
				}
				candidateCausal[id] = true
			}
			seenLensOrders[entry.SelectedOrder] = true
			view.LensResults = append(view.LensResults, result)
			view.Findings = append(view.Findings, result.Findings...)
		case CompactRoleRefuter:
			if entry.TargetIdentity != state.InitialSnapshot.Identity || entry.Lens != "" || entry.SelectedOrder != 0 || entry.RequestHash == "" || refuter != nil ||
				!compactAdmittedLensCapturePhaseIsKnown(state, entry.CapturePhaseRevision) {
				return CompactReviewView{}, invalidCompactReviewView("refuter tuple is ambiguous or mismatched")
			}
			refuter, err = decodeCompactAdmittedRefuterValue(value)
			if err != nil {
				return CompactReviewView{}, fmt.Errorf("decode active admitted refuter: %w", err)
			}
		case CompactRoleTargetedValidator:
			if entry.Lens != "" || entry.SelectedOrder != 0 || entry.RequestHash == "" || view.TargetedValidatorOutcome != "" ||
				entry.CapturePhaseRevision != state.CapturePhaseRevision {
				return CompactReviewView{}, invalidCompactReviewView("targeted-validator tuple is ambiguous or mismatched")
			}
			admitted, err := decodeCompactAdmittedTargetedValidatorValue(value)
			if err != nil {
				return CompactReviewView{}, fmt.Errorf("decode active admitted targeted validator: %w", err)
			}
			view.TargetedValidatorOutcome = admitted.Outcome
		}
	}
	refuterByID := map[string]EvidenceResult{}
	for _, result := range refuter {
		if result.FindingID == "" || !isConcreteEvidence(result.Proof) || refuterByID[result.FindingID].FindingID != "" {
			return CompactReviewView{}, invalidCompactReviewView("refuter results are malformed or duplicate")
		}
		refuterByID[result.FindingID] = result
	}
	for _, finding := range view.Findings {
		if seenFindings[finding.ID] {
			return CompactReviewView{}, invalidCompactReviewView("admitted lens findings repeat an ID")
		}
		seenFindings[finding.ID] = true
		if !isSevereSeverity(finding.Severity) {
			if candidateCausal[finding.ID] {
				return CompactReviewView{}, invalidCompactReviewView("admission marks a non-severe finding candidate-causal")
			}
			view.Outcomes[finding.ID] = OutcomeInfo
			continue
		}
		if !isSupportedEvidenceClass(finding.EvidenceClass) || !isSupportedCausalDisposition(finding.CausalDisposition) {
			return CompactReviewView{}, invalidCompactReviewView("severe finding has unsupported evidence")
		}
		causality := finding.CausalDisposition
		candidateClaim := causality == CausalIntroduced || causality == CausalBehaviorActivated || causality == CausalWorsened
		if candidateClaim && !candidateCausal[finding.ID] {
			causality = CausalUnknown
		} else if !candidateClaim && candidateCausal[finding.ID] {
			return CompactReviewView{}, invalidCompactReviewView("admission candidate-causal ID contradicts its finding")
		}
		proof := strings.Join(finding.ProofRefs, "\n")
		if !isConcreteEvidence(proof) {
			return CompactReviewView{}, invalidCompactReviewView("severe finding has no concrete proof")
		}
		view.Classifications[finding.ID] = FindingEvidence{FindingID: finding.ID, Severity: finding.Severity, Class: finding.EvidenceClass, Causality: causality, Proof: proof}
		switch {
		case finding.EvidenceClass == EvidenceInsufficient || causality == CausalUnknown:
			view.Outcomes[finding.ID] = OutcomeInconclusive
		case causality == CausalPreExisting || causality == CausalBaseOnly:
			view.Outcomes[finding.ID] = OutcomeInfo
			view.FollowUps = append(view.FollowUps, causalFollowUp(finding, proof))
		case finding.EvidenceClass == EvidenceDeterministic:
			view.Outcomes[finding.ID] = OutcomeCorroborated
			view.FixFindingIDs = append(view.FixFindingIDs, finding.ID)
		default:
			result, found := refuterByID[finding.ID]
			if !found {
				view.Outcomes[finding.ID] = OutcomeInconclusive
				continue
			}
			if result.Outcome != OutcomeCorroborated && result.Outcome != OutcomeRefuted && result.Outcome != OutcomeInconclusive {
				return CompactReviewView{}, invalidCompactReviewView("refuter outcome is unsupported")
			}
			view.Outcomes[finding.ID] = result.Outcome
			if result.Outcome == OutcomeCorroborated {
				view.FixFindingIDs = append(view.FixFindingIDs, finding.ID)
			}
		}
	}
	for id := range candidateCausal {
		if !seenFindings[id] {
			return CompactReviewView{}, invalidCompactReviewView("admission names an unadmitted finding")
		}
	}
	for id := range refuterByID {
		if _, found := view.Classifications[id]; !found || view.Classifications[id].Class != EvidenceInferential {
			return CompactReviewView{}, invalidCompactReviewView("refuter result does not match an inferential finding")
		}
	}
	sort.Strings(view.FixFindingIDs)
	view.RefuterOutcomes = append(view.RefuterOutcomes, refuter...)
	sort.Slice(view.RefuterOutcomes, func(left, right int) bool {
		return view.RefuterOutcomes[left].FindingID < view.RefuterOutcomes[right].FindingID
	})
	return view, nil
}

// ActiveAdmittedLensResult returns the one active value for a selected lens.
// A lens capture keeps its immutable reviewing phase through later normal
// lifecycle advances; a reopen only removes the exact quarantined tuple.
func (state CompactState) ActiveAdmittedLensResult(order int) (CompactAdmittedRoleResult, bool, error) {
	if order < 0 || order >= len(state.SelectedLenses) {
		return CompactAdmittedRoleResult{}, false, nil
	}
	if err := state.Validate(); err != nil {
		return CompactAdmittedRoleResult{}, false, fmt.Errorf("validate active admitted lens result: %w", err)
	}
	lens := state.SelectedLenses[order]
	var active CompactAdmittedRoleResult
	for _, entry := range state.AdmittedRoleResults {
		if state.IsAccountingOnlyAdmittedRoleResult(entry) || entry.Role != CompactRoleLens || entry.TargetIdentity != state.InitialSnapshot.Identity ||
			entry.SelectedOrder != order || entry.Lens != lens {
			continue
		}
		if compactAdmittedRoleResultWasReopened(state, entry) {
			return CompactAdmittedRoleResult{}, false, errors.New("active admitted lens result was removed by reopen") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if !compactAdmittedLensCapturePhaseIsKnown(state, entry.CapturePhaseRevision) ||
			entry.CapturePhaseRevision != state.CapturePhaseRevision && state.State == StateReviewing && len(state.ResultReopens) == 0 {
			return CompactAdmittedRoleResult{}, false, errors.New("retained admitted lens result lacks ordered reopen provenance") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if active.Role != "" {
			return CompactAdmittedRoleResult{}, false, errors.New("active admitted lens result is ambiguous") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		active = entry
	}
	if active.Role == "" {
		return CompactAdmittedRoleResult{}, false, nil
	}
	active.Value = append(json.RawMessage(nil), active.Value...)
	return active, true, nil
}

// compactAdmittedLensCapturePhaseIsKnown recognizes only phases derivable from
// immutable reviewing inputs. CompleteReview and BeginCorrection advance Pn
// without rewriting lens tuples; reopened and recaptured lenses therefore remain
// active while arbitrary stale phase hashes fail closed.
func compactAdmittedLensCapturePhaseIsKnown(state CompactState, phase string) bool {
	for epoch := 0; epoch <= state.CapturePhaseEpoch; epoch++ {
		reviewing := state
		reviewing.State = StateReviewing
		reviewing.CurrentSnapshot = state.InitialSnapshot
		reviewing.CapturePhaseEpoch = epoch
		reviewing.FixFindingIDs = []string{}
		reviewing.ProposedCorrectionLines = nil
		expected, err := deriveCompactCapturePhaseRevision(reviewing)
		if err == nil && phase == expected {
			return true
		}
	}
	return false
}

// reopenCompactAdmittedRoleResults removes one active, canonically ordered
// quarantine set and its one dependent refuter from the canonical record values.
// It retains only digest and tuple metadata for the caller's audit; no removed
// payload can be reused.
func reopenCompactAdmittedRoleResults(state CompactState, quarantineLenses []string) (CompactState, []CompactAdmittedRoleResult, error) {
	if state.State != StateValidating && state.State != StateCorrectionRequired {
		return CompactState{}, nil, errors.New("review reopen-results requires an uncorrected authority and selected lens") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	quarantined := make(map[int]string, len(quarantineLenses))
	for _, lens := range quarantineLenses {
		order := stringIndex(state.SelectedLenses, lens)
		if order < 0 || quarantined[order] != "" {
			return CompactState{}, nil, errors.New("review reopen-results requires a canonical selected lens set") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		quarantined[order] = lens
	}
	if len(quarantined) == 0 {
		return CompactState{}, nil, errors.New("review reopen-results requires a selected lens") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	target, selectedPhase := state.InitialSnapshot.Identity, ""
	selectedEntries := make(map[int]CompactAdmittedRoleResult, len(quarantined))
	for order := range quarantined {
		entry, found, err := state.ActiveAdmittedLensResult(order)
		if err != nil || !found || entry.TargetIdentity != target || compactAdmittedRoleResultWasReopened(state, entry) {
			return CompactState{}, nil, errors.New("review reopen-results selected lens is not admitted in the active capture batch") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if selectedPhase != "" && selectedPhase != entry.CapturePhaseRevision {
			return CompactState{}, nil, errors.New("review reopen-results selected lenses have ambiguous active capture history") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		selectedPhase, selectedEntries[order] = entry.CapturePhaseRevision, entry
	}
	removed := make([]CompactAdmittedRoleResult, 0, len(quarantined)+1)
	remaining := make([]CompactAdmittedRoleResult, 0, len(state.AdmittedRoleResults))
	refuterRemoved := false
	for _, entry := range state.AdmittedRoleResults {
		expected, selected := selectedEntries[entry.SelectedOrder]
		removeLens := selected && entry.Role == CompactRoleLens && entry.TargetIdentity == expected.TargetIdentity &&
			entry.CapturePhaseRevision == expected.CapturePhaseRevision && entry.Lens == expected.Lens && entry.ArtifactDigest == expected.ArtifactDigest &&
			entry.ResultHash == expected.ResultHash && compactAdmittedRoleResultCanSatisfyActiveCapture(state, entry) && !compactAdmittedRoleResultWasReopened(state, entry)
		removeRefuter := entry.Role == CompactRoleRefuter && entry.TargetIdentity == target && entry.CapturePhaseRevision == selectedPhase &&
			compactAdmittedRoleResultCanSatisfyActiveCapture(state, entry) && !compactAdmittedRoleResultWasReopened(state, entry)
		if removeRefuter && refuterRemoved {
			return CompactState{}, nil, errors.New("review reopen-results has more than one dependent refuter") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		if !removeLens && !removeRefuter {
			remaining = append(remaining, entry)
			continue
		}
		refuterRemoved = refuterRemoved || removeRefuter
		entry.Value = nil
		removed = append(removed, entry)
	}
	next := cloneCompactStateInitialAtomicStart(state)
	next.State = StateReviewing
	next.AdmittedRoleResults = remaining
	next.FixFindingIDs = []string{}
	next.ProposedCorrectionLines = nil
	next.ActualCorrectionLines = nil
	next.FixDeltaHash = EmptyFixDeltaHash
	next.OriginalCriteria = nil
	next.CorrectionRegression = nil
	next.EvidenceHash = ""
	next.TargetedValidatorAttempts = []CompactTargetedValidatorAttempt{}
	if err := next.advanceCapturePhase(); err != nil {
		return CompactState{}, nil, err
	}
	if err := next.Validate(); err != nil {
		return CompactState{}, nil, err
	}
	return next, removed, nil
}

func compactAdmittedRoleResultWasReopened(state CompactState, entry CompactAdmittedRoleResult) bool {
	for _, reopen := range state.ResultReopens {
		for _, reference := range reopen.Removed {
			if reference.Role == entry.Role && reference.Lens == entry.Lens && reference.SelectedOrder == entry.SelectedOrder &&
				reference.TargetIdentity == entry.TargetIdentity && reference.CapturePhaseRevision == entry.CapturePhaseRevision &&
				reference.RequestHash == entry.RequestHash && reference.ArtifactDigest == entry.ArtifactDigest && reference.ResultHash == entry.ResultHash {
				return true
			}
		}
	}
	return false
}

// LedgerHash derives the canonical findings-ledger binding from admitted role
// values. Compact authority never persists a second findings projection; callers
// reconstruct the ledger from the canonical active review view.
func (state CompactState) LedgerHash() string {
	view, err := state.CompactReviewView()
	if err != nil || len(view.Findings) == 0 {
		return EmptyFixDeltaHash
	}
	ledger, err := CanonicalLedger(view.Findings)
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
		return errCompactSnapshotIdentityMismatch
	}
	return nil
}

// errCompactSnapshotIdentityMismatch is the exact semantic failure a record
// frozen under a retired snapshot-identity formula produces when the current
// formula recomputes its identity (#2743: every pre-rc.2 compact-v2 record
// fails here after the #2659/PR-#2667 identity purification). It is a
// sentinel so parseCompactRecord can mark the resulting typed
// *CompactSemanticStateError as OutdatedIdentity — the clean-break policy
// keeps such records gate-invalid without rewriting their bytes, and
// diagnostics owe them an honest historical classification instead of
// narrating them as damage.
var errCompactSnapshotIdentityMismatch = errors.New("compact snapshot identity does not match its metadata")

func validateCompactReviewLifecycle(state CompactState, view CompactReviewView) error {
	fixFindingIDs, err := canonicalStrings(state.FixFindingIDs, "fix finding id")
	if err != nil || !equalStrings(fixFindingIDs, state.FixFindingIDs) {
		return errors.New("compact fix finding IDs must be canonical")
	}
	completeReview := func() error {
		if len(view.LensResults) != len(state.SelectedLenses) {
			return errors.New("post-review compact state requires every selected admitted lens result")
		}
		if !equalStrings(view.FixFindingIDs, state.FixFindingIDs) {
			return errors.New("compact fix finding IDs must match the admitted review view")
		}
		return nil
	}
	switch state.State {
	case StateReviewing:
		if len(state.FixFindingIDs) != 0 || state.ProposedCorrectionLines != nil || state.ActualCorrectionLines != nil || state.EvidenceHash != "" {
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
		if err := completeReview(); err != nil || len(state.FixFindingIDs) == 0 || state.EvidenceHash != "" {
			return errors.New("correction-required compact state is incomplete")
		}
	case StateValidating:
		if state.Recovery == nil || state.Recovery.Evidence == nil {
			if err := completeReview(); err != nil || state.EvidenceHash != "" {
				return errors.New("validating compact state is incomplete")
			}
		}
	case StateApproved:
		if err := completeReview(); err != nil {
			return err
		}
		if len(state.CorrectionAttempts) == 0 {
			if state.EvidenceHash != compactReviewEvidenceHash(view) {
				return errors.New("approved clean compact state requires admitted review evidence") // refusal:by-design human-authority: an approval without its immutable admitted-result digest requires authority inspection
			}
		} else if state.EvidenceHash != "" && !validSHA256(state.EvidenceHash) {
			return errors.New("approved corrected compact state has invalid historical verification evidence") // refusal:by-design human-authority: malformed historical evidence on an approved authority requires maintainer inspection
		}
	case StateEscalated:
		if err := completeReview(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid compact review state %q", state.State)
	}
	return nil
}

func validateCompactCorrection(state CompactState) error {
	if state.Recovery != nil && state.Recovery.Evidence != nil {
		return validateCompactRecoveredCorrection(state, *state.Recovery.Evidence)
	}
	if len(state.CorrectionAttempts) == 0 && state.CumulativeCorrectionLines != 0 {
		return errors.New("compact cumulative correction lines require persisted attempts")
	}
	if len(state.CorrectionAttempts) > 0 {
		candidateScope, scopeErr := compactCorrectionCandidateScope(state)
		if scopeErr != nil {
			return errors.New("compact correction attempt is outside frozen scope")
		}
		base, cumulative := state.InitialSnapshot.CandidateTree, 0
		for _, attempt := range state.CorrectionAttempts {
			if attempt.ProposedLines <= 0 || attempt.ActualLines < 0 || attempt.Snapshot.Kind != TargetFixDiff || attempt.Snapshot.Projection != state.InitialSnapshot.Projection || attempt.Snapshot.BaseTree != base ||
				!equalStrings(attempt.Snapshot.LedgerIDs, state.FixFindingIDs) || pathsAreSubset(attempt.Snapshot.Paths, candidateScope) != nil ||
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
	candidateScope, scopeErr := compactCorrectionCandidateScope(state)
	if scopeErr != nil {
		return errors.New("terminal correction authority does not preserve the complete reviewed candidate") // refusal:by-design world-action: contradictory persisted authority requires code or storage repair
	}
	if current.Kind != initial.Kind || current.Projection != initial.Projection || current.UnbornHead != correction.UnbornHead ||
		current.BaseTree != initial.BaseTree || current.CandidateTree != correction.CandidateTree ||
		current.IntendedUntrackedProof != correction.IntendedUntrackedProof ||
		!equalStrings(current.IntendedUntracked, initial.IntendedUntracked) || !equalStrings(current.LedgerIDs, initial.LedgerIDs) ||
		pathsAreSubset(current.Paths, candidateScope) != nil {
		return errors.New("terminal correction authority does not preserve the complete reviewed candidate") // refusal:by-design world-action: contradictory persisted authority requires code or storage repair
	}
	return nil
}

func validateCompactRecoveredCorrection(state CompactState, evidence CompactRecoveredEvidence) error {
	if evidence.Schema != CompactRecoveredEvidenceSchema ||
		evidence.Relation != string(compactTargetChangedScope) || evidence.PathRelation != string(compactPathsSame) ||
		!validSHA256(evidence.PredecessorTargetIdentity) || evidence.NativeCorrectionLines <= 0 ||
		len(evidence.AdmittedRoleReferences) == 0 {
		return errors.New("recovered correction evidence binding is incomplete")
	}
	if err := validateCompactRecoveredEvidenceReferenceShape(state.SelectedLenses, evidence.AdmittedRoleReferences); err != nil {
		return err
	}
	if err := validateCompactRecoveredEvidenceReferencesInSuccessor(state, evidence); err != nil {
		return err
	}
	if state.State != StateValidating || !snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) ||
		len(state.CorrectionAttempts) != 0 || state.CumulativeCorrectionLines != 0 || state.ProposedCorrectionLines == nil ||
		state.ActualCorrectionLines == nil || *state.ProposedCorrectionLines < evidence.NativeCorrectionLines ||
		*state.ActualCorrectionLines != evidence.NativeCorrectionLines || len(state.FixFindingIDs) == 0 || state.EvidenceHash != "" {
		return errors.New("recovered correction state does not preserve accounting-only evidence") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	return nil
}

func compactReviewEvidenceHash(view CompactReviewView) string {
	payload, _ := json.Marshal(struct {
		LensResults     []LensResult               `json:"lens_results"`
		Findings        []Finding                  `json:"findings"`
		Classifications map[string]FindingEvidence `json:"classifications"`
		Outcomes        map[string]EvidenceOutcome `json:"outcomes"`
		FixFindingIDs   []string                   `json:"fix_finding_ids"`
		FollowUps       []FollowUp                 `json:"follow_ups"`
	}{
		LensResults: view.LensResults, Findings: view.Findings, Classifications: view.Classifications,
		Outcomes: view.Outcomes, FixFindingIDs: view.FixFindingIDs, FollowUps: view.FollowUps,
	})
	sum := sha256.Sum256(append([]byte(CompactRecoveredEvidenceSchema+"/review\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validateCompactReviewInput(state CompactState, input CompactReviewInput, view CompactReviewView) error {
	if len(input.LensResults) != len(state.SelectedLenses) {
		return fmt.Errorf("compact review requires all %d selected lens results", len(state.SelectedLenses))
	}
	results := make([]LensResult, len(input.LensResults))
	for index, result := range input.LensResults {
		if result.Lens != state.SelectedLenses[index] {
			return fmt.Errorf("lens result %d does not name its selected lens", index+1)
		}
		for _, evidence := range result.Evidence {
			if evidenceReportsUnavailableInspection(evidence) {
				return fmt.Errorf("lens result %d reports unavailable candidate inspection", index+1)
			}
		}
		canonical, err := CanonicalCompactLensResult(result)
		if err != nil {
			return fmt.Errorf("lens result %d: %w", index+1, err)
		}
		results[index] = canonical
	}
	if !reflect.DeepEqual(results, view.LensResults) {
		return errors.New("compact review input does not match admitted lens results")
	}
	classifications := make(map[string]FindingEvidence, len(input.Classifications))
	for _, classification := range input.Classifications {
		expected, found := view.Classifications[classification.FindingID]
		if !found || classifications[classification.FindingID].FindingID != "" {
			return fmt.Errorf("compact review input has an unknown or duplicate classification for %q", classification.FindingID)
		}
		if classification.Severity != "" && classification.Severity != expected.Severity {
			return fmt.Errorf("compact review input classification severity does not match %q", classification.FindingID)
		}
		classification.Severity = expected.Severity
		classifications[classification.FindingID] = classification
	}
	if !reflect.DeepEqual(classifications, view.Classifications) {
		return errors.New("compact review input does not match admitted classifications")
	}
	refuterOutcomes := append([]EvidenceResult{}, input.RefuterOutcomes...)
	for _, result := range refuterOutcomes {
		if result.FindingID == "" || !isConcreteEvidence(result.Proof) {
			return fmt.Errorf("refuter result %q is invalid", result.FindingID)
		}
	}
	sort.Slice(refuterOutcomes, func(left, right int) bool {
		return refuterOutcomes[left].FindingID < refuterOutcomes[right].FindingID
	})
	if !reflect.DeepEqual(refuterOutcomes, view.RefuterOutcomes) {
		return errors.New("compact review input does not match admitted refuter outcomes")
	}
	return nil
}

func compactReviewViewHasUnresolvedFindings(view CompactReviewView) bool {
	for _, outcome := range view.Outcomes {
		if outcome == OutcomeInconclusive {
			return true
		}
	}
	return false
}

func (state *CompactState) CompleteReview(input CompactReviewInput) error {
	if state.State != StateReviewing {
		return fmt.Errorf("cannot complete review from compact state %q", state.State)
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return err
	}
	if err := validateCompactReviewInput(*state, input, view); err != nil {
		return err
	}
	state.FixFindingIDs = append([]string{}, view.FixFindingIDs...)
	if compactReviewViewHasUnresolvedFindings(view) {
		state.State = StateEscalated
	} else if len(state.FixFindingIDs) > 0 {
		state.State = StateCorrectionRequired
		if err := state.advanceCapturePhase(); err != nil {
			return err
		}
	} else {
		state.State = StateValidating
	}
	return state.Validate()
}

// CloseCleanReviewOnLastEvent closes a clean reviewed candidate when the final
// immutable lens result is the terminal lifecycle event.
func (state *CompactState) CloseCleanReviewOnLastEvent() error {
	if state.State != StateValidating || len(state.FixFindingIDs) != 0 {
		return errors.New("last review event closure requires a clean validating review") // refusal:by-design operator-knowledge: only a clean final lens result may take the no-FINALIZE closure
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return err
	}
	state.State = StateApproved
	state.EvidenceHash = compactReviewEvidenceHash(view)
	return state.Validate()
}

// ErrInvalidFindingLocation identifies reviewer locations that cannot be used
// as repository line evidence.
var ErrInvalidFindingLocation = errors.New("invalid reviewer finding location; correct it to repository/path:<positive-line> or repository/path:<positive-start>-<positive-end> before running gentle-ai review capture-result again")

// FindingLocationErrorReason is a stable machine-readable validation reason.
type FindingLocationErrorReason string

const (
	FindingLocationExpectedPathAndLine  FindingLocationErrorReason = "expected_path_and_line"
	FindingLocationLineNotInteger       FindingLocationErrorReason = "line_suffix_not_integer"
	FindingLocationLineNotPositive      FindingLocationErrorReason = "line_must_be_positive"
	FindingLocationLineOverflowsInteger FindingLocationErrorReason = "line_overflows_integer"
	FindingLocationRangeNotAscending    FindingLocationErrorReason = "range_must_be_ascending"
	FindingLocationPathNotRelative      FindingLocationErrorReason = "path_must_be_repository_relative"
	FindingLocationPathNotCanonical     FindingLocationErrorReason = "path_must_be_canonical"
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

type findingLocation struct {
	Path      string
	StartLine int
	EndLine   int
}

// findingLocationHasPositiveLines reports whether a parsed finding location
// carries strictly positive 1-based line numbers. It is a defense-in-depth
// lower bound for causality: source lines are numbered from 1, so a location
// whose start or end line is below 1 can never designate a line that exists in
// the candidate and must never be treated as candidate-causal. This holds the
// invariant even if a parser path ever yielded a non-positive line.
func findingLocationHasPositiveLines(finding findingLocation) bool {
	return finding.StartLine >= 1 && finding.EndLine >= 1
}

func parseFindingLocation(location string) (findingLocation, error) {
	fail := func(reason FindingLocationErrorReason) (findingLocation, error) {
		return findingLocation{}, &FindingLocationError{Location: location, Reason: reason}
	}
	separator := strings.LastIndexByte(location, ':')
	if separator <= 0 || separator == len(location)-1 {
		return fail(FindingLocationExpectedPathAndLine)
	}
	lineSuffix := location[separator+1:]
	if strings.HasPrefix(lineSuffix, "-") && strings.Count(lineSuffix, "-") == 1 {
		return fail(FindingLocationLineNotPositive)
	}
	startText, endText, ranged := strings.Cut(lineSuffix, "-")
	if strings.Count(lineSuffix, "-") > 1 {
		return fail(FindingLocationLineNotInteger)
	}
	start, reason := parseFindingLocationLine(startText)
	if reason != "" {
		return fail(reason)
	}
	end := start
	if ranged {
		end, reason = parseFindingLocationLine(endText)
		if reason != "" {
			return fail(reason)
		}
		if start > end {
			return fail(FindingLocationRangeNotAscending)
		}
	}
	logicalPath := location[:separator]
	if len(logicalPath) >= 3 && logicalPath[1] == ':' && logicalPath[2] == '/' &&
		((logicalPath[0] >= 'A' && logicalPath[0] <= 'Z') || (logicalPath[0] >= 'a' && logicalPath[0] <= 'z')) {
		return fail(FindingLocationPathNotRelative)
	}
	if _, pathErr := normalizeLogicalPath(strings.ReplaceAll(logicalPath, ":", "/")); pathErr != nil {
		return fail(FindingLocationPathNotCanonical)
	}
	canonical, pathErr := normalizeLogicalPath(logicalPath)
	if pathErr != nil || canonical != logicalPath {
		return fail(FindingLocationPathNotCanonical)
	}
	return findingLocation{Path: canonical, StartLine: start, EndLine: end}, nil
}

func parseFindingLocationLine(value string) (int, FindingLocationErrorReason) {
	if value == "" {
		return 0, FindingLocationLineNotInteger
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return 0, FindingLocationLineNotInteger
		}
	}
	// The all-digits check above guarantees no sign character reaches here, so
	// the value is a non-negative literal. Parse it as an unsigned integer that
	// must fit a positive Go int: bit size strconv.IntSize-1 caps the result at
	// the platform's MaxInt, so any value above it (including the [2^63, 2^64-1]
	// band that a full 64-bit parse accepted before wrapping negative via int())
	// refuses as an overflow instead of silently becoming a negative line.
	line, err := strconv.ParseUint(value, 10, strconv.IntSize-1)
	if err != nil {
		return 0, FindingLocationLineOverflowsInteger
	}
	if line == 0 {
		return 0, FindingLocationLineNotPositive
	}
	return int(line), ""
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

func validateCompactResultReopens(state CompactState) error {
	for _, reopen := range state.ResultReopens {
		lenses, err := compactResultReopenAuditQuarantineLenses(state, reopen)
		if !validSHA256(reopen.PreviousRevision) || reopen.TargetIdentity != state.InitialSnapshot.Identity || err != nil ||
			strings.TrimSpace(reopen.Reason) == "" || strings.TrimSpace(reopen.Actor) == "" ||
			strings.TrimSpace(reopen.MaintainerAuthorization) == "" || reopen.ReopenedAt.IsZero() || len(reopen.Removed) < len(lenses) {
			return errors.New("reviewer result reopen audit record is incomplete")
		}
		for index, lens := range lenses {
			reference := reopen.Removed[index]
			if reference.Role != CompactRoleLens || reference.Lens != lens || reference.SelectedOrder != stringIndex(state.SelectedLenses, lens) ||
				!validSHA256(reference.TargetIdentity) || !validSHA256(reference.CapturePhaseRevision) || !validSHA256(reference.ArtifactDigest) ||
				reference.TargetIdentity != reopen.TargetIdentity || !validSHA256(reference.ResultHash) {
				return errors.New("reviewer result reopen selected lens reference is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
			}
		}
		if len(reopen.Removed) == len(lenses) {
			continue
		}
		if len(reopen.Removed) != len(lenses)+1 {
			return errors.New("reviewer result reopen audit has an invalid selected/dependent cardinality") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
		reference := reopen.Removed[len(lenses)]
		if reference.Role != CompactRoleRefuter || reference.Lens != "" || reference.SelectedOrder != 0 ||
			!validSHA256(reference.TargetIdentity) || !validSHA256(reference.CapturePhaseRevision) || !validSHA256(reference.ArtifactDigest) ||
			reference.TargetIdentity != reopen.TargetIdentity || !validSHA256(reference.RequestHash) {
			return errors.New("reviewer result reopen refuter reference is invalid") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
		}
	}
	return nil
}

func cloneCompactStateValue(state CompactState) (CompactState, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return CompactState{}, err
	}
	var clone CompactState
	if err := json.Unmarshal(payload, &clone); err != nil {
		return CompactState{}, err
	}
	return clone, nil
}

func compactPristineReviewing(state CompactState) bool {
	return state.State == StateReviewing && snapshotsEqual(state.CurrentSnapshot, state.InitialSnapshot) &&
		len(state.AdmittedRoleResults) == 0 && len(state.FixFindingIDs) == 0 && state.ProposedCorrectionLines == nil && state.ActualCorrectionLines == nil &&
		state.FixDeltaHash == EmptyFixDeltaHash && state.OriginalCriteria == nil && state.CorrectionRegression == nil && state.EvidenceHash == "" &&
		state.InvalidationReason == "" && len(state.TargetedValidatorAttempts) == 0 &&
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

// FrozenPolicyForTargetedValidation returns the immutable policy material a
// validator needs without consulting any live policy artifact.
func (state CompactState) FrozenPolicyForTargetedValidation() (string, error) {
	if state.FrozenPolicyContent == nil {
		return "", &CompactFrozenPolicyUnavailableError{LineageID: state.LineageID, PolicyHash: state.PolicyHash}
	}
	if compactPolicyContentHash(*state.FrozenPolicyContent) != state.PolicyHash {
		return "", &CompactFrozenPolicyIntegrityError{LineageID: state.LineageID, PolicyHash: state.PolicyHash}
	}
	return *state.FrozenPolicyContent, nil
}

func compactPolicyContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
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
	remaining := state.CorrectionBudget - state.CumulativeCorrectionLines
	if proposed > remaining {
		return &CorrectionBudgetExceededError{Actual: proposed, Remaining: remaining}
	}
	value := proposed
	state.ProposedCorrectionLines = &value
	state.TargetedValidatorAttempts = []CompactTargetedValidatorAttempt{}
	if err := state.advanceCapturePhase(); err != nil {
		return err
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
	added, err := admitCorrectionScope(snapshot.Paths, state.GenesisPaths)
	if err != nil {
		return err
	}
	if actual < 0 || state.CumulativeCorrectionLines+actual > state.CorrectionBudget {
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
	state.FixDeltaHash, state.ActualCorrectionLines = fixHash, &actual
	original, regression := validation.OriginalCriteria, validation.CorrectionRegression
	state.OriginalCriteria, state.CorrectionRegression = &original, &regression
	if !original.Passed || !regression.Passed {
		// The correction did not complete, so it never earned the widened
		// delivery scope. The attempt stays on record -- it consumed the one
		// correction, and its snapshot still names every path it touched --
		// but CorrectionScopePaths keeps the frozen reviewed manifest, so a
		// companion path an escalated correction merely attempted can never
		// ride out through a delivery gate.
		state.State = StateEscalated
	} else {
		state.State = StateValidating
		state.TargetedValidatorAttempts = nil
		if len(added) > 0 {
			state.CorrectionAddedPaths = added
		}
	}
	return state.Validate()
}

// CompleteCorrectionVerification closes one bounded correction from its
// candidate-bound targeted-validator verdict. The validator is the terminal
// event: no separate final-verification evidence is accepted or persisted.
func (state *CompactState) CompleteCorrectionVerification(snapshot Snapshot, actual int, validation ScopedValidationResult, complete ...Snapshot) error {
	if len(complete) > 1 {
		return errors.New("compact correction accepts at most one complete candidate snapshot") // refusal:by-design world-action: provider code must submit one exact terminal authority
	}
	next, err := cloneCompactStateValue(*state)
	if err != nil {
		return err
	}
	if err := next.CompleteCorrection(snapshot, actual, validation); err != nil {
		return err
	}
	if next.State == StateEscalated {
		// A conclusive failed validator verdict spends the one correction attempt
		// and is terminal without consuming any general final-verification evidence.
		*state = next
		return nil
	}
	if next.State != StateValidating {
		return errors.New("compact correction checks and budget must pass before targeted validation acceptance") // refusal:by-design operator-knowledge: the caller must adjust the candidate or validation result without consuming the open correction
	}
	if len(complete) == 1 {
		next.CurrentSnapshot = complete[0]
	}
	next.State = StateApproved
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
	if state.FixFindingIDs == nil {
		state.FixFindingIDs = []string{}
	}
	if state.AdmittedRoleResults == nil {
		state.AdmittedRoleResults = []CompactAdmittedRoleResult{}
	}
	if state.TargetedValidatorAttempts == nil {
		state.TargetedValidatorAttempts = []CompactTargetedValidatorAttempt{}
	}
	if state.ResultReopens == nil {
		state.ResultReopens = []CompactResultReopen{}
	}
}
