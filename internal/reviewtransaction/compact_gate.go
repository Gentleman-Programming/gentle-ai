package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

var finalCompactGateAllowHook = func() {}

func EvaluateCompactGate(ctx context.Context, repo string, receipt CompactReceipt, input NativeGateRequestInput) NativeGateEvaluation {
	invalid := func(reason string) NativeGateEvaluation {
		return NativeGateEvaluation{Result: GateInvalidated, Reason: reason}
	}
	if err := receipt.Validate(); err != nil {
		return invalid("compact review receipt is invalid: " + err.Error())
	}
	if strings.TrimSpace(input.LineageID) != "" && input.LineageID != receipt.LineageID {
		return invalid("compact gate lineage does not match the receipt")
	}
	store, err := CompactAuthoritativeStore(ctx, repo, receipt.LineageID)
	if err != nil {
		return invalid("compact review store cannot be derived: " + err.Error())
	}
	record, err := store.Load()
	if err != nil {
		return invalid("compact review authority cannot be loaded: " + err.Error())
	}
	if _, err := CompactAuthorityLeaves(ctx, repo); err != nil {
		return invalid(err.Error())
	}
	superseded, err := CompactLineageSuperseded(ctx, repo, receipt.LineageID)
	if err != nil {
		return invalid(err.Error())
	}
	if superseded {
		return invalid("compact receipt belongs to superseded historical authority")
	}
	authoritative, err := record.State.Receipt()
	if err != nil || !compactReceiptEqual(authoritative, receipt) {
		return invalid("compact receipt does not match current authority")
	}
	denialContext := GateContext{
		Gate: input.Gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
		StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
		BaseTree: receipt.BaseTree, CandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
		FixDeltaHash: receipt.FixDeltaHash, PolicyHash: receipt.PolicyHash, LedgerHash: EmptyFixDeltaHash, EvidenceHash: receipt.EvidenceHash,
	}
	if input.Gate == GatePrePR && strings.TrimSpace(input.BaseRef) != "" {
		denialContext.PrePRBoundary = &PrePRBoundarySelection{Source: PrePRBoundaryExplicit, Selector: strings.TrimSpace(input.BaseRef)}
	}
	if receipt.TerminalState == TerminalEscalated {
		return NativeGateEvaluation{Result: GateEscalated, Reason: nativeGateReason(GateEscalated)}
	}
	request, err := buildCompactGateRequest(ctx, repo, record.State, input)
	if err != nil {
		if input.Gate == GatePrePR {
			denialContext.Denial = &GateDenial{Stage: "boundary-selection", Code: "unavailable"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "compact gate inputs cannot be derived: " + err.Error(), Context: denialContext}
		}
		return invalid("compact gate inputs cannot be derived: " + err.Error())
	}
	if (request.Gate == GatePostApply || request.Gate == GatePreCommit) && !equalStrings(request.Target.IntendedUntracked, record.State.CurrentSnapshot.IntendedUntracked) {
		return invalid("current repository target does not retain the authoritative intended-untracked paths")
	}
	if err := validateCompactUntrackedScope(ctx, repo, record.State, request); err != nil {
		return invalid(err.Error())
	}
	preimages, err := readGateArtifactPreimages(request)
	if err != nil {
		return invalid("compact gate evidence cannot be read: " + err.Error())
	}
	if len(preimages.policy) > 0 && hashArtifactPayload(preimages.policy) != record.State.PolicyHash {
		return invalid("explicit policy does not match compact authority")
	}
	snapshot, resolvedPrePR, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil {
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
	if request.Gate == GatePrePush && record.State.InitialSnapshot.Kind == TargetBaseDiff {
		if err := validateCompactPublicationRange(ctx, repo, record.State.GenesisPaths, resolvedPrePR); err != nil {
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
		}
	}
	binding := record.State.CurrentSnapshot
	strictBinding := request.Gate == GatePostApply || request.Gate == GatePreCommit || request.Gate == GatePrePush && record.State.InitialSnapshot.Kind != TargetCurrentChanges
	baseRelationshipValid := snapshot.BaseTree == receipt.BaseTree || request.Target.Kind == TargetFixDiff
	if strictBinding {
		baseRelationshipValid = snapshot.BaseTree == binding.BaseTree
	}
	gateContext := GateContext{
		Gate: request.Gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
		StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
		FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
		LedgerHash: EmptyFixDeltaHash, EvidenceHash: record.State.EvidenceHash,
		BaseRelationshipValid: baseRelationshipValid, BaseAdvance: compatibility,
	}
	if request.Gate == GatePrePR && resolvedPrePR != nil {
		boundary := resolvedPrePR.Selection
		gateContext.PrePRBoundary = &boundary
	}
	pathsMismatch := pathsAreSubset(snapshot.Paths, record.State.GenesisPaths) != nil && !compatibleAdvance
	if strictBinding {
		pathsMismatch = snapshot.PathsDigest != binding.PathsDigest
	}
	if snapshot.CandidateTree != receipt.FinalCandidateTree || pathsMismatch {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "candidate-or-paths-mismatch"}
		return NativeGateEvaluation{Result: GateScopeChanged, Reason: nativeGateReason(GateScopeChanged), Context: gateContext}
	}
	baseMismatch := snapshot.BaseTree != receipt.BaseTree && request.Target.Kind != TargetFixDiff && !compatibleAdvance
	if strictBinding {
		baseMismatch = snapshot.BaseTree != binding.BaseTree
	}
	if baseMismatch {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "base-mismatch"}
		return NativeGateEvaluation{Result: GateInvalidated, Reason: "current repository base no longer matches compact authority", Context: gateContext}
	}
	var release *ReleaseEvidence
	if request.Gate == GateRelease {
		derived, releaseErr := deriveReleaseEvidence(ctx, repo, request.Release, preimages)
		if releaseErr != nil {
			return invalid("release evidence cannot be derived: " + releaseErr.Error())
		}
		if derived.ReleaseTree != snapshot.CandidateTree {
			return invalid("release evidence does not match the current candidate tree")
		}
		release = &derived
	}
	gateContext.Release = release
	lock, lockErr := acquireStoreLock(store.lockPath)
	if lockErr != nil {
		return invalid("compact authority changed during final authorization")
	}
	defer lock.release()
	finalGateAuthorizationHook()
	finalRecord, loadErr := store.Load()
	finalSnapshot, finalRefs, snapshotErr := buildCompactLifecycleSnapshot(ctx, repo, request)
	finalUntrackedErr := validateCompactUntrackedScope(ctx, repo, record.State, request)
	finalTrackedErr := validateCompactCommittedTrackedScope(ctx, repo, request)
	_, graphErr := CompactAuthorityLeaves(ctx, repo)
	finalSuperseded, supersededErr := CompactLineageSuperseded(ctx, repo, receipt.LineageID)
	if loadErr != nil || snapshotErr != nil || finalUntrackedErr != nil || finalTrackedErr != nil || graphErr != nil || supersededErr != nil || finalSuperseded || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalSnapshot, snapshot) || !sameResolvedPrePRRefs(finalRefs, resolvedPrePR) {
		return invalid("compact authority or repository target changed during final authorization")
	}
	if request.Gate == GateRelease {
		finalPreimages, preimageErr := readGateArtifactPreimages(request)
		finalRelease, releaseErr := deriveReleaseEvidence(ctx, repo, request.Release, finalPreimages)
		if preimageErr != nil || releaseErr != nil || release == nil || finalRelease != *release {
			return invalid("release evidence changed during final authorization")
		}
	}
	finalCompactGateAllowHook()
	return NativeGateEvaluation{Result: GateAllow, Reason: nativeGateReason(GateAllow), Context: gateContext}
}

func buildCompactLifecycleSnapshot(ctx context.Context, repo string, request GateRequest) (Snapshot, *resolvedPrePRRefs, error) {
	if request.Gate == GatePreCommit && request.Target.Projection == ProjectionStaged {
		request.Target.IntendedUntracked = []string{}
	}
	if request.Target.Kind == TargetFixDiff || request.Target.Kind == TargetBaseDiff && (request.Gate == GatePostApply || request.Gate == GatePreCommit) {
		snapshot, err := (SnapshotBuilder{Repo: repo}).build(ctx, request.Target, request.Gate == GatePreCommit)
		return snapshot, nil, err
	}
	return buildLifecycleSnapshot(ctx, repo, request)
}

func buildCompactGateRequest(ctx context.Context, repo string, state CompactState, input NativeGateRequestInput) (GateRequest, error) {
	request := GateRequest{Schema: GateRequestSchema, Gate: input.Gate, PolicyArtifact: input.PolicyArtifact}
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
		headTree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, "HEAD")
		if err != nil {
			return GateRequest{}, err
		}
		if headTree == current.CandidateTree {
			dirty, err := (SnapshotBuilder{Repo: repo}).HasDirtyTrackedChanges(ctx)
			if err != nil {
				return GateRequest{}, err
			}
			if dirty {
				return GateRequest{}, errors.New("committed approved target has dirty tracked changes")
			}
			request.Target = Target{Kind: TargetBaseDiff, Projection: projection, BaseRef: current.BaseTree, IntendedUntracked: intended}
			break
		}
		request.Target = Target{Kind: TargetCurrentChanges, Projection: projection, IntendedUntracked: intended}
	case GatePrePush:
		deliveryBaseTree := map[TargetKind]string{TargetCurrentChanges: state.InitialSnapshot.BaseTree}[state.InitialSnapshot.Kind]
		target, push, err := buildPushTarget(ctx, repo, input.BaseRef, deliveryBaseTree)
		if err != nil {
			return GateRequest{}, err
		}
		request.Target, request.Push = target, push
	case GatePrePR:
		target, prePR, err := buildPrePRTarget(ctx, repo, input.BaseRef, input.PrePRCIAttestation, state.InitialSnapshot.IntendedUntracked)
		if err != nil {
			return GateRequest{}, err
		}
		request.Target, request.PrePR = target, prePR
	case GateRelease:
		head, err := resolveCommit(ctx, repo, "HEAD")
		if err != nil {
			return GateRequest{}, err
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
		return GateRequest{}, fmt.Errorf("unsupported review gate %q", input.Gate)
	}
	if request.Gate == GateRelease {
		for _, path := range []string{input.ReleaseConfiguration, input.ReleaseGenerated, input.ReleaseProvenance, input.ReleasePublicationBoundary, input.ReleaseEvidenceFreshness} {
			if strings.TrimSpace(path) == "" {
				return GateRequest{}, errors.New("release gate requires complete independent release evidence")
			}
			if _, err := os.Stat(path); err != nil {
				return GateRequest{}, err
			}
		}
	}
	return request, nil
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

func validateCompactPublicationRange(ctx context.Context, repo string, genesis []string, refs *resolvedPrePRRefs) error {
	output, err := runGit(ctx, repo, nil, nil, "log", "--format=", "--name-only", "-z", "--no-renames", refs.BaseCommit+".."+refs.HeadCommit)
	if err != nil {
		return fmt.Errorf("inspect complete publication range: %w", err)
	}
	paths := []string{}
	for _, path := range strings.Split(string(output), "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	paths, err = canonicalPaths(paths)
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
