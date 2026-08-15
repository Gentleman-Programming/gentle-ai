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
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	openCodeReviewTransportSchema     = reviewerprovider.TransportCapability
	openCodeTaskHostOutputLimit       = reviewResultArtifactLimit + 8<<10
	openCodeTransportEnvelopeMaxBytes = reviewResultArtifactLimit*2 + 8<<10
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

type openCodeTransportTaskBinding struct {
	RepositoryContext string
	Lens              string
	Role              reviewProviderRole
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
	nonce          string
}

var openCodeTransportRandom = rand.Read
var openCodeTransportCompletionTimeout = 120 * time.Second

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
	completionContext, cancel := context.WithTimeout(context.Background(), openCodeTransportCompletionTimeout)
	defer cancel()
	completion, err := decodeOpenCodeTransportEnvelopeContext(completionContext, decoder)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return openCodeTransportFailure("opencode_review_transport_completion_timeout")
		}
		return openCodeTransportFailure("opencode_review_transport_completion_missing")
	}
	if err := validateOpenCodeTransportCompletion(completion, session.nonce); err != nil {
		return err
	}
	trailingContext, cancelTrailing := context.WithTimeout(context.Background(), openCodeTransportCompletionTimeout)
	defer cancelTrailing()
	if _, err := decodeOpenCodeTransportEnvelopeContext(trailingContext, decoder); !errors.Is(err, io.EOF) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return openCodeTransportFailure("opencode_review_transport_completion_timeout")
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
	binding, err := decodeOpenCodeTransportBinding(envelope.Prompt)
	if err != nil {
		return openCodeTransportSession{}, err
	}
	root, contextBinding, err := reviewtransaction.ResolveReviewRepositoryContextBinding(ctx, binding.RepositoryContext)
	if err != nil {
		return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
	}
	store, record, err := discoverCompactFacadeReview(ctx, root, contextBinding.LineageID, false)
	if err != nil || record.Revision != contextBinding.Revision {
		return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
	}
	session := openCodeTransportSession{binding: binding, root: root, store: store, record: record}
	if binding.Role != "" {
		invocation, err := reviewProviderRoleTaskRequest(ctx, root, store.Dir, record.State, record.Revision, binding.Role)
		if err != nil {
			return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
		}
		session.providerPrompt = invocation.Prompt()
	} else {
		request, err := reviewProviderMaterialize(ctx, reviewLensContextDependencies(), binding.RepositoryContext, binding.Lens)
		if err != nil || request.Binding.Revision != record.Revision || request.Binding.Lineage != record.State.LineageID {
			return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
		}
		session.lensRequest, session.providerPrompt = request, request.Invocation.Prompt()
	}
	nonce, err := newOpenCodeTransportNonce()
	if err != nil {
		return openCodeTransportSession{}, openCodeTransportFailure("opencode_review_transport_materialization_unavailable")
	}
	session.nonce = nonce
	return session, nil
}

func openCodeTransportComplete(ctx context.Context, session openCodeTransportSession, envelope openCodeTransportEnvelope) (openCodeTransportEnvelope, error) {
	if envelope.Error != "" {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_task_transport_failed")
	}
	hostOutput, err := decodeOpenCodeTaskHostOutput([]byte(*envelope.Output))
	if err != nil {
		return openCodeTransportEnvelope{}, err
	}
	if err := authorizeReviewAuthorityMutation(ctx, session.root); err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_authority_unavailable")
	}
	store, record, err := discoverCompactFacadeReview(ctx, session.root, session.record.State.LineageID, false)
	if err != nil || store.Dir != session.store.Dir || record.Revision != session.record.Revision {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_completion_unavailable")
	}
	if session.binding.Role != "" {
		if err := openCodeTransportCaptureRole(ctx, session.root, store, record, session.binding.Role, hostOutput); err != nil {
			return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_provider_role_result_refused")
		}
		output := `{"schema":"gentle-ai.opencode-review-provider-role/v1","role":"` + string(session.binding.Role) + `","captured":true}`
		return openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "result", Output: &output}, nil
	}
	admitted, err := reviewProviderAdmitRaw(ctx, session.root, record.State, record.Revision, session.lensRequest.Frozen, session.lensRequest.Subject, hostOutput)
	if err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_reviewer_result_refused")
	}
	path := filepath.Join(store.Dir, reviewResultArtifactPath(session.lensRequest.Binding.Order, session.lensRequest.Binding.Lens))
	captured, err := store.CaptureAdmittedReviewerResult(ctx, reviewtransaction.CompactAdmittedReviewerResultRequest{
		ExpectedRevision: record.Revision, TargetIdentity: session.lensRequest.Binding.Target, FrozenContext: admitted.Frozen,
		ArtifactSubject: admitted.Subject, Inspection: admitted.Result.Inspection, Result: admitted.NativeResult,
		CandidateCausalFindingIDs: admitted.CandidateCausalFindingIDs, RawPayload: hostOutput,
		PreparePublication: func(current reviewtransaction.CompactState) error {
			return archiveQuarantinedReviewerArtifact(session.lensRequest.Store.Dir, current, session.lensRequest.Binding.Order, path)
		},
	})
	if err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_capture_failed")
	}
	if err := reviewtransaction.PublishLensContextEmission(store.Dir, reviewtransaction.LensContextEmission{
		Schema: reviewtransaction.LensContextEmissionSchema, LineageID: session.lensRequest.Binding.Lineage,
		TargetIdentity: session.lensRequest.Binding.Target, AuthorityRevision: session.lensRequest.Binding.Revision,
		Lens: session.lensRequest.Binding.Lens, SelectedOrder: session.lensRequest.Binding.Order, SubjectHash: captured.Subject.SubjectHash,
		Level: reviewtransaction.ReviewerContextLevelProviderContract,
	}); err != nil {
		return openCodeTransportEnvelope{}, openCodeTransportFailure("opencode_review_transport_emission_unavailable")
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

func decodeOpenCodeTransportBinding(prompt string) (openCodeTransportTaskBinding, error) {
	line, _, _ := strings.Cut(prompt, "\n")
	if encoded, found := strings.CutPrefix(line, reviewProviderTaskBindingHeader+" "); found {
		decoder := json.NewDecoder(strings.NewReader(encoded))
		decoder.DisallowUnknownFields()
		var binding reviewProviderTaskBinding
		if err := decoder.Decode(&binding); err != nil {
			return openCodeTransportTaskBinding{}, errors.New("opencode_review_transport_binding_invalid: Task prompt binding is not provider-issued JSON") // refusal:by-design world-action: the managed shim must relay the provider-issued task binding unchanged
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || binding.RepositoryContext == "" || binding.Role == "" {
			return openCodeTransportTaskBinding{}, errors.New("opencode_review_transport_binding_invalid: Task prompt binding is incomplete") // refusal:by-design world-action: a managed provider role task requires its complete opaque Go binding
		}
		return openCodeTransportTaskBinding{RepositoryContext: binding.RepositoryContext, Role: reviewProviderRole(binding.Role)}, nil
	}
	encoded, found := strings.CutPrefix(line, reviewLensContextBindingHeader+" ")
	if !found {
		return openCodeTransportTaskBinding{}, errors.New("opencode_review_transport_binding_invalid: Task prompt has no provider-issued review binding") // refusal:by-design world-action: the managed shim must relay a collect Task prompt emitted by native review authority
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var binding reviewLensContextBinding
	if err := decoder.Decode(&binding); err != nil {
		return openCodeTransportTaskBinding{}, errors.New("opencode_review_transport_binding_invalid: Task prompt binding is not provider-issued JSON") // refusal:by-design world-action: a generated collect Task prompt is required; caller-authored binding data is refused
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) || binding.RepositoryContext == "" || binding.Lens == "" {
		return openCodeTransportTaskBinding{}, errors.New("opencode_review_transport_binding_invalid: Task prompt binding is incomplete") // refusal:by-design world-action: a generated collect Task prompt must retain its complete provider binding
	}
	return openCodeTransportTaskBinding{RepositoryContext: binding.RepositoryContext, Lens: binding.Lens}, nil
}

func openCodeTransportCaptureRole(ctx context.Context, root string, store reviewtransaction.CompactStore, record reviewtransaction.CompactRecord, role reviewProviderRole, raw []byte) error {
	switch role {
	case reviewerprovider.RoleRefuter:
		_, err := reviewProviderCaptureRefuterRaw(ctx, root, store, record.State, record.Revision, raw)
		return err
	case reviewerprovider.RoleTargetedValidator:
		_, _, err := reviewProviderCaptureTargetedValidatorRaw(ctx, root, store, record.State, record.Revision, raw)
		return err
	default:
		return fmt.Errorf("unsupported provider role task %q", role) // refusal:by-design world-action: the relay session role is fixed by the Go-issued Task binding
	}
}

func newOpenCodeTransportNonce() (string, error) {
	nonce := make([]byte, 16)
	if _, err := openCodeTransportRandom(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce), nil
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

func reviewResultArtifactPath(order int, lens string) string {
	return fmt.Sprintf("%s/%02d-%s.json", reviewtransaction.CompactReviewerResultsDir, order, lens)
}

func openCodeTransportFailure(code string) error {
	return fmt.Errorf("%s: OpenCode Task transport did not produce a capturable reviewer result; run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition` before retrying", code)
}
