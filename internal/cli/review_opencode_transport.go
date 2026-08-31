package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	openCodeReviewTransportSchema          = reviewerprovider.TransportCapability
	openCodeTaskHostOutputLimit            = reviewResultArtifactLimit + 8<<10
	openCodeTransportEnvelopeMaxBytes      = reviewResultArtifactLimit*2 + 8<<10
	openCodeTransportMaterializationHeader = "GENTLE_AI_REVIEW_PROVIDER_MATERIALIZATION"
)

// openCodeTransportEnvelope is the strict bidirectional wire protocol shared
// with the managed OpenCode shim. It relays only opaque prompt and output
// bytes; Go owns prompt materialization, admission, and capture.
type openCodeTransportEnvelope struct {
	Schema    string  `json:"schema"`
	Operation string  `json:"operation"`
	Nonce     string  `json:"nonce,omitempty"`
	Prompt    string  `json:"prompt,omitempty"`
	Output    *string `json:"output,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type openCodeTaskOutputError struct{ Code string }

func (err *openCodeTaskOutputError) Error() string {
	return err.Code + ": OpenCode Task output is incomplete or malformed"
}

type openCodeTransportBindingError struct{ detail string }

func (err *openCodeTransportBindingError) Error() string {
	return "opencode_review_transport_binding_invalid: " + err.detail
}

func openCodeTransportBindingInvalid(detail string) error {
	return &openCodeTransportBindingError{detail: detail}
}

type openCodeTransportTaskBinding struct {
	LineageID         string
	Revision          string
	TargetIdentity    string
	RepositoryContext string
	Lens              string
	Order             openCodeHostBindingOrder
	SubjectHash       string
	Role              reviewProviderRole
	// canonicalTaskPrompt is the Go-rebuilt binding line for a provider role
	// task. It replaces the host-authored prompt before materialization so no
	// caller-authored byte can ride the relay into the reviewer child.
	canonicalTaskPrompt string
}

// openCodeHostBindingOrder admits the contract's host-owned order field. The
// collect input delivers `order` as a decimal string argument and the
// orchestration contract tells the host to assemble the binding JSON from
// exactly that input, so the JSON number 0 and the string "0" identify the
// same frozen slot. Presence is tracked so a provider-omitted order skips the
// value check instead of masquerading as slot 0.
type openCodeHostBindingOrder struct {
	value    int
	provided bool
}

func (order *openCodeHostBindingOrder) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if string(trimmed) == "null" {
		// A JSON null is an omitted field, not slot 0: the value check must
		// not bind an absent order to the first lens slot by accident.
		return nil
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return err
		}
		value, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil {
			return errors.New("order is not a decimal integer") // refusal:by-design world-action: the collect input delivers order as a decimal slot index
		}
		order.value, order.provided = value, true
		return nil
	}
	var value int
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return err
	}
	order.value, order.provided = value, true
	return nil
}

// openCodeHostLensBinding is the semantic admission shape for a first-contact
// host-assembled lens frame. The orchestration contract makes key order and
// whitespace host-owned, so admission accepts the contract's exact field set
// in any serialization the strict decoder recognizes and verifies every field
// value against the Go-resolved authority instead of comparing bytes. Unknown
// keys still refuse, missing required fields still refuse, and the reviewer
// child only ever receives Go-rebuilt canonical bytes.
type openCodeHostLensBinding struct {
	Lineage           string                   `json:"lineage"`
	Target            string                   `json:"target"`
	Lens              string                   `json:"lens"`
	Order             openCodeHostBindingOrder `json:"order"`
	Revision          string                   `json:"revision"`
	RepositoryContext string                   `json:"repository_context"`
	SubjectHash       string                   `json:"subject_hash"`
}

// openCodeTransportMaterialization carries the original, Go-issued Task
// binding alongside the provider prompt that Go reconstructed from it. A
// reintercepting hook can only pass through that exact reconstruction.
type openCodeTransportMaterialization struct {
	TaskPrompt string `json:"task_prompt"`
}

// openCodeTransportSession is deliberately process-local: a completion can
// only apply to the prompt materialized by this running relay, never to an
// independently forged bearer token.
type openCodeTransportSession struct {
	binding        openCodeTransportTaskBinding
	root           string
	store          reviewtransaction.CompactStore
	record         reviewtransaction.CompactRecord
	lensRequest    reviewProviderRequest
	providerPrompt []byte
	taskPrompt     string
	nonce          string
	passThrough    bool
}

var openCodeTransportRandom = rand.Read
var openCodeTransportTrailingClosureTimeout = 5 * time.Second

// openCodeTransportCompletionSafetyBound is a safety bound for a host that
// died silently without ever completing or closing the relay pipe; it is not
// the operating lifetime. The OpenCode host still owns the Task lifetime and
// decides the wait well inside this deadline. It deliberately mirrors
// reviewProviderRoleCaptureTimeout so relay waits share the repository's
// generous provider role capture deadline.
var openCodeTransportCompletionSafetyBound = reviewProviderRoleCaptureTimeout

func RunReviewOpenCodeTransport(args []string, stdout io.Writer) error {
	return runReviewOpenCodeTransport(args, os.Stdin, stdout)
}

func runReviewOpenCodeTransport(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New("review opencode-transport requires strict relay frames on standard input") // refusal:by-design world-action: the managed OpenCode shim starts this internal relay without caller-authored flags
	}
	if stdin == nil {
		return errors.New("opencode_review_transport_envelope_invalid: transport input is required") // refusal:by-design world-action: the generated shim must send relay frames on standard input
	}
	decoder := json.NewDecoder(io.LimitReader(stdin, openCodeTransportEnvelopeMaxBytes+1))
	decoder.DisallowUnknownFields()
	start, err := decodeOpenCodeTransportEnvelope(decoder)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("opencode_review_transport_envelope_invalid: transport input is required") // refusal:by-design world-action: the generated shim must send start and completion frames
		}
		return err
	}
	if err := validateOpenCodeTransportStart(start); err != nil {
		return err
	}
	session, err := openCodeTransportStart(context.Background(), start)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(stdout).Encode(openCodeTransportEnvelope{
		Schema: openCodeReviewTransportSchema, Operation: "prompt", Nonce: session.nonce, Prompt: string(session.providerPrompt),
	}); err != nil {
		return err
	}
	// The OpenCode host owns the Task lifetime. Its completion, pipe closure, or
	// child termination decides this wait; a tight Go deadline can preempt valid
	// work. The outer safety bound only backstops a host that died silently.
	completionContext, cancelCompletion := context.WithTimeout(context.Background(), openCodeTransportCompletionSafetyBound)
	defer cancelCompletion()
	completion, err := decodeOpenCodeTransportEnvelopeContext(completionContext, decoder)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return openCodeTransportFailure("opencode_review_transport_provider_result_missing")
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return openCodeTransportFailure("opencode_review_transport_completion_safety_bound_exceeded")
		}
		return err
	}
	if err := validateOpenCodeTransportCompletion(completion, session.nonce); err != nil {
		return err
	}
	trailingContext, cancelTrailing := context.WithTimeout(context.Background(), openCodeTransportTrailingClosureTimeout)
	defer cancelTrailing()
	if _, err := decodeOpenCodeTransportEnvelopeContext(trailingContext, decoder); !errors.Is(err, io.EOF) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return openCodeTransportFailure("opencode_review_transport_trailing_closure_timeout")
		}
		return errors.New("opencode_review_transport_envelope_invalid: relay accepts exactly one start frame and one completion frame") // refusal:by-design world-action: the shim must close the live child stdin after its matching Task completion
	}
	response, err := openCodeTransportComplete(context.Background(), session, completion)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(response)
}

func decodeOpenCodeTransportEnvelope(decoder *json.Decoder) (openCodeTransportEnvelope, error) {
	var envelope openCodeTransportEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		if errors.Is(err, io.EOF) {
			return openCodeTransportEnvelope{}, io.EOF
		}
		return openCodeTransportEnvelope{}, errors.New("opencode_review_transport_envelope_invalid: transport input must be one strict JSON envelope") // refusal:by-design world-action: the generated shim must encode the closed relay envelope without extra fields
	}
	if envelope.Schema != openCodeReviewTransportSchema {
		return openCodeTransportEnvelope{}, errors.New("opencode_review_transport_envelope_invalid: transport schema is unsupported") // refusal:by-design world-action: the managed shim must use the schema emitted by this binary
	}
	return envelope, nil
}

func decodeOpenCodeTransportEnvelopeContext(ctx context.Context, decoder *json.Decoder) (openCodeTransportEnvelope, error) {
	type result struct {
		envelope openCodeTransportEnvelope
		err      error
	}
	frames := make(chan result)
	go func() {
		envelope, err := decodeOpenCodeTransportEnvelope(decoder)
		select {
		case frames <- result{envelope: envelope, err: err}:
		case <-ctx.Done():
		}
	}()
	select {
	case frame := <-frames:
		return frame.envelope, frame.err
	case <-ctx.Done():
		return openCodeTransportEnvelope{}, ctx.Err()
	}
}

func validateOpenCodeTransportStart(envelope openCodeTransportEnvelope) error {
	if envelope.Operation != "start" || envelope.Prompt == "" || envelope.Nonce != "" || envelope.Output != nil || envelope.Error != "" {
		return errors.New("opencode_review_transport_envelope_invalid: relay start requires only the original Task prompt") // refusal:by-design world-action: the shim must relay the original bound Task prompt before the host Task starts
	}
	return nil
}

func validateOpenCodeTransportCompletion(envelope openCodeTransportEnvelope, nonce string) error {
	if envelope.Operation != "complete" || envelope.Nonce != nonce || (envelope.Output == nil && envelope.Error == "") || (envelope.Output != nil && envelope.Error != "") {
		return errors.New("opencode_review_transport_envelope_invalid: relay completion must match the live Task relay and contain exactly one host outcome") // refusal:by-design world-action: only the managed shim holding this live child may complete the bound Task
	}
	return nil
}

func openCodeTransportStart(ctx context.Context, envelope openCodeTransportEnvelope) (openCodeTransportSession, error) {
	taskPrompt, marked, err := decodeOpenCodeTransportMaterialization(envelope.Prompt)
	if err != nil {
		return openCodeTransportSession{}, err
	}
	session, err := openCodeTransportStartBound(ctx, taskPrompt)
	if err != nil {
		return openCodeTransportSession{}, err
	}
	materialized, err := openCodeTransportMaterializedPrompt(session.taskPrompt, session.providerPrompt)
	if err != nil {
		return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
	}
	session.providerPrompt = []byte(materialized)
	// Byte equality against the Go rebuild identifies a re-interception, and it
	// stays exact. It classifies the frame; it does not admit one. Carrying the
	// marker is not proof of Go authorship: an OpenCode host persists the
	// plugin-mutated Task argument -- the whole materialization -- into its own
	// transcript, so its driver model re-types a paraphrased copy on the next
	// collection attempt. Only the exact bytes are a re-interception whose
	// completion belongs to the relay that issued them; a copy is an ordinary
	// host-authored first-contact frame that this relay must materialize and
	// capture itself. Refusing it instead wedged the reviewer slot with no
	// caller-side exit. Admitting it grants nothing, because the reviewer child
	// receives only the bytes rebuilt above from live authority and every
	// echoed byte was already discarded with the incoming prompt.
	session.passThrough = marked && envelope.Prompt == materialized
	nonce, err := newOpenCodeTransportNonce()
	if err != nil {
		return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
	}
	session.nonce = nonce
	return session, nil
}

func openCodeTransportStartBound(ctx context.Context, taskPrompt string) (openCodeTransportSession, error) {
	binding, err := decodeOpenCodeTransportBinding(taskPrompt)
	if err != nil {
		return openCodeTransportSession{}, err
	}
	// The transport runs inside the repository it reviews, and it takes no
	// flags, so the process cwd is the independent source the provider-issued
	// context digest is checked against.
	root, err := (reviewtransaction.SnapshotBuilder{Repo: "."}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, binding.LineageID, false)
	if err != nil {
		// A host frame naming a lineage this repository does not hold is a
		// tampered binding, not an unavailable materialization.
		return openCodeTransportSession{}, openCodeTransportBindingInvalid("Task lineage does not match live compact review authority")
	}
	requested := reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
	}
	contextBinding, contextErr := requested, error(nil)
	if _, resolved, err := reviewtransaction.ResolveReviewRepositoryContextBinding(ctx, root, binding.RepositoryContext, requested); err == nil {
		contextBinding = resolved
	} else {
		// The digest commits to one exact repository and binding, so a mismatch
		// is a tampered frame. Let the binding validator below name the field
		// the live authority disagrees with instead of collapsing every tamper
		// into one generic materialization failure.
		contextErr = err
		contextBinding = reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: record.State.LineageID, TargetIdentity: record.State.InitialSnapshot.Identity,
			Revision: record.State.CapturePhaseRevision,
		}
		if record.State.State != reviewtransaction.StateReviewing {
			contextBinding.TargetIdentity = record.State.CurrentSnapshot.Identity
		}
	}
	if err := validateReviewProviderTaskAuthorityBinding(ReviewTransitionBinding{
		LineageID: binding.LineageID, Revision: binding.Revision, TargetIdentity: binding.TargetIdentity,
		RepositoryContext: binding.RepositoryContext,
	}, contextBinding, record); err != nil {
		return openCodeTransportSession{}, err
	}
	if contextErr != nil {
		return openCodeTransportSession{}, openCodeTransportBindingInvalid("Task repository context does not match the repository and binding it commits to")
	}
	if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
		return openCodeTransportSession{}, openCodeTransportAuthorityUnavailable(err)
	}
	// Both branches replace the host-authored task prompt with Go-rebuilt
	// canonical bytes -- the role branch with the Go-issued binding line, the
	// lens branch with the Go-marshaled authority binding -- so no
	// caller-authored byte can ride the materialization envelope into the
	// reviewer Task. Host frames are admitted by field value only; every
	// value below is checked against the Go-resolved authority, fail-closed.
	session := openCodeTransportSession{binding: binding, root: root, store: store, record: record, taskPrompt: taskPrompt}
	if binding.Role != "" {
		session.taskPrompt = binding.canonicalTaskPrompt
		invocation, err := reviewProviderRoleTaskRequest(ctx, root, store.Dir, record.State, record.State.CapturePhaseRevision, binding.Role)
		if err != nil {
			return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
		}
		session.providerPrompt = invocation.Prompt()
	} else {
		if !slices.Contains(record.State.SelectedLenses, binding.Lens) {
			return openCodeTransportSession{}, openCodeTransportBindingInvalid("Task lens is not a provider-selected lens for this review")
		}
		request, err := reviewProviderMaterialize(ctx, reviewLensContextDependencies(), root, binding.RepositoryContext, binding.Lens, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: binding.LineageID, TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
		})
		if err != nil || request.Binding.Revision != record.State.CapturePhaseRevision || request.Binding.Lineage != record.State.LineageID {
			return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
		}
		if binding.Order.provided && binding.Order.value != request.Binding.Order {
			return openCodeTransportSession{}, openCodeTransportBindingInvalid("Task order does not match the provider-selected lens slot")
		}
		if binding.SubjectHash != "" && binding.SubjectHash != request.Binding.SubjectHash {
			return openCodeTransportSession{}, openCodeTransportBindingInvalid("Task subject hash does not match the provider-issued artifact subject")
		}
		encoded, err := json.Marshal(request.Binding)
		if err != nil {
			return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
		}
		session.taskPrompt = reviewLensContextBindingHeader + " " + string(encoded)
		session.lensRequest, session.providerPrompt = request, request.Invocation.Prompt()
	}
	return session, nil
}

// openCodeProviderRoleResultEnvelope renders the exact published
// gentle-ai.opencode-review-provider-role/v1 envelope for a captured provider
// role result. It is the single wording source for those bytes, so the
// transport and the published-schema conformance test cannot drift apart.
func openCodeProviderRoleResultEnvelope(role reviewProviderRole) string {
	return `{"schema":"gentle-ai.opencode-review-provider-role/v1","role":"` + string(role) + `","captured":true}`
}

func openCodeTransportComplete(ctx context.Context, session openCodeTransportSession, envelope openCodeTransportEnvelope) (openCodeTransportEnvelope, error) {
	if envelope.Error != "" {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_task_transport_failed")
	}
	hostOutput, err := openCodeTransportCompletionHostOutput(envelope)
	if err != nil {
		return openCodeTransportEnvelope{}, err
	}
	if session.passThrough {
		output := *envelope.Output
		return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "result", Output: &output}, nil
	}
	if err := authorizeReviewAuthorityMutation(ctx, session.root); err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportAuthorityUnavailable(err)
	}
	store, record, err := discoverCompactFacadeReview(ctx, session.root, session.record.State.LineageID, false)
	if err != nil || store.Dir != session.store.Dir || record.State.CapturePhaseRevision != session.record.State.CapturePhaseRevision {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_completion_unavailable")
	}
	if session.binding.Role != "" {
		closure, err := openCodeTransportCaptureRole(ctx, session.root, store, record, session.binding.Role, hostOutput)
		if err != nil {
			return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_provider_role_result_refused")
		}
		if closure != nil {
			if session.binding.Role == reviewerprovider.RoleRefuter {
				closure.Operation = reviewCaptureRefuterCaptureOperation
			}
			payload, err := json.Marshal(closure)
			if err != nil {
				return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_provider_role_result_refused")
			}
			output := string(payload)
			return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "result", Output: &output}, nil
		}
		output := openCodeProviderRoleResultEnvelope(session.binding.Role)
		return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "result", Output: &output}, nil
	}
	admitted, err := reviewProviderAdmitRaw(ctx, session.root, record.State, record.State.CapturePhaseRevision, session.lensRequest.Frozen, session.lensRequest.Subject, hostOutput)
	if err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_reviewer_result_refused")
	}
	captured, err := store.CaptureAdmittedReviewerResult(ctx, reviewtransaction.CompactAdmittedReviewerResultRequest{
		ExpectedRevision: record.State.CapturePhaseRevision, TargetIdentity: session.lensRequest.Binding.Target, FrozenContext: admitted.Frozen,
		ArtifactSubject: admitted.Subject, Inspection: admitted.Result.Inspection, Result: admitted.NativeResult,
		CandidateCausalFindingIDs: admitted.CandidateCausalFindingIDs, RawPayload: hostOutput,
	})
	if err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_capture_failed")
	}
	currentRecord, currentErr := store.LoadContext(ctx)
	if currentErr != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_capture_failed")
	}
	closure, err := closeReviewOnLastCapturedLens(ctx, session.root, store, currentRecord, model.AgentOpenCode)
	if err != nil && !reviewLastCapturedLensClosureSuperseded(store, currentRecord) {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_capture_failed")
	}
	if closure != nil {
		payload, err := json.Marshal(closure)
		if err != nil {
			return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_capture_failed")
		}
		output := string(payload)
		return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "result", Output: &output}, nil
	}
	artifact := reviewResultArtifact{
		Schema: reviewResultArtifactSchema, Capability: reviewResultArtifactCapability,
		SHA256: captured.Slot.Digest, LineageID: session.lensRequest.Binding.Lineage,
		TargetIdentity: session.lensRequest.Binding.Target, Lens: session.lensRequest.Binding.Lens, SelectedOrder: session.lensRequest.Binding.Order,
		SubjectHash: captured.Subject.SubjectHash, AdmissionDecision: captured.Admission.Decision,
	}
	artifact.Reference = reviewResultReference(artifact)
	payload, err := json.Marshal(artifact)
	if err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_capture_failed")
	}
	output := string(payload)
	return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "result", Output: &output}, nil
}

// decodeOpenCodeTransportBinding admits the first line of a host Task prompt
// semantically: the exact contract field set in host-owned key order and
// whitespace, unknown keys refused, missing required fields refused. Field
// values are verified against the Go-resolved authority by the caller; bytes
// are never compared for a first-contact host-assembled frame, because the
// orchestration contract makes the binding serialization host-owned. Trailing
// prompt lines are host-authored task prose and never reach the reviewer
// child, which only ever receives the Go-rebuilt materialization.
func decodeOpenCodeTransportBinding(prompt string) (openCodeTransportTaskBinding, error) {
	line, _, _ := strings.Cut(prompt, "\n")
	if encoded, found := strings.CutPrefix(line, reviewProviderTaskBindingHeader+" "); found {
		decoder := json.NewDecoder(strings.NewReader(encoded))
		decoder.DisallowUnknownFields()
		var binding reviewProviderTaskBinding
		if err := decoder.Decode(&binding); err != nil {
			return openCodeTransportTaskBinding{}, openCodeTransportBindingInvalid("Task prompt binding is not provider-issued JSON")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || binding.RepositoryContext == "" || binding.Role == "" {
			return openCodeTransportTaskBinding{}, openCodeTransportBindingInvalid("Task prompt binding is incomplete")
		}
		issued, err := newReviewProviderTask(reviewProviderRole(binding.Role), ReviewTransitionBinding{
			LineageID: binding.LineageID, Revision: binding.Revision, TargetIdentity: binding.TargetIdentity,
			RepositoryContext: binding.RepositoryContext,
		})
		if err != nil {
			return openCodeTransportTaskBinding{}, openCodeTransportBindingInvalid("Task prompt binding is not a Go-issuable provider role binding")
		}
		return openCodeTransportTaskBinding{
			LineageID: binding.LineageID, Revision: binding.Revision, TargetIdentity: binding.TargetIdentity,
			RepositoryContext: binding.RepositoryContext, Role: reviewProviderRole(binding.Role),
			canonicalTaskPrompt: issued.Prompt,
		}, nil
	}
	encoded, found := strings.CutPrefix(line, reviewLensContextBindingHeader+" ")
	if !found {
		return openCodeTransportTaskBinding{}, openCodeTransportBindingInvalid("Task prompt has no provider-issued review binding")
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var binding openCodeHostLensBinding
	if err := decoder.Decode(&binding); err != nil {
		return openCodeTransportTaskBinding{}, openCodeTransportBindingInvalid("Task prompt binding is not provider-issued JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || binding.Lineage == "" || binding.Target == "" ||
		binding.Revision == "" || binding.RepositoryContext == "" || binding.Lens == "" {
		return openCodeTransportTaskBinding{}, openCodeTransportBindingInvalid("Task prompt binding is incomplete")
	}
	return openCodeTransportTaskBinding{
		LineageID: binding.Lineage, Revision: binding.Revision, TargetIdentity: binding.Target,
		RepositoryContext: binding.RepositoryContext, Lens: binding.Lens, Order: binding.Order, SubjectHash: binding.SubjectHash,
	}, nil
}

// validateReviewProviderTaskAuthorityBinding is the sole live comparison seam
// for a Go-issued provider task, its resolved opaque context, and the current
// compact authority. The adapter only carries opaque bytes; this check remains
// in Go before both materialization and capture.
func validateReviewProviderTaskAuthorityBinding(binding ReviewTransitionBinding, contextBinding reviewtransaction.ReviewRepositoryContextBinding, record reviewtransaction.CompactRecord) error {
	if binding.LineageID != contextBinding.LineageID {
		return openCodeTransportBindingInvalid("Task lineage does not match the resolved repository context")
	}
	if binding.Revision != contextBinding.Revision {
		return openCodeTransportBindingInvalid("Task revision does not match the resolved repository context")
	}
	if binding.TargetIdentity != contextBinding.TargetIdentity {
		return openCodeTransportBindingInvalid("Task target does not match the resolved repository context")
	}
	// ResolveReviewRepositoryContextBinding already validates the context target
	// against compact state. Requiring the same lineage and revision on the
	// freshly discovered record prevents a stale locator from selecting another
	// authority before any provider prompt can be materialized.
	if record.State.LineageID != binding.LineageID || record.State.CapturePhaseRevision != binding.Revision {
		return openCodeTransportBindingInvalid("Task binding does not match live compact review authority")
	}
	return nil
}

func openCodeTransportMaterializedPrompt(taskPrompt string, providerPrompt []byte) (string, error) {
	if taskPrompt == "" || len(providerPrompt) == 0 {
		return "", errors.New("provider materialization is incomplete") // refusal:by-design world-action: only a complete Go reconstruction may reach a provider Task
	}
	payload, err := json.Marshal(openCodeTransportMaterialization{TaskPrompt: taskPrompt})
	if err != nil {
		return "", err
	}
	return openCodeTransportMaterializationHeader + " " + string(payload) + "\n" + string(providerPrompt), nil
}

func decodeOpenCodeTransportMaterialization(prompt string) (taskPrompt string, reintercepted bool, err error) {
	line, providerPrompt, found := strings.Cut(prompt, "\n")
	encoded, marked := strings.CutPrefix(line, openCodeTransportMaterializationHeader+" ")
	if !marked {
		return prompt, false, nil
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var materialization openCodeTransportMaterialization
	if err := decoder.Decode(&materialization); err != nil {
		return "", false, openCodeTransportBindingInvalid("Task prompt materialization is not Go-issued JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || materialization.TaskPrompt == "" || !found || providerPrompt == "" {
		return "", false, openCodeTransportBindingInvalid("Task prompt materialization is incomplete")
	}
	return materialization.TaskPrompt, true, nil
}

func openCodeTransportCaptureRole(ctx context.Context, root string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord, role reviewProviderRole, raw []byte) (*reviewLastEventClosureResult, error) {
	switch role {
	case reviewerprovider.RoleRefuter:
		if _, err := reviewProviderCaptureRefuterRaw(ctx, root, store, record.State, record.State.CapturePhaseRevision, raw); err != nil {
			return nil, err
		}
		current, err := store.LoadContext(ctx)
		if err != nil {
			return nil, err
		}
		return closeReviewOnLastCapturedLens(ctx, root, store, current, model.AgentOpenCode)
	case reviewerprovider.RoleTargetedValidator:
		_, _, closure, err := reviewProviderCloseTargetedValidatorRaw(ctx, root, store, record.State, record.State.CapturePhaseRevision, raw)
		return closure, err
	default:
		return nil, fmt.Errorf("unsupported provider role task %q", role) // refusal:by-design world-action: the relay session role is fixed by the Go-issued Task binding
	}
}

func newOpenCodeTransportNonce() (string, error) {
	nonce := make([]byte, 16)
	if _, err := openCodeTransportRandom(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce), nil
}

// openCodeTransportCompletionHostOutput fail-closes a completion frame into
// bounded host-output bytes. A schema-valid completion may carry neither an
// error nor an output; that frame is a typed relay failure here, never a nil
// dereference, on both the pass-through and the capturing branch.
func openCodeTransportCompletionHostOutput(envelope openCodeTransportEnvelope) ([]byte, error) {
	if envelope.Output == nil {
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_empty"}
	}
	return decodeOpenCodeTaskHostOutput([]byte(*envelope.Output))
}

func decodeOpenCodeTaskHostOutput(raw []byte) ([]byte, error) {
	if len(raw) > openCodeTaskHostOutputLimit {
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_truncated"}
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_empty"}
	}
	if !bytes.HasPrefix(trimmed, []byte("<task")) && !bytes.Contains(trimmed, []byte("<task_result")) {
		return boundedOpenCodeTaskPayload(raw)
	}
	const resultOpen = "<task_result>\n"
	const resultClose = "\n</task_result>\n</task>"
	openingEnd := bytes.IndexByte(trimmed, '>')
	if openingEnd < 0 || !bytes.HasPrefix(trimmed, []byte("<task ")) || !bytes.Contains(trimmed[:openingEnd], []byte(`state="completed"`)) {
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_malformed"}
	}
	body := trimmed[openingEnd+1:]
	if !bytes.HasPrefix(body, []byte("\n"+resultOpen)) || !bytes.HasSuffix(body, []byte(resultClose)) {
		content := body
		if bytes.HasPrefix(content, []byte("\n"+resultOpen)) {
			content = content[len("\n"+resultOpen):]
		}
		if bytes.Contains(content, []byte("<task")) || bytes.Contains(content, []byte("</task")) {
			return nil, &openCodeTaskOutputError{Code: "opencode_task_output_malformed"}
		}
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_truncated"}
	}
	result := body[len("\n"+resultOpen) : len(body)-len(resultClose)]
	if len(bytes.TrimSpace(result)) == 0 || bytes.Contains(result, []byte("<task")) || bytes.Contains(result, []byte("</task")) {
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_malformed"}
	}
	return boundedOpenCodeTaskPayload(result)
}

func boundedOpenCodeTaskPayload(payload []byte) ([]byte, error) {
	if len(payload) > reviewResultArtifactLimit {
		return nil, &openCodeTaskOutputError{Code: "opencode_task_output_truncated"}
	}
	return payload, nil
}

func openCodeTransportFailure(code string) error {
	return fmt.Errorf("%s: OpenCode Task transport did not produce a capturable reviewer result; run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` before retrying", code)
}

func openCodeTransportAuthorityUnavailable(cause error) error {
	return fmt.Errorf("opencode_review_transport_authority_unavailable: OpenCode Task transport did not produce a capturable reviewer result; run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` before retrying: %w", cause)
}
