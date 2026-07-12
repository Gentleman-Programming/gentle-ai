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
	"os"
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
	PrePR            *PrePRRequest               `json:"pre_pr,omitempty"`
	Release          *ReleaseRequest             `json:"release,omitempty"`
	preimages        *gateArtifactPreimages
}

type PrePRRequest struct {
	CIAttestationArtifact string `json:"ci_attestation_artifact"`
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
		return invalid("current repository target cannot be derived: " + err.Error())
	}
	if request.Gate == GatePrePush && record.Transaction.Snapshot.Kind == TargetCurrentChanges && snapshot.BaseTree == snapshot.CandidateTree {
		return invalid("pre-push current-changes receipt requires a delivered tree change")
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
	BaseRef    string
	Remote     string
	BaseCommit string
	HeadCommit string
}

func buildLifecycleSnapshot(ctx context.Context, repo string, request GateRequest) (Snapshot, *resolvedPrePRRefs, error) {
	target, err := lifecycleTargetForGate(ctx, repo, request)
	if err != nil {
		return Snapshot{}, nil, err
	}
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.build(ctx, target, request.Gate == GatePreCommit)
	if err != nil || request.Gate != GatePrePR {
		return snapshot, nil, err
	}
	configured, err := publicationRemoteConfigured(ctx, repo)
	if err != nil || !configured {
		return snapshot, nil, err
	}
	baseRef, remote, base, err := resolveAuthoritativePublicationBase(ctx, repo)
	if err != nil {
		return Snapshot{}, nil, err
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return Snapshot{}, nil, err
	}
	snapshot, err = (SnapshotBuilder{Repo: repo}).Build(ctx, Target{
		Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: target.IntendedUntracked,
	})
	if err != nil {
		return Snapshot{}, nil, err
	}
	return snapshot, &resolvedPrePRRefs{BaseRef: baseRef, Remote: remote, BaseCommit: base, HeadCommit: head}, nil
}

func publicationRemoteConfigured(ctx context.Context, repo string) (bool, error) {
	_, configured, err := publicationRemote(ctx, repo)
	return configured, err
}

func resolveAuthoritativePublicationBase(ctx context.Context, repo string) (string, string, string, error) {
	remote, configured, err := publicationRemote(ctx, repo)
	if err != nil {
		return "", "", "", err
	}
	if !configured {
		return "", "", "", errors.New("publication remote is not configured")
	}
	output, err := runGit(ctx, repo, nil, nil, "ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", "", "", fmt.Errorf("query publication remote %q: %w", remote, err)
	}
	var baseRef, commit string
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" {
			baseRef = fields[1]
		}
		if len(fields) == 2 && fields[1] == "HEAD" && validGitTree(fields[0]) {
			commit = fields[0]
		}
	}
	if !strings.HasPrefix(baseRef, "refs/heads/") || !validGitTree(commit) {
		return "", "", "", errors.New("publication remote does not advertise a default branch HEAD")
	}
	local, err := resolveCommit(ctx, repo, commit)
	if err != nil || local != commit {
		return "", "", "", errors.New("publication base commit is not available locally; fetch before validation")
	}
	return baseRef, remote, commit, nil
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
		return Target{Kind: TargetCurrentChanges, IntendedUntracked: intended}, nil
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
