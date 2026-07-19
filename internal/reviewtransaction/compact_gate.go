package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

var finalCompactGateAllowHook = func() {}

type CompactGateTargetApplicability string

const (
	CompactGateTargetExact        CompactGateTargetApplicability = "exact"
	CompactGateTargetScopeChanged CompactGateTargetApplicability = "scope-changed"
	CompactGateTargetUnrelated    CompactGateTargetApplicability = "unrelated"
)

type CompactGateTargetAssessment struct {
	Applicability CompactGateTargetApplicability
	Expected      Snapshot
	Actual        Snapshot
}

// AssessCompactGateTarget derives only gate-target applicability. It does not
// authorize a gate, read external evidence, acquire the writer lock, or mutate
// authority. Delivery still requires EvaluateCompactGate after one exact
// receipt has been selected.
func AssessCompactGateTarget(ctx context.Context, repo string, state CompactState, input NativeGateRequestInput) (CompactGateTargetAssessment, error) {
	assessment := CompactGateTargetAssessment{Expected: state.CurrentSnapshot}
	if err := state.Validate(); err != nil {
		return assessment, fmt.Errorf("validate compact gate target authority: %w", err)
	}
	if strings.TrimSpace(input.LineageID) != "" && input.LineageID != state.LineageID {
		assessment.Applicability = CompactGateTargetUnrelated
		return assessment, nil
	}
	input.LineageID = state.LineageID
	if input.IntendedUntracked == nil {
		input.IntendedUntracked = append([]string{}, state.CurrentSnapshot.IntendedUntracked...)
	}
	// The next-slice intended set is only meaningful to EvaluateCompactGate's
	// intended-retention guard; applicability here derives purely from the
	// request target and snapshot comparison, so the signal is discarded.
	request, _, err := buildCompactGateRequest(ctx, repo, state, input)
	if err != nil && input.Gate == GatePrePush && state.Recovery != nil && state.InitialSnapshot.Kind == TargetCurrentChanges {
		if chain, ok, chainErr := deriveCompactRecoveryBinding(ctx, repo, state); chainErr == nil && ok && chain.BaseTree != state.InitialSnapshot.BaseTree {
			if retried, _, retryErr := buildCompactGateRequestWithPushBase(ctx, repo, state, input, chain.BaseTree); retryErr == nil {
				request, err = retried, nil
			}
		}
	}
	if err != nil && input.Gate == GateRelease {
		head, headErr := resolveCommit(ctx, repo, "HEAD")
		if headErr == nil {
			request = GateRequest{
				Schema:  GateRequestSchema,
				Gate:    GateRelease,
				Target:  Target{Kind: TargetExactRevision, Revision: head},
				Release: &ReleaseRequest{Revision: head},
			}
			err = nil
		}
	}
	if err != nil {
		return assessment, fmt.Errorf("derive compact gate target: %w", err)
	}
	if err := validateCompactUntrackedScope(ctx, repo, state, request); err != nil {
		assessment.Applicability = CompactGateTargetScopeChanged
		return assessment, nil
	}
	snapshot, resolvedPrePR, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		return assessment, fmt.Errorf("build compact gate target: %w", err)
	}
	assessment.Actual = snapshot
	squashedFixDelivery := compactSquashedFixDelivery(request.Gate, state, snapshot, resolvedPrePR, state.CurrentSnapshot.CandidateTree)
	strictBinding := request.Gate == GatePostApply || request.Gate == GatePreCommit ||
		request.Gate == GatePrePush && state.InitialSnapshot.Kind != TargetCurrentChanges
	pathsMatch := pathsAreSubset(snapshot.Paths, state.GenesisPaths) == nil
	baseMatches := snapshot.BaseTree == state.CurrentSnapshot.BaseTree || request.Target.Kind == TargetFixDiff
	if strictBinding {
		pathsMatch = snapshot.PathsDigest == state.CurrentSnapshot.PathsDigest || squashedFixDelivery
		baseMatches = snapshot.BaseTree == state.CurrentSnapshot.BaseTree || squashedFixDelivery
	}
	if request.Gate == GatePrePR {
		// A base advance is authorized only by the later evidence-bearing gate
		// evaluation, but it still names this receipt when candidate and paths
		// remain exact, or when a current-changes receipt provably reaches the
		// diverged publication boundary unchanged.
		baseMatches = true
		if !pathsMatch && snapshot.CandidateTree == state.CurrentSnapshot.CandidateTree {
			if proof, proofErr := deriveCurrentChangesBoundaryCompatibility(ctx, repo, state, request, snapshot, resolvedPrePR); proofErr == nil && proof.Compatible {
				pathsMatch = true
			}
		}
	}
	if snapshot.CandidateTree == state.CurrentSnapshot.CandidateTree && pathsMatch && baseMatches {
		assessment.Applicability = CompactGateTargetExact
		return assessment, nil
	}
	if (request.Gate == GatePrePush || request.Gate == GatePrePR) && state.Recovery != nil && resolvedPrePR != nil {
		rebindBaseCommit := resolvedPrePR.BaseCommit
		if request.Gate == GatePrePush {
			rebindBaseCommit = request.Target.BaseRef
		}
		if _, ok := rebindCompactRecoveryDelivery(ctx, repo, state, snapshot, state.CurrentSnapshot.CandidateTree, rebindBaseCommit, resolvedPrePR.HeadCommit); ok {
			assessment.Applicability = CompactGateTargetExact
			return assessment, nil
		}
	}
	if snapshot.BaseTree == state.CurrentSnapshot.BaseTree || snapshot.PathsDigest == state.CurrentSnapshot.PathsDigest ||
		hasAnyReviewPath(snapshot.Paths, state.GenesisPaths) {
		assessment.Applicability = CompactGateTargetScopeChanged
		return assessment, nil
	}
	assessment.Applicability = CompactGateTargetUnrelated
	return assessment, nil
}

func hasAnyReviewPath(left, right []string) bool {
	paths := make(map[string]struct{}, len(left))
	for _, logicalPath := range left {
		paths[logicalPath] = struct{}{}
	}
	for _, logicalPath := range right {
		if _, ok := paths[logicalPath]; ok {
			return true
		}
	}
	return false
}

func compactSquashedFixDelivery(gate GateKind, state CompactState, snapshot Snapshot, refs *resolvedPrePRRefs, finalCandidateTree string) bool {
	return gate == GatePrePush && state.CurrentSnapshot.Kind == TargetFixDiff && refs != nil && refs.DeliveredCommitCount == 1 &&
		snapshot.CandidateTree == finalCandidateTree && snapshot.BaseTree == state.InitialSnapshot.BaseTree &&
		equalStrings(snapshot.Paths, state.GenesisPaths) && snapshot.PathsDigest == digestPaths(state.GenesisPaths)
}

func EvaluateCompactGate(ctx context.Context, repo string, receipt CompactReceipt, input NativeGateRequestInput) NativeGateEvaluation {
	return evaluateCompactGate(ctx, repo, receipt, input, false)
}

func evaluateCompactGate(ctx context.Context, repo string, receipt CompactReceipt, input NativeGateRequestInput, authorityLockHeld bool) NativeGateEvaluation {
	var denialContext GateContext
	invalid := func(reason string, cause ...error) NativeGateEvaluation {
		return NativeGateEvaluation{Result: GateInvalidated, Reason: reason, Context: denialContext, Cause: errors.Join(cause...)}
	}
	if err := receipt.Validate(); err != nil {
		return invalid("compact review receipt is invalid: " + err.Error())
	}
	if strings.TrimSpace(input.LineageID) != "" && input.LineageID != receipt.LineageID {
		return invalid("compact gate lineage does not match the receipt")
	}
	store, err := CompactAuthoritativeStore(ctx, repo, receipt.LineageID)
	if err != nil {
		return invalid("compact review store cannot be derived: "+err.Error(), err)
	}
	record, err := store.Load()
	if err != nil {
		return invalid("compact review authority cannot be loaded: "+err.Error(), err)
	}
	if _, err := CompactAuthorityLeaves(ctx, repo); err != nil {
		return invalid(err.Error(), err)
	}
	superseded, err := CompactLineageSuperseded(ctx, repo, receipt.LineageID)
	if err != nil {
		return invalid(err.Error(), err)
	}
	if superseded {
		return invalid("compact receipt belongs to superseded historical authority")
	}
	authoritative, err := record.State.Receipt()
	if err != nil || !compactReceiptEqual(authoritative, receipt) {
		return invalid("compact receipt does not match current authority")
	}
	// The findings are immutable once loaded; derive their ledger binding once
	// so every context emitted below is consistent by construction.
	ledgerHash := record.State.LedgerHash()
	denialContext = GateContext{
		Gate: input.Gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
		StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
		BaseTree: receipt.BaseTree, CandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
		FixDeltaHash: receipt.FixDeltaHash, PolicyHash: receipt.PolicyHash, LedgerHash: ledgerHash, EvidenceHash: receipt.EvidenceHash,
	}
	if input.Gate == GatePrePR && strings.TrimSpace(input.BaseRef) != "" {
		denialContext.PrePRBoundary = &PrePRBoundarySelection{Source: PrePRBoundaryExplicit, Selector: strings.TrimSpace(input.BaseRef)}
	}
	if receipt.TerminalState == TerminalEscalated {
		return NativeGateEvaluation{Result: GateEscalated, Reason: nativeGateReason(GateEscalated)}
	}
	if (input.Gate == GatePrePush || input.Gate == GatePrePR) && record.State.InitialSnapshot.Kind == TargetCurrentChanges {
		emptyTree, emptyTreeErr := (SnapshotBuilder{Repo: repo}).emptyTree(ctx)
		if emptyTreeErr != nil {
			return invalid("repository empty tree cannot be derived: "+emptyTreeErr.Error(), emptyTreeErr)
		}
		if record.State.InitialSnapshot.UnbornHead && record.State.InitialSnapshot.BaseTree == emptyTree {
			return invalid("first publication from an empty-base review receipt is not supported")
		}
	}
	request, nextSliceIntended, err := buildCompactGateRequest(ctx, repo, record.State, input)
	if err != nil && input.Gate == GatePrePush && record.State.Recovery != nil && record.State.InitialSnapshot.Kind == TargetCurrentChanges {
		// A scope_changed recovery successor created after its predecessor's
		// delivery was committed can only restate the delivered tree as its
		// own base, so its one-commit delivery derivation always fails. Retry
		// once from the composed chain base; the later receipt binding still
		// re-verifies the complete delivery before any authorization.
		if chain, ok, chainErr := deriveCompactRecoveryBinding(ctx, repo, record.State); chainErr == nil && ok && chain.BaseTree != record.State.InitialSnapshot.BaseTree {
			if retried, retriedNextSlice, retryErr := buildCompactGateRequestWithPushBase(ctx, repo, record.State, input, chain.BaseTree); retryErr == nil {
				request, nextSliceIntended, err = retried, retriedNextSlice, nil
			}
		}
	}
	if err != nil {
		if input.Gate == GatePrePR {
			denialContext.Denial = &GateDenial{Stage: "boundary-selection", Code: "unavailable"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "compact gate inputs cannot be derived: " + err.Error(), Context: denialContext, Cause: err}
		}
		denialContext.Denial = &GateDenial{Stage: "delivery-derivation", Code: "unavailable"}
		return NativeGateEvaluation{Result: GateInvalidated, Reason: "compact gate inputs cannot be derived: " + err.Error(), Context: denialContext, Cause: err}
	}
	expectedIntended := record.State.CurrentSnapshot.IntendedUntracked
	if nextSliceIntended != nil {
		// The approved candidate is committed exactly and its frozen intended
		// paths are tracked in HEAD; only the already-computed still-untracked
		// subset can be retained by the live next-slice target.
		expectedIntended = nextSliceIntended
	}
	if (request.Gate == GatePostApply || request.Gate == GatePreCommit) && !equalStrings(request.Target.IntendedUntracked, expectedIntended) {
		denialContext.Denial = &GateDenial{Stage: "scope-validation", Code: "intended-untracked-mismatch"}
		return invalid("current repository target does not retain the authoritative intended-untracked paths")
	}
	if err := validateCompactUntrackedScope(ctx, repo, record.State, request); err != nil {
		denialContext.Denial = &GateDenial{Stage: "scope-validation", Code: "untracked-out-of-scope"}
		if compactGateInfrastructureFailure(err) {
			return invalid(err.Error(), err)
		}
		return invalid(err.Error())
	}
	preimages, err := readGateArtifactPreimages(request)
	if err != nil {
		return invalid("compact gate evidence cannot be read: "+err.Error(), err)
	}
	if len(preimages.policy) > 0 && hashArtifactPayload(preimages.policy) != record.State.PolicyHash {
		return invalid("explicit policy does not match compact authority")
	}
	snapshot, resolvedPrePR, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		// A snapshot derivation failure is either a genuine infrastructure
		// fault (git/process/context) or a semantic scope denial such as an
		// intended-untracked path that is now tracked or only partially
		// staged. Guard exactly like validateCompactUntrackedScope above:
		// attach Cause only for infrastructure faults so they still fail
		// closed, and otherwise mark a semantic denial so the invalidation
		// persists through compactInvalidationDenialBound.
		denialContext.Denial = &GateDenial{Stage: "target-derivation", Code: "unavailable"}
		if compactGateInfrastructureFailure(err) {
			return invalid("current repository target cannot be derived: "+err.Error(), err)
		}
		return invalid("current repository target cannot be derived: " + err.Error())
	}
	if request.Gate == GatePrePush && record.State.InitialSnapshot.Kind == TargetCurrentChanges && snapshot.BaseTree == snapshot.CandidateTree {
		return invalid("pre-push current-changes receipt requires a delivered tree change")
	}
	if request.Gate == GatePrePush && (resolvedPrePR == nil || resolvedPrePR.DeliveredCommitCount < 1) {
		return invalid("pre-push validation requires at least one delivered commit")
	}
	if request.Gate == GatePrePush && record.State.InitialSnapshot.Kind == TargetCurrentChanges && resolvedPrePR.DeliveredCommitCount != 1 {
		return invalid("pre-push current-changes receipt requires exactly one delivery commit")
	}
	// An empty-remote bootstrap publishes the candidate's complete history,
	// so publication-range validation is mandatory for every target kind —
	// including current-changes; no kind may skip it.
	bootstrapPublication := resolvedPrePR != nil && resolvedPrePR.Selection.Source == PrePRBoundaryEmptyRemoteBootstrap
	validatePublicationRange := request.Gate == GatePrePush && (record.State.InitialSnapshot.Kind == TargetBaseDiff || bootstrapPublication) ||
		record.State.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay && (request.Gate == GatePrePush || request.Gate == GatePrePR)
	if validatePublicationRange {
		publicationGenesis := record.State.GenesisPaths
		if record.State.Recovery != nil {
			if chain, ok, chainErr := deriveCompactRecoveryBinding(ctx, repo, record.State); chainErr == nil && ok {
				publicationGenesis = chain.GenesisPaths
			}
		}
		if err := validateReviewedPublicationRange(ctx, repo, publicationGenesis, resolvedPrePR); err != nil {
			if compactGateInfrastructureFailure(err) {
				return invalid(err.Error(), err)
			}
			return invalid(err.Error())
		}
	}
	compatibleAdvance := false
	var compatibility *BaseAdvanceCompatibility
	if request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree {
		legacyShape := Receipt{BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest}
		if proof, proofErr := deriveBaseAdvanceCompatibility(ctx, repo, legacyShape, request, snapshot, resolvedPrePR, preimages); proofErr == nil {
			compatibility = &proof
			compatibleAdvance = proof.Compatible
		} else if proof, boundaryErr := deriveCurrentChangesBoundaryCompatibility(ctx, repo, record.State, request, snapshot, resolvedPrePR); boundaryErr == nil {
			compatibility = &proof
			compatibleAdvance = proof.Compatible
		}
	}
	binding := record.State.CurrentSnapshot
	squashedFixDelivery := compactSquashedFixDelivery(request.Gate, record.State, snapshot, resolvedPrePR, receipt.FinalCandidateTree)
	strictBinding := request.Gate == GatePostApply || request.Gate == GatePreCommit || request.Gate == GatePrePush && record.State.InitialSnapshot.Kind != TargetCurrentChanges
	baseRelationshipValid := snapshot.BaseTree == receipt.BaseTree || request.Target.Kind == TargetFixDiff
	if strictBinding {
		baseRelationshipValid = snapshot.BaseTree == binding.BaseTree || squashedFixDelivery
	}
	gateContext := GateContext{
		Gate: request.Gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
		StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
		FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
		LedgerHash: ledgerHash, EvidenceHash: record.State.EvidenceHash,
		BaseRelationshipValid: baseRelationshipValid, BaseAdvance: compatibility,
	}
	if request.Gate == GatePrePR && resolvedPrePR != nil {
		boundary := resolvedPrePR.Selection
		gateContext.PrePRBoundary = &boundary
	}
	pathsMismatch := pathsAreSubset(snapshot.Paths, record.State.GenesisPaths) != nil && !compatibleAdvance
	if strictBinding {
		pathsMismatch = snapshot.PathsDigest != binding.PathsDigest && !squashedFixDelivery
	}
	baseMismatch := snapshot.BaseTree != receipt.BaseTree && request.Target.Kind != TargetFixDiff && !compatibleAdvance
	if strictBinding {
		baseMismatch = snapshot.BaseTree != binding.BaseTree && !squashedFixDelivery
	}
	recoveryBinding, recoveryBound, recoveryErr := deriveCompactRecoveryBinding(ctx, repo, record.State)
	if recoveryErr != nil {
		return invalid("compact recovery binding cannot be derived during authorization")
	}
	// A scope_changed recovery successor freezes only its own pristine scope,
	// so a delivery already covered by its receipt-bound predecessors would be
	// denied here forever. Rebind through the composed recovery chain when the
	// leaf's approved candidate is exactly the delivered tree and native Git
	// evidence proves the chain covers the complete publication range.
	var recoveryRebind *compactRecoveryBinding
	if (request.Gate == GatePrePush || request.Gate == GatePrePR) && record.State.Recovery != nil && resolvedPrePR != nil &&
		(pathsMismatch || baseMismatch) && snapshot.CandidateTree == receipt.FinalCandidateTree {
		rebindBaseCommit := resolvedPrePR.BaseCommit
		if request.Gate == GatePrePush {
			rebindBaseCommit = request.Target.BaseRef
		}
		if chain, ok := rebindCompactRecoveryDelivery(ctx, repo, record.State, snapshot, receipt.FinalCandidateTree, rebindBaseCommit, resolvedPrePR.HeadCommit); ok {
			recoveryRebind = &chain
			pathsMismatch = false
			gateContext.BaseRelationshipValid = true
			gateContext.FixDeltaHash = chain.FixDeltaHash
		}
	}
	if snapshot.CandidateTree != receipt.FinalCandidateTree || pathsMismatch {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "candidate-or-paths-mismatch"}
		diagnostics, diagnosticsErr := buildCompactScopeChangeDiagnostics(ctx, repo, record.State, record.Revision, snapshot)
		if diagnosticsErr != nil {
			gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "scope-diagnostics-unavailable"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "exact scope-change diagnostics cannot be derived: " + diagnosticsErr.Error(), Context: gateContext, Cause: diagnosticsErr}
		}
		gateContext.ScopeChange = &diagnostics
		return NativeGateEvaluation{Result: GateScopeChanged, Reason: nativeGateReason(GateScopeChanged), Context: gateContext}
	}
	if baseMismatch && recoveryRebind == nil {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "base-mismatch"}
		return NativeGateEvaluation{Result: GateInvalidated, Reason: "current repository base no longer matches compact authority", Context: gateContext}
	}
	var release *ReleaseEvidence
	if request.Gate == GateRelease {
		derived, releaseErr := deriveReleaseEvidence(ctx, repo, request.Release, preimages)
		if releaseErr != nil {
			return invalid("release evidence cannot be derived: "+releaseErr.Error(), releaseErr)
		}
		if derived.ReleaseTree != snapshot.CandidateTree {
			return invalid("release evidence does not match the current candidate tree")
		}
		release = &derived
	}
	gateContext.Release = release
	if !authorityLockHeld {
		lock, lockErr := acquireStoreLock(store.lockPath)
		if lockErr != nil {
			return invalid("compact authority changed during final authorization")
		}
		defer lock.release()
	}
	finalGateAuthorizationHook()
	finalRecord, loadErr := store.Load()
	finalSnapshot, finalRefs, snapshotErr := buildCompactLifecycleSnapshot(ctx, repo, request)
	finalUntrackedErr := validateCompactUntrackedScope(ctx, repo, record.State, request)
	finalTrackedErr := validateCompactCommittedTrackedScope(ctx, repo, request)
	_, graphErr := CompactAuthorityLeaves(ctx, repo)
	finalSuperseded, supersededErr := CompactLineageSuperseded(ctx, repo, receipt.LineageID)
	if loadErr != nil || snapshotErr != nil || finalUntrackedErr != nil || finalTrackedErr != nil || graphErr != nil || supersededErr != nil || finalSuperseded || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalSnapshot, snapshot) || !sameResolvedPrePRRefs(finalRefs, resolvedPrePR) {
		cause := errors.Join(loadErr, snapshotErr, finalUntrackedErr, finalTrackedErr, graphErr, supersededErr)
		if cause == nil {
			cause = ErrConcurrentUpdate
		}
		return invalid("compact authority or repository target changed during final authorization", cause)
	}
	finalCompactGateAllowHook()
	if recoveryBound {
		finalChain, ok, chainErr := deriveCompactRecoveryBinding(ctx, repo, finalRecord.State)
		if chainErr != nil || !ok || !reflect.DeepEqual(finalChain, recoveryBinding) {
			if chainErr == nil {
				chainErr = ErrConcurrentUpdate
			}
			return invalid("compact authority or repository target changed during final authorization", chainErr)
		}
		if recoveryRebind == nil && finalRefs != nil && (request.Gate == GatePrePush || request.Gate == GatePrePR) {
			baseCommit := finalRefs.BaseCommit
			if request.Gate == GatePrePush {
				baseCommit = request.Target.BaseRef
			}
			leaf := finalChain.Members[len(finalChain.Members)-1]
			direct := compactRecoveryBinding{Members: []CompactRecord{leaf}, BaseTree: leaf.State.InitialSnapshot.BaseTree}
			if verifyCompactRecoveryDelivery(ctx, repo, direct, receipt.FinalCandidateTree, baseCommit, finalRefs.HeadCommit) != nil {
				return invalid("compact recovery delivery changed during final authorization")
			}
		}
	}
	if request.Gate == GateRelease {
		finalPreimages, preimageErr := readGateArtifactPreimages(request)
		finalRelease, releaseErr := deriveReleaseEvidence(ctx, repo, request.Release, finalPreimages)
		if preimageErr != nil || releaseErr != nil || release == nil || finalRelease != *release {
			cause := errors.Join(preimageErr, releaseErr)
			if cause == nil {
				cause = ErrConcurrentUpdate
			}
			return invalid("release evidence changed during final authorization", cause)
		}
	}
	return NativeGateEvaluation{Result: GateAllow, Reason: nativeGateReason(GateAllow), Context: gateContext}
}

func compactGateInfrastructureFailure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var command *GitCommandError
	var processControl *GitProcessControlError
	return errors.As(err, &command) || errors.As(err, &processControl)
}

func buildCompactScopeChangeDiagnostics(ctx context.Context, repo string, state CompactState, revision string, actual Snapshot) (GateScopeChangeDiagnostics, error) {
	expected := state.CurrentSnapshot
	differing, err := (SnapshotBuilder{Repo: repo}).changedPaths(ctx, expected.CandidateTree, actual.CandidateTree)
	if err != nil {
		return GateScopeChangeDiagnostics{}, err
	}
	if len(differing) == 0 {
		differing = differingPathSet(expected.Paths, actual.Paths)
	}
	required := []string{
		"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition", "reason", "actor",
	}
	return GateScopeChangeDiagnostics{
		Expected: GateTargetEvidence{
			BaseTree: expected.BaseTree, CandidateTree: expected.CandidateTree, PathsDigest: expected.PathsDigest,
			Paths: append([]string{}, expected.Paths...),
		},
		Actual: GateTargetEvidence{
			BaseTree: actual.BaseTree, CandidateTree: actual.CandidateTree, PathsDigest: actual.PathsDigest,
			Paths: append([]string{}, actual.Paths...),
		},
		DifferingPaths: append([]string{}, differing...), DifferingPathCount: len(differing), DifferingPathsDigest: digestPaths(differing),
		PredecessorLineageID: state.LineageID, PredecessorRevision: revision,
		RecoveryOperation: "review.recover", RecoveryRequiredInputs: required,
	}, nil
}

// CompactScopeChangeDiagnostics derives read-only recovery evidence for a
// previously assessed compact gate target. It never authorizes, locks, or
// mutates review authority.
func CompactScopeChangeDiagnostics(ctx context.Context, repo string, state CompactState, revision string, actual Snapshot) (GateScopeChangeDiagnostics, error) {
	return buildCompactScopeChangeDiagnostics(ctx, repo, state, revision, actual)
}

func differingPathSet(left, right []string) []string {
	counts := make(map[string]int, len(left)+len(right))
	for _, logicalPath := range left {
		counts[logicalPath] |= 1
	}
	for _, logicalPath := range right {
		counts[logicalPath] |= 2
	}
	differing := make([]string, 0, len(counts))
	for logicalPath, membership := range counts {
		if membership != 3 {
			differing = append(differing, logicalPath)
		}
	}
	sort.Strings(differing)
	return differing
}

func buildCompactLifecycleSnapshot(ctx context.Context, repo string, request GateRequest) (Snapshot, *resolvedPrePRRefs, error) {
	if request.Gate == GatePreCommit && request.Target.Projection == ProjectionStaged {
		request.Target.IntendedUntracked = []string{}
	}
	if request.Target.Kind == TargetFixDiff || (request.Target.Kind == TargetBaseDiff || request.Target.Kind == TargetBaseWorkspaceOverlay) && (request.Gate == GatePostApply || request.Gate == GatePreCommit) {
		snapshot, err := (SnapshotBuilder{Repo: repo}).build(ctx, request.Target, request.Gate == GatePreCommit)
		return snapshot, nil, err
	}
	return buildLifecycleSnapshot(ctx, repo, request)
}

// buildCompactGateRequest derives the live gate request for the authoritative
// state. A non-nil second result reports the committed next-slice topology —
// the approved candidate tree equals HEAD while new dirty tracked work sits
// on top, so the request names the live current-changes target for
// classification instead of the delivered base-diff target — and carries the
// authoritative intended-untracked paths that remain untracked, computed once
// so callers never repeat the per-path index lookups.
func buildCompactGateRequest(ctx context.Context, repo string, state CompactState, input NativeGateRequestInput) (GateRequest, []string, error) {
	return buildCompactGateRequestWithPushBase(ctx, repo, state, input, compactPushDeliveryBaseTree(state))
}

// compactPushDeliveryBaseTree derives the reviewed delivery base a pre-push
// target must be exactly one commit from. Only current-changes reviews bind a
// one-commit delivery to their own frozen base.
func compactPushDeliveryBaseTree(state CompactState) string {
	return map[TargetKind]string{TargetCurrentChanges: state.InitialSnapshot.BaseTree}[state.InitialSnapshot.Kind]
}

func buildCompactGateRequestWithPushBase(ctx context.Context, repo string, state CompactState, input NativeGateRequestInput, deliveryBaseTree string) (GateRequest, []string, error) {
	request := GateRequest{Schema: GateRequestSchema, Gate: input.Gate, PolicyArtifact: input.PolicyArtifact}
	var nextSliceIntended []string
	switch input.Gate {
	case GatePostApply, GatePreCommit:
		intended := input.IntendedUntracked
		if intended == nil {
			intended = append([]string(nil), state.CurrentSnapshot.IntendedUntracked...)
		}
		if intended == nil {
			intended = []string{}
		}
		current := state.CurrentSnapshot
		projection := current.Projection
		if input.Gate == GatePreCommit {
			projection = ProjectionStaged
		}
		if current.Kind == TargetFixDiff {
			request.Target = Target{
				Kind: TargetFixDiff, Projection: projection, BaseRef: current.BaseTree,
				IntendedUntracked: intended, LedgerIDs: append([]string(nil), current.LedgerIDs...),
			}
			break
		}
		if current.Kind == TargetBaseWorkspaceOverlay {
			request.Target = Target{Kind: TargetBaseWorkspaceOverlay, Projection: projection, BaseRef: current.BaseTree, IntendedUntracked: intended}
			break
		}
		headTree, _, err := (SnapshotBuilder{Repo: repo}).resolveCurrentChangesBase(ctx, projection)
		if err != nil {
			return GateRequest{}, nil, err
		}
		if headTree == current.CandidateTree {
			dirty, err := (SnapshotBuilder{Repo: repo}).HasDirtyTrackedChanges(ctx)
			if err != nil {
				return GateRequest{}, nil, err
			}
			if !dirty {
				request.Target = Target{Kind: TargetBaseDiff, Projection: projection, BaseRef: current.BaseTree, IntendedUntracked: intended}
				break
			}
			// The approved target is committed exactly as reviewed and new
			// dirty tracked work sits on top: the next-slice topology. Route
			// to the live current-changes target so assessment compares
			// candidate and path scope and classifies the new work as
			// scope-changed or unrelated instead of failing input derivation.
			// A dirty worktree can never reproduce the approved candidate
			// tree, so this path can never re-authorize the receipt. Frozen
			// intended paths delivered by the approved commit are tracked now
			// and cannot join a live current-changes target; a caller-supplied
			// set that differs from the authoritative one is kept verbatim so
			// the intended-retention guard rejects it.
			nextSliceIntended, err = compactStillUntrackedIntended(ctx, repo, current.IntendedUntracked)
			if err != nil {
				return GateRequest{}, nil, err
			}
			if equalStrings(intended, current.IntendedUntracked) {
				intended = nextSliceIntended
			}
		}
		request.Target = Target{Kind: TargetCurrentChanges, Projection: projection, IntendedUntracked: intended}
	case GatePrePush:
		target, push, err := buildPushTarget(ctx, repo, input.BaseRef, deliveryBaseTree, state.InitialSnapshot.BaseTree)
		if err != nil {
			return GateRequest{}, nil, err
		}
		request.Target, request.Push = target, push
	case GatePrePR:
		target, prePR, err := buildPrePRTarget(ctx, repo, input.BaseRef, input.PrePRCIAttestation, state.InitialSnapshot.IntendedUntracked)
		if err != nil {
			return GateRequest{}, nil, err
		}
		request.Target, request.PrePR = target, prePR
	case GateRelease:
		head, err := resolveCommit(ctx, repo, "HEAD")
		if err != nil {
			return GateRequest{}, nil, err
		}
		request.Target = Target{Kind: TargetExactRevision, Revision: head}
		request.Release = &ReleaseRequest{
			Revision: head, ConfigurationArtifact: input.ReleaseConfiguration,
			GeneratedArtifact: input.ReleaseGenerated, ProvenanceArtifact: input.ReleaseProvenance,
			PublicationBoundaryArtifact: input.ReleasePublicationBoundary,
			EvidenceFreshnessArtifact:   input.ReleaseEvidenceFreshness,
			PublicationState:            PublicationStateSealed, EvidenceFreshnessState: EvidenceFreshnessCurrent,
		}
	default:
		return GateRequest{}, nil, fmt.Errorf("unsupported review gate %q", input.Gate)
	}
	if request.Gate == GateRelease {
		for _, path := range []string{input.ReleaseConfiguration, input.ReleaseGenerated, input.ReleaseProvenance, input.ReleasePublicationBoundary, input.ReleaseEvidenceFreshness} {
			if strings.TrimSpace(path) == "" {
				return GateRequest{}, nil, errors.New("release gate requires complete independent release evidence")
			}
			if _, err := os.Stat(path); err != nil {
				return GateRequest{}, nil, err
			}
		}
	}
	return request, nextSliceIntended, nil
}

// compactStillUntrackedIntended keeps only the intended-untracked paths that
// remain untracked. After an approved candidate is committed exactly as
// reviewed, its frozen intended paths are tracked in HEAD and can no longer
// participate in the live current-changes target of the next work unit. Only
// the expected not-in-index lookup exit reclassifies a path as untracked;
// git infrastructure failures (timeouts, process control, unexpected exits)
// propagate so the gate fails instead of misclassifying scope.
func compactStillUntrackedIntended(ctx context.Context, repo string, intended []string) ([]string, error) {
	remaining := []string{}
	for _, logicalPath := range intended {
		_, err := runGit(ctx, repo, nil, nil, "ls-files", "--error-unmatch", "--", literalPathspec(logicalPath))
		if err == nil {
			continue
		}
		var lookup *GitCommandError
		if !errors.As(err, &lookup) || lookup.ExitCode != 1 {
			return nil, fmt.Errorf("classify intended-untracked path %q: %w", logicalPath, err)
		}
		remaining = append(remaining, logicalPath)
	}
	return remaining, nil
}

func validateCompactUntrackedScope(ctx context.Context, repo string, state CompactState, request GateRequest) error {
	if request.Target.Projection == ProjectionStaged || request.Gate != GatePostApply && request.Gate != GatePreCommit {
		return nil
	}
	live, err := (SnapshotBuilder{Repo: repo}).DiscoverIntendedUntracked(ctx)
	if err != nil {
		return fmt.Errorf("discover current untracked paths: %w", err)
	}
	allowed := make(map[string]struct{}, len(state.CurrentSnapshot.IntendedUntracked))
	for _, path := range state.CurrentSnapshot.IntendedUntracked {
		allowed[path] = struct{}{}
	}
	for _, path := range live {
		if _, ok := allowed[path]; ok || isPostReviewLifecycleArtifact(path) {
			continue
		}
		if isChangeLocalReceiptMirror(path) && matchesAuthoritativeReceipt(repo, state, path) {
			continue
		}
		return errors.New("current repository contains untracked paths outside the authoritative review scope")
	}
	return nil
}

func validateCompactCommittedTrackedScope(ctx context.Context, repo string, request GateRequest) error {
	if request.Target.Kind != TargetBaseDiff || request.Gate != GatePostApply && request.Gate != GatePreCommit {
		return nil
	}
	dirty, err := (SnapshotBuilder{Repo: repo}).HasDirtyTrackedChanges(ctx)
	if err != nil || !dirty {
		return err
	}
	return errors.New("committed approved target has dirty tracked changes")
}

// validateReviewedPublicationRange enforces the governing publication
// invariant: nothing newly published may exceed what review authorized. The
// receipt's authority is the reviewed base→candidate delta plus the immutable
// base tree it binds, so every path touched anywhere in the delivered range
// base..head must stay inside the immutable genesis scope. For an empty-remote
// bootstrap the range base is the resolved reviewed delivery base commit —
// never the zero-OID sentinel — and the pre-base ancestry published in full by
// the first push must additionally satisfy the reviewed base tree disclosure
// invariant.
func validateReviewedPublicationRange(ctx context.Context, repo string, genesis []string, refs *resolvedPrePRRefs) error {
	if refs.Selection.Source == PrePRBoundaryEmptyRemoteBootstrap {
		if err := validateBootstrapAncestryDisclosure(ctx, repo, refs.BaseCommit); err != nil {
			return err
		}
	}
	output, err := runGit(ctx, repo, nil, nil, "log", "-m", "--format=", "--name-only", "-z", "--no-renames", refs.BaseCommit+".."+refs.HeadCommit)
	if err != nil {
		return fmt.Errorf("inspect complete publication range: %w", err)
	}
	paths, err := canonicalPaths(splitNullSeparatedPaths(output))
	if err == nil {
		err = pathsAreSubset(paths, genesis)
	}
	if err != nil {
		return fmt.Errorf("publication range exceeds immutable genesis scope: %w", err)
	}
	return nil
}

func isPostReviewLifecycleArtifact(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 4 && parts[0] == "openspec" && parts[1] == "changes" && parts[3] == "verify-report.md"
}

func isChangeLocalReceiptMirror(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 5 && parts[0] == "openspec" && parts[1] == "changes" && parts[3] == "reviews" && parts[4] == "receipt.json"
}

// matchesAuthoritativeReceipt exempts only a change-local mirror whose content
// equals the receipt derived from the terminal authority; anything else stays
// outside the authoritative review scope and fails closed.
func matchesAuthoritativeReceipt(repo string, state CompactState, path string) bool {
	authoritative, err := state.Receipt()
	if err != nil {
		return false
	}
	payload, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
	if err != nil {
		return false
	}
	mirror, err := ParseCompactReceipt(payload)
	if err != nil {
		return false
	}
	return CompactReceiptEqual(mirror, authoritative)
}
