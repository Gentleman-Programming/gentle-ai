package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	reviewResultArtifactSchema     = "gentle-ai.review-result-artifact/v2"
	reviewResultArtifactCapability = "review.native_result_artifact"
	reviewResultDryRunSchema       = "gentle-ai.review-capture-result-dry-run/v1"
	reviewAdmittedResultSchema     = reviewtransaction.AdmittedReviewerResultSchema
	reviewResultReferencePrefix    = "rart1_"
	reviewResultArtifactLimit      = 4 << 20
)

const (
	// reviewerResultSlotOccupiedCode names the one cause that is neither a
	// transport failure nor a bad binding: the slot holds a different reviewer
	// result already.
	reviewerResultSlotOccupiedCode   = "reviewer_result_slot_occupied"
	reviewerResultSlotOccupiedAction = "refresh the negotiated STATUS transition with " + reviewNextTransitionRefreshCommandV21 + " and follow its authoritative continuation"
)

func reviewReviewerResultSlotOccupiedFailure() error {
	return reviewPreflightRefusal(reviewPreflightSlotOccupiedReason, fmt.Errorf("%s: a different reviewer result already occupies this immutable slot; %s: %w", reviewerResultSlotOccupiedCode, reviewerResultSlotOccupiedAction, reviewtransaction.ErrCapturedReviewerResultSlotConflict))
}

type reviewResultArtifact struct {
	Schema            string                                      `json:"schema"`
	Capability        string                                      `json:"capability"`
	Path              string                                      `json:"path,omitempty"`
	Reference         string                                      `json:"reference,omitempty"`
	SHA256            string                                      `json:"sha256"`
	LineageID         string                                      `json:"lineage_id"`
	TargetIdentity    string                                      `json:"target_identity"`
	Lens              string                                      `json:"lens"`
	SelectedOrder     int                                         `json:"selected_order"`
	SubjectHash       string                                      `json:"subject_hash"`
	AdmissionDecision reviewtransaction.ArtifactAdmissionDecision `json:"admission_decision"`
}

type reviewResultDryRun struct {
	Schema            string                                      `json:"schema"`
	Operation         string                                      `json:"operation"`
	Validation        string                                      `json:"validation"`
	LineageID         string                                      `json:"lineage_id"`
	Lens              string                                      `json:"lens"`
	SelectedOrder     int                                         `json:"selected_order"`
	SubjectHash       string                                      `json:"subject_hash"`
	AdmissionDecision reviewtransaction.ArtifactAdmissionDecision `json:"admission_decision,omitempty"`
}

// admittedReviewerResult is the durable provider-owned envelope. Historical
// v1 files contained only model JSON; those bytes intentionally fail closed
// because they carry neither a subject nor an admission decision.
type admittedReviewerResult struct {
	Schema    string                              `json:"schema"`
	Subject   reviewtransaction.ArtifactSubject   `json:"subject"`
	Admission reviewtransaction.ArtifactAdmission `json:"admission"`
	Result    facadeReviewerResult                `json:"result"`
}

// ReviewerResultPayloadError is returned when a raw reviewer result payload is
// structurally invalid before JSON decoding is attempted. Code is
// machine-readable: "empty_result" for empty/whitespace-only payloads and
// "nested_envelope" for payloads that still contain an XML task envelope.
type ReviewerResultPayloadError struct {
	Code    string
	Message string
}

func (e *ReviewerResultPayloadError) Error() string { return e.Message }

// validateReviewerResultPayload inspects the raw bytes of a reviewer result
// before JSON decoding. It rejects two structurally distinct failure modes
// that require separate diagnostics:
//  1. empty_result: the task completed but produced no reviewer output.
//  2. nested_envelope: the reviewer output was not extracted from its XML
//     task wrapper before being passed as the strict JSON payload.
func validateReviewerResultPayload(payload []byte) error {
	if len(bytes.TrimSpace(payload)) == 0 {
		return &ReviewerResultPayloadError{
			Code:    "empty_result",
			Message: "reviewer result payload is empty or whitespace-only: the task may have completed without producing output",
		}
	}
	if bytes.Contains(payload, []byte("<task_result>")) || bytes.Contains(payload, []byte("</task_result>")) {
		if !json.Valid(payload) {
			return &ReviewerResultPayloadError{
				Code:    "nested_envelope",
				Message: "reviewer result payload contains a raw XML task envelope: extract the strict JSON reviewer output from <task_result> before capture",
			}
		}
	}
	return nil
}

var reviewArtifactAfterLstat = func() {}
var reviewArtifactRuntimeGOOS = func() string { return runtime.GOOS }
var syncReviewerArtifactDirectory = func(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

// RunReviewCaptureResult validates or captures one result bound to review authority.
func RunReviewCaptureResult(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review capture-result", stdout, "Capture one strict reviewer result in native authority and emit its bound manifest.")
	cwd := flags.String("cwd", ".", "repository path")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context; supplied by the collect transition and verified against --cwd")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact frozen target identity")
	lens := flags.String("lens", "", "exact selected lens")
	order := flags.Int("order", -1, "zero-based selected lens order")
	revision := flags.String("expected-revision", "", "exact reviewing authority revision")
	subjectHash := flags.String("subject-hash", "", "provider-issued artifact subject hash for native-Git context")
	runtimeAgent := flags.String("agent", "", "compiled reviewer runtime that invokes the provider-owned request; may be combined with --input only for compiled host-relay submissions and remains mutually exclusive with --preflight")
	input := flags.String("input", "", "raw reviewer result JSON file or - for stdin; `gentle-ai review schema reviewer` emits the schema and a working example")
	preflight := flags.Bool("preflight", false, "validate the capture binding and, when --input is supplied, the result admission without persisting anything")
	materialize := flags.Bool("materialize", false, "print the exact Go-materialized opaque provider task for a host-relay --agent runtime without capturing anything; mutually exclusive with --input and --preflight")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	providerRuntime := model.AgentID(strings.TrimSpace(*runtimeAgent))
	providerRuntimeSupplied := providerRuntime != ""
	rawInputSupplied := strings.TrimSpace(*input) != ""
	providerExecution := providerRuntimeSupplied && !rawInputSupplied
	hostRelaySubmission := providerRuntimeSupplied && rawInputSupplied
	if *materialize {
		if *preflight || strings.TrimSpace(*input) != "" {
			return reviewPreflightError(errors.New("review capture-result --materialize only prints the Go-materialized provider task and cannot be combined with --input or --preflight")) // refusal:by-design world-action: materialization is read-only and never authors or admits reviewer output
		}
		if !providerExecution {
			return reviewPreflightError(errors.New("review capture-result --materialize requires --agent naming the host-relay runtime")) // refusal:by-design operator-knowledge: only a compiled host-relay runtime identity selects the materialize form
		}
	}
	if flags.NArg() != 0 || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" ||
		strings.TrimSpace(*lens) == "" || *order < 0 || (!*preflight && !rawInputSupplied && !providerExecution) {
		return reviewPreflightError(errors.New("review capture-result requires an exact repository context, --lineage, --target, --lens, --order, and either --input or --agent (or --preflight); `gentle-ai review status --contract gentle-ai.review-integration/v1 --next-transition` prints the exact bindings and `gentle-ai review schema reviewer` emits the result schema with a working example"))
	}
	if providerRuntimeSupplied && *preflight {
		return reviewPreflightError(errors.New("review capture-result --agent cannot be combined with --preflight")) // refusal:by-design world-action: a provider runtime identity belongs only to a real materialize, capture, or host-relay submission
	}
	contextHandle := strings.TrimSpace(*repositoryContext)
	if contextHandle != "" && strings.TrimSpace(*revision) == "" {
		return reviewPreflightError(errors.New("review capture-result with --repository-context requires --expected-revision"))
	}
	if providerRuntimeSupplied {
		if contextHandle == "" {
			return reviewPreflightError(errors.New("review capture-result --agent requires the provider-issued --repository-context")) // refusal:by-design operator-knowledge: provider invocation and host-relay submission must use negotiated opaque context
		}
		if _, err := reviewRuntimeWithImmutableTransport(string(providerRuntime)); err != nil {
			return reviewPreflightError(err)
		}
		// Each refusal below names its own condition. They used to share one
		// sentence, so a report could not say which of two opposite causes
		// fired -- asking to materialize a runtime that never prints the task,
		// or omitting --materialize for a runtime that has no in-process
		// reviewer. Both state what the caller passed, what the runtime's
		// compiled transport is, and the one form that would be accepted.
		providerTransport := reviewImmutableRuntimeCapability(providerRuntime).Transport
		if *materialize {
			if reviewProviderCaptureRuntime(providerRuntime) {
				return reviewPreflightError(fmt.Errorf("review capture-result --materialize is unavailable for %q: a compiled runtime materializes internally; run the capture operation without --materialize", providerRuntime)) // refusal:by-design operator-knowledge: compiled subprocess adapters already receive the Go-materialized request in-process
			}
			if !reviewProviderHostRelayMaterializeRuntime(providerRuntime) {
				return reviewPreflightError(fmt.Errorf("review capture-result --materialize is unavailable for %q: printing the Go-materialized provider task is the host-relay form, and this runtime's compiled transport is %q; collect its reviewer result through that live host transport instead", providerRuntime, providerTransport)) // refusal:by-design world-action: only the Pi host relay collects a printed provider task
			}
		} else if hostRelaySubmission {
			if !reviewProviderHostRelayMaterializeRuntime(providerRuntime) {
				return reviewPreflightError(fmt.Errorf("review capture-result --agent %q with --input is unavailable: only a compiled host-relay runtime may submit its raw reviewer result with the provider-owned runtime binding; this runtime's compiled transport is %q", providerRuntime, providerTransport)) // refusal:by-design world-action: caller input cannot impersonate an in-process provider runtime
			}
		} else if !reviewProviderCaptureRuntime(providerRuntime) {
			if reviewProviderHostRelayMaterializeRuntime(providerRuntime) {
				return reviewPreflightError(fmt.Errorf("review capture-result --agent %q without --materialize has no in-process reviewer to run: its compiled transport is %q, whose host owns the reviewer subprocess; print the provider task with --materialize=true, run it in the host, then submit the raw result with --input and the same binding", providerRuntime, providerTransport)) // refusal:by-design world-action: the host relay owns the reviewer subprocess this process cannot launch
			}
			return reviewPreflightError(fmt.Errorf("review capture-result --agent %q has no compiled in-process reviewer adapter: its compiled transport is %q; collect its reviewer result through that live host transport instead", providerRuntime, providerTransport)) // refusal:by-design world-action: only compiled subprocess adapters use this capture path
		}
	}
	ctx := context.Background()
	var root string
	var err error
	if contextHandle != "" {
		root, err = resolveOpaqueReviewRepositoryRoot(ctx, *cwd, contextHandle, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		})
		if err != nil {
			return err
		}
	} else {
		root, err = (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
		if err != nil {
			return fmt.Errorf("resolve review repository root: %w", err)
		}
	}
	// --preflight verifies the capture binding without reading or persisting
	// any result, and --materialize only prints the Go-materialized provider
	// task, so both stay reachable under a frozen switch. Everything else
	// here publishes a reviewer result into the store.
	if !*preflight && !*materialize {
		if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
			return err
		}
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, *lineage, false)
	repositoryDescription := fmt.Sprintf("repository %q", root)
	if contextHandle != "" {
		repositoryDescription = "the provider-issued repository context"
	}
	if err != nil {
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_authority_unavailable", "refresh the exact native next_transition before retrying", err)
		}
		return reviewPreflightError(fmt.Errorf("resolve reviewing authority for lineage %q under %s: %w; if the review was started in a different repository (for example a nested one), re-run with --cwd set to that repository", *lineage, repositoryDescription, err))
	}
	state := record.State
	if state.State != reviewtransaction.StateReviewing || state.LineageID != *lineage || state.InitialSnapshot.Identity != *target ||
		(strings.TrimSpace(*revision) != "" && state.CapturePhaseRevision != *revision) || *order >= len(state.SelectedLenses) || state.SelectedLenses[*order] != *lens {
		if contextHandle != "" {
			return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, fmt.Errorf("capture binding does not match the current reviewing authority under the provider-issued repository context; ask the parent orchestrator to refresh the exact native next transition by running %s", reviewNextTransitionRefreshCommandV21))
		}
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, fmt.Errorf("capture binding does not match the current reviewing authority under repository %q; verify the frozen lineage, target, lens, and order for that repository, or re-run with --cwd set to the repository where the review was started", root))
	}
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: root}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("derive reviewer artifact subject: %w", err))
	}
	subject, err := reviewtransaction.NewArtifactSubject(state, state.CapturePhaseRevision, frozen, *lens, *order, "")
	if err != nil {
		return reviewPreflightError(fmt.Errorf("derive reviewer artifact subject: %w", err))
	}
	if *preflight && strings.TrimSpace(*input) == "" {
		if *subjectHash != "" && *subjectHash != subject.SubjectHash {
			legacyFrozen, legacyErr := (reviewtransaction.SnapshotBuilder{Repo: root}).WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
			if legacyErr != nil {
				return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("review capture preflight subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"))
			}
			legacySubject, legacyErr := reviewtransaction.NewLegacyArtifactSubject(state, state.CapturePhaseRevision, legacyFrozen, *lens, *order, "")
			if legacyErr != nil || *subjectHash != legacySubject.SubjectHash {
				return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("review capture preflight subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"))
			}
			frozen, subject = legacyFrozen, legacySubject
		}
		publicRoot := root
		if contextHandle != "" {
			publicRoot = ""
		}
		return encodeReviewJSON(stdout, reviewCapturePreflightResult{
			Schema: reviewCapturePreflightSchema, Capability: reviewCapturePreflightCapability, RepositoryRoot: publicRoot,
			LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Lens: *lens, SelectedOrder: *order,
			ArtifactSubject: subject, BaseTree: frozen.BaseTree, CandidateTree: frozen.CandidateTree,
			ChangedPathManifest: append([]reviewtransaction.ChangedPathManifestEntry{}, frozen.ChangedPathManifest...),
		})
	}
	var rawPayload []byte
	var admitted reviewProviderAdmittedResult
	if providerExecution {
		request, materializeErr := reviewProviderMaterialize(ctx, reviewLensContextDependencies(), root, contextHandle, *lens, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		})
		if materializeErr != nil {
			return reviewPreflightError(materializeErr)
		}
		if request.Binding.Lineage != state.LineageID || request.Binding.Target != state.InitialSnapshot.Identity || request.Binding.Revision != state.CapturePhaseRevision || request.Binding.Lens != *lens || request.Binding.Order != *order || request.Subject != subject {
			return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("provider reviewer invocation no longer matches the current reviewing authority")) // refusal:by-design world-action: only a fresh negotiated collection binding may invoke a provider runtime
		}
		if *materialize {
			if *subjectHash != "" && *subjectHash != subject.SubjectHash {
				return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("materialize subject hash does not match the provider-owned authority; refresh the binding with gentle-ai review status --cwd <repo> --contract <same-contract> --next-transition"))
			}
			// The host relay pipes these exact bytes verbatim into its fresh
			// locked-down reviewer subprocess, so they leave here raw: no JSON
			// envelope, no trailing newline, and nothing captured, consumed, or
			// mutated. Submission returns through the existing --input path
			// with this same binding.
			if _, err := stdout.Write(request.Invocation.Prompt()); err != nil {
				return fmt.Errorf("write materialized provider task: %w", err)
			}
			return nil
		}
		adapter, adapterErr := reviewProviderAdapter(reviewProviderRoleLens, providerRuntime)
		if adapterErr != nil {
			return reviewPreflightError(adapterErr)
		}
		// --agent is refused together with --preflight above, so this branch
		// never runs under preflight and its preservation needs no guard.
		admitted, rawPayload, err = reviewProviderCaptureWithOneCorrection(ctx, reviewProviderCapture{
			root: root, runtime: providerRuntime, adapter: adapter, state: state, frozen: frozen, subject: subject,
		}, request.Invocation)
		if err != nil {
			return reviewPreflightError(err)
		}
	} else {
		rawPayload, err = readFacadeBytes(*input)
		if err != nil {
			return reviewPreflightError(fmt.Errorf("read reviewer result: %w", err))
		}
		admitted, err = reviewProviderAdmitRaw(ctx, root, state, state.CapturePhaseRevision, frozen, subject, rawPayload)
		if err != nil {
			// A host-submitted result gets no corrective re-invocation, since
			// the host owns the reviewer, but its rejected bytes are preserved
			// the same way. This is the only branch --preflight can reach, and
			// --preflight persists nothing, as documented.
			if *preflight {
				return reviewPreflightError(err)
			}
			return reviewPreflightError(fmt.Errorf("%w%s", err, reviewRejectedResultClause(ctx, root, reviewRejectedResultMeta{
				LineageID: state.LineageID, Lens: *lens, Attempt: 1, Reason: err.Error(),
			}, rawPayload)))
		}
	}
	frozen, subject = admitted.Frozen, admitted.Subject
	if *preflight {
		return encodeReviewJSON(stdout, reviewResultDryRun{
			Schema: reviewResultDryRunSchema, Operation: "review/capture-result", Validation: "accepted",
			LineageID: state.LineageID, Lens: *lens, SelectedOrder: *order, SubjectHash: subject.SubjectHash,
			AdmissionDecision: admitted.Admission.Decision,
		})
	}
	captured, err := store.CaptureAdmittedReviewerResult(ctx, reviewtransaction.CompactAdmittedReviewerResultRequest{
		ExpectedRevision: state.CapturePhaseRevision, TargetIdentity: *target, FrozenContext: frozen,
		ArtifactSubject: subject, Inspection: admitted.Result.Inspection, Result: admitted.NativeResult,
		CandidateCausalFindingIDs: admitted.CandidateCausalFindingIDs, RawPayload: rawPayload,
	})
	if err != nil {
		if errors.Is(err, reviewtransaction.ErrCapturedReviewerResultSlotConflict) {
			return reviewReviewerResultSlotOccupiedFailure()
		}
		if contextHandle != "" {
			return reviewOpaqueContextCause("repository_context_capture_failed", "retry capture-result with the same exact binding or refresh status", err)
		}
		return reviewPreflightError(err)
	}
	currentRecord, currentErr := store.LoadContext(ctx)
	if currentErr != nil {
		return reviewPreflightError(currentErr)
	}
	closure, err := closeReviewOnLastCapturedLens(ctx, root, store, currentRecord, providerRuntime)
	if err != nil && !reviewLastCapturedLensClosureSuperseded(store, currentRecord) {
		return reviewPreflightError(err)
	}
	if closure != nil {
		return encodeReviewJSON(stdout, closure)
	}
	artifact := reviewResultArtifact{
		Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability,
		SHA256: captured.Slot.Digest, LineageID: state.LineageID,
		TargetIdentity: state.InitialSnapshot.Identity, Lens: *lens, SelectedOrder: *order,
		SubjectHash: captured.Subject.SubjectHash, AdmissionDecision: captured.Admission.Decision,
	}
	artifact.Reference = reviewResultReference(artifact)
	return encodeReviewJSON(stdout, artifact)
}

func reviewResultReference(artifact reviewResultArtifact) string {
	preimage := struct {
		Schema, Capability, SHA256, LineageID, TargetIdentity, Lens, SubjectHash string
		SelectedOrder                                                            int
		AdmissionDecision                                                        reviewtransaction.ArtifactAdmissionDecision
	}{
		Schema: artifact.Schema, Capability: artifact.Capability, SHA256: artifact.SHA256,
		LineageID: artifact.LineageID, TargetIdentity: artifact.TargetIdentity,
		Lens: artifact.Lens, SelectedOrder: artifact.SelectedOrder, SubjectHash: artifact.SubjectHash,
		AdmissionDecision: artifact.AdmissionDecision,
	}
	payload, _ := json.Marshal(preimage)
	return reviewResultReferencePrefix + strings.TrimPrefix(facadePayloadHash(payload), "sha256:")
}

// discoverCapturedReviewerArtifacts reads only the canonical native capture
// locations. It makes status restart-safe without exposing provider paths or
// asking a consumer to reconstruct result manifests.
func discoverCapturedReviewerArtifacts(ctx context.Context, repo, _ string, state reviewtransaction.CompactState, phase string) ([]ReviewTransitionArtifact, error) {
	if phase != state.CapturePhaseRevision {
		return nil, errors.New("captured reviewer discovery requires the current compact capture phase") // refusal:by-design operator-knowledge: refresh STATUS and use its current capture phase before discovering reviewer artifacts
	}
	if _, err := state.CompactReviewView(); err != nil {
		return nil, fmt.Errorf("derive captured reviewer artifacts from admitted authority: %w", err)
	}
	frozen, err := reviewerArtifactFrozenContext(ctx, repo, state)
	if err != nil {
		return nil, err
	}
	artifacts := make([]ReviewTransitionArtifact, 0, len(state.SelectedLenses))
	for order, lens := range state.SelectedLenses {
		entry, found, lookupErr := state.ActiveAdmittedLensResult(order)
		if lookupErr != nil {
			return nil, fmt.Errorf("lookup active captured reviewer result %d: %w", order, lookupErr)
		}
		if !found {
			continue
		}
		_, subject, err := decodeBoundAdmittedReviewerResult(ctx, repo, append(entry.Value, '\n'), entry.ArtifactDigest, state, entry.CapturePhaseRevision, order, frozen)
		if err != nil {
			return nil, fmt.Errorf("verify captured reviewer admission %d: %w", order, err)
		}
		artifacts = append(artifacts, ReviewTransitionArtifact{
			Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability, SHA256: entry.ArtifactDigest,
			LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Lens: lens, SelectedOrder: order,
			SubjectHash: subject.SubjectHash, AdmissionDecision: reviewtransaction.ArtifactAdmissionCompleted,
		})
	}
	return artifacts, nil
}

// capturedCompactReviewView combines public artifact verification with the
// transaction-owned semantic interpretation. The view intentionally ignores
// retained projections, while discovery preserves the public artifact subject
// and G1 retained-phase provenance.
func capturedCompactReviewView(ctx context.Context, repo, storeDir string, state reviewtransaction.CompactState, phase string) (reviewtransaction.CompactReviewView, []ReviewTransitionArtifact, error) {
	artifacts, err := discoverCapturedReviewerArtifacts(ctx, repo, storeDir, state, phase)
	if err != nil {
		return reviewtransaction.CompactReviewView{}, nil, err
	}
	if len(artifacts) != len(state.SelectedLenses) {
		return reviewtransaction.CompactReviewView{}, nil, fmt.Errorf("last-event closure requires all %d captured reviewer result(s); capture each missing one with `%s` (see `%s` for the exact lineage/target/lens/order bindings)", len(state.SelectedLenses), reviewCaptureResultCommandName(), reviewNextTransitionRefreshCommand)
	}
	view, err := state.CompactReviewView()
	if err != nil {
		return reviewtransaction.CompactReviewView{}, nil, fmt.Errorf("derive captured reviewer semantics from admitted authority: %w", err)
	}
	if len(view.LensResults) != len(artifacts) {
		return reviewtransaction.CompactReviewView{}, nil, errors.New("captured reviewer view does not cover every admitted artifact") // refusal:by-design world-action: compact role evidence must map one-to-one to public artifacts
	}
	return view, artifacts, nil
}

func reviewerArtifactFrozenContext(ctx context.Context, repo string, state reviewtransaction.CompactState) (reviewtransaction.FrozenCandidateContext, error) {
	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(ctx, state.InitialSnapshot)
	if err != nil {
		return reviewtransaction.FrozenCandidateContext{}, fmt.Errorf("derive frozen reviewer artifact context: %w", err)
	}
	return frozen, nil
}

func decodeBoundAdmittedReviewerResult(ctx context.Context, repo string, payload []byte, artifactDigest string, state reviewtransaction.CompactState, storedPhase string, order int, frozen reviewtransaction.FrozenCandidateContext) (facadeReviewerResult, reviewtransaction.ArtifactSubject, error) {
	var envelope admittedReviewerResult
	if err := decodeFacadeJSONBytes(payload, &envelope); err != nil {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, err
	}
	if order < 0 || order >= len(state.SelectedLenses) ||
		state.State == reviewtransaction.StateReviewing && envelope.Subject.AuthorityRevision != storedPhase {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, errors.New("captured reviewer result does not bind its stored capture phase") // refusal:by-design world-action: this structural compact-authority invariant requires a provider code fix; no operator command can safely repair it
	}
	subjectFrozen := frozen
	var expected reviewtransaction.ArtifactSubject
	var err error
	if envelope.Subject.Schema == reviewtransaction.ArtifactSubjectSchemaV1 {
		subjectFrozen, err = (reviewtransaction.SnapshotBuilder{Repo: repo}).WithLegacyCandidateDiff(ctx, state.InitialSnapshot, frozen)
		if err == nil {
			expected, err = reviewtransaction.NewLegacyArtifactSubject(
				state, envelope.Subject.AuthorityRevision, subjectFrozen, state.SelectedLenses[order], order, envelope.Subject.CorrectionTargetIdentity,
			)
		}
	} else {
		expected, err = reviewtransaction.NewArtifactSubject(
			state, envelope.Subject.AuthorityRevision, frozen, state.SelectedLenses[order], order, envelope.Subject.CorrectionTargetIdentity,
		)
	}
	if err != nil {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, err
	}
	result, err := decodeAdmittedReviewerResult(ctx, payload, expected, subjectFrozen)
	if err != nil {
		return facadeReviewerResult{}, reviewtransaction.ArtifactSubject{}, err
	}
	// Completed-state projections are retained only for compatibility. Artifact
	// verification stays bound to the admitted envelope; CompactReviewView
	// independently derives completed semantics from those same role values.
	return result, expected, nil
}

func decodeAdmittedReviewerResult(ctx context.Context, payload []byte, expected reviewtransaction.ArtifactSubject, frozen reviewtransaction.FrozenCandidateContext) (facadeReviewerResult, error) {
	var envelope admittedReviewerResult
	if err := decodeFacadeJSONBytes(payload, &envelope); err != nil {
		return facadeReviewerResult{}, err
	}
	wantSchema := reviewAdmittedResultSchema
	if expected.Schema == reviewtransaction.ArtifactSubjectSchemaV1 {
		wantSchema = reviewtransaction.AdmittedReviewerResultSchemaV1
	}
	if envelope.Schema != wantSchema || !reflect.DeepEqual(envelope.Subject, expected) {
		return facadeReviewerResult{}, errors.New("captured reviewer result does not contain the exact provider-owned subject")
	}
	if err := envelope.Admission.Validate(expected); err != nil {
		return facadeReviewerResult{}, err
	}
	canonical, err := json.Marshal(envelope.Result)
	if err != nil {
		return facadeReviewerResult{}, err
	}
	canonical = append(canonical, '\n')
	native := envelope.Result.nativeLensResult()
	native.Lens = expected.Lens
	result, revalidated, err := reviewtransaction.AdmitArtifact(ctx, reviewtransaction.ArtifactAdmissionRequest{
		ExpectedSubject: expected, FrozenContext: frozen, EchoedSubjectHash: envelope.Result.SubjectHash,
		Inspection: envelope.Result.Inspection, Result: native,
		CandidateCausalFindingIDs: envelope.Admission.CandidateCausalFindingIDs,
		RawPayload:                canonical, CanonicalPayload: canonical,
	})
	if err != nil || revalidated.Decision != reviewtransaction.ArtifactAdmissionCompleted ||
		revalidated.CanonicalSHA256 != envelope.Admission.CanonicalSHA256 || result.ResultHash != envelope.Admission.ResultHash {
		return facadeReviewerResult{}, errors.New("captured reviewer result no longer satisfies its admission record")
	}
	return envelope.Result, nil
}

func verifiedCandidateCausalFindingIDs(ctx context.Context, repo string, snapshot reviewtransaction.Snapshot, result reviewtransaction.LensResult) ([]string, error) {
	ids := make([]string, 0)
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	for _, finding := range result.Findings {
		if !facadeSevere(finding.Severity) {
			continue
		}
		switch finding.CausalDisposition {
		case reviewtransaction.CausalIntroduced, reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalWorsened:
			changed, err := builder.CandidateLocationSupportsCausality(ctx, snapshot, finding.Location, finding.CausalDisposition)
			if err != nil {
				if errors.Is(err, reviewtransaction.ErrInvalidFindingLocation) {
					return nil, reviewtransaction.NewArtifactLocationAdmissionError(finding.ID, finding.Location, err)
				}
				return nil, fmt.Errorf("verify candidate causality for finding %q: %w", finding.ID, err)
			}
			if changed {
				ids = append(ids, finding.ID)
			}
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func decodeFacadeJSONBytes(payload []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("input contains multiple JSON values")
	}
	return nil
}
