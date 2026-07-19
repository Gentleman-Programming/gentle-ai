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
	TargetStatusActionStart             TargetStatusAction = "start"
	TargetStatusActionFinalize          TargetStatusAction = "finalize"
	TargetStatusActionValidate          TargetStatusAction = "validate"
	TargetStatusActionRecover           TargetStatusAction = "recover"
	TargetStatusActionMaintainer        TargetStatusAction = "maintainer_action"
	TargetStatusActionSelectLineage     TargetStatusAction = "select_lineage"
	TargetStatusActionRepairAuthority   TargetStatusAction = "repair_authority"
	TargetStatusActionReconcileFinalize TargetStatusAction = "reconcile_finalize"
	TargetStatusActionStop              TargetStatusAction = "stop"
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
	Applicability        TargetApplicability    `json:"applicability"`
	AuthorityVersion     AuthorityVersion       `json:"authority_version,omitempty"`
	LineageID            string                 `json:"lineage_id,omitempty"`
	State                State                  `json:"state,omitempty"`
	Generation           int                    `json:"generation,omitempty"`
	Revision             string                 `json:"revision,omitempty"`
	ReceiptIdentity      string                 `json:"receipt_identity,omitempty"`
	Action               TargetStatusAction     `json:"action"`
	Replayability        Replayability          `json:"replayability"`
	OriginalChangedLines int                    `json:"original_changed_lines,omitempty"`
	Tier                 RiskLevel              `json:"tier,omitempty"`
	CorrectionBudget     int                    `json:"correction_budget,omitempty"`
	TargetIdentity       string                 `json:"target_identity"`
	Projection           TargetProjectionStatus `json:"projection"`
	CandidateLineageIDs  []string               `json:"candidate_lineage_ids"`
}

type targetStatusCandidate struct {
	version            AuthorityVersion
	lineage            string
	compact            *CompactRecord
	legacy             *ValidatedChain
	receiptIdentity    string
	receiptPublished   bool
	receiptReplayable  bool
	pendingFinalize    bool
	correctionRecovery bool
}

// AssessTargetStatus classifies the selected live Git projection against
// validated authority. It only reads Git objects and authority bytes.
func AssessTargetStatus(ctx context.Context, repo string, request TargetStatusRequest) (TargetStatusResult, error) {
	if request.LineageID != "" {
		request.LineageID = strings.TrimSpace(request.LineageID)
		if err := validateLineageID(request.LineageID); err != nil {
			return TargetStatusResult{}, err
		}
	}
	live, err := (SnapshotBuilder{Repo: repo}).Build(ctx, request.Target)
	if err != nil {
		return TargetStatusResult{}, err
	}
	base := TargetStatusResult{
		TargetIdentity:      live.Identity,
		Projection:          targetProjectionFromSnapshot(live),
		CandidateLineageIDs: []string{},
	}

	var compactStores []CompactStore
	if request.LineageID == "" {
		compactStores, err = CompactAuthorityLeaves(ctx, repo)
	} else {
		compactStores, err = DiscoverCompactStores(ctx, repo)
		for index := len(compactStores) - 1; index >= 0; index-- {
			if compactStores[index].lineageID != request.LineageID {
				compactStores = append(compactStores[:index], compactStores[index+1:]...)
			}
		}
	}
	if err != nil {
		return corruptedTargetStatus(base), nil
	}
	compact := make(map[string]targetStatusCandidate, len(compactStores))
	for _, store := range compactStores {
		record, loadErr := store.Load()
		if loadErr != nil {
			return corruptedTargetStatus(base), nil
		}
		identity, published, replayable, receiptErr := inspectCompactTargetReceipt(store, record.State)
		if receiptErr != nil {
			return corruptedTargetStatus(base), nil
		}
		pending, pendingErr := store.PendingFinalizeAttemptReadOnly()
		if pendingErr != nil {
			return corruptedTargetStatus(base), nil
		}
		copy := record
		compact[record.State.LineageID] = targetStatusCandidate{
			version: AuthorityVersionCompact, lineage: record.State.LineageID, compact: &copy,
			receiptIdentity: identity, receiptPublished: published, receiptReplayable: replayable, pendingFinalize: pending != nil,
		}
	}

	legacyStores, err := DiscoverAuthoritativeStores(ctx, repo)
	if err != nil {
		return corruptedTargetStatus(base), nil
	}
	legacy := make(map[string]ValidatedChain, len(legacyStores))
	legacyStoreByLineage := make(map[string]Store, len(legacyStores))
	for _, store := range legacyStores {
		if request.LineageID != "" && store.lineageID != request.LineageID {
			continue
		}
		chain, loadErr := store.LoadChain()
		if loadErr != nil {
			return corruptedTargetStatus(base), nil
		}
		lineage := chain.Records[len(chain.Records)-1].Transaction.LineageID
		legacy[lineage] = chain
		legacyStoreByLineage[lineage] = store
	}
	for lineage := range compact {
		if _, mixed := legacy[lineage]; mixed {
			return corruptedTargetStatus(base), nil
		}
	}

	candidates := []targetStatusCandidate{}
	scopeChangedCandidates := []targetStatusCandidate{}
	unassessableScopeCandidates := []targetStatusCandidate{}
	for lineage, candidate := range compact {
		if request.LineageID != "" && request.LineageID != lineage {
			continue
		}
		state := candidate.compact.State
		if state.State == StateEscalated {
			requested := state
			requested.InitialSnapshot = live
			if compactStartDeliveryScopeMatches(state, requested) {
				candidate.correctionRecovery = compactEscalatedRecoveryTargetChanged(state.CurrentSnapshot, live)
				candidates = append(candidates, candidate)
				continue
			}
		} else if state.State == StateCorrectionRequired {
			requested := candidate.compact.State
			requested.InitialSnapshot = live
			switch classifyCompactCorrectionTarget(ctx, repo, state, requested) {
			case compactCorrectionTargetResume, compactCorrectionTargetBlocked:
				candidates = append(candidates, candidate)
				continue
			case compactCorrectionTargetRecover:
				candidate.correctionRecovery = true
				candidates = append(candidates, candidate)
				continue
			}
		} else if compactLiveTargetMatchesSnapshot(ctx, repo, state, live, true) {
			candidates = append(candidates, candidate)
			continue
		}
		if request.LineageID == "" && candidate.receiptPublished && (state.State == StateApproved || state.State == StateEscalated) {
			gate := GatePostApply
			if live.Projection == ProjectionStaged {
				gate = GatePreCommit
			}
			assessment, assessErr := AssessCompactGateTarget(ctx, repo, state, NativeGateRequestInput{
				Gate: gate, LineageID: state.LineageID, IntendedUntracked: append([]string{}, state.CurrentSnapshot.IntendedUntracked...),
			})
			if assessErr != nil {
				unassessableScopeCandidates = append(unassessableScopeCandidates, candidate)
				continue
			}
			if assessment.Applicability == CompactGateTargetScopeChanged {
				scopeChangedCandidates = append(scopeChangedCandidates, candidate)
			}
		}
	}
	for lineage, chain := range legacy {
		if request.LineageID != "" && request.LineageID != lineage {
			continue
		}
		transaction := chain.Records[len(chain.Records)-1].Transaction
		if legacyLiveTargetMatchesSnapshot(ctx, repo, transaction, live) {
			receiptIdentity := ""
			if transaction.State == StateApproved {
				receiptIdentity, err = inspectLegacyTargetReceipt(legacyStoreByLineage[lineage], transaction)
				if err != nil {
					return corruptedTargetStatus(base), nil
				}
			}
			copy := chain
			candidates = append(candidates, targetStatusCandidate{
				version: AuthorityVersionLegacy, lineage: lineage, legacy: &copy,
				receiptIdentity: receiptIdentity, receiptPublished: receiptIdentity != "",
			})
		}
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
	if len(candidates) == 0 && len(scopeChangedCandidates) > 0 && len(scopeChangedCandidates)+len(unassessableScopeCandidates) > 1 {
		scopeChangedCandidates = append(scopeChangedCandidates, unassessableScopeCandidates...)
		sort.Slice(scopeChangedCandidates, func(i, j int) bool {
			return scopeChangedCandidates[i].lineage < scopeChangedCandidates[j].lineage
		})
		base.Applicability = TargetApplicabilityAmbiguous
		base.Action = TargetStatusActionSelectLineage
		base.Replayability = ReplayabilityStatusRequired
		for _, candidate := range scopeChangedCandidates {
			base.CandidateLineageIDs = append(base.CandidateLineageIDs, candidate.lineage)
		}
		return base, nil
	}
	if len(candidates) == 0 && len(unassessableScopeCandidates) > 0 {
		return corruptedTargetStatus(base), nil
	}

	switch len(candidates) {
	case 0:
		base.Applicability = TargetApplicabilityUnrelated
		base.Action = TargetStatusActionStart
		base.Replayability = ReplayabilityNotReplayable
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
		result.OriginalChangedLines, result.Tier, result.CorrectionBudget = state.OriginalChangedLines, state.RiskLevel, state.CorrectionBudget
		result.Projection = targetProjectionFromCompact(state, result.Projection)
		result.ReceiptIdentity = candidate.receiptIdentity
		if candidate.pendingFinalize {
			result.Action, result.Replayability = TargetStatusActionReconcileFinalize, ReplayabilityStatusRequired
			return result
		}
		if candidate.correctionRecovery {
			result.Action, result.Replayability = TargetStatusActionRecover, ReplayabilityManualActionRequired
			return result
		}
		if state.State == StateEscalated || compactHistoricalFailedValidator(state) {
			result.Action, result.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
			return result
		}
		if !candidate.receiptPublished && candidate.receiptReplayable {
			result.Action, result.Replayability = TargetStatusActionFinalize, ReplayabilityExactReplaySafe
			return result
		}
		result.Action, result.Replayability = targetStatusAction(state.State)
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

func inspectCompactTargetReceipt(store CompactStore, state CompactState) (identity string, published, replayable bool, err error) {
	payload, readErr := os.ReadFile(store.ReceiptPath())
	if errors.Is(readErr, os.ErrNotExist) {
		if state.State != StateApproved && state.State != StateEscalated {
			return "", false, false, nil
		}
		if _, deriveErr := state.Receipt(); deriveErr != nil {
			return "", false, false, fmt.Errorf("derive terminal compact receipt replay proof: %w", deriveErr)
		}
		return "", false, true, nil
	}
	if readErr != nil {
		return "", false, false, fmt.Errorf("read compact target receipt: %w", readErr)
	}
	expected, deriveErr := state.Receipt()
	if deriveErr != nil {
		return "", false, false, errors.New("non-terminal compact authority has a published receipt")
	}
	existing, parseErr := ParseCompactReceipt(payload)
	if parseErr != nil {
		return "", false, false, fmt.Errorf("parse compact target receipt: %w", parseErr)
	}
	if !CompactReceiptEqual(existing, expected) {
		return "", false, false, errors.New("compact target receipt does not equal the derived receipt")
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), true, false, nil
}

func targetStatusAction(state State) (TargetStatusAction, Replayability) {
	switch state {
	case StateReviewing, StateCorrectionRequired, StateValidating:
		return TargetStatusActionFinalize, ReplayabilityNotReplayable
	case StateApproved:
		return TargetStatusActionValidate, ReplayabilityNotReplayable
	case StateInvalidated:
		return TargetStatusActionRecover, ReplayabilityManualActionRequired
	case StateEscalated:
		return TargetStatusActionMaintainer, ReplayabilityManualActionRequired
	default:
		return TargetStatusActionFinalize, ReplayabilityNotReplayable
	}
}

func compactLiveTargetMatchesSnapshot(ctx context.Context, repo string, state CompactState, live Snapshot, requireCurrentCandidate bool) bool {
	initial := state.InitialSnapshot
	proof := initial.IntendedUntrackedProof
	if requireCurrentCandidate {
		proof = state.CurrentSnapshot.IntendedUntrackedProof
	}
	if initial.Projection != live.Projection || !compactStartTargetKindsCompatible(initial.Kind, live.Kind) ||
		initial.BaseTree != live.BaseTree || requireCurrentCandidate && state.CurrentSnapshot.CandidateTree != live.CandidateTree ||
		pathsAreSubset(live.Paths, state.GenesisPaths) != nil || !equalStrings(initial.IntendedUntracked, live.IntendedUntracked) ||
		proof != live.IntendedUntrackedProof || len(live.LedgerIDs) != 0 {
		return false
	}
	return (SnapshotBuilder{Repo: repo}).ValidateEvidence(ctx, live) == nil
}

func legacyLiveTargetMatchesSnapshot(ctx context.Context, repo string, transaction Transaction, live Snapshot) bool {
	genesis := transaction.GenesisPaths
	if len(genesis) == 0 {
		genesis = transaction.Snapshot.Paths
	}
	kindsMatch := compactStartTargetKindsCompatible(transaction.Snapshot.Kind, live.Kind) ||
		transaction.Snapshot.Kind == TargetFixDiff && (live.Kind == TargetCurrentChanges || live.Kind == TargetBaseDiff)
	if transaction.Snapshot.Projection != live.Projection || !kindsMatch ||
		transaction.BaseTree != live.BaseTree || transaction.FinalCandidateTree != live.CandidateTree ||
		pathsAreSubset(live.Paths, genesis) != nil || !equalStrings(transaction.Snapshot.IntendedUntracked, live.IntendedUntracked) ||
		transaction.Snapshot.IntendedUntrackedProof != live.IntendedUntrackedProof || len(live.LedgerIDs) != 0 {
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
