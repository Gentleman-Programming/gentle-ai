package reviewtransaction

import (
	"bytes"
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
	if err := validateCompactReceiptMirrorScope(ctx, repo, state, request); err != nil {
		assessment.Applicability = CompactGateTargetScopeChanged
		return assessment, nil
	}
	snapshot, resolvedPrePR, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		return assessment, fmt.Errorf("build compact gate target: %w", err)
	}
	assessment.Actual = snapshot
	var compatibility *BaseAdvanceCompatibility
	if (request.Gate == GatePreCommit || request.Gate == GatePrePush) && state.CurrentSnapshot.Kind == TargetBaseDiff && state.Recovery == nil && strings.TrimSpace(input.BaseRef) != "" {
		if receipt, receiptErr := state.Receipt(); receiptErr == nil {
			proof, proofErr := deriveExplicitBaseAdvanceCompatibility(ctx, repo, Receipt{
				BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest,
			}, request, snapshot, gateArtifactPreimages{})
			if proofErr == nil {
				compatibility = &proof
			}
		}
	}
	if compatibility != nil {
		proofExpected := state.CurrentSnapshot
		proofExpected.Projection = snapshot.Projection
		if classifyCompactTargetRelation(proofExpected, snapshot, state.GenesisPaths,
			compactTargetRelationEvidence{CompatibleAdvance: compatibility}).Kind == compactTargetCompatibleAdvance {
			assessment.Applicability = CompactGateTargetExact
			return assessment, nil
		}
	}
	subsetProof, err := proveCompactPrePushMonotonicSubset(ctx, repo, state, state.CurrentSnapshot.CandidateTree, state.InitialSnapshot.BaseTree, input, snapshot, resolvedPrePR)
	if err != nil {
		return assessment, fmt.Errorf("prove compact pre-push monotonic subset: %w", err)
	}
	if subsetProof.Allowed {
		assessment.Applicability = CompactGateTargetExact
		return assessment, nil
	}
	squashedFixDelivery := compactSquashedFixDelivery(request.Gate, state, snapshot, resolvedPrePR, state.CurrentSnapshot.CandidateTree)
	strictBinding := request.Gate == GatePostApply || request.Gate == GatePreCommit ||
		request.Gate == GatePrePush && state.InitialSnapshot.Kind != TargetCurrentChanges
	pathsMatch := pathsAreSubset(snapshot.Paths, state.GenesisPaths) == nil
	baseMatches := snapshot.BaseTree == state.CurrentSnapshot.BaseTree || request.Target.Kind == TargetFixDiff || squashedFixDelivery
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
	relationExpected := state.CurrentSnapshot
	// PRE-COMMIT deliberately projects the frozen workspace authority through
	// the staged index. The request builder owns that projection choice, so the
	// relation algebra compares content/scope inside the selected gate
	// projection rather than treating the gate's own staged view as unrelated.
	relationExpected.Projection = snapshot.Projection
	relation := classifyCompactTargetRelation(relationExpected, snapshot, state.GenesisPaths, compactTargetRelationEvidence{})
	if relation.Kind != compactTargetUnsafe {
		assessment.Applicability = CompactGateTargetScopeChanged
		return assessment, nil
	}
	assessment.Applicability = CompactGateTargetUnrelated
	return assessment, nil
}

func compactSquashedFixDelivery(gate GateKind, state CompactState, snapshot Snapshot, refs *resolvedPrePRRefs, finalCandidateTree string) bool {
	return gate == GatePrePush && state.CurrentSnapshot.Kind == TargetFixDiff && refs != nil && refs.DeliveredCommitCount == 1 &&
		snapshot.CandidateTree == finalCandidateTree && snapshot.BaseTree == state.InitialSnapshot.BaseTree &&
		equalStrings(snapshot.Paths, state.GenesisPaths) && snapshot.PathsDigest == digestPaths(state.GenesisPaths)
}

func EvaluateCompactGate(ctx context.Context, repo string, receipt CompactReceipt, input NativeGateRequestInput) NativeGateEvaluation {
	return attachGateVerdictRelation(evaluateCompactGate(ctx, repo, receipt, input, false))
}

// attachGateVerdictRelation is Wave 5 Slice 3's additive wiring point
// (design decision 3, task 4.2): it populates the new Relation/Next fields
// for the denial shapes gateVerdict already covers this slice -- the
// "changed" relation (candidate-or-paths-mismatch and base-mismatch
// denials, the exact two the 5 named per-gate deny goldens exercise). It
// NEVER changes Result, Reason, or Context -- only adds the two new
// fields -- so every existing composite literal and test stays
// byte-identical. Full relation classification for every other outcome
// (exact, compatible_base_advance, ambiguous, unknown, etc.) is
// deliberately NOT wired here yet: see NativeGateEvaluation's own doc
// comment (gate.go) for why guessing those classifications now would
// violate the matrix harness's "never a fabricated pass" rule.
func attachGateVerdictRelation(evaluation NativeGateEvaluation) NativeGateEvaluation {
	if evaluation.Context.Denial == nil {
		return evaluation
	}
	var relation CandidateRelation
	switch {
	case evaluation.Result == GateScopeChanged && evaluation.Context.Denial.Code == "candidate-or-paths-mismatch":
		relation = ShadowRelationChanged
	case evaluation.Result == GateInvalidated && evaluation.Context.Denial.Code == "base-mismatch":
		relation = ShadowRelationChanged
	default:
		return evaluation
	}
	_, next := gateVerdict(evaluation.Context.Gate, relation, evaluation.Context)
	evaluation.Relation = relation
	evaluation.Next = &next
	return evaluation
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
	// Scoped to the receipt's own lineage: the gate is authorizing exactly
	// this candidate through exactly this authority chain, so a defect on a
	// branch that chain never inherits from is not evidence about it. Asking
	// the whole repository instead is what let one historical entry deny
	// delivery for every unrelated candidate in every worktree (2086, 2241).
	if err := CompactAuthorityLineageBlocked(ctx, repo, receipt.LineageID); err != nil {
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
		// The denial context is already derived above, and an escalated denial
		// needs it more than most: the only continuation out of a terminal
		// escalation is a successor authority, and its two selectors are this
		// lineage and this store revision. Dropping the context published an
		// all-zero envelope and left the human surface with nothing to name.
		return NativeGateEvaluation{Result: GateEscalated, Reason: compactEscalatedGateReason(record.State), Context: denialContext}
	}
	if (input.Gate == GatePrePush || input.Gate == GatePrePR) && record.State.InitialSnapshot.Kind == TargetCurrentChanges {
		emptyTree, emptyTreeErr := (SnapshotBuilder{Repo: repo}).emptyTree(ctx)
		if emptyTreeErr != nil {
			return invalid("repository empty tree cannot be derived: "+emptyTreeErr.Error(), emptyTreeErr)
		}
		if record.State.InitialSnapshot.UnbornHead && record.State.InitialSnapshot.BaseTree == emptyTree {
			return invalid("first publication cannot be derived from an empty-base review receipt; commit an authorized empty root, then run gentle-ai review start --committed-only with --base-ref set to that commit's SHA")
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
	if err := validateCompactReceiptMirrorScope(ctx, repo, record.State, request); err != nil {
		denialContext.Denial = &GateDenial{Stage: "scope-validation", Code: "receipt-mirror-mismatch"}
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
		// staged. Guard exactly like validateCompactReceiptMirrorScope above:
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
	subsetProof, subsetProofErr := proveCompactPrePushMonotonicSubset(ctx, repo, record.State, receipt.FinalCandidateTree, receipt.BaseTree, input, snapshot, resolvedPrePR)
	if subsetProofErr != nil {
		return invalid("compact pre-push monotonic subset cannot be proven: "+subsetProofErr.Error(), subsetProofErr)
	}
	// An empty-remote bootstrap publishes the candidate's complete history,
	// so publication-range validation is mandatory for every target kind —
	// including current-changes; no kind may skip it.
	bootstrapPublication := resolvedPrePR != nil && resolvedPrePR.Selection.Source == PrePRBoundaryEmptyRemoteBootstrap
	validatePublicationRange := request.Gate == GatePrePush && (record.State.InitialSnapshot.Kind == TargetBaseDiff || bootstrapPublication) ||
		record.State.InitialSnapshot.Kind == TargetBaseWorkspaceOverlay && (request.Gate == GatePrePush || request.Gate == GatePrePR)
	if validatePublicationRange && !subsetProof.Allowed {
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
	recoveryBinding, recoveryBound, recoveryErr := deriveCompactRecoveryBinding(ctx, repo, record.State)
	if recoveryErr != nil {
		return invalid("compact recovery binding cannot be derived during authorization")
	}
	recoveryAdvance := request.Gate == GatePrePR && recoveryBound && snapshot.BaseTree != recoveryBinding.BaseTree
	localAdvance := (request.Gate == GatePreCommit || request.Gate == GatePrePush) && record.State.CurrentSnapshot.Kind == TargetBaseDiff && record.State.Recovery == nil && strings.TrimSpace(input.BaseRef) != ""
	compatibleAdvance := false
	var compatibility *BaseAdvanceCompatibility
	var recoveryCompatibility *BaseAdvanceCompatibility
	if recoveryAdvance {
		proof, proofErr := deriveCompactRecoveryAdvanceCompatibility(ctx, repo, recoveryBinding, request, snapshot, resolvedPrePR, preimages)
		if proofErr != nil {
			denialContext.Denial = &GateDenial{Stage: "base-advance", Code: "unproven"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "compatible pre-PR recovery base advance cannot be proven: " + proofErr.Error(), Context: denialContext}
		}
		compatibility = &proof
		recoveryCompatibility = &proof
		compatibleAdvance = proof.Compatible
	} else if localAdvance {
		legacyShape := Receipt{BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest}
		proof, proofErr := deriveExplicitBaseAdvanceCompatibility(ctx, repo, legacyShape, request, snapshot, preimages)
		if proofErr != nil {
			denialContext.Denial = &GateDenial{Stage: "base-advance", Code: "unproven"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "compatible local base advance cannot be proven: " + proofErr.Error(), Context: denialContext}
		}
		compatibility = &proof
		compatibleAdvance = proof.Compatible
	} else if request.Gate == GatePrePR && prePRBoundaryAdvanced(resolvedPrePR) && snapshot.CandidateTree == receipt.FinalCandidateTree && snapshot.PathsDigest == receipt.PathsDigest {
		if record.State.InitialSnapshot.Kind == TargetCurrentChanges {
			proof, proofErr := deriveCurrentChangesBoundaryCompatibility(ctx, repo, record.State, request, snapshot, resolvedPrePR)
			if proofErr != nil {
				denialContext.Denial = &GateDenial{Stage: "base-advance", Code: "unproven"}
				return NativeGateEvaluation{Result: GateInvalidated, Reason: "compatible pre-PR base advance cannot be proven: " + proofErr.Error(), Context: denialContext}
			}
			compatibility = &proof
			compatibleAdvance = proof.Compatible
		} else {
			legacyShape := Receipt{BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest}
			proof, proofErr := deriveBaseAdvanceCompatibility(ctx, repo, legacyShape, request, snapshot, resolvedPrePR, preimages, prePRAttestationRequested(request))
			if proofErr != nil {
				denialContext.Denial = &GateDenial{Stage: "base-advance", Code: "unproven"}
				return NativeGateEvaluation{Result: GateInvalidated, Reason: "compatible pre-PR base advance cannot be proven: " + proofErr.Error(), Context: denialContext}
			}
			compatibility = &proof
			compatibleAdvance = proof.Compatible
		}
	}
	binding := record.State.CurrentSnapshot
	squashedFixDelivery := compactSquashedFixDelivery(request.Gate, record.State, snapshot, resolvedPrePR, receipt.FinalCandidateTree)
	strictBinding := request.Gate == GatePostApply || request.Gate == GatePreCommit || request.Gate == GatePrePush && record.State.InitialSnapshot.Kind != TargetCurrentChanges
	baseRelationshipValid := snapshot.BaseTree == receipt.BaseTree || request.Target.Kind == TargetFixDiff || squashedFixDelivery
	if strictBinding {
		baseRelationshipValid = snapshot.BaseTree == binding.BaseTree || squashedFixDelivery
	}
	if subsetProof.Allowed {
		baseRelationshipValid = true
	}
	if compatibleAdvance {
		baseRelationshipValid = true
	}
	gateContext := GateContext{
		Gate: request.Gate, LineageID: receipt.LineageID, Generation: receipt.Generation,
		StoreRevision: record.Revision, GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundleDigest: record.Revision,
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
		FixDeltaHash: record.State.FixDeltaHash, PolicyHash: record.State.PolicyHash,
		LedgerHash: ledgerHash, EvidenceHash: record.State.EvidenceHash,
		BaseRelationshipValid: baseRelationshipValid, BaseAdvance: compatibility,
	}
	if snapshot.BaseTree != receipt.BaseTree {
		// The base_tree emitted above is the derived target's base. When that
		// is not the tree the review was performed against -- the fix-diff
		// base a correction receipt derives is the case that reached the
		// community -- name the reviewed base too, instead of leaving one
		// field name covering two different values.
		gateContext.ReceiptBaseTree = receipt.BaseTree
	}
	if request.Gate == GatePrePR && resolvedPrePR != nil {
		boundary := resolvedPrePR.Selection
		gateContext.PrePRBoundary = &boundary
	}
	pathsMismatch := pathsAreSubset(snapshot.Paths, record.State.GenesisPaths) != nil && !compatibleAdvance
	if strictBinding {
		pathsMismatch = snapshot.PathsDigest != binding.PathsDigest && !squashedFixDelivery && !compatibleAdvance
	}
	// expectedBaseTree is the single comparand the base check below uses, so
	// the value published in the denial is by construction the value the gate
	// required. Deriving the expectation separately would let the two drift,
	// and a denial that names an expectation it did not enforce is worse than
	// one that names none. The two branches are exactly the previous
	// receipt/binding split -- this selects nothing new.
	expectedBaseTree := receipt.BaseTree
	if strictBinding {
		expectedBaseTree = binding.BaseTree
	}
	baseMismatch := snapshot.BaseTree != expectedBaseTree && request.Target.Kind != TargetFixDiff && !compatibleAdvance
	if strictBinding {
		baseMismatch = snapshot.BaseTree != expectedBaseTree && !squashedFixDelivery && !compatibleAdvance
	}
	// A scope_changed recovery successor freezes only its own pristine scope,
	// so a delivery already covered by its receipt-bound predecessors would be
	// denied here forever. Rebind through the composed recovery chain when the
	// leaf's approved candidate is exactly the delivered tree and native Git
	// evidence proves the chain covers the complete publication range.
	var recoveryRebind *compactRecoveryBinding
	if (request.Gate == GatePrePush || request.Gate == GatePrePR) && record.State.Recovery != nil && resolvedPrePR != nil &&
		(pathsMismatch || baseMismatch || recoveryAdvance && compatibleAdvance) && snapshot.CandidateTree == receipt.FinalCandidateTree {
		rebindBaseCommit := resolvedPrePR.BaseCommit
		if request.Gate == GatePrePush {
			rebindBaseCommit = request.Target.BaseRef
		}
		if chain, ok := rebindCompactRecoveryDeliveryWithCompatibility(ctx, repo, record.State, snapshot, receipt.FinalCandidateTree, rebindBaseCommit, resolvedPrePR.HeadCommit, recoveryCompatibility); ok {
			recoveryRebind = &chain
			pathsMismatch = false
			gateContext.BaseRelationshipValid = true
			gateContext.FixDeltaHash = chain.FixDeltaHash
		}
	}
	if recoveryAdvance && recoveryRebind == nil {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "recovery-chain-advance-unproven"}
		return NativeGateEvaluation{Result: GateInvalidated, Reason: "advanced recovery delivery requires trusted compatibility and full-chain verification", Context: gateContext}
	}
	// guard:population reviewed-subset-delivery too-tight: legitimate pre-push deliveries are strict immutable receipt-scope subsets with a proven monotonic reviewed base-to-final history
	if !subsetProof.Allowed && ((snapshot.CandidateTree != receipt.FinalCandidateTree && !compatibleAdvance) || pathsMismatch) {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "candidate-or-paths-mismatch"}
		diagnostics, diagnosticsErr := CompactScopeChangeDiagnostics(ctx, repo, record.State, record.Revision, snapshot, request.Gate)
		if diagnosticsErr != nil {
			gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "scope-diagnostics-unavailable"}
			return NativeGateEvaluation{Result: GateInvalidated, Reason: "exact scope-change diagnostics cannot be derived: " + diagnosticsErr.Error(), Context: gateContext, Cause: diagnosticsErr}
		}
		gateContext.ScopeChange = &diagnostics
		return NativeGateEvaluation{Result: GateScopeChanged, Reason: nativeGateReason(GateScopeChanged), Context: gateContext}
	}
	if !subsetProof.Allowed && baseMismatch && recoveryRebind == nil {
		gateContext.Denial = &GateDenial{Stage: "receipt-binding", Code: "base-mismatch"}
		// Publish both sides, exactly as the sibling candidate-or-paths
		// denial already does. Without the expectation the envelope repeats
		// the base the operator supplied and says nothing about the one it
		// wanted, which reads as the gate contradicting itself.
		gateContext.BaseMismatch = &GateBaseMismatchDiagnostics{Expected: expectedBaseTree, Actual: snapshot.BaseTree}
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
		// This window is a re-derivation, not a mutation: everything below the
		// lock only reads. Transient contention with a concurrent writer
		// therefore says nothing about the candidate, and reporting it as
		// `invalidated` told an operator the receipt was damaged when the
		// authority was healthy the whole time (1861). Wait it out first; only
		// a wait that genuinely elapses is reported, and it is reported as a
		// non-verdict the caller retries rather than as damage.
		lock, lockErr := acquireStoreLockForReadOnlyEvaluation(ctx, store.lockPath)
		if lockErr != nil {
			var contended *AuthorityLockTimeoutError
			if errors.As(lockErr, &contended) {
				evaluation := invalid("compact authority lock is held by a concurrent review operation", lockErr)
				evaluation.Contended = true
				return evaluation
			}
			return invalid("compact authority changed during final authorization", lockErr)
		}
		defer lock.release()
	}
	finalGateAuthorizationHook()
	finalRecord, loadErr := store.Load()
	finalSnapshot, finalRefs, snapshotErr := buildCompactLifecycleSnapshot(ctx, repo, request)
	finalMirrorErr := validateCompactReceiptMirrorScope(ctx, repo, record.State, request)
	finalTrackedErr := validateCompactCommittedTrackedScope(ctx, repo, request)
	graphErr := CompactAuthorityLineageBlocked(ctx, repo, receipt.LineageID)
	finalSuperseded, supersededErr := CompactLineageSuperseded(ctx, repo, receipt.LineageID)
	finalSubsetProof, finalSubsetProofErr := proveCompactPrePushMonotonicSubset(ctx, repo, finalRecord.State, receipt.FinalCandidateTree, receipt.BaseTree, input, finalSnapshot, finalRefs)
	if loadErr != nil || snapshotErr != nil || finalMirrorErr != nil || finalTrackedErr != nil || graphErr != nil || supersededErr != nil || finalSubsetProofErr != nil || finalSuperseded || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalSnapshot, snapshot) || !sameResolvedPrePRRefs(finalRefs, resolvedPrePR) || !reflect.DeepEqual(finalSubsetProof, subsetProof) {
		cause := errors.Join(loadErr, snapshotErr, finalMirrorErr, finalTrackedErr, graphErr, supersededErr, finalSubsetProofErr)
		if cause == nil {
			cause = ErrConcurrentUpdate
		}
		return invalid("compact authority or repository target changed during final authorization", cause)
	}
	finalCompactGateAllowHook()
	finalRecoveryBinding := recoveryBinding
	if recoveryBound {
		finalChain, ok, chainErr := deriveCompactRecoveryBinding(ctx, repo, finalRecord.State)
		if chainErr != nil || !ok || !reflect.DeepEqual(finalChain, recoveryBinding) {
			if chainErr == nil {
				chainErr = ErrConcurrentUpdate
			}
			return invalid("compact authority or repository target changed during final authorization", chainErr)
		}
		finalRecoveryBinding = finalChain
	}
	if compatibility != nil {
		finalPreimages, preimageErr := rereadGateArtifactPreimages(request)
		var finalCompatibility BaseAdvanceCompatibility
		compatibilityErr := preimageErr
		if compatibilityErr == nil {
			if recoveryAdvance {
				finalCompatibility, compatibilityErr = deriveCompactRecoveryAdvanceCompatibility(ctx, repo, finalRecoveryBinding, request, finalSnapshot, finalRefs, finalPreimages)
			} else if localAdvance {
				legacyShape := Receipt{BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest}
				finalCompatibility, compatibilityErr = deriveExplicitBaseAdvanceCompatibility(ctx, repo, legacyShape, request, finalSnapshot, finalPreimages)
			} else if record.State.InitialSnapshot.Kind == TargetCurrentChanges {
				finalCompatibility, compatibilityErr = deriveCurrentChangesBoundaryCompatibility(ctx, repo, finalRecord.State, request, finalSnapshot, finalRefs)
			} else {
				legacyShape := Receipt{BaseTree: receipt.BaseTree, FinalCandidateTree: receipt.FinalCandidateTree, PathsDigest: receipt.PathsDigest}
				finalCompatibility, compatibilityErr = deriveBaseAdvanceCompatibility(ctx, repo, legacyShape, request, finalSnapshot, finalRefs, finalPreimages, prePRAttestationRequested(request))
			}
		}
		if compatibilityErr != nil || finalCompatibility != *compatibility {
			if compatibilityErr == nil {
				compatibilityErr = ErrConcurrentUpdate
			}
			return invalid("compatible base evidence changed during final authorization", compatibilityErr)
		}
		if recoveryAdvance {
			recoveryCompatibility = &finalCompatibility
		}
	}
	// Re-verify the composed chain under the lock exactly when the evaluation
	// above rested on it. `recoveryBound` only says a chain composes; it does
	// not say this delivery is authorized by one. A successor whose own frozen
	// receipt already binds the delivery exactly (candidate, base, and paths
	// all match) is authorized by that receipt alone, and whole-chain delivery
	// verification is unsatisfiable for it once the predecessor's segment has
	// been published: the composed chain base is an ancestor of the live
	// publication base, so `verifyCompactRecoveryDelivery` can never find it.
	// Gating on `recoveryBound` therefore blocked precisely the recovery the
	// scope-change denial had just told the operator to run — a named
	// continuation that walks into a second wall. Gating on the rebind keeps
	// the check wherever the allow actually depends on the chain: any delivery
	// spanning a predecessor segment mismatches the leaf's frozen base, which
	// is what sets `recoveryRebind` in the first place.
	if recoveryRebind != nil && finalRefs != nil && (request.Gate == GatePrePush || request.Gate == GatePrePR) {
		baseCommit := finalRefs.BaseCommit
		if request.Gate == GatePrePush {
			baseCommit = request.Target.BaseRef
		}
		// A refusal that discards its cause is a defect even when the refusal
		// is right: keep the verifier's specific reason on the evaluation.
		if deliveryErr := verifyCompactRecoveryRelationDelivery(ctx, repo, finalRecoveryBinding, finalSnapshot, receipt.FinalCandidateTree, baseCommit, finalRefs.HeadCommit, recoveryCompatibility); deliveryErr != nil {
			return invalid("compact recovery delivery changed during final authorization: "+deliveryErr.Error(), deliveryErr)
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
		// `required` is the operation's complete input set and stays that way
		// on the wire. Which of those the recovery mints for itself depends on
		// the predecessor state, so it is derived here rather than assumed.
		RecoveryDerivedInputs: RecoverySelfDerivedInputs(state.State),
	}, nil
}

// CompactScopeChangeDiagnostics derives read-only recovery evidence for a
// previously assessed compact gate target. It never authorizes, locks, or
// mutates review authority.
//
// The gate is part of the derivation because the recovery a denial may name is
// gate-conditional. A bare `review recover` freezes a current-changes
// successor over the LIVE workspace, which is the right answer at post-apply
// and pre-commit, where the drifted work is still dirty. At pre-push over a
// current-changes receipt it is the wrong answer: the delivery is already
// committed, the workspace is clean, so the bare successor freezes an empty
// scope (base_tree == candidate_tree) that re-trips the same one-commit
// delivery rule the denial came from. For that case, and only that case, the
// diagnostics additionally name the committed base-diff shape and the derived
// publication boundary it must be frozen against.
func CompactScopeChangeDiagnostics(ctx context.Context, repo string, state CompactState, revision string, actual Snapshot, gate GateKind) (GateScopeChangeDiagnostics, error) {
	diagnostics, err := buildCompactScopeChangeDiagnostics(ctx, repo, state, revision, actual)
	if err != nil {
		return GateScopeChangeDiagnostics{}, err
	}
	if baseRef, _, ok := compactCommittedDeliveryRecovery(ctx, repo, state, gate); ok {
		diagnostics.RecoveryScope, diagnostics.RecoveryBaseRef = RecoveryScopeCommittedBaseDiff, baseRef
	}
	return diagnostics, nil
}

// CompactDeliveryShapeScopeChangeDiagnostics derives recovery evidence for the
// pre-push scope-changed class whose target ASSESSMENT errored -- the
// deterministic one-commit delivery rule (ErrReviewedDeliveryNotOneCommit) and
// the ambiguous published delivery base (GateDeliveryBaseResolutionError) both
// fail before any actual snapshot exists, so buildCompactScopeChangeDiagnostics
// is not even callable there. It derives the actual side independently, from
// the publication boundary the pre-push gate itself binds to HEAD, which is
// simultaneously the evidence and the scope the recommended recovery freezes.
//
// It returns an error -- never a fabricated recommendation -- whenever no
// correct recovery is derivable: a non-current-changes receipt (which never
// armed the one-commit rule), an unresolvable or bootstrap-empty publication
// boundary, or a boundary that already equals the candidate, meaning the
// delivery is fully published and there is nothing left for a successor to
// freeze. Callers keep the honest terminal fallback in those cases.
func CompactDeliveryShapeScopeChangeDiagnostics(ctx context.Context, repo string, state CompactState, revision string, gate GateKind) (GateScopeChangeDiagnostics, error) {
	baseRef, actual, ok := compactCommittedDeliveryRecovery(ctx, repo, state, gate)
	if !ok {
		return GateScopeChangeDiagnostics{}, errors.New("no committed base-diff recovery is derivable for this delivery")
	}
	diagnostics, err := buildCompactScopeChangeDiagnostics(ctx, repo, state, revision, actual)
	if err != nil {
		return GateScopeChangeDiagnostics{}, err
	}
	diagnostics.RecoveryScope, diagnostics.RecoveryBaseRef = RecoveryScopeCommittedBaseDiff, baseRef
	return diagnostics, nil
}

// compactCommittedDeliveryRecovery derives the committed base-diff recovery a
// pre-push scope-changed denial can honestly name, together with the successor
// scope that recovery would freeze.
//
// The gate predicate is compactPushDeliveryBaseTree's own: only a
// current-changes receipt binds a one-commit delivery to its frozen base, so
// only that receipt shape can be blocked by a delivery the bare recovery
// cannot express. The returned base ref is the merge base of HEAD and the
// resolved publication boundary -- an immutable commit id, never a remote ref
// name that could move between reading this message and running the recovery,
// and never a hardcoded default branch.
//
// It reports false, and therefore no recommendation, on every condition it
// cannot prove: a non-pre-push gate, a non-current-changes receipt, an
// unresolvable boundary or HEAD, a non-unique merge base (which the gate
// itself already refuses), an unbuildable base-diff snapshot, an empty
// base-diff whose successor would freeze nothing, a predecessor that is not in
// a state a scope-changed recovery accepts, and -- the load-bearing one -- a
// derived successor scope the recovery authority itself would refuse. The last
// check is validateCompactRecoveryEdge's own predicate, evaluated here before
// anything is named, so a denial can never point at a `review recover` that
// exits non-zero with "approved predecessor scope has not changed". That case
// is real: a delivery split across a number of commits other than one, but
// carrying byte-identical content to the approved candidate, changes only the
// commit topology, and no single recover invocation expresses it.
func compactCommittedDeliveryRecovery(ctx context.Context, repo string, state CompactState, gate GateKind) (string, Snapshot, bool) {
	if gate != GatePrePush || compactPushDeliveryBaseTree(state) == "" || state.State != StateApproved {
		return "", Snapshot{}, false
	}
	selection, err := selectPrePushBoundary(ctx, repo, "")
	if err != nil || selection.Source == PrePRBoundaryEmptyRemoteBootstrap {
		// An empty remote has no publication range to diff against, so no
		// base-diff recovery exists to name.
		return "", Snapshot{}, false
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return "", Snapshot{}, false
	}
	output, err := runGit(ctx, repo, nil, nil, "merge-base", "--all", head, selection.Commit)
	bases := strings.Fields(string(output))
	if err != nil || len(bases) != 1 {
		return "", Snapshot{}, false
	}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(ctx, Target{Kind: TargetBaseDiff, BaseRef: bases[0], IntendedUntracked: []string{}})
	if err != nil || snapshot.BaseTree == snapshot.CandidateTree {
		return "", Snapshot{}, false
	}
	if !compactRecoveryScopeChanged(state.CurrentSnapshot, snapshot) {
		return "", Snapshot{}, false
	}
	return bases[0], snapshot, true
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
		snapshot, err := (SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(ctx, request.Target)
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
		if input.Gate == GatePreCommit && strings.TrimSpace(input.BaseRef) != "" && current.Kind == TargetBaseDiff {
			request.Target = Target{Kind: TargetBaseWorkspaceOverlay, Projection: projection, BaseRef: input.BaseRef, IntendedUntracked: []string{}}
			break
		}
		if current.Kind == TargetFixDiff {
			request.Target = Target{
				Kind: TargetFixDiff, Projection: projection, BaseRef: current.BaseTree,
				IntendedUntracked: intended, LedgerIDs: append([]string(nil), current.LedgerIDs...),
			}
			break
		}
		if current.Kind == TargetBaseWorkspaceOverlay {
			request.Target = Target{Kind: TargetBaseWorkspaceOverlay, Projection: projection, BaseRef: state.InitialSnapshot.BaseTree, IntendedUntracked: intended}
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
// a cached-path inventory classifies the complete set in one subprocess; Git
// infrastructure failures propagate instead of misclassifying scope.
func compactStillUntrackedIntended(ctx context.Context, repo string, intended []string) ([]string, error) {
	remaining := []string{}
	if len(intended) == 0 {
		return remaining, nil
	}
	output, err := runGitInventory(ctx, repo, "ls-files", "--cached", "-z")
	if err != nil {
		return nil, fmt.Errorf("classify intended-untracked paths: %w", err)
	}
	tracked := nulSeparatedPathSet(output)
	for _, logicalPath := range intended {
		if _, isTracked := tracked[logicalPath]; !isTracked {
			remaining = append(remaining, logicalPath)
		}
	}
	return remaining, nil
}

// validateCompactReceiptMirrorScope is what survives of the untracked-scope
// gate after #2394. The gate used to deny whenever ANY unignored untracked
// path was absent from the frozen intended set, which only ever passed because
// START had swept every such path into that set. With the sweep gone, an
// untracked path cannot enter the derived candidate and therefore cannot
// violate scope, so the generic denial became a pure false blocker: one stray
// file in the worktree would have blocked every commit.
//
// A change-local receipt mirror is different in kind. It is not scope, it is
// authority evidence a later reader may believe, so a mirror that contradicts
// the authoritative receipt still fails closed.
func validateCompactReceiptMirrorScope(ctx context.Context, repo string, state CompactState, request GateRequest) error {
	if request.Target.Projection == ProjectionStaged || request.Gate != GatePostApply && request.Gate != GatePreCommit {
		return nil
	}
	live, err := (SnapshotBuilder{Repo: repo}).DiscoverUnignoredUntracked(ctx)
	if err != nil {
		return fmt.Errorf("discover current untracked paths: %w", err)
	}
	for _, path := range live {
		if isChangeLocalReceiptMirror(path) && !matchesAuthoritativeReceipt(repo, state, path) {
			return errors.New("change-local review receipt mirror does not match the authoritative receipt") // refusal:by-design world-action: only the operator can decide whether the divergent mirror file should be deleted or replaced, and no command of this product may overwrite evidence a human placed
		}
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
// base tree it binds, so every path touched in HEAD outside the reviewed and
// tracking reachability boundaries must stay inside the immutable genesis
// scope. For an empty-remote
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
	reviewed := refs.Selection.Commit
	if reviewed == "" || refs.Selection.Source == PrePRBoundaryEmptyRemoteBootstrap {
		reviewed = refs.BaseCommit
	}
	revisions := []string{refs.HeadCommit}
	exclusions := []string{reviewed}
	if refs.TrackingPresent {
		exclusions = append(exclusions, refs.TrackingBoundary.Commit)
	}
	seen := map[string]struct{}{}
	for _, excluded := range exclusions {
		if excluded == "" {
			continue
		}
		if _, duplicate := seen[excluded]; duplicate {
			continue
		}
		seen[excluded] = struct{}{}
		_, err := runGit(ctx, repo, nil, nil, "merge-base", "--is-ancestor", refs.HeadCommit, excluded)
		if err == nil {
			return errors.New("publication exclusion contains HEAD and would erase the publication range")
		}
		var commandErr *GitCommandError
		if !errors.As(err, &commandErr) || commandErr.ExitCode != 1 {
			return fmt.Errorf("validate publication exclusion ancestry: %w", err)
		}
		revisions = append(revisions, "^"+excluded)
	}
	paths, err := collectReviewedPublicationPaths(ctx, repo, revisions)
	if err == nil {
		err = pathsAreSubset(paths, genesis)
	}
	if err != nil {
		return fmt.Errorf("publication range exceeds immutable genesis scope: %w", err)
	}
	return nil
}

type compactPrePushMonotonicSubsetProof struct {
	Applies bool
	Allowed bool
	Reason  string
}

// proveCompactPrePushMonotonicSubset authorizes only a strict, already-reviewed
// publication suffix. It follows immutable Git content from the uniquely
// identified reviewed base to HEAD; branch, agent, and transport do not enter
// the proof.
func proveCompactPrePushMonotonicSubset(ctx context.Context, repo string, state CompactState, finalTree, reviewedBaseTree string, input NativeGateRequestInput, snapshot Snapshot, refs *resolvedPrePRRefs) (compactPrePushMonotonicSubsetProof, error) {
	proof := compactPrePushMonotonicSubsetProof{}
	if input.Gate != GatePrePush || state.InitialSnapshot.Kind == TargetCurrentChanges || state.Recovery != nil || len(snapshot.Paths) == 0 || len(snapshot.Paths) >= len(state.GenesisPaths) {
		return proof, nil
	}
	proof.Applies = true
	if pathsAreSubset(snapshot.Paths, state.GenesisPaths) != nil {
		proof.Reason = "outgoing paths are outside immutable receipt scope"
		return proof, nil
	}
	if snapshot.CandidateTree != finalTree {
		proof.Reason = "derived pre-push candidate does not match the approved final candidate"
		return proof, nil
	}
	output, err := runGit(ctx, repo, nil, nil, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return proof, fmt.Errorf("resolve pre-push HEAD tree: %w", err)
	}
	if strings.TrimSpace(string(output)) != finalTree {
		proof.Reason = "HEAD tree does not match the approved final candidate"
		return proof, nil
	}
	if refs == nil || !refs.TrackingPresent || refs.Selection.Commit == "" || refs.Selection.Commit != refs.TrackingBoundary.Commit || refs.HeadCommit == "" {
		proof.Reason = "tracking publication boundary is incomplete or ambiguous"
		return proof, nil
	}
	reviewedBase, found, err := compactUniqueCommitForTree(ctx, repo, reviewedBaseTree, refs.HeadCommit)
	if err != nil {
		return proof, err
	}
	if !found {
		proof.Reason = "reviewed base commit is missing or ambiguous in HEAD ancestry"
		return proof, nil
	}
	for _, pair := range [][2]string{{reviewedBase, refs.TrackingBoundary.Commit}, {refs.TrackingBoundary.Commit, refs.HeadCommit}} {
		ancestor, ancestorErr := compactCommitIsAncestor(ctx, repo, pair[0], pair[1])
		if ancestorErr != nil {
			return proof, ancestorErr
		}
		if !ancestor {
			proof.Reason = "reviewed base, tracking prefix, and HEAD do not form an ancestry chain"
			return proof, nil
		}
	}
	if reason, historyErr := validateCompactMonotonicHistory(ctx, repo, reviewedBase, refs.HeadCommit, state.GenesisPaths, reviewedBaseTree, finalTree); historyErr != nil {
		return proof, historyErr
	} else if reason != "" {
		proof.Reason = reason
		return proof, nil
	}
	proof.Allowed = true
	return proof, nil
}

func compactUniqueCommitForTree(ctx context.Context, repo, tree, head string) (string, bool, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-list", "--topo-order", "--no-commit-header", "--format=%H%x00%T", head)
	if err != nil {
		return "", false, fmt.Errorf("enumerate reviewed-base ancestry: %w", err)
	}
	matches := []string{}
	for _, pair := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(pair, "\x00", 2)
		if len(parts) == 2 && parts[1] == tree {
			matches = append(matches, parts[0])
		}
	}
	if len(matches) != 1 {
		return "", false, nil
	}
	return matches[0], true, nil
}

func compactCommitIsAncestor(ctx context.Context, repo, ancestor, descendant string) (bool, error) {
	_, err := runGit(ctx, repo, nil, nil, "merge-base", "--is-ancestor", ancestor, descendant)
	if err == nil {
		return true, nil
	}
	var commandErr *GitCommandError
	if errors.As(err, &commandErr) && commandErr.ExitCode == 1 {
		return false, nil
	}
	return false, fmt.Errorf("validate monotonic ancestry: %w", err)
}

func validateCompactMonotonicHistory(ctx context.Context, repo, base, head string, genesis []string, baseTree, finalTree string) (string, error) {
	baseEntries, err := listCompactContentTreeEntries(ctx, repo, baseTree)
	if err != nil {
		return "", err
	}
	finalEntries, err := listCompactContentTreeEntries(ctx, repo, finalTree)
	if err != nil {
		return "", err
	}
	output, err := runGit(ctx, repo, nil, nil, "rev-list", "--topo-order", "--reverse", base+".."+head)
	if err != nil {
		return "", fmt.Errorf("enumerate monotonic publication history: %w", err)
	}
	commits := strings.Fields(string(output))
	if len(commits) == 0 {
		return "reviewed base has no newly reachable publication commits", nil
	}
	approved := make(map[string]bool, len(genesis))
	for _, path := range genesis {
		approved[path] = true
	}
	known := map[string]bool{base: true}
	for _, commit := range commits {
		known[commit] = true
	}
	entries := map[string]map[string][]byte{base: baseEntries}
	for _, commit := range commits {
		current, currentErr := listCompactContentTreeEntries(ctx, repo, commit)
		if currentErr != nil {
			return "", currentErr
		}
		entries[commit] = current
		if reason := validateCompactEndpointEntries(baseEntries, finalEntries, current); reason != "" {
			return reason, nil
		}
		parentsOutput, parentsErr := runGit(ctx, repo, nil, nil, "rev-list", "--parents", "-n", "1", commit)
		if parentsErr != nil {
			return "", fmt.Errorf("read monotonic publication parents: %w", parentsErr)
		}
		parents := strings.Fields(string(parentsOutput))
		if len(parents) < 2 || parents[0] != commit {
			return "publication commit has a missing parent (shallow or grafted repository topology)", nil
		}
		for _, parent := range parents[1:] {
			if !known[parent] {
				return "publication commit parent falls outside reviewed-base ancestry", nil
			}
			parentEntries := entries[parent]
			if parentEntries == nil {
				parentEntries, currentErr = listCompactContentTreeEntries(ctx, repo, parent)
				if currentErr != nil {
					return "", currentErr
				}
				entries[parent] = parentEntries
			}
			if reason := validateCompactMonotonicEdge(baseEntries, finalEntries, approved, parentEntries, current); reason != "" {
				return reason, nil
			}
		}
	}
	return "", nil
}

func listCompactContentTreeEntries(ctx context.Context, repo, tree string) (map[string][]byte, error) {
	output, err := runGitInventory(ctx, repo, "ls-tree", "-r", "-z", tree)
	if err != nil {
		return nil, fmt.Errorf("list monotonic content tree entries: %w", err)
	}
	return parseTreeEntries(output)
}

func validateCompactEndpointEntries(base, final, current map[string][]byte) string {
	for _, path := range compactTreeEntryPaths(current) {
		if _, baseOK := base[path]; !baseOK {
			if _, finalOK := final[path]; !finalOK {
				return fmt.Sprintf("publication path %q is outside immutable receipt scope", path)
			}
		}
	}
	for _, path := range compactTreeEntryPaths(base, final) {
		if entry := current[path]; !bytes.Equal(entry, base[path]) && !bytes.Equal(entry, final[path]) {
			return fmt.Sprintf("publication entry at %q is outside reviewed base/final endpoints", path)
		}
	}
	return ""
}

func validateCompactMonotonicEdge(base, final map[string][]byte, approved map[string]bool, parent, current map[string][]byte) string {
	for _, path := range compactTreeEntryPaths(base, final, parent, current) {
		if bytes.Equal(parent[path], current[path]) {
			continue
		}
		if bytes.Equal(parent[path], base[path]) && bytes.Equal(current[path], final[path]) && approved[path] {
			continue
		}
		return fmt.Sprintf("publication edge reverses or extends reviewed entry at %q", path)
	}
	return ""
}

func compactTreeEntryPaths(entries ...map[string][]byte) []string {
	set := map[string]bool{}
	for _, tree := range entries {
		for path := range tree {
			set[path] = true
		}
	}
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func collectReviewedPublicationPaths(ctx context.Context, repo string, revisions []string) ([]string, error) {
	pathSet := func(args ...string) (map[string]bool, error) {
		output, err := runGit(ctx, repo, nil, nil, args...)
		if err != nil {
			return nil, err
		}
		paths := map[string]bool{}
		for _, path := range splitNullSeparatedPaths(output) {
			paths[path] = true
		}
		return paths, nil
	}
	args := append([]string{"log", "--no-merges", "--format=", "--name-only", "-z", "--no-renames"}, revisions...)
	paths, err := pathSet(args...)
	if err != nil {
		return nil, err
	}
	args = append([]string{"rev-list", "--merges"}, revisions...)
	output, err := runGit(ctx, repo, nil, nil, args...)
	if err != nil {
		return nil, err
	}
	for _, merge := range strings.Fields(string(output)) {
		output, err = runGit(ctx, repo, nil, nil, "rev-list", "--parents", "-n", "1", merge)
		if err != nil {
			return nil, err
		}
		parents := strings.Fields(string(output))
		if len(parents) != 3 || parents[0] != merge {
			return nil, fmt.Errorf("publication merge %s must have exactly two parents", merge)
		}
		output, err = runGit(ctx, repo, nil, nil, "merge-base", "--all", parents[1], parents[2])
		if err != nil {
			return nil, err
		}
		bases := strings.Fields(string(output))
		if len(bases) != 1 {
			return nil, fmt.Errorf("publication merge %s must have exactly one merge base", merge)
		}
		pairs := [][2]string{{bases[0], parents[1]}, {bases[0], parents[2]}, {parents[1], merge}, {parents[2], merge}}
		sets := make([]map[string]bool, len(pairs))
		for i, pair := range pairs {
			sets[i], err = pathSet("diff-tree", "--no-commit-id", "--name-only", "-z", "--no-renames", "-r", pair[0], pair[1])
			if err != nil {
				return nil, err
			}
		}
		candidates := map[string]bool{}
		for path := range sets[2] {
			candidates[path] = true
		}
		for path := range sets[3] {
			candidates[path] = true
		}
		for path := range candidates {
			// Exclude only a path carried unchanged from exactly one parent;
			// every other merge delta is candidate-authored publication scope.
			if (sets[0][path] && !sets[1][path] && !sets[2][path]) || (sets[1][path] && !sets[0][path] && !sets[3][path]) {
				continue
			}
			paths[path] = true
		}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	return canonicalPaths(result)
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
