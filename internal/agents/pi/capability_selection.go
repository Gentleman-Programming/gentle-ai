package pi

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

const (
	modelRoutingCapabilitiesRequest = `{"version":1,"contract":"gentle-pi.model-routing/v1","operation":"capabilities"}` + "\n"
	modelRoutingCapabilitiesTimeout = 5 * time.Second
	modelRoutingCapabilitiesStderr  = 4 << 10
)

type ProbeErrorKind string

const (
	ProbeErrorTransportInvalid                  ProbeErrorKind = "transport-invalid"
	ProbeErrorTransportStart                    ProbeErrorKind = "transport-start"
	ProbeErrorTransportWait                     ProbeErrorKind = "transport-wait"
	ProbeErrorTransportCanceled                 ProbeErrorKind = "transport-canceled"
	ProbeErrorTransportTimeout                  ProbeErrorKind = "transport-timeout"
	ProbeErrorTransportStdoutOverflow           ProbeErrorKind = "transport-stdout-overflow"
	ProbeErrorTransportStderrOverflow           ProbeErrorKind = "transport-stderr-overflow"
	ProbeErrorTransportNonzeroExit              ProbeErrorKind = "transport-nonzero-exit"
	ProbeErrorTransportTermination              ProbeErrorKind = "transport-termination"
	ProbeErrorTransportUnsupportedPlatform      ProbeErrorKind = "transport-unsupported-platform"
	ProbeErrorTransportUnknown                  ProbeErrorKind = "transport-unknown"
	ProbeErrorCapabilitiesMalformed             ProbeErrorKind = "capabilities-malformed"
	ProbeErrorCapabilitiesOversized             ProbeErrorKind = "capabilities-oversized"
	ProbeErrorCapabilitiesUnsupportedVersion    ProbeErrorKind = "capabilities-unsupported-version"
	ProbeErrorCapabilitiesUnsupportedContract   ProbeErrorKind = "capabilities-unsupported-contract"
	ProbeErrorCapabilitiesUnsupportedOperation  ProbeErrorKind = "capabilities-unsupported-operation"
	ProbeErrorCapabilitiesExplicitRemoteFailure ProbeErrorKind = "capabilities-explicit-remote-failure"
	ProbeErrorCapabilitiesInvalidOperations     ProbeErrorKind = "capabilities-invalid-operations"
	ProbeErrorCapabilitiesUnknown               ProbeErrorKind = "capabilities-unknown"
)

// ProbeError omits Cause from Error so transport stderr never reaches callers.
type ProbeError struct {
	Candidate        ModelRoutingCandidate
	Kind             ProbeErrorKind
	TransportKind    TransportErrorKind
	CapabilitiesKind CapabilitiesErrorKind
	Cause            error
}

func (e *ProbeError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Pi model-routing probe error (%s) for %q", e.Kind, e.Candidate.Path)
}
func (e *ProbeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
func (e *ProbeError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	typed, ok := target.(*ProbeError)
	return ok && typed != nil && e.Kind != "" && e.Kind == typed.Kind
}

type modelRoutingTransport func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error)

func ProbeModelRoutingCapabilities(ctx context.Context, candidate ModelRoutingCandidate) (ModelRoutingCandidate, Capabilities, error) {
	return probeModelRoutingCapabilities(ctx, candidate, RunBoundedModelRoutingProcess)
}
func probeModelRoutingCapabilities(ctx context.Context, candidate ModelRoutingCandidate, transport modelRoutingTransport) (ModelRoutingCandidate, Capabilities, error) {
	candidate = cleanModelRoutingCandidate(candidate)
	if transport == nil {
		return candidate, Capabilities{}, newProbeError(candidate, ProbeErrorTransportUnknown, "", "", errors.New("nil model-routing transport"))
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return candidate, Capabilities{}, newProbeError(candidate, contextProbeKind(err), contextTransportKind(err), "", err)
		}
	}
	options := ModelRoutingProcessOptions{
		Timeout:         modelRoutingCapabilitiesTimeout,
		MaxRequestBytes: len(modelRoutingCapabilitiesRequest),
		MaxStdoutBytes:  MaxCapabilitiesResponseBytes,
		MaxStderrBytes:  modelRoutingCapabilitiesStderr,
	}
	result, err := transport(ctx, candidate.Path, []byte(modelRoutingCapabilitiesRequest), options)
	if err != nil {
		return candidate, Capabilities{}, probeTransportError(candidate, err)
	}
	capabilities, err := ParseCapabilitiesResponse(cloneTransportBytes(result.Stdout))
	if err != nil {
		return candidate, Capabilities{}, probeCapabilitiesError(candidate, err)
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return candidate, Capabilities{}, newProbeError(candidate, contextProbeKind(err), contextTransportKind(err), "", err)
		}
	}
	return candidate, cloneCapabilities(capabilities), nil
}
func SelectCompatibleModelRoutingCandidate(ctx context.Context, candidates []ModelRoutingCandidate) (ModelRoutingCandidate, Capabilities, error) {
	return selectCompatibleModelRoutingCandidate(ctx, candidates, RunBoundedModelRoutingProcess)
}
func selectCompatibleModelRoutingCandidate(ctx context.Context, candidates []ModelRoutingCandidate, transport modelRoutingTransport) (ModelRoutingCandidate, Capabilities, error) {
	if len(candidates) == 0 {
		return ModelRoutingCandidate{}, Capabilities{}, &SelectionError{Kind: SelectionErrorNoCandidates}
	}
	if ctx == nil {
		return ModelRoutingCandidate{}, Capabilities{}, &SelectionError{Kind: SelectionErrorInvalidContext, Cause: ErrSelectionInvalidContext}
	}
	if err := ctx.Err(); err != nil {
		return ModelRoutingCandidate{}, Capabilities{}, &SelectionError{Kind: selectionContextKind(err), Cause: err}
	}
	failures := make([]error, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return ModelRoutingCandidate{}, Capabilities{}, &SelectionError{Kind: selectionContextKind(err), Cause: err}
		}
		cleaned, capabilities, err := probeModelRoutingCapabilities(ctx, candidate, transport)
		if err == nil {
			return cleaned, cloneCapabilities(capabilities), nil
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return ModelRoutingCandidate{}, Capabilities{}, &SelectionError{Kind: selectionContextKind(contextErr), Cause: contextErr}
		}
		failures = append(failures, err)
	}
	return ModelRoutingCandidate{}, Capabilities{}, &SelectionError{Kind: SelectionErrorAllCandidatesFailed, Failures: cloneErrors(failures)}
}
func cleanModelRoutingCandidate(candidate ModelRoutingCandidate) ModelRoutingCandidate {
	candidate.Path = filepath.Clean(candidate.Path)
	return candidate
}
func cloneCapabilities(capabilities Capabilities) Capabilities {
	capabilities.Operations = append([]string(nil), capabilities.Operations...)
	return capabilities
}
func cloneErrors(values []error) []error {
	if values == nil {
		return nil
	}
	return append([]error(nil), values...)
}
func newProbeError(candidate ModelRoutingCandidate, kind ProbeErrorKind, transportKind TransportErrorKind, capabilitiesKind CapabilitiesErrorKind, cause error) error {
	return &ProbeError{Candidate: candidate, Kind: kind, TransportKind: transportKind, CapabilitiesKind: capabilitiesKind, Cause: cause}
}

var transportProbeKinds = map[TransportErrorKind]ProbeErrorKind{
	TransportErrorInvalidOptions: ProbeErrorTransportInvalid, TransportErrorInvalidPath: ProbeErrorTransportInvalid, TransportErrorInvalidRequest: ProbeErrorTransportInvalid,
	TransportErrorStart: ProbeErrorTransportStart, TransportErrorWait: ProbeErrorTransportWait, TransportErrorCanceled: ProbeErrorTransportCanceled, TransportErrorTimeout: ProbeErrorTransportTimeout,
	TransportErrorStdoutOverflow: ProbeErrorTransportStdoutOverflow, TransportErrorStderrOverflow: ProbeErrorTransportStderrOverflow, TransportErrorNonzeroExit: ProbeErrorTransportNonzeroExit,
	TransportErrorTermination: ProbeErrorTransportTermination, TransportErrorUnsupportedPlatform: ProbeErrorTransportUnsupportedPlatform,
}

func probeTransportError(candidate ModelRoutingCandidate, cause error) error {
	var transportErr *TransportError
	if !errors.As(cause, &transportErr) || transportErr == nil {
		return newProbeError(candidate, ProbeErrorTransportUnknown, "", "", cause)
	}
	kind := transportProbeKinds[transportErr.Kind]
	if kind == "" {
		kind = ProbeErrorTransportUnknown
	}
	return newProbeError(candidate, kind, transportErr.Kind, "", cause)
}

var capabilitiesProbeKinds = map[CapabilitiesErrorKind]ProbeErrorKind{
	CapabilitiesErrorMalformed: ProbeErrorCapabilitiesMalformed, CapabilitiesErrorOversized: ProbeErrorCapabilitiesOversized,
	CapabilitiesErrorUnsupportedVersion: ProbeErrorCapabilitiesUnsupportedVersion, CapabilitiesErrorUnsupportedContract: ProbeErrorCapabilitiesUnsupportedContract,
	CapabilitiesErrorUnsupportedOperation: ProbeErrorCapabilitiesUnsupportedOperation, CapabilitiesErrorExplicitRemoteFailure: ProbeErrorCapabilitiesExplicitRemoteFailure,
	CapabilitiesErrorInvalidOperations: ProbeErrorCapabilitiesInvalidOperations,
}

func probeCapabilitiesError(candidate ModelRoutingCandidate, cause error) error {
	var capabilitiesErr *CapabilitiesError
	if !errors.As(cause, &capabilitiesErr) || capabilitiesErr == nil {
		return newProbeError(candidate, ProbeErrorCapabilitiesUnknown, "", "", cause)
	}
	kind := capabilitiesProbeKinds[capabilitiesErr.Kind]
	if kind == "" {
		kind = ProbeErrorCapabilitiesUnknown
	}
	return newProbeError(candidate, kind, "", capabilitiesErr.Kind, cause)
}
func contextProbeKind(err error) ProbeErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return ProbeErrorTransportTimeout
	}
	return ProbeErrorTransportCanceled
}
func contextTransportKind(err error) TransportErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return TransportErrorTimeout
	}
	return TransportErrorCanceled
}

type SelectionErrorKind string

const (
	SelectionErrorNoCandidates        SelectionErrorKind = "no-candidates"
	SelectionErrorInvalidContext      SelectionErrorKind = "invalid-context"
	SelectionErrorCanceled            SelectionErrorKind = "canceled"
	SelectionErrorTimeout             SelectionErrorKind = "timeout"
	SelectionErrorAllCandidatesFailed SelectionErrorKind = "all-candidates-failed"
)

var (
	ErrNoModelRoutingCandidates = errors.New("no model-routing candidates were provided")
	ErrSelectionInvalidContext  = errors.New("nil selection context")
)

type SelectionError struct {
	Kind     SelectionErrorKind
	Failures []error
	Cause    error
}

func (e *SelectionError) Error() string {
	if e == nil {
		return "<nil>"
	}
	switch e.Kind {
	case SelectionErrorNoCandidates:
		return "Pi model-routing candidate selection failed: no candidates"
	case SelectionErrorInvalidContext:
		return "Pi model-routing candidate selection failed: invalid context"
	case SelectionErrorCanceled:
		return "Pi model-routing candidate selection canceled"
	case SelectionErrorTimeout:
		return "Pi model-routing candidate selection timed out"
	default:
		return fmt.Sprintf("Pi model-routing candidate selection failed: %d candidates rejected", len(e.Failures))
	}
}
func (e *SelectionError) Unwrap() []error {
	if e == nil {
		return nil
	}
	if len(e.Failures) > 0 {
		return cloneErrors(e.Failures)
	}
	if e.Cause != nil {
		return []error{e.Cause}
	}
	if e.Kind == SelectionErrorNoCandidates {
		return []error{ErrNoModelRoutingCandidates}
	}
	return nil
}
func (e *SelectionError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if typed, ok := target.(*SelectionError); ok {
		return typed != nil && e.Kind != "" && e.Kind == typed.Kind
	}
	return target == ErrNoModelRoutingCandidates && e.Kind == SelectionErrorNoCandidates
}
func selectionContextKind(err error) SelectionErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return SelectionErrorTimeout
	}
	return SelectionErrorCanceled
}
