package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const GateRequestSchema = "gentle-ai.review-gate-request/v1"

type GateRequest struct {
	Schema           string                      `json:"schema"`
	Gate             GateKind                    `json:"gate"`
	Target           Target                      `json:"target"`
	StoreDir         string                      `json:"store_dir,omitempty"`
	StoreRevision    string                      `json:"store_revision"`
	GenesisRevision  string                      `json:"genesis_revision"`
	ChainIdentity    string                      `json:"chain_identity"`
	BundleDigest     string                      `json:"bundle_digest"`
	PolicyArtifact   string                      `json:"policy_artifact"`
	PolicyContent    string                      `json:"policy_content,omitempty"`
	LedgerArtifact   string                      `json:"ledger_artifact"`
	LedgerContent    string                      `json:"ledger_content,omitempty"`
	FixDeltaArtifact string                      `json:"fix_delta_artifact,omitempty"`
	FixDeltaContent  string                      `json:"fix_delta_content,omitempty"`
	EvidenceArtifact string                      `json:"evidence_artifact"`
	EvidenceContent  string                      `json:"evidence_content,omitempty"`
	ExternalEvidence ExternalEvidenceDisposition `json:"external_evidence,omitempty"`
	Push             *PushRequest                `json:"push,omitempty"`
	PrePR            *PrePRRequest               `json:"pre_pr,omitempty"`
	Release          *ReleaseRequest             `json:"release,omitempty"`
	preimages        *gateArtifactPreimages
}

type PrePRRequest struct {
	CIAttestationArtifact string                  `json:"ci_attestation_artifact"`
	Boundary              *PrePRBoundarySelection `json:"boundary,omitempty"`
	PushRemote            string                  `json:"push_remote,omitempty"`
	PushRemoteIdentity    string                  `json:"push_remote_identity,omitempty"`
}

type PushRequest struct {
	Boundary           PrePRBoundarySelection `json:"boundary"`
	MergeBase          string                 `json:"merge_base"`
	PushRemote         string                 `json:"push_remote"`
	PushRemoteIdentity string                 `json:"push_remote_identity"`
	DeliveryBaseTree   string                 `json:"delivery_base_tree,omitempty"`
	// ReviewedBaseTree is set only for empty-remote bootstrap boundaries and
	// carries the authoritative reviewed base tree so re-derivation can locate
	// the same delivery base without a remote publication boundary.
	ReviewedBaseTree string `json:"reviewed_base_tree,omitempty"`
}

type PrePRBoundarySource string

const (
	PrePRBoundaryExplicit           PrePRBoundarySource = "explicit"
	PrePRBoundaryPublicationDefault PrePRBoundarySource = "publication-default"
	// PrePRBoundaryEmptyRemoteBootstrap marks a pre-push first-publication
	// boundary: the configured upstream remote advertises no refs at all, so
	// the publication base is the explicit zero OID instead of an advertised
	// remote commit.
	PrePRBoundaryEmptyRemoteBootstrap PrePRBoundarySource = "empty-remote-bootstrap"
)

// PrePRBoundarySelection records how the immutable pre-PR range boundary was
// selected. It is evidence only; receipt binding still authorizes the range.
type PrePRBoundarySelection struct {
	Source         PrePRBoundarySource `json:"source"`
	Selector       string              `json:"selector"`
	Commit         string              `json:"commit"`
	Remote         string              `json:"remote,omitempty"`
	RemoteRef      string              `json:"remote_ref,omitempty"`
	RemoteIdentity string              `json:"remote_identity,omitempty"`
}

type ReleaseRequest struct {
	Revision                    string                 `json:"revision"`
	ConfigurationArtifact       string                 `json:"configuration_artifact"`
	ConfigurationContent        string                 `json:"configuration_content,omitempty"`
	GeneratedArtifact           string                 `json:"generated_artifact"`
	GeneratedContent            string                 `json:"generated_content,omitempty"`
	ProvenanceArtifact          string                 `json:"provenance_artifact"`
	ProvenanceContent           string                 `json:"provenance_content,omitempty"`
	PublicationBoundaryArtifact string                 `json:"publication_boundary_artifact"`
	PublicationBoundaryContent  string                 `json:"publication_boundary_content,omitempty"`
	PublicationState            PublicationState       `json:"publication_state"`
	EvidenceFreshnessArtifact   string                 `json:"evidence_freshness_artifact"`
	EvidenceFreshnessContent    string                 `json:"evidence_freshness_content,omitempty"`
	EvidenceFreshnessState      EvidenceFreshnessState `json:"evidence_freshness_state"`
}

type gateArtifactPreimages struct {
	policy, ledger, fixDelta, evidence                    []byte
	configuration, generated, provenance                  []byte
	publicationBoundary, evidenceFreshness, ciAttestation []byte
}

type NativeGateEvaluation struct {
	Result  GateResult
	Reason  string
	Context GateContext
	Cause   error `json:"-"`
}

var finalGateAuthorizationHook = func() {}
var artifactPreimagesReadHook = func() {}

func ParseGateRequest(payload []byte) (GateRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request GateRequest
	if err := decoder.Decode(&request); err != nil {
		return GateRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return GateRequest{}, errors.New("multiple JSON values in review gate request")
	}
	if err := validateGateRequest(request); err != nil {
		return GateRequest{}, err
	}
	return request, nil
}

func HashArtifact(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("artifact path is required")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func EvaluateNativeGate(ctx context.Context, repo string, receipt Receipt, request GateRequest) NativeGateEvaluation {
	invalid := func(reason string) NativeGateEvaluation {
		return NativeGateEvaluation{Result: GateInvalidated, Reason: reason}
	}
	if err := validateReceiptStructure(receipt); err != nil {
		return invalid("review receipt is invalid: " + err.Error())
	}
	if err := validateGateRequest(request); err != nil {
		return invalid("review gate request is invalid: " + err.Error())
	}
	store, err := AuthoritativeStore(ctx, repo, receipt.LineageID)
	if err != nil {
		return invalid("authoritative review store cannot be derived: " + err.Error())
	}
	chain, err := store.LoadChain()
	if err != nil {
		return invalid("authoritative review transaction cannot be loaded: " + err.Error())
	}
	record := chain.Records[len(chain.Records)-1]
	revision := chain.HeadRevision
	if revision != request.StoreRevision || chain.GenesisRevision != request.GenesisRevision || chain.Identity != request.ChainIdentity {
		return invalid("authoritative review transaction chain identity is stale")
	}
	bundleDigest := request.BundleDigest
	if request.BundleDigest != request.ChainIdentity {
		bundle, err := store.ExportBundle()
		if err != nil {
			return invalid("authoritative review chain bundle identity cannot be derived: " + err.Error())
		}
		if bundle.BundleDigest != request.BundleDigest {
			return invalid("authoritative review transaction chain identity is stale")
		}
		bundleDigest = bundle.BundleDigest
	}
	authoritativeReceipt, err := record.Transaction.Receipt()
	if err != nil {
		return invalid("authoritative review transaction is non-terminal: " + err.Error())
	}
	if !reflect.DeepEqual(authoritativeReceipt, receipt) {
		return invalid("receipt does not match the authoritative transaction revision")
	}
	if request.Gate == GatePostApply || request.Gate == GatePreCommit {
		requestedProjection, requestErr := canonicalProjection(request.Target.Projection)
		authoritativeProjection, authorityErr := canonicalProjection(record.Transaction.Snapshot.Projection)
		if requestErr != nil || authorityErr != nil || requestedProjection != authoritativeProjection {
			return invalid("current repository target projection does not match the authoritative transaction snapshot")
		}
	}
	denialContext := GateContext{
		Gate: request.Gate, LineageID: record.Transaction.LineageID, Generation: record.Transaction.Generation,
		StoreRevision: revision, GenesisRevision: chain.GenesisRevision, ChainIdentity: chain.Identity, BundleDigest: bundleDigest,
		BaseTree: receipt.BaseTree, CandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
		FixDeltaHash: receipt.FixDeltaHash, PolicyHash: receipt.PolicyHash, LedgerHash: receipt.LedgerHash, EvidenceHash: receipt.EvidenceHash,
	}
	if request.Gate == GatePrePR && request.PrePR != nil && request.PrePR.Boundary != nil {
		boundary := *request.PrePR.Boundary
		denialContext.PrePRBoundary = &boundary
	}
	if request.Gate == GatePreCommit && request.Target.Kind != TargetCurrentChanges {
		return invalid("pre-commit target must derive current repository changes")
	}
	if (request.Gate == GatePreCommit || request.Gate == GatePostApply && request.Target.Kind == TargetCurrentChanges) && !equalStrings(request.Target.IntendedUntracked, record.Transaction.Snapshot.IntendedUntracked) {
		return invalid("current repository target does not retain the authoritative intended-untracked paths")
	}
	preimages, err := readGateArtifactPreimages(request)
	if err != nil {
		return invalid("persisted gate artifacts cannot be read: " + err.Error())
	}
	artifactPreimagesReadHook()
	snapshot, resolvedPrePR, err := buildLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		if request.Gate == GatePrePR {
			denialContext.Denial = &GateDenial{Stage: "boundary-selection", Code: "unavailable"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "current repository target cannot be derived: " + err.Error(), Context: denialContext}
		}
		return invalid("current repository target cannot be derived: " + err.Error())
	}
	if request.Gate == GatePrePush && record.Transaction.Snapshot.Kind == TargetCurrentChanges && snapshot.BaseTree == snapshot.CandidateTree {
		return invalid("pre-push current-changes receipt requires a delivered tree change")
	}
	if request.Gate == GatePrePush && (resolvedPrePR == nil || resolvedPrePR.DeliveredCommitCount < 1) {
		return invalid("pre-push validation requires at least one delivered commit")
	}
	if request.Gate == GatePrePush && record.Transaction.Snapshot.Kind == TargetCurrentChanges && resolvedPrePR.DeliveredCommitCount != 1 {
		return invalid("pre-push current-changes receipt requires exactly one delivery commit")
	}
	if request.Gate == GatePrePush && resolvedPrePR.Selection.Source == PrePRBoundaryEmptyRemoteBootstrap {
		// A first publication to an empty remote transfers the candidate's
		// complete history, so it must satisfy the same publication invariant
		// as the compact path: the delivered range stays inside the reviewed
		// paths and pre-base ancestry stays disclosed by the reviewed base
		// tree.
		if err := validateReviewedPublicationRange(ctx, repo, record.Transaction.Snapshot.Paths, resolvedPrePR); err != nil {
			return invalid(err.Error())
		}
	}
	policyHash := authoritativeReceipt.PolicyHash
	if hasArtifactSource(request.PolicyArtifact, request.PolicyContent) {
		policyHash = hashArtifactPayload(preimages.policy)
	}
	ledgerHash := authoritativeReceipt.LedgerHash
	if hasArtifactSource(request.LedgerArtifact, request.LedgerContent) {
		var ledgerFindingsHash string
		ledgerHash, ledgerFindingsHash, err = hashLedgerPayload(preimages.ledger)
		if err != nil {
			return invalid("frozen ledger cannot be validated: " + err.Error())
		}
		if ledgerFindingsHash != record.Transaction.LedgerFindingsHash {
			return invalid("frozen ledger findings do not match the authoritative transaction")
		}
	}
	evidenceHash := authoritativeReceipt.EvidenceHash
	if hasArtifactSource(request.EvidenceArtifact, request.EvidenceContent) {
		evidenceHash = hashArtifactPayload(preimages.evidence)
	}
	fixDeltaHash := record.Transaction.FixDeltaHash
	if record.Transaction.Snapshot.Kind == TargetFixDiff {
		fixDeltaHash = FixDeltaHashForSnapshot(record.Transaction.Snapshot)
	}
	if request.FixDeltaContent != "" {
		fixDeltaHash, err = validateFixDeltaArtifact(preimages.fixDelta, record.Transaction)
		if err != nil {
			return invalid("canonical fix delta artifact cannot be validated: " + err.Error())
		}
	}

	gateContext := GateContext{
		Gate: request.Gate, LineageID: record.Transaction.LineageID, Generation: record.Transaction.Generation,
		StoreRevision: revision, GenesisRevision: chain.GenesisRevision, ChainIdentity: chain.Identity,
		BundleDigest: bundleDigest,
		BaseTree:     snapshot.BaseTree, CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
		FixDeltaHash: fixDeltaHash, PolicyHash: policyHash, LedgerHash: ledgerHash, EvidenceHash: evidenceHash,
		BaseRelationshipValid: snapshot.BaseTree == receipt.BaseTree,
		ExternalEvidence:      request.ExternalEvidence,
	}
	if request.Gate == GatePrePR && resolvedPrePR != nil {
		boundary := resolvedPrePR.Selection
		gateContext.PrePRBoundary = &boundary
	}
	if request.Gate == GatePrePR && snapshot.BaseTree != receipt.BaseTree {
		if compatibility, compatibilityErr := deriveBaseAdvanceCompatibility(ctx, repo, receipt, request, snapshot, resolvedPrePR, preimages); compatibilityErr == nil {
			gateContext.BaseAdvance = &compatibility
		}
	}
	if request.Gate == GateRelease {
		release, err := deriveReleaseEvidence(ctx, repo, request.Release, preimages)
		if err != nil {
			return invalid("release boundary cannot be derived: " + err.Error())
		}
		if release.ReleaseTree != snapshot.CandidateTree {
			return invalid("immutable release tree does not match the current candidate tree")
		}
		gateContext.Release = &release
	}
	result := validateDerivedGate(receipt, gateContext)
	if request.Gate == GatePrePR && result != GateAllow {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: string(result)}
	}
	if result == GateAllow {
		finalGateAuthorizationHook()
		finalSnapshot, finalRefs, err := buildLifecycleSnapshot(ctx, repo, request)
		finalChain, authorityErr := store.LoadChain()
		if err != nil || authorityErr != nil || finalChain.HeadRevision != revision || finalChain.GenesisRevision != chain.GenesisRevision || finalChain.Identity != chain.Identity || !reflect.DeepEqual(finalSnapshot, snapshot) || !sameResolvedPrePRRefs(finalRefs, resolvedPrePR) {
			return invalid("authority or repository target changed during final authorization")
		}
	}
	return NativeGateEvaluation{Result: result, Reason: nativeGateReason(result), Context: gateContext}
}

type resolvedPrePRRefs struct {
	Selection            PrePRBoundarySelection
	HeadCommit           string
	BaseCommit           string
	DeliveredCommitCount int
	PushRemote           string
}

func buildLifecycleSnapshot(ctx context.Context, repo string, request GateRequest) (Snapshot, *resolvedPrePRRefs, error) {
	if request.Gate == GatePrePush {
		target, refs, err := prePushTargetForRequest(ctx, repo, request)
		if err != nil {
			return Snapshot{}, nil, err
		}
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, target)
		return snapshot, refs, err
	}
	target, err := lifecycleTargetForGate(ctx, repo, request)
	if err != nil {
		return Snapshot{}, nil, err
	}
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.build(ctx, target, request.Gate == GatePreCommit)
	if err != nil || request.Gate != GatePrePR {
		return snapshot, nil, err
	}
	selection, err := prePRBoundaryForRequest(ctx, repo, request)
	if err != nil {
		return Snapshot{}, nil, err
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return Snapshot{}, nil, err
	}
	snapshot, err = (SnapshotBuilder{Repo: repo}).Build(ctx, Target{
		Kind: TargetBaseDiff, BaseRef: selection.Commit, IntendedUntracked: target.IntendedUntracked,
	})
	if err != nil {
		return Snapshot{}, nil, err
	}
	count, err := commitCount(ctx, repo, selection.Commit, head)
	if err != nil {
		return Snapshot{}, nil, err
	}
	pushRemote, _, _ := publicationRemote(ctx, repo)
	pushIdentity, err := pushRepositoryIdentity(ctx, repo, pushRemote)
	if err != nil || request.PrePR.PushRemote != pushRemote || request.PrePR.PushRemoteIdentity != pushIdentity {
		return Snapshot{}, nil, errors.New("push destination remote changed during validation")
	}
	return snapshot, &resolvedPrePRRefs{Selection: selection, HeadCommit: head, BaseCommit: selection.Commit, DeliveredCommitCount: count, PushRemote: pushRemote}, nil
}

func prePRBoundaryForRequest(ctx context.Context, repo string, request GateRequest) (PrePRBoundarySelection, error) {
	if request.PrePR != nil && request.PrePR.Boundary != nil {
		selection := *request.PrePR.Boundary
		selector := ""
		switch selection.Source {
		case PrePRBoundaryExplicit:
			selector = selection.Selector
		case PrePRBoundaryPublicationDefault:
		default:
			return PrePRBoundarySelection{}, errors.New("unsupported pre-PR boundary source")
		}
		resolved, err := selectPrePRBoundary(ctx, repo, selector)
		if err != nil {
			return PrePRBoundarySelection{}, err
		}
		if resolved != selection {
			return PrePRBoundarySelection{}, errors.New("pre-PR boundary changed during validation")
		}
		return resolved, nil
	}
	if request.PrePR == nil && strings.TrimSpace(request.Target.BaseRef) != "" {
		return selectPrePRBoundary(ctx, repo, request.Target.BaseRef)
	}
	return selectPrePRBoundary(ctx, repo, "")
}

func selectPrePRBoundary(ctx context.Context, repo, selector string) (PrePRBoundarySelection, error) {
	selector = strings.TrimSpace(selector)
	var selection PrePRBoundarySelection
	var err error
	if selector != "" {
		if filepath.IsAbs(selector) {
			return PrePRBoundarySelection{}, errors.New("explicit pre-PR base selector must not be an absolute path")
		}
		selection, err = resolveAdvertisedSelector(ctx, repo, selector, PrePRBoundaryExplicit)
	} else {
		var ref, remote, commit string
		ref, remote, commit, err = resolveAuthoritativePublicationBase(ctx, repo)
		identity, identityErr := remoteRepositoryIdentity(ctx, repo, remote)
		if err == nil {
			err = identityErr
		}
		selection = PrePRBoundarySelection{Source: PrePRBoundaryPublicationDefault, Selector: ref, Commit: commit, Remote: remote, RemoteRef: ref, RemoteIdentity: identity}
	}
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	bases, mergeErr := runGit(ctx, repo, nil, nil, "merge-base", "--all", head, selection.Commit)
	if mergeBases := strings.Fields(string(bases)); mergeErr != nil || len(mergeBases) != 1 {
		return PrePRBoundarySelection{}, fmt.Errorf("publication base has %d merge bases; pass --base-ref <remote>/<branch>", len(mergeBases))
	}
	return selection, nil
}

func buildPrePRTarget(ctx context.Context, repo, selector, ciAttestation string, intendedUntracked []string) (Target, *PrePRRequest, error) {
	selection, err := selectPrePRBoundary(ctx, repo, selector)
	if err != nil {
		return Target{}, nil, err
	}
	pushRemote, _, _ := publicationRemote(ctx, repo)
	pushIdentity, err := pushRepositoryIdentity(ctx, repo, pushRemote)
	if err != nil {
		return Target{}, nil, err
	}
	return Target{Kind: TargetBaseDiff, BaseRef: selection.Commit, IntendedUntracked: append([]string(nil), intendedUntracked...)},
		&PrePRRequest{CIAttestationArtifact: ciAttestation, Boundary: &selection, PushRemote: pushRemote, PushRemoteIdentity: pushIdentity}, nil
}

func publicationRemoteConfigured(ctx context.Context, repo string) (bool, error) {
	_, configured, err := publicationRemote(ctx, repo)
	return configured, err
}

func resolveAuthoritativePublicationBase(ctx context.Context, repo string) (string, string, string, error) {
	remote, configured, err := publicationRemote(ctx, repo)
	if err != nil || !configured {
		return "", "", "", errors.New("publication target remote is not configured; pass --base-ref <remote>/<branch>")
	}
	output, err := runGit(ctx, repo, nil, nil, "ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", "", "", fmt.Errorf("query publication target default branch: %w", err)
	}
	ref := ""
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" && strings.HasPrefix(fields[1], "refs/heads/") {
			ref = fields[1]
		}
	}
	if ref == "" {
		return "", "", "", errors.New("publication target default branch is unavailable; pass --base-ref <remote>/<branch>")
	}
	selection, err := advertisedRemoteRef(ctx, repo, remote, ref, remote+"/"+strings.TrimPrefix(ref, "refs/heads/"), PrePRBoundaryPublicationDefault)
	if err != nil {
		return "", "", "", err
	}
	return selection.RemoteRef, selection.Remote, selection.Commit, nil
}

func resolveTrackingUpstreamBase(ctx context.Context, repo string) (string, string, string, error) {
	branch := currentBranch(ctx, repo)
	if branch == "" {
		return "", "", "", errors.New("configured upstream is unavailable on detached HEAD; pass --base-ref <remote>/<branch>")
	}
	remote, ref, ok := configuredUpstreamRef(ctx, repo, branch)
	if !ok {
		return "", "", "", errors.New("configured upstream is missing or ambiguous; pass --base-ref <remote>/<branch>")
	}
	selection, err := advertisedRemoteRef(ctx, repo, remote, ref, remote+"/"+strings.TrimPrefix(ref, "refs/heads/"), PrePRBoundaryPublicationDefault)
	if err != nil {
		return "", "", "", err
	}
	return selection.RemoteRef, selection.Remote, selection.Commit, nil
}

func resolveAdvertisedSelector(ctx context.Context, repo, selector string, source PrePRBoundarySource) (PrePRBoundarySelection, error) {
	output, err := runGit(ctx, repo, nil, nil, "remote")
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	remotes := strings.Fields(string(output))
	matches := []PrePRBoundarySelection{}
	for _, remote := range remotes {
		identity, identityErr := remoteRepositoryIdentity(ctx, repo, remote)
		if identityErr != nil {
			continue
		}
		branch := selector
		if strings.HasPrefix(selector, remote+"/") {
			branch = strings.TrimPrefix(selector, remote+"/")
		} else if strings.Contains(selector, "/") {
			continue
		}
		if validGitTree(selector) {
			branch = ""
		} else if _, err := runGit(ctx, repo, nil, nil, "check-ref-format", "--branch", branch); err != nil {
			continue
		}
		remoteOutput, queryErr := runGit(ctx, repo, nil, nil, "ls-remote", "--heads", remote, branch)
		if queryErr != nil {
			continue
		}
		for _, line := range strings.Split(string(remoteOutput), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && strings.HasPrefix(fields[1], "refs/heads/") &&
				(validGitTree(selector) && fields[0] == selector || !validGitTree(selector) && fields[1] == "refs/heads/"+branch) {
				matches = append(matches, PrePRBoundarySelection{Source: source, Selector: selector, Commit: fields[0], Remote: remote, RemoteRef: fields[1], RemoteIdentity: identity})
			}
		}
	}
	if len(matches) != 1 {
		return PrePRBoundarySelection{}, fmt.Errorf("explicit pre-PR base %q is missing or ambiguous on advertised remote branches; pass --base-ref <remote>/<branch>", selector)
	}
	local, err := resolveCommit(ctx, repo, matches[0].Commit)
	if err != nil || local != matches[0].Commit {
		return PrePRBoundarySelection{}, errors.New("advertised base commit is not available locally; fetch before validation")
	}
	return matches[0], nil
}

func advertisedRemoteRef(ctx context.Context, repo, remote, ref, selector string, source PrePRBoundarySource) (PrePRBoundarySelection, error) {
	identity, err := remoteRepositoryIdentity(ctx, repo, remote)
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	output, err := runGit(ctx, repo, nil, nil, "ls-remote", "--heads", remote, ref)
	if err != nil {
		return PrePRBoundarySelection{}, fmt.Errorf("query base remote %q: %w", remote, err)
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 || fields[1] != ref || !validGitTree(fields[0]) {
		return PrePRBoundarySelection{}, fmt.Errorf("base selector %q is not a current advertised remote branch; pass --base-ref <remote>/<branch>", selector)
	}
	local, err := resolveCommit(ctx, repo, fields[0])
	if err != nil || local != fields[0] {
		return PrePRBoundarySelection{}, errors.New("advertised base commit is not available locally; fetch before validation")
	}
	return PrePRBoundarySelection{Source: source, Selector: selector, Commit: fields[0], Remote: remote, RemoteRef: ref, RemoteIdentity: identity}, nil
}

func remoteRepositoryIdentity(ctx context.Context, repo, remote string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "remote."+remote+".url")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "", errors.New("publication remote URL is not configured")
	}
	return repositoryLocationIdentity(ctx, repo, strings.TrimSpace(string(output)))
}

func pushRepositoryIdentity(ctx context.Context, repo, remote string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "remote."+remote+".pushurl")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return remoteRepositoryIdentity(ctx, repo, remote)
	}
	return repositoryLocationIdentity(ctx, repo, strings.TrimSpace(string(output)))
}

func repositoryLocationIdentity(ctx context.Context, repo, value string) (string, error) {
	if parsed, parseErr := url.Parse(value); parseErr == nil && parsed.Scheme != "" && parsed.Opaque == "" {
		value = strings.ToLower(parsed.Host) + "/" + strings.Trim(parsed.Path, "/")
	} else {
		if colon := strings.Index(value, ":"); colon > 0 && !strings.Contains(value[:colon], "/") {
			host := value[:colon]
			if at := strings.LastIndex(host, "@"); at >= 0 {
				host = host[at+1:]
			}
			value = strings.ToLower(host) + "/" + value[colon+1:]
		} else if !filepath.IsAbs(value) {
			root, rootErr := runGit(ctx, repo, nil, nil, "rev-parse", "--show-toplevel")
			if rootErr != nil {
				return "", errors.New("publication remote identity cannot be derived")
			}
			value = filepath.Clean(filepath.Join(strings.TrimSpace(string(root)), value))
		}
	}
	sum := sha256.Sum256([]byte(strings.TrimSuffix(strings.TrimRight(value, "/"), ".git")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func publicationRemote(ctx context.Context, repo string) (string, bool, error) {
	branchOutput, _ := runGit(ctx, repo, nil, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	branch := strings.TrimSpace(string(branchOutput))
	keys := make([]string, 0, 4)
	if branch != "" {
		keys = append(keys, "branch."+branch+".pushRemote")
	}
	keys = append(keys, "remote.pushDefault")
	if branch != "" {
		keys = append(keys, "branch."+branch+".remote")
	}
	for _, key := range keys {
		output, err := runGit(ctx, repo, nil, nil, "config", "--get", key)
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return strings.TrimSpace(string(output)), true, nil
		}
	}
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "remote.origin.url")
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return "origin", true, nil
	}
	return "", false, nil
}

func sameResolvedPrePRRefs(left, right *resolvedPrePRRefs) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func currentBranch(ctx context.Context, repo string) string {
	output, _ := runGit(ctx, repo, nil, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	return strings.TrimSpace(string(output))
}

func buildPushTarget(ctx context.Context, repo, selector, deliveryBaseTree, reviewedBaseTree string) (Target, *PushRequest, error) {
	selection, err := selectPrePushBoundary(ctx, repo, selector)
	if err != nil {
		return Target{}, nil, err
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return Target{}, nil, err
	}
	if selection.Source == PrePRBoundaryEmptyRemoteBootstrap {
		return buildBootstrapPushTarget(ctx, repo, selection, head, deliveryBaseTree, reviewedBaseTree)
	}
	bases, err := runGit(ctx, repo, nil, nil, "merge-base", "--all", head, selection.Commit)
	mergeBases := strings.Fields(string(bases))
	if err != nil || len(mergeBases) != 1 {
		return Target{}, nil, fmt.Errorf("publication base has %d merge bases; pass --base-ref <remote>/<branch>", len(mergeBases))
	}
	pushRemote, configured, err := publicationRemote(ctx, repo)
	if err != nil || !configured {
		return Target{}, nil, errors.New("push destination remote is not configured")
	}
	pushIdentity, err := pushRepositoryIdentity(ctx, repo, pushRemote)
	if err != nil {
		return Target{}, nil, err
	}
	push := &PushRequest{Boundary: selection, MergeBase: mergeBases[0], PushRemote: pushRemote, PushRemoteIdentity: pushIdentity, DeliveryBaseTree: deliveryBaseTree}
	base := push.MergeBase
	if deliveryBaseTree != "" {
		base, err = reviewedDeliveryBase(ctx, repo, push.MergeBase, head, deliveryBaseTree)
		count, countErr := commitCount(ctx, repo, base, head)
		if err != nil || countErr != nil || count != 1 {
			return Target{}, nil, errors.New("reviewed delivery is not exactly one commit from its reviewed base")
		}
	}
	return Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}}, push, nil
}

func selectPrePushBoundary(ctx context.Context, repo, selector string) (PrePRBoundarySelection, error) {
	if strings.TrimSpace(selector) != "" {
		return selectPrePRBoundary(ctx, repo, selector)
	}
	ref, remote, commit, err := resolveTrackingUpstreamBase(ctx, repo)
	if err != nil {
		if bootstrap, ok := emptyRemoteBootstrapBoundary(ctx, repo); ok {
			return bootstrap, nil
		}
		return PrePRBoundarySelection{}, err
	}
	identity, err := remoteRepositoryIdentity(ctx, repo, remote)
	return PrePRBoundarySelection{Source: PrePRBoundaryPublicationDefault, Selector: ref, Commit: commit, Remote: remote, RemoteRef: ref, RemoteIdentity: identity}, err
}

// configuredUpstreamRef resolves the configured upstream remote and branch
// ref for a local branch. It reports false when the upstream is missing,
// ambiguous, or not a branch ref.
func configuredUpstreamRef(ctx context.Context, repo, branch string) (string, string, bool) {
	output, err := runGit(ctx, repo, nil, nil, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	parts := strings.SplitN(strings.TrimSpace(string(output)), "\x00", 2)
	if err != nil || len(parts) != 2 || parts[0] == "" || !strings.HasPrefix(parts[1], "refs/heads/") {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// emptyRemoteBootstrapBoundary recognizes the first-publication case only:
// the current branch has a configured upstream whose remote advertises no
// refs at all (no HEAD, branches, or tags). The boundary content-binds the
// destination remote identity and intended ref with the explicit zero OID as
// publication base. Any other condition declines so the original fail-closed
// error is preserved: detached HEAD, a missing or ambiguous configured
// upstream, an unreachable remote or failed ls-remote, a remote with any
// advertised ref, an unresolvable remote identity, or an unresolvable HEAD
// commit.
func emptyRemoteBootstrapBoundary(ctx context.Context, repo string) (PrePRBoundarySelection, bool) {
	branch := currentBranch(ctx, repo)
	if branch == "" {
		return PrePRBoundarySelection{}, false
	}
	remote, ref, ok := configuredUpstreamRef(ctx, repo, branch)
	if !ok {
		return PrePRBoundarySelection{}, false
	}
	advertised, err := runGit(ctx, repo, nil, nil, "ls-remote", remote)
	if err != nil || strings.TrimSpace(string(advertised)) != "" {
		return PrePRBoundarySelection{}, false
	}
	identity, err := remoteRepositoryIdentity(ctx, repo, remote)
	if err != nil {
		return PrePRBoundarySelection{}, false
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return PrePRBoundarySelection{}, false
	}
	return PrePRBoundarySelection{
		Source: PrePRBoundaryEmptyRemoteBootstrap, Selector: ref, Commit: strings.Repeat("0", len(head)),
		Remote: remote, RemoteRef: ref, RemoteIdentity: identity,
	}, true
}

// buildBootstrapPushTarget derives the pre-push target for a first
// publication to a verified empty remote. The reviewed base commit must be
// uniquely present in the candidate's complete history and, for
// current-changes receipts, the delivery must be exactly one commit beyond
// it. The gate evaluation then enforces the bootstrap publication invariant
// for every target kind: the delivered range reviewedBase..head must stay
// inside the immutable genesis scope, and pre-base ancestry — published in
// full by the first push — must satisfy both content-disclosure rules of
// validateBootstrapAncestryDisclosure: every path it ever named must exist in
// the reviewed base tree, and every blob object it reaches must be
// byte-identical to a blob that tree contains. Disclosure covers repository
// content only — tracked paths and blob bytes; commit and tag metadata
// (messages, author/committer identities, timestamps) is not review-bound
// anywhere in the gate model, on bootstrap or ordinary publication alike —
// see the scope note on validateBootstrapAncestryDisclosure. Everything else
// fails closed.
func buildBootstrapPushTarget(ctx context.Context, repo string, selection PrePRBoundarySelection, head, deliveryBaseTree, reviewedBaseTree string) (Target, *PushRequest, error) {
	// Both parameters originate from the receipt snapshot's BaseTree field:
	// deliveryBaseTree is the same value gated to current-changes receipts so
	// the one-commit delivery constraint below applies. The override can
	// therefore never select a genuinely divergent tree.
	bootstrapTree := reviewedBaseTree
	if deliveryBaseTree != "" {
		bootstrapTree = deliveryBaseTree
	}
	if bootstrapTree == "" {
		return Target{}, nil, errors.New("empty-remote bootstrap requires the authoritative reviewed base tree; pass --base-ref <remote>/<branch> once the remote is published")
	}
	pushRemote, configured, err := publicationRemote(ctx, repo)
	if err != nil || !configured {
		return Target{}, nil, errors.New("push destination remote is not configured")
	}
	if pushRemote != selection.Remote {
		return Target{}, nil, errors.New("empty-remote bootstrap requires the push destination to be the verified empty upstream remote")
	}
	pushIdentity, err := pushRepositoryIdentity(ctx, repo, pushRemote)
	if err != nil {
		return Target{}, nil, err
	}
	if pushIdentity != selection.RemoteIdentity {
		return Target{}, nil, errors.New("empty-remote bootstrap requires the push destination identity to match the verified empty remote")
	}
	push := &PushRequest{Boundary: selection, MergeBase: selection.Commit, PushRemote: pushRemote, PushRemoteIdentity: pushIdentity, DeliveryBaseTree: deliveryBaseTree, ReviewedBaseTree: reviewedBaseTree}
	base, err := bootstrapReviewedDeliveryBase(ctx, repo, head, bootstrapTree)
	if err != nil {
		return Target{}, nil, err
	}
	if deliveryBaseTree != "" {
		count, countErr := commitCount(ctx, repo, base, head)
		if countErr != nil || count != 1 {
			return Target{}, nil, errors.New("reviewed delivery is not exactly one commit from its reviewed base")
		}
	}
	return Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}}, push, nil
}

// bootstrapReviewedDeliveryBase locates the reviewed delivery base commit in
// the candidate's complete history because an empty remote provides no
// publication range boundary. A single subprocess lists every commit with its
// tree so the walk stays one call regardless of history length.
func bootstrapReviewedDeliveryBase(ctx context.Context, repo, head, reviewedTree string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "log", "--format=%H %T", head)
	if err != nil {
		return "", err
	}
	matches := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return "", errors.New("publication history listing is malformed")
		}
		if fields[1] == reviewedTree {
			matches = append(matches, fields[0])
		}
	}
	if len(matches) != 1 {
		return "", errors.New("reviewed delivery base commit is missing or ambiguous in publication range")
	}
	return matches[0], nil
}

// validateBootstrapAncestryDisclosure enforces the bootstrap publication
// invariant for pre-base history. A push to an empty remote transfers every
// object reachable from HEAD, including ancestry behind the reviewed delivery
// base that no publication gate ever examined. That ancestry is admissible
// only under two disclosure rules bound to the reviewed base tree — exactly
// the immutable context the receipt binds:
//
//  1. Path disclosure: every path pre-base history ever named must exist in
//     the reviewed base tree, so no published tree entry carries a file name
//     review never saw.
//  2. Blob disclosure: every blob object reachable from the reviewed base
//     commit must be byte-identical to some blob the reviewed base tree
//     contains, path-agnostic. Renaming disclosed content is admissible; any
//     historical content revision absent from the base tree — for example a
//     secret later overwritten at the same path — fails closed even though
//     its path is disclosed.
//
// Symlink entries are blobs and participate in both rules. Gitlink
// (submodule) entries disclose only their path: their commit OIDs reference
// external objects that history traversal never transfers, so they are
// excluded from blob disclosure. Failing either rule requires squashing
// pre-publication history (or re-creating an orphan first commit) so the
// first publication contains only reviewed content.
//
// Scope: disclosure covers repository CONTENT — tracked paths and blob bytes
// — and nothing else. Commit and tag metadata (messages, author/committer
// identities, timestamps) is not review-bound anywhere in the gate model: no
// receipt hashes or binds message text, and an ordinary (non-bootstrap)
// pre-push equally publishes the reviewed delivery commit's message without
// any review binding it. A pre-base commit created with `git commit
// --allow-empty` therefore passes both rules even when its message contains a
// secret, and the bootstrap push transfers that commit object; bootstrap does
// not widen a guarantee that never existed. Operators who need message
// hygiene should follow the same remediation the deny messages give — squash
// pre-publication history or re-create an orphan first commit — which reduces
// pre-base history to messages the operator wrote deliberately at publication
// time. TestBootstrapDisclosureScopeExcludesCommitMetadata pins this
// boundary.
func validateBootstrapAncestryDisclosure(ctx context.Context, repo, baseCommit string) error {
	touchedOutput, err := runGit(ctx, repo, nil, nil, "log", "-m", "--format=", "--name-only", "-z", "--no-renames", baseCommit)
	if err != nil {
		return fmt.Errorf("inspect bootstrap pre-base history: %w", err)
	}
	touched, err := canonicalPaths(splitNullSeparatedPaths(touchedOutput))
	if err != nil {
		return fmt.Errorf("bootstrap pre-base history paths are not canonical: %w", err)
	}
	disclosedPaths, disclosedBlobs, err := reviewedBaseTreeDisclosure(ctx, repo, baseCommit)
	if err != nil {
		return err
	}
	for _, path := range touched {
		if _, ok := disclosedPaths[path]; !ok {
			return fmt.Errorf("empty-remote bootstrap publishes pre-base history path %q that is not disclosed by the reviewed base tree; squash pre-publication history (or re-create an orphan first commit) so the first publication contains only reviewed content, then re-run review", path)
		}
	}
	return validateBootstrapReachableBlobs(ctx, repo, baseCommit, disclosedBlobs)
}

// reviewedBaseTreeDisclosure lists the reviewed base tree once and returns
// its canonical path set and its blob OID set. Symlink entries are blobs and
// join both sets; gitlink entries contribute only their path because their
// commit OIDs reference objects outside this repository.
func reviewedBaseTreeDisclosure(ctx context.Context, repo, baseCommit string) (map[string]struct{}, map[string]struct{}, error) {
	output, err := runGit(ctx, repo, nil, nil, "ls-tree", "-r", "-z", "--full-tree", baseCommit)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect reviewed bootstrap base tree: %w", err)
	}
	names := []string{}
	blobs := map[string]struct{}{}
	for _, entry := range splitNullSeparatedPaths(output) {
		meta, path, found := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !found || path == "" || len(fields) != 3 {
			return nil, nil, errors.New("reviewed bootstrap base tree listing is malformed")
		}
		names = append(names, path)
		if fields[1] == "blob" {
			blobs[fields[2]] = struct{}{}
		}
	}
	canonical, err := canonicalPaths(names)
	if err != nil {
		return nil, nil, fmt.Errorf("reviewed bootstrap base tree paths are not canonical: %w", err)
	}
	paths := make(map[string]struct{}, len(canonical))
	for _, path := range canonical {
		paths[path] = struct{}{}
	}
	return paths, blobs, nil
}

// validateBootstrapReachableBlobs enforces blob-level disclosure: every blob
// object reachable from the reviewed base commit must be a member of the
// reviewed base tree's blob set. The subprocess count stays constant
// regardless of history size: rev-list enumerates each reachable object once
// with a representative path, and one cat-file batch resolves every object
// type; anything unresolvable fails closed.
//
// The cat-file --batch-check pass exists deliberately: do not "simplify" this
// to `git rev-list --objects --filter=object:type=blob`. Filter semantics
// proved version-shaky during design probing — git 2.55 dropped a traversed
// commit's line while keeping the provided one — so type resolution through
// one batch-check invocation is the version-stable approach.
func validateBootstrapReachableBlobs(ctx context.Context, repo, baseCommit string, disclosed map[string]struct{}) error {
	output, err := runGit(ctx, repo, nil, nil, "rev-list", "--objects", baseCommit)
	if err != nil {
		return fmt.Errorf("enumerate bootstrap pre-base objects: %w", err)
	}
	oids := []string{}
	representative := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		oid, path, _ := strings.Cut(line, " ")
		oids = append(oids, oid)
		representative[oid] = path
	}
	if len(oids) == 0 {
		return errors.New("bootstrap pre-base object listing is empty")
	}
	typed, err := runGit(ctx, repo, nil, []byte(strings.Join(oids, "\n")+"\n"), "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return fmt.Errorf("resolve bootstrap pre-base object types: %w", err)
	}
	types := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(typed)), "\n") {
		oid, objectType, found := strings.Cut(line, " ")
		switch {
		case found && (objectType == "commit" || objectType == "tree" || objectType == "blob" || objectType == "tag"):
			types[oid] = objectType
		default:
			return errors.New("bootstrap pre-base object types are unresolvable")
		}
	}
	for _, oid := range oids {
		objectType, resolved := types[oid]
		if !resolved {
			return errors.New("bootstrap pre-base object types are incomplete")
		}
		if objectType != "blob" {
			continue
		}
		if _, ok := disclosed[oid]; !ok {
			return fmt.Errorf("empty-remote bootstrap publishes pre-base history blob %s at %q whose content is not disclosed by the reviewed base tree; squash pre-publication history (or re-create an orphan first commit) so the first publication contains only reviewed content, then re-run review", oid, representative[oid])
		}
	}
	return nil
}

func splitNullSeparatedPaths(output []byte) []string {
	paths := []string{}
	seen := map[string]struct{}{}
	for _, path := range strings.Split(string(output), "\x00") {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func reviewedDeliveryBase(ctx context.Context, repo, publicationBase, head, reviewedTree string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-list", head, "^"+publicationBase)
	if err != nil {
		return "", err
	}
	matches := []string{}
	for _, commit := range append(strings.Fields(string(output)), publicationBase) {
		tree, treeErr := runGit(ctx, repo, nil, nil, "rev-parse", commit+"^{tree}")
		if treeErr != nil {
			return "", treeErr
		}
		if strings.TrimSpace(string(tree)) == reviewedTree {
			matches = append(matches, commit)
		}
	}
	if len(matches) != 1 {
		return "", errors.New("reviewed delivery base commit is missing or ambiguous in publication range")
	}
	return matches[0], nil
}

func prePushTargetForRequest(ctx context.Context, repo string, request GateRequest) (Target, *resolvedPrePRRefs, error) {
	selector := ""
	if request.Push != nil && request.Push.Boundary.Source == PrePRBoundaryExplicit {
		selector = request.Push.Boundary.Selector
	}
	deliveryBaseTree, reviewedBaseTree := "", ""
	if request.Push != nil {
		deliveryBaseTree = request.Push.DeliveryBaseTree
		reviewedBaseTree = request.Push.ReviewedBaseTree
	}
	target, push, err := buildPushTarget(ctx, repo, selector, deliveryBaseTree, reviewedBaseTree)
	if err != nil {
		return Target{}, nil, err
	}
	if request.Push != nil && *request.Push != *push {
		return Target{}, nil, errors.New("push publication boundary changed during validation")
	}
	if request.Target.Kind == TargetBaseDiff && request.Target.BaseRef != "" && request.Target.BaseRef != target.BaseRef {
		return Target{}, nil, errors.New("committed publication merge base changed during validation")
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return Target{}, nil, err
	}
	count, err := commitCount(ctx, repo, target.BaseRef, head)
	if err != nil {
		return Target{}, nil, err
	}
	baseCommit := push.MergeBase
	if push.Boundary.Source == PrePRBoundaryEmptyRemoteBootstrap {
		// The zero-OID boundary sentinel only records that no advertised
		// publication base exists; it must never anchor validation. The
		// resolved reviewed delivery base commit is the real range boundary
		// that publication-range and ancestry-disclosure checks scope to.
		baseCommit = target.BaseRef
	}
	return target, &resolvedPrePRRefs{Selection: push.Boundary, HeadCommit: head, BaseCommit: baseCommit, DeliveredCommitCount: count, PushRemote: push.PushRemote}, nil
}

func commitCount(ctx context.Context, repo, base, head string) (int, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-list", "--count", base+".."+head)
	if err != nil {
		return 0, err
	}
	var count int
	_, err = fmt.Sscan(strings.TrimSpace(string(output)), &count)
	return count, err
}

func resolveCommit(ctx context.Context, repo, revision string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-parse", "--verify", strings.TrimSpace(revision)+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// lifecycleTargetForGate deliberately derives the candidate from the event's
// live repository context. A caller-selected historical commit/range may
// describe review evidence, but it must never authorize a newer HEAD.
func lifecycleTargetForGate(ctx context.Context, repo string, request GateRequest) (Target, error) {
	switch request.Gate {
	case GatePostApply, GatePreCommit:
		intended := request.Target.IntendedUntracked
		if intended == nil {
			intended = []string{}
		}
		return Target{Kind: TargetCurrentChanges, Projection: request.Target.Projection, IntendedUntracked: intended}, nil
	case GatePrePush:
		head, err := runGit(ctx, repo, nil, nil, "rev-parse", "HEAD")
		if err != nil {
			return Target{}, err
		}
		return Target{Kind: TargetExactRevision, Revision: strings.TrimSpace(string(head))}, nil
	case GatePrePR:
		if request.Target.Kind != TargetBaseDiff || strings.TrimSpace(request.Target.BaseRef) == "" {
			return Target{}, errors.New("pre-PR validation requires an explicit base-diff target")
		}
		return Target{
			Kind: TargetBaseDiff, BaseRef: request.Target.BaseRef,
			IntendedUntracked: append([]string(nil), request.Target.IntendedUntracked...),
		}, nil
	case GateRelease:
		if request.Target.Kind != TargetExactRevision || request.Release == nil {
			return Target{}, errors.New("release validation requires an exact current release revision")
		}
		head, err := runGit(ctx, repo, nil, nil, "rev-parse", "HEAD")
		if err != nil {
			return Target{}, err
		}
		if strings.TrimSpace(request.Release.Revision) != strings.TrimSpace(string(head)) || request.Target.Revision != request.Release.Revision {
			return Target{}, errors.New("release revision is not the current HEAD")
		}
		return Target{Kind: TargetExactRevision, Revision: request.Release.Revision}, nil
	default:
		return Target{}, errors.New("unsupported lifecycle gate")
	}
}

func validateGateRequest(request GateRequest) error {
	if request.Schema != GateRequestSchema {
		return errors.New("unsupported review gate request schema")
	}
	switch request.Gate {
	case GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease:
	default:
		return fmt.Errorf("unsupported review gate %q", request.Gate)
	}
	if !validSHA256(request.StoreRevision) || !validSHA256(request.GenesisRevision) || !validSHA256(request.ChainIdentity) || !validSHA256(request.BundleDigest) {
		return errors.New("gate request requires the exact authoritative store revision, genesis, chain identity, and bundle digest")
	}
	if request.Gate == GateRelease && request.Release == nil {
		return errors.New("release gate requires an immutable release request")
	}
	if request.Gate != GateRelease && request.Release != nil {
		return errors.New("release request is only valid at the release gate")
	}
	if request.Gate != GatePrePR && request.PrePR != nil {
		return errors.New("pre-PR evidence is only valid at the pre-PR gate")
	}
	if request.Gate != GatePrePush && request.Push != nil {
		return errors.New("push evidence is only valid at the pre-push gate")
	}
	if request.Gate == GatePrePush && (request.Push == nil || request.Push.PushRemoteIdentity == "" || request.Push.Boundary.RemoteIdentity == "") {
		return errors.New("pre-push gate requires repository-bound push evidence")
	}
	if request.Gate == GatePrePR && (request.PrePR == nil || request.PrePR.Boundary == nil || request.PrePR.PushRemoteIdentity == "" || request.PrePR.Boundary.RemoteIdentity == "") {
		return errors.New("pre-PR gate requires repository-bound publication evidence; pass --base-ref <remote>/<branch>")
	}
	switch request.ExternalEvidence {
	case ExternalEvidenceNone, ExternalEvidenceInvalidating, ExternalEvidenceEscalating:
	default:
		return fmt.Errorf("invalid external evidence disposition %q", request.ExternalEvidence)
	}
	return nil
}

func deriveReleaseEvidence(ctx context.Context, repo string, request *ReleaseRequest, preimages gateArtifactPreimages) (ReleaseEvidence, error) {
	if request == nil {
		return ReleaseEvidence{}, errors.New("release request is missing")
	}
	revision, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetExactRevision, Revision: request.Revision})
	if err != nil {
		return ReleaseEvidence{}, err
	}
	release := ReleaseEvidence{
		ReleaseTree: revision.CandidateTree, ConfigurationHash: hashArtifactPayload(preimages.configuration),
		GeneratedArtifactHash: hashArtifactPayload(preimages.generated), ProvenanceHash: hashArtifactPayload(preimages.provenance),
		PublicationBoundaryHash: hashArtifactPayload(preimages.publicationBoundary), PublicationState: request.PublicationState,
		EvidenceFreshnessHash: hashArtifactPayload(preimages.evidenceFreshness), EvidenceFreshnessState: request.EvidenceFreshnessState,
	}
	if err := validateReleaseEvidence(release); err != nil {
		return ReleaseEvidence{}, err
	}
	return release, nil
}

func hashLedgerArtifact(path string) (string, error) {
	hash, _, err := hashLedgerArtifactBinding(path)
	return hash, err
}

func hashArtifactSource(path, content string) (string, error) {
	if strings.TrimSpace(content) != "" {
		sum := sha256.Sum256([]byte(content))
		return "sha256:" + hex.EncodeToString(sum[:]), nil
	}
	return HashArtifact(path)
}

func hashArtifactPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func hashLedgerArtifactSource(path, content string) (string, string, error) {
	if strings.TrimSpace(content) == "" {
		return hashLedgerArtifactBinding(path)
	}
	return hashLedgerPayload([]byte(content))
}

func hashLedgerArtifactBinding(path string) (string, string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return hashLedgerPayload(payload)
}

func hashLedgerPayload(payload []byte) (string, string, error) {
	return validateCanonicalLedger(payload, nil, "")
}

func readGateArtifactPreimages(request GateRequest) (gateArtifactPreimages, error) {
	if request.preimages != nil {
		return *request.preimages, nil
	}
	read := func(label, path, content string, required bool) ([]byte, error) {
		if content != "" {
			return []byte(content), nil
		}
		if strings.TrimSpace(path) == "" {
			if required {
				return nil, fmt.Errorf("%s artifact is required", label)
			}
			return nil, nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s artifact: %w", label, err)
		}
		return payload, nil
	}
	var result gateArtifactPreimages
	var err error
	for _, item := range []struct {
		label, path, content string
		required             bool
		destination          *[]byte
	}{
		{"policy", request.PolicyArtifact, request.PolicyContent, false, &result.policy},
		{"ledger", request.LedgerArtifact, request.LedgerContent, false, &result.ledger},
		{"fix delta", request.FixDeltaArtifact, request.FixDeltaContent, false, &result.fixDelta},
		{"verification evidence", request.EvidenceArtifact, request.EvidenceContent, false, &result.evidence},
	} {
		*item.destination, err = read(item.label, item.path, item.content, item.required)
		if err != nil {
			return gateArtifactPreimages{}, err
		}
	}
	if request.PrePR != nil && request.PrePR.CIAttestationArtifact != "" {
		result.ciAttestation, err = read("PRE-PR CI attestation", request.PrePR.CIAttestationArtifact, "", true)
		if err != nil {
			return gateArtifactPreimages{}, err
		}
	}
	if request.Release != nil {
		for _, item := range []struct {
			label, path, content string
			destination          *[]byte
		}{
			{"release configuration", request.Release.ConfigurationArtifact, request.Release.ConfigurationContent, &result.configuration},
			{"generated manifest", request.Release.GeneratedArtifact, request.Release.GeneratedContent, &result.generated},
			{"release provenance", request.Release.ProvenanceArtifact, request.Release.ProvenanceContent, &result.provenance},
			{"publication boundary", request.Release.PublicationBoundaryArtifact, request.Release.PublicationBoundaryContent, &result.publicationBoundary},
			{"evidence freshness", request.Release.EvidenceFreshnessArtifact, request.Release.EvidenceFreshnessContent, &result.evidenceFreshness},
		} {
			*item.destination, err = read(item.label, item.path, item.content, true)
			if err != nil {
				return gateArtifactPreimages{}, err
			}
		}
	}
	return result, nil
}

func hasArtifactSource(path, content string) bool {
	return strings.TrimSpace(path) != "" || content != ""
}

func validateFixDeltaArtifact(payload []byte, transaction Transaction) (string, error) {
	if transaction.FixDeltaHash == EmptyFixDeltaHash {
		if len(payload) != 0 {
			return "", errors.New("uncorrected lineage must not name a fix delta artifact")
		}
		return EmptyFixDeltaHash, nil
	}
	if len(payload) == 0 {
		return "", errors.New("corrected lineage requires the persisted fix delta artifact")
	}
	snapshot, derived, err := HashFixDeltaArtifact(payload)
	if err != nil {
		return "", err
	}
	if !reflect.DeepEqual(snapshot, transaction.Snapshot) || snapshot.Kind != TargetFixDiff {
		return "", errors.New("fix delta artifact does not match the terminal correction snapshot")
	}
	if derived != transaction.FixDeltaHash {
		return "", errors.New("fix delta artifact identity does not match the terminal transaction")
	}
	return derived, nil
}

func HashFixDeltaArtifact(payload []byte) (Snapshot, string, error) {
	var snapshot Snapshot
	if err := decodeStrictJSON(payload, &snapshot, "fix delta"); err != nil {
		return Snapshot{}, "", err
	}
	if err := validateSnapshot(snapshot); err != nil || snapshot.Kind != TargetFixDiff {
		return Snapshot{}, "", errors.New("fix delta artifact requires a valid fix-diff snapshot")
	}
	return snapshot, FixDeltaHashForSnapshot(snapshot), nil
}

func decodeStrictJSON(payload []byte, destination any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("parse %s artifact: %w", label, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%s artifact contains multiple JSON values", label)
	}
	return nil
}

func HashLedgerArtifact(path string) (string, error) {
	return hashLedgerArtifact(path)
}

func nativeGateReason(result GateResult) string {
	switch result {
	case GateAllow:
		return "authoritative transaction, current repository target, and content-bound artifacts match"
	case GateScopeChanged:
		return "current repository target no longer matches the reviewed scope"
	case GateEscalated:
		return "transaction or external evidence is terminally escalated"
	default:
		return "content-bound policy, ledger, fix delta, verify evidence, base, or release evidence does not match"
	}
}
