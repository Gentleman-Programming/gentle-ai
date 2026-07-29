package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type TargetApplicability string

const (
	TargetApplicabilityCurrent   TargetApplicability = "current_target"
	TargetApplicabilityUnrelated TargetApplicability = "unrelated"
	TargetApplicabilityAmbiguous TargetApplicability = "ambiguous"
	TargetApplicabilityCorrupted TargetApplicability = "corrupted"
)

type TargetStatusAction string

const (
	TargetStatusActionStart                  TargetStatusAction = "start"
	TargetStatusActionFinalize               TargetStatusAction = "finalize"
	TargetStatusActionValidate               TargetStatusAction = "validate"
	TargetStatusActionRecover                TargetStatusAction = "recover"
	TargetStatusActionRetryFinalVerification TargetStatusAction = "retry_final_verification"
	TargetStatusActionMaintainer             TargetStatusAction = "maintainer_action"
	TargetStatusActionSelectLineage          TargetStatusAction = "select_lineage"
	TargetStatusActionRepairAuthority        TargetStatusAction = "repair_authority"
	TargetStatusActionReconcileFinalize      TargetStatusAction = "reconcile_finalize"
	TargetStatusActionStop                   TargetStatusAction = "stop"
)

type Replayability string

const (
	ReplayabilityNotReplayable        Replayability = "not_replayable"
	ReplayabilityExactReplaySafe      Replayability = "exact_replay_safe"
	ReplayabilityStatusRequired       Replayability = "status_required"
	ReplayabilityManualActionRequired Replayability = "manual_action_required"
)

type TargetStatusRequest struct {
	Target    Target
	LineageID string
}

type TargetProjectionStatus struct {
	Kind                    TargetKind `json:"kind"`
	Projection              Projection `json:"projection"`
	BaseTree                string     `json:"base_tree"`
	InitialReviewTree       string     `json:"initial_review_tree"`
	CurrentCandidateTree    string     `json:"current_candidate_tree"`
	PathsDigest             string     `json:"paths_digest"`
	Paths                   []string   `json:"paths"`
	IntendedUntracked       []string   `json:"intended_untracked"`
	IntendedUntrackedProof  string     `json:"intended_untracked_proof"`
	InitialSnapshotIdentity string     `json:"initial_snapshot_identity"`
	CurrentSnapshotIdentity string     `json:"current_snapshot_identity"`
}

type TargetStatusResult struct {
	Applicability           TargetApplicability                `json:"applicability"`
	AuthorityVersion        AuthorityVersion                   `json:"authority_version,omitempty"`
	LineageID               string                             `json:"lineage_id,omitempty"`
	State                   State                              `json:"state,omitempty"`
	Generation              int                                `json:"generation,omitempty"`
	Revision                string                             `json:"revision,omitempty"`
	ReceiptIdentity         string                             `json:"receipt_identity,omitempty"`
	Action                  TargetStatusAction                 `json:"action"`
	ActionDisposition       RecoveryDisposition                `json:"action_disposition,omitempty"`
	Replayability           Replayability                      `json:"replayability"`
	OriginalChangedLines    int                                `json:"original_changed_lines,omitempty"`
	Tier                    RiskLevel                          `json:"tier,omitempty"`
	CorrectionBudget        int                                `json:"correction_budget,omitempty"`
	SelectedLenses          []string                           `json:"selected_lenses,omitempty"`
	TargetIdentity          string                             `json:"target_identity"`
	AuthorityTargetIdentity string                             `json:"authority_target_identity,omitempty"`
	Projection              TargetProjectionStatus             `json:"projection"`
	CandidateLineageIDs     []string                           `json:"candidate_lineage_ids"`
	FinalVerificationRetry  *FinalVerificationRetryEligibility `json:"final_verification_retry,omitempty"`
}

type targetStatusCandidate struct {
	version            AuthorityVersion
	lineage            string
	compact            *CompactRecord
	legacy             *ValidatedChain
	legacyStore        *Store
	receiptIdentity    string
	receiptPublished   bool
	receiptCanonical   bool
	receiptReplayable  bool
	pendingFinalize    bool
	correctionRecovery bool
	// recoveryDisposition names the `review recover --disposition` value the
	// recovery rules accept for this candidate. It is only set when the
	// recommended action is recovery; guidance never invents a disposition.
	recoveryDisposition    RecoveryDisposition
	finalVerificationRetry *FinalVerificationRetryEligibility
}

// AssessTargetStatus classifies the selected live Git projection against
// validated authority. It only reads Git objects and authority bytes.
func AssessTargetStatus(ctx context.Context, repo string, request TargetStatusRequest) (TargetStatusResult, error) {
	result, _, err := AssessTargetStatusWithSnapshot(ctx, repo, request)
	return result, err
}

// AssessTargetStatusWithSnapshot returns the exact live snapshot used for the
// status classification so callers can derive related routing artifacts from
// the same immutable candidate tree instead of rereading a mutable worktree.
func AssessTargetStatusWithSnapshot(ctx context.Context, repo string, request TargetStatusRequest) (TargetStatusResult, Snapshot, error) {
	if request.LineageID != "" {
		request.LineageID = strings.TrimSpace(request.LineageID)
		if err := validateLineageID(request.LineageID); err != nil {
			return TargetStatusResult{}, Snapshot{}, err
		}
	}
	live, err := (SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, request.Target)
	if err != nil {
		return TargetStatusResult{}, Snapshot{}, err
	}
	result, err := assessTargetStatusSnapshot(ctx, repo, request, live)
	return result, live, err
}

func assessTargetStatusSnapshot(ctx context.Context, repo string, request TargetStatusRequest, live Snapshot) (TargetStatusResult, error) {
	base := TargetStatusResult{
		TargetIdentity:      live.Identity,
		Projection:          targetProjectionFromSnapshot(live),
		CandidateLineageIDs: []string{},
	}

	view, err := loadTargetStatusAuthorityView(ctx, repo, request)
	if err != nil {
		return targetStatusFailure(base, err)
	}

	candidates := []targetStatusCandidate{}
	scopeChangedCandidates := []targetStatusCandidate{}
	approvedScopeRecovery := []targetStatusCandidate{}
	for lineage, candidate := range view.compact {
		if request.LineageID != "" && request.LineageID != lineage {
			continue
		}
		state := candidate.compact.State
		if state.State == StateInvalidated && state.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay &&
			state.InitialSnapshot.Projection == ProjectionStaged && live.Kind == TargetBaseWorkspaceOverlay &&
			live.Projection == ProjectionStaged && state.InitialSnapshot.BaseTree == live.BaseTree &&
			live.Identity != state.CurrentSnapshot.Identity &&
			(compactLiveTargetMatchesValidatedSnapshot(state, live, false) || compactRecoveryAddsGenesisPath(state, live)) {
			candidate.correctionRecovery, candidate.recoveryDisposition = true, RecoveryInvalidated
			candidates = append(candidates, candidate)
			continue
		}
		if state.State == StateApproved && candidate.receiptPublished && candidate.receiptCanonical {
			eligible, eligibilityErr := compactApprovedStagedScopeRecovery(ctx, repo, state, live)
			if eligibilityErr != nil {
				return targetStatusFailure(base, eligibilityErr)
			}
			if eligible {
				candidate.correctionRecovery = true
				candidate.recoveryDisposition = RecoveryScopeChanged
				candidates = append(candidates, candidate)
				continue
			}
		}
		// The same predicates START uses to refuse a fresh lineage against an
		// approved predecessor: either the frozen delivery scope has a changed
		// candidate, or a disjoint base advance preserves the exact feature patch.
		// Classifying either relationship more weakly makes negotiated status emit
		// a START the store then refuses while recovery authorization stays unreachable.
		// Like the staged scope-expansion edge above, only a published
		// canonical receipt may bind an approved predecessor for recovery; a
		// receiptless approved authority keeps its publication-repair routing.
		// Collection stays separate from governing candidates: the sole such
		// predecessor mirrors START's single-recovery-candidate answer below,
		// while plural matches keep the Phase 3e "nothing governs" listing.
		if candidate.receiptPublished && candidate.receiptCanonical {
			rebasedRecovery, rebasedErr := compactApprovedRebasedScopeRecovery(ctx, repo, state, live)
			if rebasedErr != nil {
				return targetStatusFailure(base, rebasedErr)
			}
			if compactApprovedScopeChangedRecovery(state, live) || rebasedRecovery {
				recovery := candidate
				recovery.correctionRecovery = true
				recovery.recoveryDisposition = RecoveryScopeChanged
				approvedScopeRecovery = append(approvedScopeRecovery, recovery)
			}
		}
		if state.State == StateEscalated {
			requested := state
			requested.InitialSnapshot = live
			if compactStartDeliveryScopeMatches(state, requested) {
				candidate.correctionRecovery = compactEscalatedRecoveryTargetChanged(state.CurrentSnapshot, live)
				if candidate.correctionRecovery {
					candidate.recoveryDisposition = RecoveryEscalated
				} else if compactAccountingOnlyEscalation(state) {
					// An accounting-only escalation (both original review and
					// correction regression passed; only the cumulative
					// correction line count crossed the budget) has a native
					// evidence-bound RecoveryEscalated edge that does not
					// require a changed target. Offering it here mirrors that
					// edge instead of dead-ending the operator at Stop.
					candidate.correctionRecovery = true
					candidate.recoveryDisposition = RecoveryEscalated
				} else if eligibility, ok, inspectErr := InspectCompactFinalVerificationRetrySource(ctx, repo, state.LineageID, candidate.compact.Revision); inspectErr != nil {
					return targetStatusFailure(base, inspectErr)
				} else if ok {
					candidate.finalVerificationRetry = &eligibility
				}
				candidates = append(candidates, candidate)
				continue
			}
		} else if state.State == StateCorrectionRequired {
			claim, claimErr := classifyCompactCorrectionTargetForStatus(ctx, repo, state, live)
			if claimErr != nil {
				return targetStatusFailure(base, claimErr)
			}
			switch claim {
			case compactCorrectionTargetResume, compactCorrectionTargetBlocked:
				candidates = append(candidates, candidate)
				continue
			case compactCorrectionTargetRecover:
				candidate.correctionRecovery = true
				candidate.recoveryDisposition = compactCorrectionRecoveryDisposition(state, live)
				candidates = append(candidates, candidate)
				continue
			}
		} else if compactLiveTargetMatchesValidatedSnapshot(state, live, true) {
			candidates = append(candidates, candidate)
			continue
		}
		if request.LineageID == "" && candidate.receiptPublished && (state.State == StateApproved || state.State == StateEscalated) {
			if projectCompactTerminalHistory(state, live) == compactTerminalHistoryScopeChanged {
				scopeChangedCandidates = append(scopeChangedCandidates, candidate)
			}
		}
	}
	for lineage, candidate := range view.legacy {
		if request.LineageID != "" && request.LineageID != lineage {
			continue
		}
		chain := *candidate.legacy
		transaction := chain.Records[len(chain.Records)-1].Transaction
		if legacyLiveTargetMatchesValidatedSnapshot(transaction, live) {
			if transaction.State == StateApproved {
				if candidate.legacyStore == nil {
					return corruptedTargetStatus(base), nil
				}
				identity, receiptErr := inspectLegacyTargetReceipt(*candidate.legacyStore, transaction)
				if receiptErr != nil {
					return targetStatusFailure(base, receiptErr)
				}
				candidate.receiptIdentity = identity
				candidate.receiptPublished = identity != ""
			}
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 && len(approvedScopeRecovery) == 1 {
		// START answers recover for exactly one approved delivery-scope
		// predecessor with no other claimant, so status must bind that same
		// predecessor for recovery instead of routing to a START the store
		// refuses (issue #1826). Plural matches stay stale listings below.
		candidates = approvedScopeRecovery
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].lineage != candidates[j].lineage {
			return candidates[i].lineage < candidates[j].lineage
		}
		return candidates[i].version < candidates[j].version
	})
	sort.Slice(scopeChangedCandidates, func(i, j int) bool {
		return scopeChangedCandidates[i].lineage < scopeChangedCandidates[j].lineage
	})
	if len(candidates) == 0 && len(scopeChangedCandidates) > 1 {
		// Two or more stale (scope-changed) lineages never decide anything
		// by themselves: with no EXACTLY governing candidate, nothing
		// governs this live target, so the sole continuation is the same
		// "start fresh" shape the zero-candidate case already reports below
		// (including its own overlay+staged safety stop, a live-projection
		// check unrelated to lineage history). The stale lineages stay
		// listed in CandidateLineageIDs purely so recovering one of them
		// remains a discoverable OPTION, never a required disambiguation
		// chore forced by history alone.
		base.Applicability = TargetApplicabilityUnrelated
		base.Action, base.Replayability = TargetStatusActionStart, ReplayabilityNotReplayable
		if live.Kind == TargetBaseWorkspaceOverlay && live.Projection == ProjectionStaged {
			base.Action, base.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
		}
		for _, candidate := range scopeChangedCandidates {
			base.CandidateLineageIDs = append(base.CandidateLineageIDs, candidate.lineage)
		}
		return base, nil
	}
	switch len(candidates) {
	case 0:
		base.Applicability = TargetApplicabilityUnrelated
		base.Action, base.Replayability = TargetStatusActionStart, ReplayabilityNotReplayable
		if live.Kind == TargetBaseWorkspaceOverlay && live.Projection == ProjectionStaged {
			base.Action, base.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
		}
		return base, nil
	case 1:
		return targetStatusForCandidate(base, candidates[0]), nil
	default:
		base.Applicability = TargetApplicabilityAmbiguous
		base.Action = TargetStatusActionSelectLineage
		base.Replayability = ReplayabilityStatusRequired
		for _, candidate := range candidates {
			base.CandidateLineageIDs = append(base.CandidateLineageIDs, candidate.lineage)
		}
		return base, nil
	}
}

func corruptedTargetStatus(result TargetStatusResult) TargetStatusResult {
	result.Applicability = TargetApplicabilityCorrupted
	result.Action = TargetStatusActionRepairAuthority
	result.Replayability = ReplayabilityManualActionRequired
	return result
}

func targetStatusForCandidate(result TargetStatusResult, candidate targetStatusCandidate) TargetStatusResult {
	result.Applicability = TargetApplicabilityCurrent
	result.AuthorityVersion = candidate.version
	result.LineageID = candidate.lineage
	if candidate.compact != nil {
		record := *candidate.compact
		state := record.State
		result.State, result.Generation, result.Revision = state.State, state.Generation, record.Revision
		result.AuthorityTargetIdentity = state.CurrentSnapshot.Identity
		result.OriginalChangedLines, result.Tier, result.CorrectionBudget = state.OriginalChangedLines, state.RiskLevel, state.CorrectionBudget
		result.SelectedLenses = append([]string{}, state.SelectedLenses...)
		result.Projection = targetProjectionFromCompact(state, result.Projection)
		result.ReceiptIdentity = candidate.receiptIdentity
		if candidate.pendingFinalize {
			result.Action, result.Replayability = TargetStatusActionReconcileFinalize, ReplayabilityStatusRequired
			return result
		}
		if candidate.finalVerificationRetry != nil {
			eligibility := *candidate.finalVerificationRetry
			result.FinalVerificationRetry = &eligibility
			result.Action, result.Replayability = TargetStatusActionRetryFinalVerification, ReplayabilityManualActionRequired
			result.ActionDisposition = RecoveryFinalVerificationRetry
			return result
		}
		if candidate.correctionRecovery {
			result.Action, result.Replayability = TargetStatusActionRecover, ReplayabilityManualActionRequired
			result.ActionDisposition = candidate.recoveryDisposition
			return result
		}
		if state.State == StateEscalated || state.State == StateCorrectionRequired && state.CorrectionAttemptConsumed() {
			result.Action, result.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
			return result
		}
		if !candidate.receiptPublished && candidate.receiptReplayable {
			result.Action, result.Replayability = TargetStatusActionFinalize, ReplayabilityExactReplaySafe
			return result
		}
		result.Action, result.Replayability, result.ActionDisposition = targetStatusAction(state.State)
		return result
	}
	chain := *candidate.legacy
	transaction := chain.Records[len(chain.Records)-1].Transaction
	result.State, result.Generation, result.Revision = transaction.State, transaction.Generation, chain.HeadRevision
	if transaction.OriginalChangedLines != nil {
		result.OriginalChangedLines = *transaction.OriginalChangedLines
	}
	if transaction.CorrectionBudget != nil {
		result.CorrectionBudget = *transaction.CorrectionBudget
	}
	result.Tier = transaction.RiskLevel
	result.Projection = targetProjectionFromLegacy(transaction, result.Projection)
	result.ReceiptIdentity = candidate.receiptIdentity
	if transaction.State == StateApproved {
		result.Action, result.Replayability = TargetStatusActionValidate, ReplayabilityNotReplayable
	} else {
		result.Action, result.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
	}
	return result
}

func inspectLegacyTargetReceipt(store Store, transaction Transaction) (string, error) {
	payload, err := os.ReadFile(filepath.Join(store.Dir, "artifacts", "receipt.json"))
	if err != nil {
		return "", fmt.Errorf("read legacy target receipt: %w", err)
	}
	existing, err := ParseReceipt(payload)
	if err != nil {
		return "", fmt.Errorf("parse legacy target receipt: %w", err)
	}
	expected, err := transaction.Receipt()
	if err != nil {
		return "", fmt.Errorf("derive terminal legacy receipt: %w", err)
	}
	canonical, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		return "", fmt.Errorf("canonicalize legacy target receipt: %w", err)
	}
	canonical = append(canonical, '\n')
	if !reflect.DeepEqual(existing, expected) || !bytes.Equal(payload, canonical) {
		return "", errors.New("legacy target receipt does not equal the canonical derived receipt")
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type compactTargetArtifactObservation struct {
	exists    bool
	identity  string
	content   []byte
	canonical []byte
}

func newCompactTargetArtifactObservation(payload, canonical []byte) compactTargetArtifactObservation {
	sum := sha256.Sum256(payload)
	return compactTargetArtifactObservation{
		exists: true, identity: "sha256:" + hex.EncodeToString(sum[:]),
		content: append([]byte(nil), payload...), canonical: append([]byte(nil), canonical...),
	}
}

func compactTargetArtifactObservationsEqual(left, right compactTargetArtifactObservation) bool {
	return left.exists == right.exists && left.identity == right.identity &&
		bytes.Equal(left.content, right.content) && bytes.Equal(left.canonical, right.canonical)
}

type compactTargetReceiptObservation struct {
	artifact   compactTargetArtifactObservation
	published  bool
	replayable bool
}

func inspectCompactTargetReceipt(store CompactStore, state CompactState) (compactTargetReceiptObservation, error) {
	payload, readErr := os.ReadFile(store.ReceiptPath())
	if errors.Is(readErr, os.ErrNotExist) {
		if state.State != StateApproved && state.State != StateEscalated {
			return compactTargetReceiptObservation{}, nil
		}
		if _, deriveErr := state.Receipt(); deriveErr != nil {
			return compactTargetReceiptObservation{}, fmt.Errorf("derive terminal compact receipt replay proof: %w", deriveErr)
		}
		return compactTargetReceiptObservation{replayable: true}, nil
	}
	if readErr != nil {
		return compactTargetReceiptObservation{}, fmt.Errorf("read compact target receipt: %w", readErr)
	}
	observation := compactTargetReceiptObservation{
		artifact: newCompactTargetArtifactObservation(payload, nil),
	}
	expected, deriveErr := state.Receipt()
	if deriveErr != nil {
		return observation, errors.New("non-terminal compact authority has a published receipt")
	}
	existing, parseErr := ParseCompactReceipt(payload)
	if parseErr != nil {
		return observation, fmt.Errorf("parse compact target receipt: %w", parseErr)
	}
	if !CompactReceiptEqual(existing, expected) {
		return observation, errors.New("compact target receipt does not equal the derived receipt")
	}
	canonical, marshalErr := json.MarshalIndent(existing, "", "  ")
	if marshalErr != nil {
		return observation, fmt.Errorf("canonicalize compact target receipt: %w", marshalErr)
	}
	observation.artifact.canonical = append(canonical, '\n')
	observation.published = true
	return observation, nil
}

// targetStatusAction maps a state to the single operation that state accepts.
// When that operation is recovery it also names the disposition the recovery
// rules accept, so guidance never routes an operator to a bare `recover` whose
// --disposition they must guess.
func targetStatusAction(state State) (TargetStatusAction, Replayability, RecoveryDisposition) {
	switch state {
	case StateReviewing, StateCorrectionRequired, StateValidating:
		return TargetStatusActionFinalize, ReplayabilityNotReplayable, ""
	case StateApproved:
		return TargetStatusActionValidate, ReplayabilityNotReplayable, ""
	case StateInvalidated:
		return TargetStatusActionRecover, ReplayabilityManualActionRequired, RecoveryInvalidated
	case StateEscalated:
		return TargetStatusActionMaintainer, ReplayabilityManualActionRequired, ""
	default:
		return TargetStatusActionFinalize, ReplayabilityNotReplayable, ""
	}
}

func compactLiveTargetMatchesSnapshot(ctx context.Context, repo string, state CompactState, live Snapshot, requireCurrentCandidate bool) bool {
	if !compactLiveTargetMatchesValidatedSnapshot(state, live, requireCurrentCandidate) {
		return false
	}
	return (SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, live) == nil
}

func legacyLiveTargetMatchesSnapshot(ctx context.Context, repo string, transaction Transaction, live Snapshot) bool {
	if !legacyLiveTargetMatchesValidatedSnapshot(transaction, live) {
		return false
	}
	return (SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, live) == nil
}

func targetProjectionFromSnapshot(snapshot Snapshot) TargetProjectionStatus {
	return TargetProjectionStatus{
		Kind: snapshot.Kind, Projection: snapshot.Projection, BaseTree: snapshot.BaseTree,
		InitialReviewTree: snapshot.CandidateTree, CurrentCandidateTree: snapshot.CandidateTree,
		PathsDigest: snapshot.PathsDigest, Paths: append([]string(nil), snapshot.Paths...),
		IntendedUntracked: append([]string(nil), snapshot.IntendedUntracked...), IntendedUntrackedProof: snapshot.IntendedUntrackedProof,
		InitialSnapshotIdentity: snapshot.Identity, CurrentSnapshotIdentity: snapshot.Identity,
	}
}

func targetProjectionFromCompact(state CompactState, projection TargetProjectionStatus) TargetProjectionStatus {
	projection.InitialReviewTree = state.InitialSnapshot.CandidateTree
	projection.InitialSnapshotIdentity = state.InitialSnapshot.Identity
	return projection
}

func targetProjectionFromLegacy(transaction Transaction, projection TargetProjectionStatus) TargetProjectionStatus {
	projection.InitialReviewTree = transaction.InitialReviewTree
	return projection
}
