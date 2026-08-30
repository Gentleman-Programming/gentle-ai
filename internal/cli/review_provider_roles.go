package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

var errReviewProviderRefuterNotRequired = errors.New("provider refuter request has no inferential findings; continue through the remaining capture route") // refusal:by-design operator-knowledge: the native closure branch consumes this sentinel and derives the remaining capture transition; no caller-selected command exists

// errReviewProviderRefuterResultNotCaptured and its targeted-validator twin
// are typed absence, not damage: last-event closure distinguishes an
// unoccupied slot (continue on the ordinary route) from unverifiable captured
// bytes (fail closed).
var errReviewProviderRefuterResultNotCaptured = errors.New("provider refuter result is not captured") // refusal:by-design operator-knowledge: capture the Go-issued provider refuter batch before closure

var errReviewProviderTargetedValidatorResultNotCaptured = errors.New("provider targeted validator result is not captured") // refusal:by-design operator-knowledge: capture the Go-issued validator result before closure

const maxInconclusiveTargetedValidations = 3

type reviewProviderRole = reviewerprovider.Role

const reviewProviderTaskBindingHeader = "GENTLE_AI_REVIEW_PROVIDER_TASK"

type reviewProviderTaskBinding struct {
	LineageID         string `json:"lineage_id"`
	Revision          string `json:"revision"`
	TargetIdentity    string `json:"target_identity"`
	RepositoryContext string `json:"repository_context"`
	Role              string `json:"role"`
}

// newReviewProviderTask produces an opaque host task that only Go may
// materialize and admit through the live OpenCode relay.
func newReviewProviderTask(role reviewProviderRole, binding ReviewTransitionBinding) (ReviewProviderTask, error) {
	agent := reviewProviderRoleOpenCodeAgent(role)
	if agent == "" || binding.LineageID == "" || !providerSHA256(binding.Revision) || !providerSHA256(binding.TargetIdentity) ||
		reviewtransaction.ValidateReviewRepositoryContextHandle(binding.RepositoryContext) != nil {
		return ReviewProviderTask{}, errors.New("provider role task binding is incomplete") // refusal:by-design world-action: only a Go-issued STATUS transition may bind a managed provider role task
	}
	payload, err := json.Marshal(reviewProviderTaskBinding{
		LineageID: binding.LineageID, Revision: binding.Revision, TargetIdentity: binding.TargetIdentity,
		RepositoryContext: binding.RepositoryContext, Role: string(role),
	})
	if err != nil {
		return ReviewProviderTask{}, err
	}
	return ReviewProviderTask{Agent: agent, Role: string(role), Prompt: reviewProviderTaskBindingHeader + " " + string(payload)}, nil
}

func reviewProviderRoleOpenCodeAgent(role reviewProviderRole) string {
	switch role {
	case reviewerprovider.RoleRefuter:
		return "review-refuter"
	case reviewerprovider.RoleTargetedValidator:
		return "review-validator"
	default:
		return ""
	}
}

func reviewProviderRoleTaskSchema(role reviewProviderRole) string {
	contract, err := reviewerprovider.ContractFor(role)
	if err != nil {
		return ""
	}
	return string(contract.ResultSchema)
}

func reviewProviderRoleTaskRequest(ctx context.Context, repo, storeDir string, state reviewtransaction.CompactState, revision string, role reviewProviderRole) (reviewerprovider.Invocation, error) {
	switch role {
	case reviewerprovider.RoleRefuter:
		request, err := reviewProviderNewRefuterRequest(ctx, repo, storeDir, state, revision)
		if err != nil {
			return reviewerprovider.Invocation{}, err
		}
		return request.Invocation, nil
	case reviewerprovider.RoleTargetedValidator:
		correction, err := reviewProviderTargetedValidatorCorrection(ctx, repo, state)
		if err != nil {
			return reviewerprovider.Invocation{}, err
		}
		request, err := reviewProviderNewTargetedValidatorRequest(ctx, repo, state, revision, correction)
		if err != nil {
			return reviewerprovider.Invocation{}, err
		}
		return request.Invocation, nil
	default:
		return reviewerprovider.Invocation{}, fmt.Errorf("unsupported provider role task %q", role) // refusal:by-design world-action: OpenCode may invoke only compiled provider roles
	}
}

type reviewProviderRefuterRequest struct {
	Schema           string                           `json:"schema"`
	RequestHash      string                           `json:"request_hash"`
	LineageID        string                           `json:"lineage_id"`
	AuthorityVersion string                           `json:"authority_revision"`
	TargetIdentity   string                           `json:"target_identity"`
	SnapshotIdentity string                           `json:"snapshot_identity"`
	Claims           []reviewtransaction.RefuterClaim `json:"claims"`
	Evidence         []reviewProviderEvidence         `json:"evidence"`
	Invocation       reviewerprovider.Invocation      `json:"-"`
}

// compactProviderRoleResult is the transaction-owned durable shape after Go
// admits provider transport bytes. It intentionally excludes provider wire
// fields such as request hashes and proof-ref arrays.
type compactProviderRefuterResult struct {
	Results []reviewtransaction.EvidenceResult `json:"results"`
}

type compactProviderTargetedValidatorResult struct {
	Outcome  string                                             `json:"outcome"`
	Evidence reviewtransaction.CompactTargetedValidatorEvidence `json:"evidence"`
}

type reviewProviderEvidence struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func reviewProviderNewRefuterRequest(ctx context.Context, repo, storeDir string, state reviewtransaction.CompactState, revision string) (reviewProviderRefuterRequest, error) {
	contract, err := reviewProviderRoleContractFor(reviewProviderRoleRefuter)
	if err != nil {
		return reviewProviderRefuterRequest{}, err
	}
	if err := state.Validate(); err != nil {
		return reviewProviderRefuterRequest{}, err
	}
	if state.State != reviewtransaction.StateReviewing {
		return reviewProviderRefuterRequest{}, errors.New("provider refuter request requires reviewing authority") // refusal:by-design operator-knowledge: refresh the current provider request from reviewing authority before invoking a refuter
	}
	if revision != state.CapturePhaseRevision {
		return reviewProviderRefuterRequest{}, errors.New("provider refuter request requires the current compact capture phase") // refusal:by-design operator-knowledge: refresh the exact current capture phase before materializing a refuter request
	}
	view, _, err := capturedCompactReviewView(ctx, repo, storeDir, state, revision)
	if err != nil {
		return reviewProviderRefuterRequest{}, err
	}
	claims, err := reviewProviderRefuterClaims(state.InitialSnapshot.Identity, compactReviewInputFromView(view))
	if err != nil {
		return reviewProviderRefuterRequest{}, err
	}
	evidence, err := reviewProviderMaterializeEvidence(ctx, repo, state.InitialSnapshot)
	if err != nil {
		return reviewProviderRefuterRequest{}, err
	}
	request := reviewProviderRefuterRequest{
		Schema: contract.RequestSchemaID, LineageID: state.LineageID, AuthorityVersion: revision,
		TargetIdentity: state.InitialSnapshot.Identity, SnapshotIdentity: state.InitialSnapshot.Identity,
		Claims: claims, Evidence: evidence,
	}
	request.RequestHash = facadeValueHash("provider-refuter-request", struct {
		Schema, LineageID, AuthorityVersion, TargetIdentity, SnapshotIdentity string
		Claims                                                                []reviewtransaction.RefuterClaim
		Evidence                                                              []reviewProviderEvidence
	}{request.Schema, request.LineageID, request.AuthorityVersion, request.TargetIdentity, request.SnapshotIdentity, request.Claims, request.Evidence})
	prompt, err := reviewProviderRolePrompt(contract, request)
	if err != nil {
		return reviewProviderRefuterRequest{}, err
	}
	request.Invocation = reviewerprovider.NewInvocation(prompt)
	return request, nil
}

func reviewProviderRefuterClaims(snapshot string, input reviewtransaction.CompactReviewInput) ([]reviewtransaction.RefuterClaim, error) {
	claims := make([]reviewtransaction.RefuterClaim, 0)
	for _, classification := range input.Classifications {
		if classification.Class != reviewtransaction.EvidenceInferential {
			continue
		}
		switch classification.Causality {
		case reviewtransaction.CausalIntroduced, reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalWorsened:
			claims = append(claims, reviewtransaction.RefuterClaim{FindingID: classification.FindingID, SnapshotIdentity: snapshot, Proof: classification.Proof})
		}
	}
	if len(claims) == 0 {
		return nil, errReviewProviderRefuterNotRequired
	}
	return reviewProviderCanonicalRefuterClaims(snapshot, claims)
}

func reviewProviderCanonicalRefuterClaims(snapshot string, claims []reviewtransaction.RefuterClaim) ([]reviewtransaction.RefuterClaim, error) {
	seen := make(map[string]struct{}, len(claims))
	canonical := make([]reviewtransaction.RefuterClaim, len(claims))
	for index, claim := range claims {
		claim.FindingID, claim.Proof = strings.TrimSpace(claim.FindingID), strings.TrimSpace(claim.Proof)
		if claim.FindingID == "" || claim.SnapshotIdentity != snapshot || !reviewProviderConcreteEvidence(claim.Proof) {
			return nil, errors.New("provider refuter request has an invalid inferential claim") // refusal:by-design world-action: malformed provider-owned inferential claims require a code fix before a refuter can run
		}
		if _, exists := seen[claim.FindingID]; exists {
			return nil, fmt.Errorf("provider refuter request repeats finding %q", claim.FindingID) // refusal:by-design world-action: duplicate claims violate the one-result-per-finding provider contract
		}
		seen[claim.FindingID], canonical[index] = struct{}{}, claim
	}
	sort.Slice(canonical, func(left, right int) bool { return canonical[left].FindingID < canonical[right].FindingID })
	return canonical, nil
}

// reviewProviderTargetedValidatorRequest carries the opaque repository context
// because this role, alone among the provider roles, is expected to inspect
// the immutable corrected candidate itself. The handle stays opaque and
// path-free: it is a locator, not authorization, and resolving it revalidates
// the repository and the current compact authority before any tree is read.
// Without it the inspection recipe in the role's prompt would name a command
// no validator could assemble, which is the second half of #3380.
type reviewProviderTargetedValidatorRequest struct {
	ValidationRequest reviewtransaction.TargetedValidationRequest `json:"validation_request"`
	FixDeltaHash      string                                      `json:"fix_delta_hash"`
	RepositoryContext string                                      `json:"repository_context"`
	Evidence          []reviewProviderEvidence                    `json:"evidence"`
	Invocation        reviewerprovider.Invocation                 `json:"-"`
}

type providerValidationResultWire struct {
	TargetedValidationRequestHash string                       `json:"targeted_validation_request_hash"`
	CorrectionTargetIdentity      string                       `json:"correction_target_identity"`
	OriginalCriteria              providerValidationCheckWire  `json:"original_criteria"`
	CorrectionRegression          providerValidationCheckWire  `json:"correction_regression"`
	FollowUps                     []reviewtransaction.FollowUp `json:"follow_ups"`
}

type providerValidationCheckWire struct {
	Passed   *bool    `json:"passed"`
	Evidence []string `json:"evidence"`
}

func reviewProviderNewTargetedValidatorRequest(ctx context.Context, repo string, state reviewtransaction.CompactState, revision string, correction reviewtransaction.Snapshot) (reviewProviderTargetedValidatorRequest, error) {
	contract, err := reviewProviderRoleContractFor(reviewProviderRoleTargetedValidator)
	if err != nil {
		return reviewProviderTargetedValidatorRequest{}, err
	}
	var request reviewtransaction.TargetedValidationRequest
	if state.State == reviewtransaction.StateEscalated {
		request, err = reviewtransaction.RebuildAdmittedTargetedValidationRequest(state, revision)
	} else {
		request, err = reviewtransaction.BuildTargetedValidationRequestFromSnapshot(ctx, repo, state, revision, correction)
	}
	if err != nil {
		return reviewProviderTargetedValidatorRequest{}, err
	}
	fixDeltaHash := reviewtransaction.FixDeltaHashForSnapshot(correction)
	if !providerSHA256(fixDeltaHash) {
		return reviewProviderTargetedValidatorRequest{}, errors.New("provider targeted validator request has an invalid fix delta hash") // refusal:by-design world-action: the provider-created validator request must carry immutable fix identity
	}
	// The same binding STATUS publishes for this transition, so the handle the
	// validator is handed is byte-identical to the one the orchestrator holds
	// and the derivation stays deterministic for the transport's re-interception
	// byte comparison.
	repositoryContext, err := reviewtransaction.DeriveReviewRepositoryContextHandle(ctx, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: request.LineageID, TargetIdentity: request.CorrectionTargetIdentity, Revision: request.ExpectedRevision,
	})
	if err != nil {
		return reviewProviderTargetedValidatorRequest{}, err
	}
	evidence, err := reviewProviderMaterializeEvidence(ctx, repo, correction)
	if err != nil {
		return reviewProviderTargetedValidatorRequest{}, err
	}
	providerRequest := reviewProviderTargetedValidatorRequest{
		ValidationRequest: request, FixDeltaHash: fixDeltaHash, RepositoryContext: repositoryContext, Evidence: evidence,
	}
	prompt, err := reviewProviderRolePrompt(contract, providerRequest)
	if err != nil {
		return reviewProviderTargetedValidatorRequest{}, err
	}
	providerRequest.Invocation = reviewerprovider.NewInvocation(prompt)
	return providerRequest, nil
}

func reviewProviderMaterializeEvidence(ctx context.Context, repo string, snapshot reviewtransaction.Snapshot) ([]reviewProviderEvidence, error) {
	deps := reviewLensContextDependencies()
	inspector, err := deps.prepare(reviewtransaction.SnapshotBuilder{Repo: repo}, ctx, snapshot)
	if err != nil {
		return nil, reviewLensContextInspectionFailure(ctx, err)
	}
	defer deps.close(inspector)
	frozen := inspector.FrozenCandidateContext()
	// The aggregate byte budget is the whole bound. The 32-entry cap that used
	// to sit above it measured the wrong thing and is gone (issue #3367); this
	// role request is additionally bounded by its contract's prompt limit.
	budget := reviewLensContextByteBudget
	evidence := make([]reviewProviderEvidence, 0, len(frozen.ChangedPathManifest))
	for index, entry := range frozen.ChangedPathManifest {
		payload, err := deps.inspect(ctx, inspector, "patch", index, "")
		if err != nil {
			return nil, reviewLensContextInspectionFailure(ctx, err)
		}
		if len(bytes.TrimSpace(payload)) == 0 && !entry.ModeOnly && !entry.Deleted {
			return nil, reviewLensContextRefusal("lens_context_empty_patch", reviewLensContextEmptyPatchAction)
		}
		budget -= len(entry.Path) + len(payload)
		if budget < 0 {
			return nil, reviewLensContextRefusal("lens_context_budget_exceeded", reviewLensContextBudgetAction)
		}
		evidence = append(evidence, reviewProviderEvidence{Path: entry.Path, Content: string(payload)})
	}
	return evidence, nil
}

func reviewProviderRolePrompt(contract reviewProviderRoleContract, request any) ([]byte, error) {
	if contract.PromptInstruction == "" {
		return nil, fmt.Errorf("provider role %q has no non-lens prompt", contract.Role) // refusal:by-design world-action: every provider role needs an explicit native prompt
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	instruction := contract.PromptInstruction
	if targeted, ok := request.(reviewProviderTargetedValidatorRequest); ok {
		// JSON necessarily escapes policy line breaks. Materialize the same
		// request-bound bytes before the machine-readable input so the validator
		// can inspect the exact frozen policy text without a second policy model.
		instruction += "\n\nFrozen policy:\n" + targeted.ValidationRequest.PolicyContent
	}
	prompt := []byte(fmt.Sprintf("%s\n\nInput:\n%s\n\nOutput schema:\n%s", instruction, payload, contract.ResultSchema))
	if len(prompt) > contract.ResultLimit {
		return nil, fmt.Errorf("provider %s prompt exceeds the native %d byte limit", contract.Role, contract.ResultLimit) // refusal:by-design operator-knowledge: provider evidence is never truncated; split the candidate
	}
	return prompt, nil
}

func reviewProviderAdmitRefuterRaw(request reviewProviderRefuterRequest, raw []byte) (facadeRefuterResult, error) {
	contract, err := reviewProviderRoleContractFor(reviewProviderRoleRefuter)
	if err != nil || request.Schema != contract.RequestSchemaID || request.RequestHash == "" {
		return facadeRefuterResult{}, errors.New("provider refuter request is incomplete") // refusal:by-design world-action: refuter admission requires a Go-created request binding
	}
	claims, err := reviewProviderCanonicalRefuterClaims(request.SnapshotIdentity, request.Claims)
	if err != nil {
		return facadeRefuterResult{}, err
	}
	payload, err := reviewProviderExtractRoleRaw(reviewProviderRoleRefuter, raw)
	if err != nil {
		return facadeRefuterResult{}, err
	}
	var result facadeRefuterResult
	if err := decodeFacadeJSONBytes(payload, &result); err != nil {
		return facadeRefuterResult{}, fmt.Errorf("decode provider refuter result: %w", err)
	}
	if result.RequestHash != request.RequestHash || result.Results == nil {
		return facadeRefuterResult{}, errors.New("provider refuter result does not bind the requested batch") // refusal:by-design operator-knowledge: return the exact request hash and complete results array from the Go-issued batch
	}
	expected := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		expected[claim.FindingID] = struct{}{}
	}
	if len(result.Results) != len(expected) {
		return facadeRefuterResult{}, errors.New("provider refuter result must cover every inferential finding exactly once") // refusal:by-design operator-knowledge: return one result for every provider-issued inferential finding
	}
	seen := make(map[string]struct{}, len(result.Results))
	for index := range result.Results {
		outcome := &result.Results[index]
		outcome.FindingID = strings.TrimSpace(outcome.FindingID)
		if _, found := expected[outcome.FindingID]; !found {
			return facadeRefuterResult{}, fmt.Errorf("provider refuter result names unexpected finding %q", outcome.FindingID) // refusal:by-design operator-knowledge: answer only provider-issued findings
		}
		if _, duplicate := seen[outcome.FindingID]; duplicate {
			return facadeRefuterResult{}, fmt.Errorf("provider refuter result repeats finding %q", outcome.FindingID) // refusal:by-design operator-knowledge: return each finding once
		}
		seen[outcome.FindingID] = struct{}{}
		switch outcome.Outcome {
		case reviewtransaction.OutcomeCorroborated, reviewtransaction.OutcomeRefuted, reviewtransaction.OutcomeInconclusive:
		default:
			return facadeRefuterResult{}, fmt.Errorf("provider refuter result %q has unsupported outcome %q", outcome.FindingID, outcome.Outcome) // refusal:by-design operator-knowledge: choose a supported outcome
		}
		if err := reviewProviderConcreteStrings(outcome.ProofRefs, "provider refuter proof_refs"); err != nil {
			return facadeRefuterResult{}, err
		}
	}
	sort.Slice(result.Results, func(left, right int) bool { return result.Results[left].FindingID < result.Results[right].FindingID })
	return result, nil
}

func reviewProviderCaptureRefuterRaw(ctx context.Context, repo string, store reviewtransaction.CompactStore, state reviewtransaction.CompactState, revision string, raw []byte) (facadeRefuterResult, error) {
	request, err := reviewProviderNewRefuterRequest(ctx, repo, store.Dir, state, revision)
	if err != nil {
		return facadeRefuterResult{}, err
	}
	result, err := reviewProviderAdmitRefuterRaw(request, raw)
	if err != nil {
		return facadeRefuterResult{}, err
	}
	payload, err := canonicalProviderRoleResult(compactProviderRefuterResult{Results: result.native()})
	if err != nil {
		return facadeRefuterResult{}, err
	}
	err = store.CaptureAdmittedRefuterResult(ctx, reviewtransaction.CompactAdmittedRefuterResultRequest{
		ExpectedRevision: revision, TargetIdentity: state.InitialSnapshot.Identity, RequestHash: request.RequestHash, Payload: payload,
		PreparePublication: func(current reviewtransaction.CompactState) error {
			currentRequest, err := reviewProviderNewRefuterRequest(ctx, repo, store.Dir, current, revision)
			if err != nil {
				return err
			}
			currentResult, err := reviewProviderAdmitRefuterRaw(currentRequest, raw)
			if err != nil {
				return err
			}
			currentPayload, err := canonicalProviderRoleResult(compactProviderRefuterResult{Results: currentResult.native()})
			if err != nil || !bytes.Equal(currentPayload, payload) {
				return errors.New("provider refuter result changed while capture was pending") // refusal:by-design operator-knowledge: refresh captured lenses and rerun the provider
			}
			return nil
		},
	})
	if err != nil {
		return facadeRefuterResult{}, err
	}
	return result, nil
}

func reviewProviderCaptureRefuter(ctx context.Context, repo string, store reviewtransaction.CompactStore, state reviewtransaction.CompactState, revision string, agent model.AgentID) (facadeRefuterResult, bool, error) {
	request, err := reviewProviderNewRefuterRequest(ctx, repo, store.Dir, state, revision)
	if errors.Is(err, errReviewProviderRefuterNotRequired) {
		return facadeRefuterResult{}, false, nil
	}
	if err != nil {
		return facadeRefuterResult{}, false, err
	}
	adapter, err := reviewProviderAdapter(reviewProviderRoleRefuter, agent)
	if err != nil {
		return facadeRefuterResult{}, false, err
	}
	raw, err := adapter.Review(ctx, request.Invocation)
	if err != nil {
		return facadeRefuterResult{}, false, fmt.Errorf("invoke provider refuter: %w", err)
	}
	result, err := reviewProviderCaptureRefuterRaw(ctx, repo, store, state, revision, raw)
	return result, err == nil, err
}

func readCapturedProviderRefuterResult(ctx context.Context, repo, storeDir string, state reviewtransaction.CompactState, revision string) ([]reviewtransaction.EvidenceResult, error) {
	request, err := reviewProviderNewRefuterRequest(ctx, repo, storeDir, state, revision)
	if err != nil {
		return nil, err
	}
	if _, found := state.AdmittedRoleResult(reviewtransaction.CompactRoleRefuter, revision, state.InitialSnapshot.Identity, request.RequestHash); !found {
		return nil, errReviewProviderRefuterResultNotCaptured
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return nil, fmt.Errorf("captured provider refuter result is no longer admitted: %w", err)
	}
	return append([]reviewtransaction.EvidenceResult(nil), view.RefuterOutcomes...), nil
}

func reviewProviderAdmitTargetedValidatorRaw(request reviewProviderTargetedValidatorRequest, raw []byte) (facadeValidationResult, reviewtransaction.ScopedValidationResult, error) {
	if err := reviewtransaction.ValidateTargetedValidationRequest(request.ValidationRequest); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, err
	}
	if !providerSHA256(request.FixDeltaHash) {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, errors.New("provider targeted validator request has an invalid fix delta hash") // refusal:by-design world-action: validator admission requires Go-owned immutable fix identity
	}
	if err := reviewtransaction.ValidateReviewRepositoryContextHandle(request.RepositoryContext); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, errors.New("provider targeted validator request has no opaque repository context") // refusal:by-design world-action: a validator asked to inspect the frozen candidate must be issued the locator that reaches it
	}
	payload, err := reviewProviderExtractRoleRaw(reviewProviderRoleTargetedValidator, raw)
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, err
	}
	var result facadeValidationResult
	var wire providerValidationResultWire
	if err := decodeFacadeJSONBytes(payload, &result); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, fmt.Errorf("decode provider targeted validator result: %w", err)
	}
	if err := decodeFacadeJSONBytes(payload, &wire); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, fmt.Errorf("decode provider targeted validator wire result: %w", err)
	}
	if wire.OriginalCriteria.Passed == nil || wire.CorrectionRegression.Passed == nil || result.FollowUps == nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, errors.New("provider targeted validator result requires passed checks and an explicit follow_ups array") // refusal:by-design operator-knowledge: every targeted validator response must explicitly declare its result
	}
	if err := reviewProviderConcreteStrings(result.OriginalCriteria.Evidence, "provider original criteria evidence"); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, err
	}
	if err := reviewProviderConcreteStrings(result.CorrectionRegression.Evidence, "provider correction regression evidence"); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, err
	}
	for index, followUp := range result.FollowUps {
		if strings.TrimSpace(followUp.Observation) == "" {
			return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, fmt.Errorf("provider validator follow_up[%d] requires observation", index) // refusal:by-design operator-knowledge: every follow-up needs its observed fact
		}
		if err := reviewProviderConcreteStrings(followUp.ProofRefs, fmt.Sprintf("provider validator follow_up[%d] proof_refs", index)); err != nil {
			return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, err
		}
	}
	native, err := result.compact(request.FixDeltaHash, request.ValidationRequest.FixFindingIDs, request.ValidationRequest)
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, err
	}
	return result, native, nil
}

func reviewProviderCloseTargetedValidatorRaw(ctx context.Context, repo string, store reviewtransaction.CompactStore, state reviewtransaction.CompactState, revision string, raw []byte) (facadeValidationResult, reviewtransaction.ScopedValidationResult, *reviewLastEventClosureResult, error) {
	correction, err := reviewProviderTargetedValidatorCorrection(ctx, repo, state)
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	request, err := reviewProviderNewTargetedValidatorRequest(ctx, repo, state, revision, correction)
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	result, native, err := reviewProviderAdmitTargetedValidatorRaw(request, raw)
	if err != nil {
		// An incomplete inspection remains a retryable non-verdict. Persist only a
		// hash-only attempt descriptor; rejected bytes, paths, and evidence bodies
		// never enter durable authority.
		if errors.Is(err, errReviewTargetedValidationInconclusive) {
			if _, ledgerErr := store.RecordInconclusiveTargetedValidatorAttempt(ctx, request.ValidationRequest, facadePayloadHash(raw)); ledgerErr != nil {
				return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, ledgerErr
			}
		}
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	evidence := reviewProviderTargetedValidatorEvidence(result)
	payload, err := canonicalProviderRoleResult(compactProviderTargetedValidatorResult{
		Outcome: reviewProviderTargetedValidatorOutcome(native), Evidence: evidence,
	})
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	capture := reviewtransaction.CompactAdmittedTargetedValidatorResultRequest{
		ExpectedRequest: request.ValidationRequest, Payload: payload, Evidence: &evidence, Validation: &native,
	}
	if reviewProviderTargetedValidatorOutcome(native) == "failed" {
		actual, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ChangedLines(ctx, correction)
		if err != nil {
			return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
		}
		complete, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildCorrectedCandidate(ctx, state.InitialSnapshot, correction)
		if err != nil {
			return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
		}
		capture.Complete = func(next *reviewtransaction.CompactState) error {
			return next.CompleteCorrectionVerification(correction, actual, native, complete)
		}
	}
	if err := store.CaptureAdmittedTargetedValidatorResult(ctx, capture); err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	current, err := store.LoadContext(ctx)
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	if reviewProviderTargetedValidatorOutcome(native) == "passed" {
		closure, err := closeCorrectionOnCapturedValidator(ctx, repo, store, current, correction, request.ValidationRequest, native)
		if err != nil {
			return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
		}
		return result, native, closure, nil
	}
	closure, err := newCorrectionCapturedValidatorClosure(repo, current.State, current.Revision, request.ValidationRequest)
	if err != nil {
		return facadeValidationResult{}, reviewtransaction.ScopedValidationResult{}, nil, err
	}
	return result, native, closure, nil
}

func reviewProviderTargetedValidatorEvidence(result facadeValidationResult) reviewtransaction.CompactTargetedValidatorEvidence {
	return reviewtransaction.CompactTargetedValidatorEvidence{
		TargetedValidationRequestHash: result.TargetedValidationRequestHash,
		CorrectionTargetIdentity:      result.CorrectionTargetIdentity,
		OriginalCriteria: reviewtransaction.CompactTargetedValidatorCheckEvidence{
			Passed: result.OriginalCriteria.Passed, Evidence: append([]string(nil), result.OriginalCriteria.Evidence...),
		},
		CorrectionRegression: reviewtransaction.CompactTargetedValidatorCheckEvidence{
			Passed: result.CorrectionRegression.Passed, Evidence: append([]string(nil), result.CorrectionRegression.Evidence...),
		},
		FollowUps: append([]reviewtransaction.FollowUp{}, result.FollowUps...),
	}
}

func reviewProviderTargetedValidatorOutcome(validation reviewtransaction.ScopedValidationResult) string {
	if validation.OriginalCriteria.Passed && validation.CorrectionRegression.Passed {
		return "passed"
	}
	return "failed"
}

func readCapturedProviderTargetedValidatorResult(ctx context.Context, repo, _ string, state reviewtransaction.CompactState, revision string) (string, error) {
	correction, err := reviewProviderTargetedValidatorCorrection(ctx, repo, state)
	if err != nil {
		return "", err
	}
	request, err := reviewProviderNewTargetedValidatorRequest(ctx, repo, state, revision, correction)
	if err != nil {
		return "", err
	}
	if _, found := state.AdmittedRoleResult(reviewtransaction.CompactRoleTargetedValidator, revision, request.ValidationRequest.CorrectionTargetIdentity, request.ValidationRequest.RequestHash); !found {
		if len(state.TargetedValidatorAttempts) >= maxInconclusiveTargetedValidations {
			return "", reviewtransaction.ErrCompactTargetedValidatorAttemptsExhausted
		}
		if len(state.TargetedValidatorAttempts) > 0 {
			return "", errReviewTargetedValidationInconclusive
		}
		return "", errReviewProviderTargetedValidatorResultNotCaptured
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return "", fmt.Errorf("captured provider targeted validator result is no longer admitted: %w", err)
	}
	if view.TargetedValidatorOutcome == "" {
		return "", errors.New("captured provider targeted validator result has no admitted outcome") // refusal:by-design world-action: an occupied validator slot must expose one transaction-owned outcome
	}
	return view.TargetedValidatorOutcome, nil
}

func reviewProviderTargetedValidatorCorrection(ctx context.Context, repo string, state reviewtransaction.CompactState) (reviewtransaction.Snapshot, error) {
	if state.State == reviewtransaction.StateEscalated && len(state.CorrectionAttempts) > 0 {
		return state.CorrectionAttempts[len(state.CorrectionAttempts)-1].Snapshot, nil
	}
	if state.State != reviewtransaction.StateCorrectionRequired || state.ProposedCorrectionLines == nil || state.CorrectionAttemptConsumed() {
		return reviewtransaction.Snapshot{}, errors.New("provider targeted validator request requires an open correction") // refusal:by-design world-action: validator evidence applies only to one open forecasted correction
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return reviewtransaction.Snapshot{}, fmt.Errorf("derive provider targeted validator scope from admitted authority: %w", err)
	}
	return (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetFixDiff, Projection: state.InitialSnapshot.Projection, BaseRef: state.CurrentSnapshot.CandidateTree,
		IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: view.FixFindingIDs,
	})
}

func canonicalProviderRoleResult(result any) ([]byte, error) {
	payload, err := json.Marshal(result)
	return append(payload, '\n'), err
}

func reviewProviderConcreteStrings(values []string, label string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s requires at least one concrete value", label) // refusal:by-design operator-knowledge: reviewer output must include concrete evidence
	}
	for index, value := range values {
		if !reviewProviderConcreteEvidence(value) {
			return fmt.Errorf("%s[%d] must be concrete", label, index) // refusal:by-design operator-knowledge: replace empty evidence with an observed concrete value
		}
	}
	return nil
}

func reviewProviderConcreteEvidence(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "n/a" && value != "na" && value != "none" && value != "todo" && value != "tbd" &&
		value != "pass" && value != "passed" && value != "success" && value != "placeholder"
}

func providerSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}
