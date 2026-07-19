package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
)

const CompactPrePRChainProofDomain = "gentle-ai.compact-pre-pr-chain/v1"

type compactPrePRChainMember struct {
	store   CompactStore
	record  CompactRecord
	receipt CompactReceipt
}

type compactPrePRChainAuthorityProof struct {
	LineageID   string `json:"lineage_id"`
	Revision    string `json:"revision"`
	StateHash   string `json:"state_hash"`
	ReceiptHash string `json:"receipt_hash,omitempty"`
}

type compactPrePRChainMemberProof struct {
	LineageID    string         `json:"lineage_id"`
	Revision     string         `json:"revision"`
	Receipt      CompactReceipt `json:"receipt"`
	GenesisPaths []string       `json:"genesis_paths"`
	ChangedPaths []string       `json:"changed_paths"`
	TouchedPaths []string       `json:"touched_paths"`
}

type compactPrePRChainCommitProof struct {
	Commit     string `json:"commit"`
	Parent     string `json:"parent"`
	ParentTree string `json:"parent_tree"`
	Tree       string `json:"tree"`
}

type compactPrePRChainProof struct {
	Domain             string                            `json:"domain"`
	Boundary           PrePRBoundarySelection            `json:"boundary"`
	PushRemote         string                            `json:"push_remote"`
	PushRemoteIdentity string                            `json:"push_remote_identity"`
	BaseCommit         string                            `json:"base_commit"`
	HeadCommit         string                            `json:"head_commit"`
	BaseTree           string                            `json:"base_tree"`
	HeadTree           string                            `json:"head_tree"`
	Authority          []compactPrePRChainAuthorityProof `json:"authority"`
	Members            []compactPrePRChainMemberProof    `json:"members"`
	Commits            []compactPrePRChainCommitProof    `json:"commits"`
	ChangedPaths       []string                          `json:"changed_paths"`
	TouchedPaths       []string                          `json:"touched_paths"`
	PublicationPaths   []string                          `json:"publication_paths"`
	BaseAdvance        *BaseAdvanceCompatibility         `json:"base_advance,omitempty"`
}

type compactPrePRChainDerivation struct {
	proof    compactPrePRChainProof
	identity string
	context  GateContext
	lockPath string
}

var errCompactPrePRChainNotApplicable = errors.New("compact pre-PR receipt composition is not applicable")

// EvaluateCompactPrePRChain composes existing compact-v2 authority in memory.
// It is intentionally unavailable for explicit lineage selection and every
// gate except pre-PR, preserving direct single-receipt behavior.
func EvaluateCompactPrePRChain(ctx context.Context, repo string, input NativeGateRequestInput) (NativeGateEvaluation, bool) {
	invalid := func(reason string, cause error) NativeGateEvaluation {
		return NativeGateEvaluation{
			Result: GateInvalidated, Reason: reason, Cause: cause,
			Context: GateContext{Gate: GatePrePR, Denial: &GateDenial{Stage: "receipt-composition", Code: "invalid-chain"}},
		}
	}
	if input.Gate != GatePrePR || strings.TrimSpace(input.LineageID) != "" {
		return invalid(errCompactPrePRChainNotApplicable.Error(), errCompactPrePRChainNotApplicable), false
	}
	derived, attempted, err := deriveCompactPrePRChain(ctx, repo, input)
	if !attempted {
		return invalid(errCompactPrePRChainNotApplicable.Error(), errCompactPrePRChainNotApplicable), false
	}
	if err != nil {
		return invalid("compact pre-PR receipt composition denied: "+err.Error(), err), true
	}
	lock, err := acquireStoreLock(derived.lockPath)
	if err != nil {
		return invalid("compact pre-PR receipt composition changed during final authorization", err), true
	}
	defer lock.release()

	finalGateAuthorizationHook()
	final, finalAttempted, err := deriveCompactPrePRChain(ctx, repo, input)
	if err != nil || !finalAttempted || final.identity != derived.identity || !reflect.DeepEqual(final.proof, derived.proof) {
		return invalid("compact pre-PR receipt composition changed during final authorization", err), true
	}
	finalCompactGateAllowHook()
	return NativeGateEvaluation{Result: GateAllow, Reason: nativeGateReason(GateAllow), Context: derived.context}, true
}

func deriveCompactPrePRChain(ctx context.Context, repo string, input NativeGateRequestInput) (compactPrePRChainDerivation, bool, error) {
	target, prePR, err := buildPrePRTarget(ctx, repo, input.BaseRef, input.PrePRCIAttestation, nil)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	request := GateRequest{Schema: GateRequestSchema, Gate: GatePrePR, Target: target, PrePR: prePR, PolicyArtifact: input.PolicyArtifact}
	preimages, err := readGateArtifactPreimages(request)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	snapshot, refs, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil || refs == nil {
		return compactPrePRChainDerivation{}, true, errors.Join(err, errors.New("resolved pre-PR publication boundary is unavailable"))
	}
	report, err := InventoryAuthority(ctx, repo)
	if err != nil || !report.Complete || !report.Authoritative {
		return compactPrePRChainDerivation{}, true, errors.Join(err, errors.New("complete authoritative review inventory is unavailable"))
	}
	for _, entry := range report.Entries {
		if entry.Version == AuthorityVersionLegacy {
			status, statusErr := AssessTargetStatus(ctx, repo, TargetStatusRequest{Target: target, LineageID: entry.LineageID})
			if statusErr != nil || status.Applicability != TargetApplicabilityUnrelated {
				return compactPrePRChainDerivation{}, true, errors.Join(statusErr, errors.New("applicable legacy-v1 and compact authority are ambiguous"))
			}
		}
	}

	stores, err := DiscoverCompactStores(ctx, repo)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	if len(stores) < 2 {
		return compactPrePRChainDerivation{}, false, errCompactPrePRChainNotApplicable
	}
	leaves, err := CompactAuthorityLeaves(ctx, repo)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	leafSet := make(map[string]struct{}, len(leaves))
	for _, store := range leaves {
		leafSet[store.lineageID] = struct{}{}
	}
	authority := make([]compactPrePRChainAuthorityProof, 0, len(stores))
	members := make([]compactPrePRChainMember, 0, len(stores))
	for _, store := range stores {
		statePayload, readErr := os.ReadFile(store.StatePath())
		if readErr != nil {
			return compactPrePRChainDerivation{}, true, fmt.Errorf("read compact authority %q: %w", store.lineageID, readErr)
		}
		record, loadErr := store.Load()
		if loadErr != nil {
			return compactPrePRChainDerivation{}, true, fmt.Errorf("load compact authority %q: %w", store.lineageID, loadErr)
		}
		proof := compactPrePRChainAuthorityProof{
			LineageID: record.State.LineageID, Revision: record.Revision,
			StateHash: compactPrePRChainPayloadHash("state", statePayload),
		}
		receiptPayload, receiptErr := os.ReadFile(store.ReceiptPath())
		terminal := record.State.State == StateApproved || record.State.State == StateEscalated
		if terminal {
			if receiptErr != nil {
				return compactPrePRChainDerivation{}, true, fmt.Errorf("read terminal compact receipt %q: %w", store.lineageID, receiptErr)
			}
			receipt, parseErr := ParseCompactReceipt(receiptPayload)
			authoritative, deriveErr := record.State.Receipt()
			if parseErr != nil || deriveErr != nil || !compactReceiptEqual(receipt, authoritative) {
				return compactPrePRChainDerivation{}, true, fmt.Errorf("compact receipt %q does not match current authority", store.lineageID)
			}
			proof.ReceiptHash = compactPrePRChainPayloadHash("receipt", receiptPayload)
			if _, leaf := leafSet[store.lineageID]; leaf && record.State.State == StateApproved {
				members = append(members, compactPrePRChainMember{store: store, record: record, receipt: receipt})
			}
		} else if receiptErr == nil {
			return compactPrePRChainDerivation{}, true, fmt.Errorf("nonterminal compact authority %q has a receipt", store.lineageID)
		} else if !os.IsNotExist(receiptErr) {
			return compactPrePRChainDerivation{}, true, receiptErr
		}
		authority = append(authority, proof)
	}
	if len(members) < 2 {
		return compactPrePRChainDerivation{}, true, errors.New("no receipt chain contains at least two authoritative approved members")
	}

	path, compatibility, err := selectCompactPrePRChain(ctx, repo, request, preimages, snapshot, refs, members)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return compactPrePRChainDerivation{}, true, fmt.Errorf("assess complete composed target risk: %w", err)
	}
	if assessment.Level == RiskHigh || assessment.ChangedLines > LargeChangeLines {
		return compactPrePRChainDerivation{}, true, errors.New("complete composed target is high risk; full-target review is required")
	}
	if len(preimages.policy) > 0 {
		policyHash := hashArtifactPayload(preimages.policy)
		for _, member := range path {
			if member.record.State.PolicyHash != policyHash {
				return compactPrePRChainDerivation{}, true, errors.New("explicit pre-PR policy is not bound to every composed receipt")
			}
		}
	}

	proof, err := buildCompactPrePRChainProof(ctx, repo, request, refs, snapshot, authority, path, compatibility)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	identity, err := compactPrePRChainProofIdentity(proof)
	if err != nil {
		return compactPrePRChainDerivation{}, true, err
	}
	last := path[len(path)-1]
	fixHashes := make([]string, len(path))
	policyHashes := make([]string, len(path))
	evidenceHashes := make([]string, len(path))
	for index, member := range path {
		fixHashes[index] = member.receipt.FixDeltaHash
		policyHashes[index] = member.receipt.PolicyHash
		evidenceHashes[index] = member.receipt.EvidenceHash
	}
	boundary := refs.Selection
	gateContext := GateContext{
		Gate: GatePrePR, LineageID: last.receipt.LineageID, Generation: last.receipt.Generation,
		StoreRevision: last.record.Revision, GenesisRevision: path[0].record.Revision,
		ChainIdentity: identity, BundleDigest: identity,
		BaseTree: proof.BaseTree, CandidateTree: proof.HeadTree, PathsDigest: digestPaths(proof.PublicationPaths),
		FixDeltaHash: compactPrePRChainValuesHash("fix-delta", fixHashes),
		PolicyHash:   compactPrePRChainValuesHash("policy", policyHashes),
		LedgerHash:   EmptyFixDeltaHash, EvidenceHash: compactPrePRChainValuesHash("evidence", evidenceHashes),
		BaseRelationshipValid: compatibility == nil, BaseAdvance: compatibility, PrePRBoundary: &boundary,
	}
	return compactPrePRChainDerivation{proof: proof, identity: identity, context: gateContext, lockPath: path[0].store.lockPath}, true, nil
}

func selectCompactPrePRChain(ctx context.Context, repo string, request GateRequest, preimages gateArtifactPreimages, snapshot Snapshot, refs *resolvedPrePRRefs, members []compactPrePRChainMember) ([]compactPrePRChainMember, *BaseAdvanceCompatibility, error) {
	adjacency := make(map[string][]compactPrePRChainMember)
	for _, member := range members {
		adjacency[member.receipt.BaseTree] = append(adjacency[member.receipt.BaseTree], member)
	}
	for base := range adjacency {
		sort.Slice(adjacency[base], func(i, j int) bool {
			return adjacency[base][i].receipt.LineageID < adjacency[base][j].receipt.LineageID
		})
	}
	if compactPrePRChainHasCycle(adjacency) {
		return nil, nil, errors.New("compact receipt graph contains a cycle")
	}
	type candidate struct {
		path          []compactPrePRChainMember
		compatibility *BaseAdvanceCompatibility
	}
	candidates := []candidate{}
	starts := make([]string, 0, len(adjacency))
	for base := range adjacency {
		starts = append(starts, base)
	}
	sort.Strings(starts)
	for _, base := range starts {
		var compatibility *BaseAdvanceCompatibility
		if base != snapshot.BaseTree {
			paths, err := (SnapshotBuilder{Repo: repo}).changedPaths(ctx, base, snapshot.CandidateTree)
			if err != nil {
				continue
			}
			synthetic := Receipt{BaseTree: base, FinalCandidateTree: snapshot.CandidateTree, PathsDigest: digestPaths(paths)}
			proof, proofErr := deriveBaseAdvanceCompatibility(ctx, repo, synthetic, request, snapshot, refs, preimages)
			if proofErr != nil {
				continue
			}
			compatibility = &proof
		}
		paths := compactPrePRChainPaths(base, snapshot.CandidateTree, adjacency, map[string]bool{}, nil, len(members)+1)
		for _, path := range paths {
			if len(path) >= 2 {
				candidates = append(candidates, candidate{path: path, compatibility: compatibility})
			}
		}
	}
	if len(candidates) == 0 {
		return nil, nil, errors.New("no exact compact receipt chain covers the selected base through HEAD")
	}
	if len(candidates) != 1 {
		return nil, nil, errors.New("multiple viable compact receipt chains cover the selected base through HEAD")
	}
	path := candidates[0].path
	pathNodes := map[string]bool{path[0].receipt.BaseTree: true}
	for _, member := range path {
		pathNodes[member.receipt.FinalCandidateTree] = true
	}
	incoming := make(map[string]int)
	for _, edges := range adjacency {
		for _, edge := range edges {
			incoming[edge.receipt.FinalCandidateTree]++
		}
	}
	for node := range pathNodes {
		wantOutgoing := 1
		if node == snapshot.CandidateTree {
			wantOutgoing = 0
		}
		if len(adjacency[node]) != wantOutgoing {
			return nil, nil, errors.New("compact receipt chain contains a fork")
		}
		wantIncoming := 1
		if node == path[0].receipt.BaseTree {
			wantIncoming = 0
		}
		if incoming[node] != wantIncoming {
			return nil, nil, errors.New("compact receipt chain contains a convergence")
		}
	}
	return path, candidates[0].compatibility, nil
}

func compactPrePRChainPaths(current, target string, adjacency map[string][]compactPrePRChainMember, visiting map[string]bool, path []compactPrePRChainMember, limit int) [][]compactPrePRChainMember {
	if current == target {
		return [][]compactPrePRChainMember{append([]compactPrePRChainMember(nil), path...)}
	}
	if visiting[current] || limit <= 0 {
		return nil
	}
	visiting[current] = true
	defer delete(visiting, current)
	result := [][]compactPrePRChainMember{}
	for _, member := range adjacency[current] {
		result = append(result, compactPrePRChainPaths(member.receipt.FinalCandidateTree, target, adjacency, visiting, append(path, member), limit-1)...)
		if len(result) >= 2 {
			return result
		}
	}
	return result
}

func compactPrePRChainHasCycle(adjacency map[string][]compactPrePRChainMember) bool {
	visited := map[string]bool{}
	active := map[string]bool{}
	var visit func(string) bool
	visit = func(tree string) bool {
		if active[tree] {
			return true
		}
		if visited[tree] {
			return false
		}
		visited[tree], active[tree] = true, true
		for _, member := range adjacency[tree] {
			if visit(member.receipt.FinalCandidateTree) {
				return true
			}
		}
		active[tree] = false
		return false
	}
	for tree := range adjacency {
		if visit(tree) {
			return true
		}
	}
	return false
}

func buildCompactPrePRChainProof(ctx context.Context, repo string, request GateRequest, refs *resolvedPrePRRefs, snapshot Snapshot, authority []compactPrePRChainAuthorityProof, members []compactPrePRChainMember, compatibility *BaseAdvanceCompatibility) (compactPrePRChainProof, error) {
	baseCommit := refs.BaseCommit
	if compatibility != nil {
		mergeBase, err := runGit(ctx, repo, nil, nil, "merge-base", refs.Selection.Commit, refs.HeadCommit)
		if err != nil {
			return compactPrePRChainProof{}, err
		}
		baseCommit = strings.TrimSpace(string(mergeBase))
	}
	baseTree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, baseCommit)
	if err != nil || baseTree != members[0].receipt.BaseTree {
		return compactPrePRChainProof{}, errors.New("first compact receipt does not match the resolved publication base")
	}
	commits, err := compactPrePRLinearCommits(ctx, repo, baseCommit, refs.HeadCommit)
	if err != nil {
		return compactPrePRChainProof{}, err
	}
	memberProofs := make([]compactPrePRChainMemberProof, 0, len(members))
	genesisUnion := []string{}
	previousTree, previousIndex := baseTree, -1
	for _, member := range members {
		if member.receipt.BaseTree != previousTree {
			return compactPrePRChainProof{}, errors.New("compact receipt chain contains a tree gap or reordered member")
		}
		boundary := -1
		for index := previousIndex + 1; index < len(commits); index++ {
			if commits[index].Tree == previousTree {
				continue
			}
			if commits[index].Tree != member.receipt.FinalCandidateTree {
				return compactPrePRChainProof{}, errors.New("publication commit does not match the next compact receipt boundary")
			}
			boundary = index
			break
		}
		if boundary < 0 {
			return compactPrePRChainProof{}, errors.New("compact receipt boundary is absent from publication history")
		}
		changed, err := (SnapshotBuilder{Repo: repo}).changedPaths(ctx, member.receipt.BaseTree, member.receipt.FinalCandidateTree)
		if err != nil {
			return compactPrePRChainProof{}, err
		}
		touched := []string{}
		for index := previousIndex + 1; index <= boundary; index++ {
			paths, err := (SnapshotBuilder{Repo: repo}).changedPaths(ctx, commits[index].ParentTree, commits[index].Tree)
			if err != nil {
				return compactPrePRChainProof{}, err
			}
			touched = append(touched, paths...)
		}
		touched, err = compactPrePRPathUnion(touched)
		if err != nil || pathsAreSubset(changed, member.record.State.GenesisPaths) != nil || pathsAreSubset(touched, member.record.State.GenesisPaths) != nil {
			return compactPrePRChainProof{}, errors.New("compact receipt segment exceeds its immutable genesis paths")
		}
		memberProofs = append(memberProofs, compactPrePRChainMemberProof{
			LineageID: member.receipt.LineageID, Revision: member.record.Revision, Receipt: member.receipt,
			GenesisPaths: append([]string(nil), member.record.State.GenesisPaths...),
			ChangedPaths: changed, TouchedPaths: touched,
		})
		genesisUnion = append(genesisUnion, member.record.State.GenesisPaths...)
		previousTree, previousIndex = member.receipt.FinalCandidateTree, boundary
	}
	for index := previousIndex + 1; index < len(commits); index++ {
		if commits[index].Tree != previousTree {
			return compactPrePRChainProof{}, errors.New("publication commit remains after all compact receipt boundaries")
		}
	}
	if previousTree != snapshot.CandidateTree {
		return compactPrePRChainProof{}, errors.New("compact receipt chain final tree does not equal live HEAD")
	}
	changed, err := (SnapshotBuilder{Repo: repo}).changedPaths(ctx, baseTree, snapshot.CandidateTree)
	if err != nil {
		return compactPrePRChainProof{}, err
	}
	touched := []string{}
	for _, commit := range commits {
		paths, err := (SnapshotBuilder{Repo: repo}).changedPaths(ctx, commit.ParentTree, commit.Tree)
		if err != nil {
			return compactPrePRChainProof{}, err
		}
		touched = append(touched, paths...)
	}
	touched, err = compactPrePRPathUnion(touched)
	if err != nil {
		return compactPrePRChainProof{}, err
	}
	publicationPaths, err := compactPrePRPathUnion(changed, touched)
	if err != nil {
		return compactPrePRChainProof{}, err
	}
	genesisUnion, err = compactPrePRPathUnion(genesisUnion)
	if err != nil || pathsAreSubset(publicationPaths, genesisUnion) != nil {
		return compactPrePRChainProof{}, errors.New("publication range contains paths outside composed receipt authority")
	}
	proof := compactPrePRChainProof{
		Domain: CompactPrePRChainProofDomain, Boundary: refs.Selection,
		PushRemote: request.PrePR.PushRemote, PushRemoteIdentity: request.PrePR.PushRemoteIdentity,
		BaseCommit: baseCommit, HeadCommit: refs.HeadCommit, BaseTree: baseTree, HeadTree: snapshot.CandidateTree,
		Authority: authority, Members: memberProofs, Commits: commits,
		ChangedPaths: changed, TouchedPaths: touched, PublicationPaths: publicationPaths, BaseAdvance: compatibility,
	}
	return proof, nil
}

func compactPrePRLinearCommits(ctx context.Context, repo, baseCommit, headCommit string) ([]compactPrePRChainCommitProof, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-list", "--reverse", "--parents", baseCommit+".."+headCommit)
	if err != nil {
		return nil, fmt.Errorf("inspect compact receipt publication commits: %w", err)
	}
	previousCommit := baseCommit
	previousTree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, baseCommit)
	if err != nil {
		return nil, err
	}
	commits := []compactPrePRChainCommitProof{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || fields[1] != previousCommit {
			return nil, errors.New("compact receipt composition requires one exact linear publication ancestry")
		}
		tree, err := (SnapshotBuilder{Repo: repo}).resolveTree(ctx, fields[0])
		if err != nil {
			return nil, err
		}
		commits = append(commits, compactPrePRChainCommitProof{Commit: fields[0], Parent: previousCommit, ParentTree: previousTree, Tree: tree})
		previousCommit, previousTree = fields[0], tree
	}
	if len(commits) == 0 || previousCommit != headCommit {
		return nil, errors.New("compact receipt publication history does not reach live HEAD")
	}
	return commits, nil
}

func compactPrePRChainProofIdentity(proof compactPrePRChainProof) (string, error) {
	payload, err := json.Marshal(proof)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(CompactPrePRChainProofDomain+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func compactPrePRChainPayloadHash(kind string, payload []byte) string {
	sum := sha256.Sum256(append([]byte(CompactPrePRChainProofDomain+"/"+kind+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func compactPrePRChainValuesHash(kind string, values []string) string {
	payload, _ := json.Marshal(values)
	return compactPrePRChainPayloadHash(kind, payload)
}

func compactPrePRPathUnion(groups ...[]string) ([]string, error) {
	unique := map[string]struct{}{}
	for _, group := range groups {
		for _, path := range group {
			unique[path] = struct{}{}
		}
	}
	paths := make([]string, 0, len(unique))
	for path := range unique {
		paths = append(paths, path)
	}
	return canonicalPaths(paths)
}
