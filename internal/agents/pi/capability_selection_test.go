package pi

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestProbeModelRoutingCapabilitiesUsesFixedRequestAndCleansCandidate(t *testing.T) {
	candidate := ModelRoutingCandidate{Path: "/tmp/model-routing/.", Source: "PATH"}
	var gotPath string
	var gotRequest []byte
	var gotOptions ModelRoutingProcessOptions
	transport := func(_ context.Context, path string, request []byte, options ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		gotPath = path
		gotRequest = append([]byte(nil), request...)
		gotOptions = options
		return ModelRoutingProcessResult{Stdout: []byte(validCapabilitiesResponse)}, nil
	}

	cleaned, capabilities, err := probeModelRoutingCapabilities(context.Background(), candidate, transport)
	if err != nil {
		t.Fatalf("probe error = %v", err)
	}
	if gotPath != "/tmp/model-routing" {
		t.Fatalf("transport path = %q; want cleaned candidate path", gotPath)
	}
	if string(gotRequest) != `{"version":1,"contract":"gentle-pi.model-routing/v1","operation":"capabilities"}`+"\n" {
		t.Fatalf("transport request = %q; want fixed capabilities request", gotRequest)
	}
	wantOptions := ModelRoutingProcessOptions{Timeout: modelRoutingCapabilitiesTimeout, MaxRequestBytes: len(modelRoutingCapabilitiesRequest), MaxStdoutBytes: MaxCapabilitiesResponseBytes, MaxStderrBytes: modelRoutingCapabilitiesStderr}
	if gotOptions != wantOptions {
		t.Fatalf("transport options = %+v; want %+v", gotOptions, wantOptions)
	}
	want := Capabilities{Contract: modelRoutingContract, Supported: true, Operations: []string{"capabilities", "inspect", "validate", "apply"}}
	if !reflect.DeepEqual(capabilities, want) || cleaned != (ModelRoutingCandidate{Path: "/tmp/model-routing", Source: "PATH"}) {
		t.Fatalf("result = %#v, %#v; want cleaned candidate and capabilities %#v", cleaned, capabilities, want)
	}
}
func TestProbeMapsTransportAndCapabilitiesErrorsWithoutDiagnostics(t *testing.T) {
	candidate := ModelRoutingCandidate{Path: "/tmp/model-routing", Source: "PATH"}
	transportKinds := []TransportErrorKind{TransportErrorInvalidOptions, TransportErrorInvalidPath, TransportErrorInvalidRequest, TransportErrorStart, TransportErrorWait, TransportErrorCanceled, TransportErrorTimeout, TransportErrorStdoutOverflow, TransportErrorStderrOverflow, TransportErrorNonzeroExit, TransportErrorTermination, TransportErrorUnsupportedPlatform}
	for _, transportKind := range transportKinds {
		t.Run(string(transportKind), func(t *testing.T) {
			cause := errors.New("stderr-secret")
			err := probeTransportError(candidate, transportError(transportKind, cause))
			var typed *ProbeError
			if !errors.As(err, &typed) || typed.TransportKind != transportKind || !errors.Is(err, cause) || strings.Contains(err.Error(), "stderr-secret") {
				t.Fatalf("probe error = %#v; want mapped kind, cause, and no diagnostic", err)
			}
		})
	}
	capabilityKinds := []CapabilitiesErrorKind{CapabilitiesErrorMalformed, CapabilitiesErrorOversized, CapabilitiesErrorUnsupportedVersion, CapabilitiesErrorUnsupportedContract, CapabilitiesErrorUnsupportedOperation, CapabilitiesErrorExplicitRemoteFailure, CapabilitiesErrorInvalidOperations}
	for _, capabilitiesKind := range capabilityKinds {
		t.Run(string(capabilitiesKind), func(t *testing.T) {
			cause := errors.New("parser-secret")
			err := probeCapabilitiesError(candidate, capabilitiesError(capabilitiesKind, cause))
			var typed *ProbeError
			if !errors.As(err, &typed) || typed.CapabilitiesKind != capabilitiesKind || !errors.Is(err, cause) || typed.Kind == ProbeErrorCapabilitiesUnknown {
				t.Fatalf("probe error = %#v; want exact parser mapping", err)
			}
		})
	}
}
func TestSelectCompatibleModelRoutingCandidateIsOrderedAndAggregates(t *testing.T) {
	var calls []string
	firstCause, secondCause := errors.New("first"), errors.New("second")
	transport := func(_ context.Context, path string, _ []byte, _ ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		calls = append(calls, path)
		switch path {
		case "/first":
			return ModelRoutingProcessResult{}, transportError(TransportErrorStart, firstCause)
		case "/second":
			return ModelRoutingProcessResult{Stdout: []byte(validCapabilitiesResponse)}, nil
		default:
			return ModelRoutingProcessResult{}, transportError(TransportErrorWait, secondCause)
		}
	}
	candidate, capabilities, err := selectCompatibleModelRoutingCandidate(context.Background(), []ModelRoutingCandidate{{Path: "/first", Source: "PATH"}, {Path: "/second/.", Source: "package"}, {Path: "/third", Source: "PATH"}}, transport)
	if err != nil || candidate.Path != "/second" || !capabilities.Supported || !reflect.DeepEqual(calls, []string{"/first", "/second"}) {
		t.Fatalf("selection = %#v, %#v, %v; calls=%q", candidate, capabilities, err, calls)
	}

	calls = nil
	_, _, err = selectCompatibleModelRoutingCandidate(context.Background(), []ModelRoutingCandidate{{Path: "/first"}, {Path: "/third"}}, transport)
	var selectionErr *SelectionError
	if !errors.As(err, &selectionErr) || selectionErr.Kind != SelectionErrorAllCandidatesFailed || len(selectionErr.Unwrap()) != 2 || !errors.Is(err, firstCause) || !errors.Is(err, secondCause) || !reflect.DeepEqual(calls, []string{"/first", "/third"}) {
		t.Fatalf("aggregate = %#v; calls=%q", err, calls)
	}
}
func TestSelectCompatibleModelRoutingCandidateStopsOnCancelAndEmptyIsTyped(t *testing.T) {
	if _, _, err := SelectCompatibleModelRoutingCandidate(context.Background(), nil); err == nil || !errors.Is(err, ErrNoModelRoutingCandidates) {
		t.Fatalf("empty selection error = %v; want typed empty error", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	transport := func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		calls++
		return ModelRoutingProcessResult{}, nil
	}
	_, _, err := selectCompatibleModelRoutingCandidate(ctx, []ModelRoutingCandidate{{Path: "/first"}}, transport)
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("pre-canceled selection = %v, calls=%d; want immediate cancellation", err, calls)
	}
}
