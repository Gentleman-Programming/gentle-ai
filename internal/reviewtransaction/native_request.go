package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// NativeGateRequestInput contains artifact locations and lifecycle inputs only.
// Repository, store, bundle, tree, digest, and chain identities are derived.
type NativeGateRequestInput struct {
	Gate                       GateKind
	LineageID                  string
	BundleArtifact             string
	PolicyArtifact             string
	LedgerArtifact             string
	FixDeltaArtifact           string
	EvidenceArtifact           string
	IntendedUntracked          []string
	BaseRef                    string
	PrePRCIAttestation         string
	ReleaseConfiguration       string
	ReleaseGenerated           string
	ReleaseProvenance          string
	ReleasePublicationBoundary string
	ReleaseEvidenceFreshness   string
}

func BuildNativeGateRequest(ctx context.Context, repo string, input NativeGateRequestInput) (GateRequest, error) {
	if strings.TrimSpace(input.LineageID) == "" {
		return GateRequest{}, errors.New("native gate request requires lineage")
	}
	store, err := AuthoritativeStore(ctx, repo, input.LineageID)
	if err != nil {
		return GateRequest{}, fmt.Errorf("derive authoritative review store: %w", err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		return GateRequest{}, fmt.Errorf("load authoritative review chain: %w", err)
	}
	transaction := chain.Records[len(chain.Records)-1].Transaction
	bundleDigest := chain.Identity
	if strings.TrimSpace(input.BundleArtifact) != "" {
		authoritative, err := store.ExportBundle()
		if err != nil {
			return GateRequest{}, fmt.Errorf("export authoritative review bundle: %w", err)
		}
		payload, err := os.ReadFile(input.BundleArtifact)
		if err != nil {
			return GateRequest{}, fmt.Errorf("read review bundle artifact: %w", err)
		}
		named, err := ParseChainBundle(payload)
		if err != nil {
			return GateRequest{}, fmt.Errorf("parse review bundle artifact: %w", err)
		}
		if !reflect.DeepEqual(named, authoritative) {
			return GateRequest{}, errors.New("named review bundle does not match the authoritative repository chain")
		}
		bundleDigest = authoritative.BundleDigest
	}
	request := GateRequest{
		Schema: GateRequestSchema, Gate: input.Gate,
		StoreRevision: chain.HeadRevision, GenesisRevision: chain.GenesisRevision,
		ChainIdentity: chain.Identity, BundleDigest: bundleDigest,
		PolicyArtifact: input.PolicyArtifact, LedgerArtifact: input.LedgerArtifact,
		FixDeltaArtifact: input.FixDeltaArtifact, EvidenceArtifact: input.EvidenceArtifact,
	}
	switch input.Gate {
	case GatePostApply, GatePreCommit:
		intended := input.IntendedUntracked
		if intended == nil {
			intended = append([]string(nil), transaction.Snapshot.IntendedUntracked...)
			if intended == nil {
				intended = []string{}
			}
		}
		request.Target = Target{Kind: TargetCurrentChanges, IntendedUntracked: intended}
	case GatePrePush:
		deliveryBaseTree := map[TargetKind]string{TargetCurrentChanges: transaction.Snapshot.BaseTree}[transaction.Snapshot.Kind]
		target, push, err := buildPushTarget(ctx, repo, input.BaseRef, deliveryBaseTree, transaction.Snapshot.BaseTree)
		if err != nil {
			return GateRequest{}, err
		}
		request.Target, request.Push = target, push
	case GatePrePR:
		target, prePR, err := buildPrePRTarget(ctx, repo, input.BaseRef, input.PrePRCIAttestation, nil)
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
			PublicationState:            PublicationStateSealed,
			EvidenceFreshnessState:      EvidenceFreshnessCurrent,
		}
	default:
		return GateRequest{}, fmt.Errorf("unsupported review gate %q", input.Gate)
	}
	preimages, err := readGateArtifactPreimages(request)
	if err != nil {
		return GateRequest{}, err
	}
	if strings.TrimSpace(input.FixDeltaArtifact) != "" {
		transaction := chain.Records[len(chain.Records)-1].Transaction
		if _, err := validateFixDeltaArtifact(preimages.fixDelta, transaction); err != nil {
			return GateRequest{}, err
		}
		request.FixDeltaContent = string(preimages.fixDelta)
	}
	request.preimages = &preimages
	if err := validateGateRequest(request); err != nil {
		return GateRequest{}, err
	}
	return request, nil
}
