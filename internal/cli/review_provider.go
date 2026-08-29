package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewProviderRoleLens              = string(reviewerprovider.RoleLens)
	reviewProviderRoleRefuter           = string(reviewerprovider.RoleRefuter)
	reviewProviderRoleTargetedValidator = string(reviewerprovider.RoleTargetedValidator)
)

const reviewProviderTransportCapability = reviewerprovider.TransportCapability

// reviewProviderRoleContract is the Go-owned registry for every provider role.
// Runtime adapters receive only its materialized Invocation and never select a
// prompt, schema, byte policy, admission rule, or durable slot.
type reviewProviderRoleContract = reviewerprovider.Contract

func reviewProviderRoleContractFor(role string) (reviewProviderRoleContract, error) {
	contract, err := reviewerprovider.ContractFor(reviewerprovider.Role(role))
	if err != nil {
		return reviewProviderRoleContract{}, fmt.Errorf("unsupported review provider role %q; add its Go-owned contract before it can be invoked", role) // refusal:by-design world-action: provider roles are compiled authority, not runtime-selected extensions
	}
	if !json.Valid(contract.ResultSchema) {
		return reviewProviderRoleContract{}, fmt.Errorf("review provider role %q has invalid result schema; repair the compiled contract before invoking this role", role) // refusal:by-design world-action: invalid Go-owned schema cannot be fixed by a runtime retry
	}
	return contract, nil
}

func reviewProviderRoleContracts() []reviewProviderRoleContract {
	return reviewerprovider.Contracts()
}

// reviewProviderRequest is the materialized lens invocation. Its fields remain
// inside Go; adapters receive only Invocation.
type reviewProviderRequest struct {
	Store      reviewtransaction.CompactStore
	Binding    reviewLensContextBinding
	Subject    reviewtransaction.ArtifactSubject
	Frozen     reviewtransaction.FrozenCandidateContext
	Invocation reviewerprovider.Invocation
}

// reviewProviderAdmittedResult is the complete native interpretation of raw
// reviewer bytes before immutable capture selects the role slot.
type reviewProviderAdmittedResult struct {
	Frozen                    reviewtransaction.FrozenCandidateContext
	Subject                   reviewtransaction.ArtifactSubject
	Result                    facadeReviewerResult
	NativeResult              reviewtransaction.LensResult
	CandidateCausalFindingIDs []string
	Admission                 reviewtransaction.ArtifactAdmission
}

// reviewProviderMaterialize deliberately delegates to the current lens-context
// assembly. It does not emit a delivery descriptor or mutate review authority.
func reviewProviderMaterialize(ctx context.Context, deps reviewLensContextDeps, repo, repositoryContext, lens string, requested reviewtransaction.ReviewRepositoryContextBinding) (request reviewProviderRequest, err error) {
	authority, err := resolveReviewLensAuthority(ctx, deps, repo, repositoryContext, lens, requested)
	if err != nil {
		return reviewProviderRequest{}, err
	}
	prompt, err := reviewLensContextBlock(ctx, deps, authority.Inspector, authority.Binding, authority.Subject, authority.Frozen)
	if err != nil {
		return reviewLensContextCleanup(ctx, reviewProviderRequest{}, err, func() error { return deps.close(authority.Inspector) })
	}
	request = reviewProviderRequest{
		Store: authority.Store, Binding: authority.Binding, Subject: authority.Subject, Frozen: authority.Frozen,
		Invocation: reviewerprovider.NewInvocation(prompt),
	}
	return reviewLensContextCleanup(ctx, request, nil, func() error { return deps.close(authority.Inspector) })
}

// reviewProviderExtractRoleRaw is the common raw-output boundary. It refuses
// malformed, oversized, or multiple objects before role-specific decoding.
func reviewProviderExtractRoleRaw(role string, raw []byte) ([]byte, error) {
	contract, err := reviewProviderRoleContractFor(role)
	if err != nil {
		return nil, err
	}
	if err := validateReviewerResultPayload(raw); err != nil {
		return nil, err
	}
	payload, decision, err := reviewtransaction.ExtractBoundedSingleJSONObject(raw, contract.ResultLimit)
	if err != nil {
		return nil, fmt.Errorf("%s provider result admission %s: %w", contract.Role, decision, err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, errors.New("review provider result contains no JSON object; return exactly one JSON object and retry capture") // refusal:by-design operator-knowledge: the runtime must provide its final structured result
	}
	return payload, nil
}

// reviewProviderAdmitLensRaw preserves the native pre-capture semantics while
// keeping durable capture outside this materialization and admission slice.
func reviewProviderAdmitLensRaw(ctx context.Context, root string, state reviewtransaction.CompactState, revision string, frozen reviewtransaction.FrozenCandidateContext, subject reviewtransaction.ArtifactSubject, raw []byte) (reviewtransaction.ReviewerResult, error) {
	admitted, err := reviewProviderAdmitRaw(ctx, root, state, revision, frozen, subject, raw)
	if err != nil {
		return reviewtransaction.ReviewerResult{}, err
	}
	return reviewtransaction.ReviewerResult{SubjectHash: admitted.Result.SubjectHash, Inspection: admitted.Result.Inspection, Lens: admitted.NativeResult.Lens, Findings: admitted.NativeResult.Findings, Evidence: admitted.NativeResult.Evidence}, nil
}

// reviewProviderAdmitRaw preserves capture-result's strict native admission
// without publishing bytes or mutating authority.
func reviewProviderAdmitRaw(ctx context.Context, root string, state reviewtransaction.CompactState, revision string, frozen reviewtransaction.FrozenCandidateContext, subject reviewtransaction.ArtifactSubject, raw []byte) (reviewProviderAdmittedResult, error) {
	payload, err := reviewProviderExtractRoleRaw(reviewProviderRoleLens, raw)
	if err != nil {
		return reviewProviderAdmittedResult{}, err
	}
	var result facadeReviewerResult
	if err := decodeFacadeJSONBytes(payload, &result); err != nil {
		return reviewProviderAdmittedResult{}, fmt.Errorf("decode reviewer result: %w", err)
	}
	if result.Findings == nil || result.Evidence == nil {
		return reviewProviderAdmittedResult{}, errors.New("reviewer result requires explicit findings and evidence arrays") // refusal:by-design operator-knowledge: reviewers must resubmit explicit findings and evidence arrays
	}
	if result.SubjectHash != subject.SubjectHash {
		legacyFrozen, legacyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
		if legacyErr == nil {
			legacySubject, subjectErr := reviewtransaction.NewLegacyArtifactSubject(state, revision, legacyFrozen, subject.Lens, subject.SelectedOrder, "")
			if subjectErr == nil && result.SubjectHash == legacySubject.SubjectHash {
				frozen, subject = legacyFrozen, legacySubject
			}
		}
	}
	if _, err := prepareCompactReviewerResults(reviewtransaction.CompactState{SelectedLenses: []string{subject.Lens}}, []facadeReviewerResult{result}, facadeRefuterResult{}); err != nil {
		return reviewProviderAdmittedResult{}, err
	}
	canonical, err := reviewtransaction.CanonicalizeReviewerResult(payload, subject.Lens)
	if err != nil {
		return reviewProviderAdmittedResult{}, err
	}
	canonicalPayload, err := json.Marshal(canonical)
	if err != nil {
		return reviewProviderAdmittedResult{}, err
	}
	canonicalPayload = append(canonicalPayload, '\n')
	native := reviewtransaction.LensResult{Lens: canonical.Lens, Findings: canonical.Findings, Evidence: canonical.Evidence}
	canonicalForCausality, err := reviewtransaction.CanonicalCompactLensResult(native)
	if err != nil {
		return reviewProviderAdmittedResult{}, fmt.Errorf("canonicalize reviewer result: %w", err)
	}
	candidateCausalIDs, err := verifiedCandidateCausalFindingIDs(ctx, root, state.InitialSnapshot, canonicalForCausality)
	if err != nil {
		return reviewProviderAdmittedResult{}, err
	}
	_, admission, err := reviewtransaction.AdmitArtifact(ctx, reviewtransaction.ArtifactAdmissionRequest{
		ExpectedSubject: subject, FrozenContext: frozen, EchoedSubjectHash: result.SubjectHash,
		Inspection: result.Inspection, Result: native, CandidateCausalFindingIDs: candidateCausalIDs,
		RawPayload: raw, CanonicalPayload: canonicalPayload,
	})
	if err != nil {
		return reviewProviderAdmittedResult{}, err
	}
	return reviewProviderAdmittedResult{Frozen: frozen, Subject: subject, Result: result, NativeResult: native, CandidateCausalFindingIDs: candidateCausalIDs, Admission: admission}, nil
}
