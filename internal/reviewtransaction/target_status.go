package reviewtransaction

import (
	"context"
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
	TargetStatusActionStart           TargetStatusAction = "start"
	TargetStatusActionValidate        TargetStatusAction = "validate"
	TargetStatusActionRecover         TargetStatusAction = "recover"
	TargetStatusActionMaintainer      TargetStatusAction = "maintainer_action"
	TargetStatusActionSelectLineage   TargetStatusAction = "select_lineage"
	TargetStatusActionRepairAuthority TargetStatusAction = "repair_authority"
	TargetStatusActionStop            TargetStatusAction = "stop"
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
	PrePR     *PrePRRequest
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

// TargetStatusDecision is the core-owned executable projection of one status
// classification. Adapters render Selector and RecoverySelector; they do not
// reclassify the target relationship or reconstruct recovery representability.
type TargetStatusDecision struct {
	CandidateRelation                  TargetApplicability
	SemanticTransition                 TargetStatusAction
	TargetIdentity                     string
	Selector                           Target
	RecoverySelector                   *Target
	SelectorFreeAccountingOnlyRecovery bool
	FrozenReviewing                    bool
}

type TargetStatusResult struct {
	Applicability                      TargetApplicability    `json:"applicability"`
	AuthorityVersion                   AuthorityVersion       `json:"authority_version,omitempty"`
	LineageID                          string                 `json:"lineage_id,omitempty"`
	State                              State                  `json:"state,omitempty"`
	Generation                         int                    `json:"generation,omitempty"`
	Revision                           string                 `json:"revision,omitempty"`
	Action                             TargetStatusAction     `json:"action"`
	ActionDisposition                  RecoveryDisposition    `json:"action_disposition,omitempty"`
	Replayability                      Replayability          `json:"replayability"`
	OriginalChangedLines               int                    `json:"original_changed_lines,omitempty"`
	Tier                               RiskLevel              `json:"tier,omitempty"`
	CorrectionBudget                   int                    `json:"correction_budget,omitempty"`
	CorrectionBudgetPolicy             string                 `json:"correction_budget_policy,omitempty"`
	SelectedLenses                     []string               `json:"selected_lenses,omitempty"`
	TargetIdentity                     string                 `json:"target_identity"`
	AuthorityTargetIdentity            string                 `json:"authority_target_identity,omitempty"`
	Projection                         TargetProjectionStatus `json:"projection"`
	CandidateLineageIDs                []string               `json:"candidate_lineage_ids"`
	Decision                           TargetStatusDecision   `json:"-"`
	authorityTargetKind                TargetKind
	authorityProjection                Projection
	selectorFreeAccountingOnlyRecovery bool
	frozenReviewing                    bool
}

type targetStatusCandidate struct {
	version                     AuthorityVersion
	lineage                     string
	compact                     *CompactRecord
	legacy                      *ValidatedChain
	legacyStore                 *Store
	correctionRecovery          bool
	frozenReviewing             bool
	frozenReviewingPendingSlots bool
	frozenReviewingDrifted      bool
	// selectorFreeAccountingOnlyRecovery is carried from the eligibility
	// predicate so projection never guesses it from snapshot identity domains.
	selectorFreeAccountingOnlyRecovery bool
	// recoveryDisposition names the `review recover --disposition` value the
	// recovery rules accept for this candidate. It is only set when the
	// recommended action is recovery; guidance never invents a disposition.
	recoveryDisposition RecoveryDisposition
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
	if request.LineageID == "" && request.Target.Kind == TargetCurrentChanges && request.Target.Projection == ProjectionWorkspace {
		candidates, recoveryErr := selectorlessCommittedBaseDiffCorrections(ctx, repo)
		if recoveryErr != nil {
			return TargetStatusResult{}, Snapshot{}, recoveryErr
		}
		switch len(candidates) {
		case 0:
		case 1:
			live, request.LineageID = candidates[0].snapshot, candidates[0].lineage
		default:
			lineages := make([]string, len(candidates))
			for index, candidate := range candidates {
				lineages[index] = candidate.lineage
			}
			return projectTargetStatusDecision(TargetStatusResult{
				Applicability: TargetApplicabilityAmbiguous, Action: TargetStatusActionSelectLineage,
				Replayability: ReplayabilityStatusRequired, TargetIdentity: live.Identity,
				Projection: targetProjectionFromSnapshot(live), CandidateLineageIDs: lineages,
			}), live, nil
		}
	}
	result, err := assessTargetStatusSnapshot(ctx, repo, request, live)
	if err == nil {
		result = projectTargetStatusDecision(result)
		result = bindTargetStatusDecisionBaseRef(result, request.Target, request.PrePR)
	}
	return result, live, err
}

type selectorlessCommittedBaseDiffCorrection struct {
	lineage  string
	snapshot Snapshot
}

func selectorlessCommittedBaseDiffCorrections(ctx context.Context, repo string) ([]selectorlessCommittedBaseDiffCorrection, error) {
	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return nil, err
	}
	candidates := []selectorlessCommittedBaseDiffCorrection{}
	for _, store := range stores {
		record, loadErr := store.LoadContext(ctx)
		if loadErr != nil {
			if IsCompactAuthorityOperationalFailure(loadErr) {
				return nil, loadErr
			}
			continue
		}
		live, rebuildErr := RebuildCommittedBaseDiffCorrectionCandidate(ctx, repo, record.State)
		if rebuildErr != nil {
			if IsCompactAuthorityOperationalFailure(rebuildErr) || IsCorrectionBudgetExceeded(rebuildErr) {
				return nil, rebuildErr
			}
			continue
		}
		candidates = append(candidates, selectorlessCommittedBaseDiffCorrection{lineage: record.State.LineageID, snapshot: live})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lineage < candidates[j].lineage })
	return candidates, nil
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
		if request.LineageID != "" && request.Target.Kind == TargetCurrentChanges && request.Target.Projection != ProjectionStaged &&
			!compactLiveTargetMatchesValidatedSnapshot(state, live, true) {
			eligible, pendingSlots, eligibilityErr := explicitReviewingCompactCandidate(ctx, repo, candidate)
			if eligibilityErr != nil {
				return targetStatusFailure(base, eligibilityErr)
			}
			if eligible {
				candidate.frozenReviewing = true
				candidate.frozenReviewingPendingSlots = pendingSlots
				if !pendingSlots {
					candidate.frozenReviewingDrifted = frozenReviewingCandidateDrifted(ctx, repo, state)
				}
				candidates = append(candidates, candidate)
				continue
			}
		}
		if state.State == StateInvalidated && state.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay &&
			state.InitialSnapshot.Projection == ProjectionStaged && live.Kind == TargetBaseWorkspaceOverlay &&
			live.Projection == ProjectionStaged && state.InitialSnapshot.BaseTree == live.BaseTree &&
			live.Identity != state.CurrentSnapshot.Identity &&
			(compactLiveTargetMatchesValidatedSnapshot(state, live, false) || compactRecoveryAddsGenesisPath(state, live)) {
			candidate.correctionRecovery, candidate.recoveryDisposition = true, RecoveryInvalidated
			candidates = append(candidates, candidate)
			continue
		}
		if state.State == StateCorrectionRequired {
			_, eligible, eligibilityErr := compactCorrectionRequiredStagedScopeRecovery(ctx, repo, state, live)
			if eligibilityErr != nil {
				return targetStatusFailure(base, eligibilityErr)
			}
			if eligible {
				candidate.correctionRecovery, candidate.recoveryDisposition = true, RecoveryScopeChanged
				candidates = append(candidates, candidate)
				continue
			}
		}
		if state.State == StateEscalated {
			if compactEscalatedRecoveryTargetChanged(state.CurrentSnapshot, live) {
				candidate.correctionRecovery = true
				candidate.recoveryDisposition = RecoveryEscalated
				candidates = append(candidates, candidate)
				continue
			}
			requested := state
			requested.InitialSnapshot = live
			if compactStartDeliveryScopeMatches(state, requested) {
				if compactAccountingOnlyEscalation(state) {
					// An accounting-only escalation (both original review and
					// correction regression passed; only the cumulative
					// correction line count crossed the budget) has a native
					// evidence-bound RecoveryEscalated edge that does not
					// require a changed target. Offering it here mirrors that
					// edge instead of dead-ending the operator at Stop.
					candidate.correctionRecovery = true
					candidate.recoveryDisposition = RecoveryEscalated
					candidate.selectorFreeAccountingOnlyRecovery = true
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
		if request.LineageID == "" && (state.State == StateApproved || state.State == StateEscalated) &&
			projectCompactTerminalHistory(state, live) == compactTerminalHistoryScopeChanged {
			scopeChangedCandidates = append(scopeChangedCandidates, candidate)
		}
	}
	for lineage, candidate := range view.legacy {
		if request.LineageID != "" && request.LineageID != lineage {
			continue
		}
		chain := *candidate.legacy
		transaction := chain.Records[len(chain.Records)-1].Transaction
		if legacyLiveTargetMatchesValidatedSnapshot(transaction, live) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 && request.LineageID != "" {
		_, compactExists := view.compact[request.LineageID]
		_, legacyExists := view.legacy[request.LineageID]
		if !compactExists && !legacyExists {
			// #2645: an explicitly requested lineage that owns no authority at
			// all must not reclassify the live target as fresh. START's own
			// discovery resolves whatever authority exactly governs this
			// candidate regardless of the requested name, so STATUS has to
			// advertise that same decision — one recursion with the
			// restriction lifted keeps the answer in this single place.
			unrestricted := request
			unrestricted.LineageID = ""
			return assessTargetStatusSnapshot(ctx, repo, unrestricted, live)
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
		} else if live.Kind == TargetBaseDiff && len(live.Paths) == 0 {
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
		} else if live.Kind == TargetBaseDiff && len(live.Paths) == 0 {
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

// explicitReviewingCompactCandidate admits a drifted reviewing authority only
// after its immutable review inputs and every canonical result slot are safe.
// It also reports whether a reviewer result remains to be captured.
func explicitReviewingCompactCandidate(ctx context.Context, repo string, candidate targetStatusCandidate) (bool, bool, error) {
	if candidate.compact == nil {
		return false, false, nil
	}
	state := candidate.compact.State
	if state.State != StateReviewing || len(state.SelectedLenses) == 0 {
		return false, false, nil
	}
	superseded, err := CompactLineageSuperseded(ctx, repo, state.LineageID)
	if err != nil || superseded {
		return false, false, err
	}
	store, err := CompactAuthoritativeStore(ctx, repo, state.LineageID)
	if err != nil {
		return false, false, err
	}
	pending := false
	for order, lens := range state.SelectedLenses {
		slot, err := ReadCompactReviewerResultSlot(store.Dir, order, lens)
		if err != nil {
			return false, false, err
		}
		pending = pending || !slot.Occupied
	}
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return false, false, err
	}
	for order, lens := range state.SelectedLenses {
		if _, err := NewArtifactSubject(state, candidate.compact.Revision, frozen, lens, order, ""); err != nil {
			return false, false, err
		}
	}
	return true, pending, nil
}

// frozenReviewingCandidateDrifted reports whether the live worktree, projected
// through the frozen candidate's own selector, no longer reproduces the frozen
// candidate tree. A fully captured frozen review continues generically to
// finalize only while that candidate stays coherent; post-capture worktree
// drift keeps the stop, and any projection failure fails closed as drift.
func frozenReviewingCandidateDrifted(ctx context.Context, repo string, state CompactState) bool {
	frozen := state.InitialSnapshot
	target := Target{Kind: frozen.Kind, Projection: frozen.Projection, IntendedUntracked: append([]string{}, frozen.IntendedUntracked...)}
	if target.Kind == TargetBaseDiff || target.Kind == TargetBaseWorkspaceOverlay {
		target.BaseRef = frozen.BaseTree
	}
	live, err := (SnapshotBuilder{Repo: repo}).Build(ctx, target)
	return err != nil || live.CandidateTree != frozen.CandidateTree
}

/*
func compactLocalBaseAdvanceCompatibility(ctx context.Context, repo string, state CompactState, target Target, live Snapshot) *BaseAdvanceCompatibility {
	if state.CurrentSnapshot.Kind != TargetBaseDiff || state.Recovery != nil || target.Kind != TargetBaseDiff || strings.TrimSpace(target.BaseRef) == "" {
		return nil
	}
	receipt, err := state.Receipt()
	if err != nil {
		return nil
	}
	proof, err := deriveExplicitBaseAdvanceCompatibility(ctx, repo, Receipt{
		BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
	}, GateRequest{Gate: GatePrePush, Target: target}, live, gateArtifactPreimages{})
	if err != nil {
		return nil
	}
	return &proof
}

func compactPrePRContentCompatible(ctx context.Context, repo string, state CompactState, snapshot Snapshot, prePR *PrePRRequest) bool {
	if prePR == nil || prePR.Boundary == nil || prePR.Boundary.Commit == prePR.Boundary.MergeBase {
		return true
	}
	receipt, err := state.Receipt()
	if err != nil {
		return false
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return false
	}
	_, err = deriveBaseAdvanceCompatibility(ctx, repo, Receipt{
		BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
	}, GateRequest{Gate: GatePrePR, PrePR: prePR}, snapshot, &resolvedPrePRRefs{
		Selection: *prePR.Boundary, HeadCommit: head,
	}, gateArtifactPreimages{}, false)
	return err == nil
}

*/

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
		if candidate.frozenReviewing {
			result.TargetIdentity = state.InitialSnapshot.Identity
			result.Projection = targetProjectionFromSnapshot(state.InitialSnapshot)
			result.frozenReviewing = true
		}
		result.State, result.Generation, result.Revision = state.State, state.Generation, record.Revision
		result.AuthorityTargetIdentity = state.CurrentSnapshot.Identity
		result.authorityTargetKind, result.authorityProjection = state.InitialSnapshot.Kind, state.InitialSnapshot.Projection
		result.OriginalChangedLines, result.Tier, result.CorrectionBudget, result.CorrectionBudgetPolicy = state.OriginalChangedLines, state.RiskLevel, state.CorrectionBudget, state.CorrectionBudgetPolicy
		result.SelectedLenses = append([]string{}, state.SelectedLenses...)
		result.Projection = targetProjectionFromCompact(state, result.Projection)
		if candidate.frozenReviewing && !candidate.frozenReviewingPendingSlots && candidate.frozenReviewingDrifted {
			result.Action, result.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
			return result
		}
		if candidate.correctionRecovery {
			result.Action, result.Replayability = TargetStatusActionRecover, ReplayabilityManualActionRequired
			result.ActionDisposition = candidate.recoveryDisposition
			result.selectorFreeAccountingOnlyRecovery = candidate.selectorFreeAccountingOnlyRecovery
			return result
		}
		if state.State == StateEscalated || state.State == StateCorrectionRequired && state.CorrectionAttemptConsumed() {
			result.Action, result.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
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
	result.authorityTargetKind, result.authorityProjection = transaction.Snapshot.Kind, transaction.Snapshot.Projection
	result.Projection = targetProjectionFromLegacy(transaction, result.Projection)
	if transaction.State == StateApproved {
		result.Action, result.Replayability = TargetStatusActionValidate, ReplayabilityNotReplayable
	} else {
		result.Action, result.Replayability = TargetStatusActionStop, ReplayabilityManualActionRequired
	}
	return result
}

func projectTargetStatusDecision(result TargetStatusResult) TargetStatusResult {
	selector := Target{
		Kind: result.Projection.Kind, Projection: result.Projection.Projection,
		IntendedUntracked: append([]string{}, result.Projection.IntendedUntracked...),
	}
	if selector.Projection == "" {
		selector.Projection = ProjectionWorkspace
	}
	if selector.Kind == TargetBaseDiff || selector.Kind == TargetBaseWorkspaceOverlay {
		selector.BaseRef = result.Projection.BaseTree
	}
	decision := TargetStatusDecision{
		CandidateRelation: result.Applicability, SemanticTransition: result.Action,
		TargetIdentity: result.TargetIdentity, Selector: selector, FrozenReviewing: result.frozenReviewing,
	}
	if result.Action != TargetStatusActionRecover {
		result.Decision = decision
		return result
	}
	if result.selectorFreeAccountingOnlyRecovery {
		// This is the one evidence-bound recovery that intentionally reuses an
		// unchanged target. Its absence of a selector is an explicit core
		// decision, never an adapter inference from a nil pointer.
		decision.SelectorFreeAccountingOnlyRecovery = true
		result.Decision = decision
		return result
	}

	authorityKind, authorityProjection := result.authorityTargetKind, result.authorityProjection
	if authorityKind == "" {
		authorityKind = TargetCurrentChanges
	}
	if authorityProjection == "" {
		authorityProjection = ProjectionWorkspace
	}
	representable := authorityKind == selector.Kind
	stagedScopeRecovery := result.ActionDisposition == RecoveryScopeChanged &&
		(result.State == StateApproved || result.State == StateCorrectionRequired) &&
		authorityKind == TargetBaseDiff && selector.Kind == TargetBaseWorkspaceOverlay &&
		selector.Projection == ProjectionStaged
	approvedRebasedRecovery := result.ActionDisposition == RecoveryScopeChanged && result.State == StateApproved && selector.Kind == TargetBaseDiff
	representable = representable || stagedScopeRecovery || approvedRebasedRecovery
	if !representable {
		result.Decision = decision
		return result
	}
	recovery := selector
	if stagedScopeRecovery || result.ActionDisposition == RecoveryInvalidated && selector.Kind == TargetBaseWorkspaceOverlay && selector.Projection == ProjectionStaged {
		recovery.Projection = ProjectionStaged
	} else if authorityProjection != selector.Projection {
		if !approvedRebasedRecovery && result.ActionDisposition != RecoveryEscalated {
			result.Decision = decision
			return result
		}
		recovery.Projection = selector.Projection
	}
	decision.RecoverySelector = &recovery
	result.Decision = decision
	return result
}

func bindTargetStatusDecisionBaseRef(result TargetStatusResult, requested Target, prePR *PrePRRequest) TargetStatusResult {
	baseRef := strings.TrimSpace(requested.BaseRef)
	if prePR != nil && prePR.Boundary != nil {
		// The merge-base binds identity; the advertised boundary is the exact
		// selector a Pre-PR follow-up must replay.
		baseRef = strings.TrimSpace(prePR.Boundary.Selector)
	}
	if baseRef == "" {
		return result
	}
	if result.Decision.Selector.Kind == TargetBaseDiff || result.Decision.Selector.Kind == TargetBaseWorkspaceOverlay {
		result.Decision.Selector.BaseRef = baseRef
	}
	if result.Decision.RecoverySelector != nil &&
		(result.Decision.RecoverySelector.Kind == TargetBaseDiff || result.Decision.RecoverySelector.Kind == TargetBaseWorkspaceOverlay) {
		result.Decision.RecoverySelector.BaseRef = baseRef
	}
	return result
}

// targetStatusAction maps a state to the single operation that state accepts.
// When that operation is recovery it also names the disposition the recovery
// rules accept, so guidance never routes an operator to a bare `recover` whose
// --disposition they must guess.
func targetStatusAction(state State) (TargetStatusAction, Replayability, RecoveryDisposition) {
	switch state {
	case StateReviewing, StateCorrectionRequired, StateValidating, StateApproved:
		return TargetStatusActionStop, ReplayabilityManualActionRequired, ""
	case StateInvalidated:
		return TargetStatusActionRecover, ReplayabilityManualActionRequired, RecoveryInvalidated
	case StateEscalated:
		return TargetStatusActionMaintainer, ReplayabilityManualActionRequired, ""
	default:
		return TargetStatusActionStop, ReplayabilityManualActionRequired, ""
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
