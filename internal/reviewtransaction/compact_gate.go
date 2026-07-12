package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

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
	authoritative, err := record.State.Receipt()
	if err != nil || !compactReceiptEqual(authoritative, receipt) {
		return invalid("compact receipt does not match current authority")
	}
	if receipt.TerminalState == TerminalEscalated {
		return NativeGateEvaluation{Result: GateEscalated, Reason: nativeGateReason(GateEscalated)}
	}
	request, err := buildCompactGateRequest(ctx, repo, record.State, input)
	if err != nil {
		return invalid("compact gate inputs cannot be derived: " + err.Error())
	}
	if (request.Gate == GatePostApply || request.Gate == GatePreCommit) && !equalStrings(request.Target.IntendedUntracked, record.State.CurrentSnapshot.IntendedUntracked) {
		return invalid("current repository target does not retain the authoritative intended-untracked paths")
	}
	preimages, err := readGateArtifactPreimages(request)
	if err != nil {
		return invalid("compact gate evidence cannot be read: " + err.Error())
	}
	if len(preimages.policy) > 0 && hashArtifactPayload(preimages.policy) != record.State.PolicyHash {
		return invalid("explicit policy does not match compact authority")
	}
	snapshot, resolvedPrePR, err := buildLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		return invalid("current repository target cannot be derived: " + err.Error())
	}
	if request.Gate == GatePrePush && record.State.InitialSnapshot.Kind == TargetCurrentChanges && snapshot.BaseTree == snapshot.CandidateTree {
		return invalid("pre-push current-changes receipt requires a delivered tree change")
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
	if (snapshot.CandidateTree != receipt.FinalCandidateTree || snapshot.PathsDigest != receipt.PathsDigest) && !compatibleAdvance {
		return NativeGateEvaluation{Result: GateScopeChanged, Reason: nativeGateReason(GateScopeChanged)}
	}
	if snapshot.BaseTree != receipt.BaseTree && !compatibleAdvance {
		return invalid("current repository base no longer matches compact authority")
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
	gateContext := GateContext{
		Gate: request.Gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
		StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
		FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
		LedgerHash: EmptyFixDeltaHash, EvidenceHash: record.State.EvidenceHash,
		BaseRelationshipValid: snapshot.BaseTree == receipt.BaseTree, BaseAdvance: compatibility, Release: release,
	}
	finalGateAuthorizationHook()
	finalRecord, loadErr := store.Load()
	finalSnapshot, finalRefs, snapshotErr := buildLifecycleSnapshot(ctx, repo, request)
	if loadErr != nil || snapshotErr != nil || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalSnapshot, snapshot) || !sameResolvedPrePRRefs(finalRefs, resolvedPrePR) {
		return invalid("compact authority or repository target changed during final authorization")
	}
	if request.Gate == GateRelease {
		finalPreimages, preimageErr := readGateArtifactPreimages(request)
		finalRelease, releaseErr := deriveReleaseEvidence(ctx, repo, request.Release, finalPreimages)
		if preimageErr != nil || releaseErr != nil || release == nil || finalRelease != *release {
			return invalid("release evidence changed during final authorization")
		}
	}
	return NativeGateEvaluation{Result: GateAllow, Reason: nativeGateReason(GateAllow), Context: gateContext}
}

func buildCompactGateRequest(ctx context.Context, repo string, state CompactState, input NativeGateRequestInput) (GateRequest, error) {
	request := GateRequest{Schema: GateRequestSchema, Gate: input.Gate, PolicyArtifact: input.PolicyArtifact}
	switch input.Gate {
	case GatePostApply, GatePreCommit:
		intended := input.IntendedUntracked
		if intended == nil {
			intended = append([]string(nil), state.CurrentSnapshot.IntendedUntracked...)
		}
		request.Target = Target{Kind: TargetCurrentChanges, IntendedUntracked: intended}
	case GatePrePush:
		head, err := resolveCommit(ctx, repo, "HEAD")
		if err != nil {
			return GateRequest{}, err
		}
		request.Target = Target{Kind: TargetExactRevision, Revision: head}
	case GatePrePR:
		_, _, baseCommit, err := resolveAuthoritativePublicationBase(ctx, repo)
		if err != nil {
			return GateRequest{}, err
		}
		if strings.TrimSpace(input.BaseRef) != "" {
			expected, expectedErr := resolveCommit(ctx, repo, input.BaseRef)
			if expectedErr != nil || expected != baseCommit {
				return GateRequest{}, errors.New("compact pre-PR base does not match the remote publication boundary")
			}
		}
		request.Target = Target{
			Kind: TargetBaseDiff, BaseRef: baseCommit,
			IntendedUntracked: append([]string(nil), state.InitialSnapshot.IntendedUntracked...),
		}
		if strings.TrimSpace(input.PrePRCIAttestation) != "" {
			request.PrePR = &PrePRRequest{CIAttestationArtifact: input.PrePRCIAttestation}
		}
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
