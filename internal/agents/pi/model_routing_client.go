package pi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	modelRoutingTimeout = 5 * time.Second
	modelRoutingRequest = 64 << 10
	modelRoutingStderr  = 4 << 10
)

// ModelRoutingRequestContext identifies the context used by a model-routing
// operation. ConfigHome and LoadExtensions are omitted from the wire request
// when they have their zero values.
type ModelRoutingRequestContext struct {
	CWD            string
	AgentDir       string
	Target         ModelRoutingTarget
	ConfigHome     string
	LoadExtensions bool
}

// ModelRoutingClientErrorKind classifies a model-routing client failure.
type ModelRoutingClientErrorKind string

const (
	ModelRoutingClientErrorInvalidClient ModelRoutingClientErrorKind = "invalid-client"
	ModelRoutingClientErrorTransport     ModelRoutingClientErrorKind = "transport"
	ModelRoutingClientErrorSemantic      ModelRoutingClientErrorKind = "semantic"
	ModelRoutingClientErrorProtocol      ModelRoutingClientErrorKind = "protocol"
)

var (
	ErrModelRoutingClientInvalidClient = errors.New("nil model-routing client")
	ErrModelRoutingClientTransport     = errors.New("model-routing client transport failure")
	ErrModelRoutingClientSemantic      = errors.New("model-routing client semantic failure")
	ErrModelRoutingClientProtocol      = errors.New("model-routing client protocol failure")
)

// ModelRoutingClientError reports a failure after the client boundary. Cause
// remains available to errors.Is/errors.As, but Error deliberately contains no
// provider diagnostics, stderr, or cause text.
type ModelRoutingClientError struct {
	Candidate     ModelRoutingCandidate
	Kind          ModelRoutingClientErrorKind
	TransportKind TransportErrorKind
	ExitCode      int
	ExitClass     string
	Cause         error
}

func (e *ModelRoutingClientError) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := fmt.Sprintf("Pi model-routing client error (%s) for %q", e.Kind, e.Candidate.Path)
	if e.ExitCode != 0 {
		message += fmt.Sprintf(" exit code %d", e.ExitCode)
	}
	return message
}
func (e *ModelRoutingClientError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *ModelRoutingClientError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if typed, ok := target.(*ModelRoutingClientError); ok {
		return typed != nil && e.Kind != "" && e.Kind == typed.Kind
	}
	sentinel, ok := modelRoutingClientErrorSentinels[e.Kind]
	return ok && target == sentinel
}

var modelRoutingClientErrorSentinels = map[ModelRoutingClientErrorKind]error{
	ModelRoutingClientErrorInvalidClient: ErrModelRoutingClientInvalidClient,
	ModelRoutingClientErrorTransport:     ErrModelRoutingClientTransport,
	ModelRoutingClientErrorSemantic:      ErrModelRoutingClientSemantic,
	ModelRoutingClientErrorProtocol:      ErrModelRoutingClientProtocol,
}

// ModelRoutingClient invokes the selected model-routing executable for bounded
// operations. Its transport is fixed for production construction and scoped to
// the instance for tests.
type ModelRoutingClient struct {
	candidate    ModelRoutingCandidate
	capabilities Capabilities
	transport    modelRoutingTransport
}

// NewModelRoutingClient constructs a model-routing client using the bounded
// process transport and the selected candidate/capabilities.
func NewModelRoutingClient(candidate ModelRoutingCandidate, capabilities Capabilities) *ModelRoutingClient {
	return newModelRoutingClient(candidate, capabilities, RunBoundedModelRoutingProcess)
}

func newModelRoutingClient(candidate ModelRoutingCandidate, capabilities Capabilities, transport modelRoutingTransport) *ModelRoutingClient {
	return &ModelRoutingClient{
		candidate:    cleanModelRoutingCandidate(candidate),
		capabilities: cloneCapabilities(capabilities),
		transport:    transport,
	}
}

// Candidate returns the selected candidate by value.
func (c *ModelRoutingClient) Candidate() ModelRoutingCandidate {
	if c == nil {
		return ModelRoutingCandidate{}
	}
	return c.candidate
}

// Capabilities returns a defensive copy of the selected capabilities.
func (c *ModelRoutingClient) Capabilities() Capabilities {
	if c == nil {
		return Capabilities{}
	}
	return cloneCapabilities(c.capabilities)
}

// Inspect runs exactly one bounded inspect operation and returns the parsed
// inspection. Exit codes 2, 3, and 4 return their authoritative inspection with
// a typed semantic error; all other protocol or transport failures return no
// inspection.
func (c *ModelRoutingClient) Inspect(ctx context.Context, request ModelRoutingRequestContext) (ModelRoutingInspection, error) {
	if c == nil {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorInvalidClient, ModelRoutingCandidate{}, "", 0, "", ErrModelRoutingClientInvalidClient)
	}
	if c.transport == nil {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, "", 0, "", ErrModelRoutingClientTransport)
	}
	if ctx == nil {
		cause := transportError(TransportErrorInvalidOptions, ErrTransportInvalidOptions)
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, TransportErrorInvalidOptions, 0, "", cause)
	}
	if err := ctx.Err(); err != nil {
		kind := TransportErrorCanceled
		if errors.Is(err, context.DeadlineExceeded) {
			kind = TransportErrorTimeout
		}
		cause := transportError(kind, err)
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, kind, 0, "", cause)
	}

	payload, err := marshalModelRoutingInspectRequest(request)
	if err != nil {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", 0, "", err)
	}
	result, callErr := c.transport(ctx, c.candidate.Path, cloneTransportBytes(payload), ModelRoutingProcessOptions{
		Timeout:         modelRoutingTimeout,
		MaxRequestBytes: modelRoutingRequest,
		MaxStdoutBytes:  MaxModelRoutingResponseBytes,
		MaxStderrBytes:  modelRoutingStderr,
	})
	if callErr != nil {
		return c.handleTransportResult(result, callErr)
	}
	if result.ExitCode != 0 {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, "", ErrModelRoutingClientProtocol)
	}

	response, parseErr := ParseModelRoutingInspectResponse(cloneTransportBytes(result.Stdout))
	if parseErr != nil {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, "", parseErr)
	}
	if !response.OK || response.ExitClass != "success" {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, response.ExitClass, ErrModelRoutingClientProtocol)
	}
	return response.Result, nil
}

// Validate runs exactly one bounded validate operation and returns the parsed
// validation result. Exit codes 2, 3, and 4 return their authoritative result with
// a typed semantic error; all other protocol or transport failures return no result.
func (c *ModelRoutingClient) Validate(ctx context.Context, request ModelRoutingRequestContext, draft ModelRoutingDraft) (ModelRoutingValidationResult, error) {
	if c == nil {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorInvalidClient, ModelRoutingCandidate{}, "", 0, "", ErrModelRoutingClientInvalidClient)
	}
	if c.transport == nil {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, "", 0, "", ErrModelRoutingClientTransport)
	}
	if ctx == nil {
		cause := transportError(TransportErrorInvalidOptions, ErrTransportInvalidOptions)
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, TransportErrorInvalidOptions, 0, "", cause)
	}
	if err := ctx.Err(); err != nil {
		kind := TransportErrorCanceled
		if errors.Is(err, context.DeadlineExceeded) {
			kind = TransportErrorTimeout
		}
		cause := transportError(kind, err)
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, kind, 0, "", cause)
	}

	payload, err := marshalModelRoutingValidateRequest(request, draft)
	if err != nil {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", 0, "", err)
	}
	result, callErr := c.transport(ctx, c.candidate.Path, cloneTransportBytes(payload), ModelRoutingProcessOptions{
		Timeout:         modelRoutingTimeout,
		MaxRequestBytes: modelRoutingRequest,
		MaxStdoutBytes:  MaxModelRoutingResponseBytes,
		MaxStderrBytes:  modelRoutingStderr,
	})
	if callErr != nil {
		return c.handleValidateTransportResult(result, callErr)
	}
	if result.ExitCode != 0 {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, "", ErrModelRoutingClientProtocol)
	}

	response, parseErr := ParseModelRoutingValidateResponse(cloneTransportBytes(result.Stdout))
	if parseErr != nil {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, "", parseErr)
	}
	if !response.OK || response.ExitClass != "success" || !response.Result.OK {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, response.ExitClass, ErrModelRoutingClientProtocol)
	}
	return response.Result, nil
}

// Apply runs exactly one bounded apply operation and returns the provider-owned
// result. Recognized nonzero exits return their authoritative result with a typed
// semantic error; all other protocol or transport failures return no result.
func (c *ModelRoutingClient) Apply(ctx context.Context, request ModelRoutingRequestContext, draft ModelRoutingDraft) (ModelRoutingApplyResult, error) {
	if c == nil {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorInvalidClient, ModelRoutingCandidate{}, "", 0, "", ErrModelRoutingClientInvalidClient)
	}
	if c.transport == nil {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, "", 0, "", ErrModelRoutingClientTransport)
	}
	if ctx == nil {
		cause := transportError(TransportErrorInvalidOptions, ErrTransportInvalidOptions)
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, TransportErrorInvalidOptions, 0, "", cause)
	}
	if err := ctx.Err(); err != nil {
		kind := TransportErrorCanceled
		if errors.Is(err, context.DeadlineExceeded) {
			kind = TransportErrorTimeout
		}
		cause := transportError(kind, err)
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, kind, 0, "", cause)
	}

	payload, err := marshalModelRoutingApplyRequest(request, draft)
	if err != nil {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", 0, "", err)
	}
	result, callErr := c.transport(ctx, c.candidate.Path, cloneTransportBytes(payload), ModelRoutingProcessOptions{
		Timeout:         modelRoutingTimeout,
		MaxRequestBytes: modelRoutingRequest,
		MaxStdoutBytes:  MaxModelRoutingResponseBytes,
		MaxStderrBytes:  modelRoutingStderr,
	})
	if callErr != nil {
		return c.handleApplyTransportResult(result, callErr)
	}
	if result.ExitCode != 0 {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, "", ErrModelRoutingClientProtocol)
	}

	response, parseErr := ParseModelRoutingApplyResponse(cloneTransportBytes(result.Stdout))
	if parseErr != nil {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, "", parseErr)
	}
	if !response.OK || response.ExitClass != "success" || !response.Result.OK || response.Result.Outcome != ModelRoutingApplyOutcomeSuccess || !response.Result.Saved {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, "", result.ExitCode, response.ExitClass, ErrModelRoutingClientProtocol)
	}
	return response.Result, nil
}

func (c *ModelRoutingClient) handleApplyTransportResult(result ModelRoutingProcessResult, callErr error) (ModelRoutingApplyResult, error) {
	var transportErr *TransportError
	if !errors.As(callErr, &transportErr) || transportErr == nil {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, "", result.ExitCode, "", callErr)
	}
	if transportErr.Kind != TransportErrorNonzeroExit {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, transportErr.Kind, result.ExitCode, "", callErr)
	}
	expected, ok := map[int]struct {
		exitClass string
		outcome   ModelRoutingApplyOutcome
		saved     bool
	}{
		2: {"invalid-input", ModelRoutingApplyOutcomeValidationFailure, false},
		3: {"unsupported-contract", ModelRoutingApplyOutcomeValidationFailure, false},
		4: {"unavailable-runtime", ModelRoutingApplyOutcomeUnavailableRuntime, false},
		5: {"persistence", ModelRoutingApplyOutcomePersistenceFailure, false},
		6: {"partial", ModelRoutingApplyOutcomePartial, true},
	}[result.ExitCode]
	if !ok {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, "", errors.Join(callErr, ErrModelRoutingClientProtocol))
	}

	response, parseErr := ParseModelRoutingApplyResponse(cloneTransportBytes(result.Stdout))
	if parseErr != nil {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, "", errors.Join(callErr, parseErr))
	}
	if response.OK || response.Result.OK || response.ExitClass != expected.exitClass || response.Result.Outcome != expected.outcome || response.Result.Saved != expected.saved {
		return ModelRoutingApplyResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, response.ExitClass, errors.Join(callErr, ErrModelRoutingClientProtocol))
	}
	return response.Result, modelRoutingClientError(ModelRoutingClientErrorSemantic, c.candidate, transportErr.Kind, result.ExitCode, response.ExitClass, callErr)
}

func (c *ModelRoutingClient) handleValidateTransportResult(result ModelRoutingProcessResult, callErr error) (ModelRoutingValidationResult, error) {
	var transportErr *TransportError
	if !errors.As(callErr, &transportErr) || transportErr == nil {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, "", result.ExitCode, "", callErr)
	}
	if transportErr.Kind != TransportErrorNonzeroExit {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, transportErr.Kind, result.ExitCode, "", callErr)
	}
	if result.ExitCode != 2 && result.ExitCode != 3 && result.ExitCode != 4 {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, "", errors.Join(callErr, ErrModelRoutingClientProtocol))
	}

	response, parseErr := ParseModelRoutingValidateResponse(cloneTransportBytes(result.Stdout))
	if parseErr != nil {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, "", errors.Join(callErr, parseErr))
	}
	wantClass := map[int]string{2: "invalid-input", 3: "unsupported-contract", 4: "unavailable-runtime"}[result.ExitCode]
	if response.OK || response.Result.OK || response.ExitClass != wantClass {
		return ModelRoutingValidationResult{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, response.ExitClass, errors.Join(callErr, ErrModelRoutingClientProtocol))
	}
	return response.Result, modelRoutingClientError(ModelRoutingClientErrorSemantic, c.candidate, transportErr.Kind, result.ExitCode, response.ExitClass, callErr)
}

func (c *ModelRoutingClient) handleTransportResult(result ModelRoutingProcessResult, callErr error) (ModelRoutingInspection, error) {
	var transportErr *TransportError
	if !errors.As(callErr, &transportErr) || transportErr == nil {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, "", result.ExitCode, "", callErr)
	}
	if transportErr.Kind != TransportErrorNonzeroExit {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorTransport, c.candidate, transportErr.Kind, result.ExitCode, "", callErr)
	}
	if result.ExitCode != 2 && result.ExitCode != 3 && result.ExitCode != 4 {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, "", callErr)
	}

	response, parseErr := ParseModelRoutingInspectResponse(cloneTransportBytes(result.Stdout))
	if parseErr != nil {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, "", errors.Join(callErr, parseErr))
	}
	wantClass := map[int]string{2: "invalid-input", 3: "unsupported-contract", 4: "unavailable-runtime"}[result.ExitCode]
	if response.OK || response.ExitClass != wantClass {
		return ModelRoutingInspection{}, modelRoutingClientError(ModelRoutingClientErrorProtocol, c.candidate, transportErr.Kind, result.ExitCode, response.ExitClass, errors.Join(callErr, ErrModelRoutingClientProtocol))
	}
	return response.Result, modelRoutingClientError(ModelRoutingClientErrorSemantic, c.candidate, transportErr.Kind, result.ExitCode, response.ExitClass, callErr)
}

func modelRoutingClientError(kind ModelRoutingClientErrorKind, candidate ModelRoutingCandidate, transportKind TransportErrorKind, exitCode int, exitClass string, cause error) error {
	return &ModelRoutingClientError{
		Candidate:     candidate,
		Kind:          kind,
		TransportKind: transportKind,
		ExitCode:      exitCode,
		ExitClass:     exitClass,
		Cause:         cause,
	}
}

func marshalModelRoutingInspectRequest(request ModelRoutingRequestContext) ([]byte, error) {
	payload, err := json.Marshal(struct {
		Version        int                `json:"version"`
		Contract       string             `json:"contract"`
		Operation      string             `json:"operation"`
		CWD            string             `json:"cwd"`
		AgentDir       string             `json:"agentDir"`
		Target         ModelRoutingTarget `json:"target"`
		ConfigHome     string             `json:"configHome,omitempty"`
		LoadExtensions bool               `json:"loadExtensions,omitempty"`
	}{
		Version:        modelRoutingVersion,
		Contract:       modelRoutingContract,
		Operation:      "inspect",
		CWD:            request.CWD,
		AgentDir:       request.AgentDir,
		Target:         request.Target,
		ConfigHome:     request.ConfigHome,
		LoadExtensions: request.LoadExtensions,
	})
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func marshalModelRoutingValidateRequest(request ModelRoutingRequestContext, draft ModelRoutingDraft) ([]byte, error) {
	return marshalModelRoutingDraftRequest(request, draft, ModelRoutingOperationValidate)
}

func marshalModelRoutingApplyRequest(request ModelRoutingRequestContext, draft ModelRoutingDraft) ([]byte, error) {
	return marshalModelRoutingDraftRequest(request, draft, ModelRoutingOperationApply)
}

func marshalModelRoutingDraftRequest(request ModelRoutingRequestContext, draft ModelRoutingDraft, operation ModelRoutingOperation) ([]byte, error) {
	payload, err := json.Marshal(struct {
		Version        int                   `json:"version"`
		Contract       string                `json:"contract"`
		Operation      ModelRoutingOperation `json:"operation"`
		CWD            string                `json:"cwd"`
		AgentDir       string                `json:"agentDir"`
		Target         ModelRoutingTarget    `json:"target"`
		ConfigHome     string                `json:"configHome,omitempty"`
		LoadExtensions bool                  `json:"loadExtensions,omitempty"`
		Draft          ModelRoutingDraft     `json:"draft"`
	}{
		Version:        modelRoutingVersion,
		Contract:       modelRoutingContract,
		Operation:      operation,
		CWD:            request.CWD,
		AgentDir:       request.AgentDir,
		Target:         request.Target,
		ConfigHome:     request.ConfigHome,
		LoadExtensions: request.LoadExtensions,
		Draft:          cloneModelRoutingDraft(draft),
	})
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}
