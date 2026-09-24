package pi

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func clientCapabilities() Capabilities {
	return Capabilities{Contract: modelRoutingContract, Supported: true, Operations: []string{"capabilities", "inspect", "validate", "apply"}}
}
func clientEnvelope(t *testing.T, ok bool, class string) []byte {
	t.Helper()
	return inspectEnvelopeFixture(richInspectResultWithWarning(t), class, ok)
}
func TestModelRoutingClientInspectRequestAndCopies(t *testing.T) {
	for _, tt := range []struct {
		name, config string
		load         bool
		optional     string
	}{
		{"required", "", false, ""},
		{"config", "/config", false, `,"configHome":"/config"`},
		{"extensions", "", true, `,"loadExtensions":true`},
		{"all", "/config", true, `,"configHome":"/config","loadExtensions":true`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls int
			var gotPath string
			var gotRequest []byte
			var gotOptions ModelRoutingProcessOptions
			transport := func(_ context.Context, path string, request []byte, options ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				calls++
				gotPath, gotRequest, gotOptions = path, append([]byte(nil), request...), options
				return ModelRoutingProcessResult{Stdout: clientEnvelope(t, true, "success")}, nil
			}
			candidate := ModelRoutingCandidate{Path: "/tmp/model-routing/.", Source: "PATH"}
			capabilities := clientCapabilities()
			client := newModelRoutingClient(candidate, capabilities, transport)
			candidate.Path, capabilities.Operations[0] = "/changed", "changed"
			got, err := client.Inspect(context.Background(), ModelRoutingRequestContext{CWD: "/workspace", AgentDir: "/agents", Target: ModelRoutingTargetProject, ConfigHome: tt.config, LoadExtensions: tt.load})
			if err != nil || got.Contract != modelRoutingContract || calls != 1 {
				t.Fatalf("inspect = %#v, %v; calls=%d", got, err, calls)
			}
			want := `{"version":1,"contract":"` + modelRoutingContract + `","operation":"inspect","cwd":"/workspace","agentDir":"/agents","target":"project"` + tt.optional + "}\n"
			if string(gotRequest) != want || gotPath != "/tmp/model-routing" {
				t.Fatalf("path/request = %q, %q; want %q, %q", gotPath, gotRequest, "/tmp/model-routing", want)
			}
			wantOptions := ModelRoutingProcessOptions{Timeout: modelRoutingInspectTimeout, MaxRequestBytes: 64 << 10, MaxStdoutBytes: MaxModelRoutingResponseBytes, MaxStderrBytes: 4 << 10}
			if gotOptions != wantOptions || !reflect.DeepEqual(client.Candidate(), ModelRoutingCandidate{Path: "/tmp/model-routing", Source: "PATH"}) || !reflect.DeepEqual(client.Capabilities(), clientCapabilities()) {
				t.Fatalf("client state/options = %#v, %#v, %#v", client.Candidate(), client.Capabilities(), gotOptions)
			}
			copyCaps := client.Capabilities()
			copyCaps.Operations[0] = "mutated"
			if client.Capabilities().Operations[0] != "capabilities" {
				t.Fatal("capabilities accessor aliases client state")
			}
		})
	}
}

func TestModelRoutingClientSemanticAndProtocolMatrix(t *testing.T) {
	for _, tt := range []struct {
		name  string
		code  int
		class string
	}{
		{"invalid-input", 2, "invalid-input"}, {"unsupported-contract", 3, "unsupported-contract"}, {"unavailable-runtime", 4, "unavailable-runtime"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("stderr-provider-secret")
			transport := func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				return ModelRoutingProcessResult{ExitCode: tt.code, Stdout: clientEnvelope(t, false, tt.class)}, transportError(TransportErrorNonzeroExit, cause)
			}
			got, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), transport).Inspect(context.Background(), ModelRoutingRequestContext{Target: ModelRoutingTargetProject})
			var clientErr *ModelRoutingClientError
			var transportErr *TransportError
			if err == nil || got.Contract != modelRoutingContract || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorSemantic || clientErr.ExitCode != tt.code || clientErr.ExitClass != tt.class || !errors.As(err, &transportErr) || transportErr.Kind != TransportErrorNonzeroExit || !errors.Is(err, cause) || strings.Contains(err.Error(), "stderr-provider-secret") {
				t.Fatalf("semantic result/error = %#v, %T %v", got, err, err)
			}
		})
	}
	valid := clientEnvelope(t, true, "success")
	for _, tt := range []struct {
		name string
		code int
		body []byte
		err  error
	}{
		{"exit5", 5, valid, transportError(TransportErrorNonzeroExit, errors.New("exit5-secret"))},
		{"exit6", 6, valid, transportError(TransportErrorNonzeroExit, errors.New("exit6-secret"))},
		{"typed-code0", 0, valid, transportError(TransportErrorNonzeroExit, errors.New("code0-secret"))},
		{"nonzero-without-error", 2, valid, nil},
		{"ok-mismatch", 0, clientEnvelope(t, false, "success"), nil},
		{"class-mismatch", 0, clientEnvelope(t, true, "invalid-input"), nil},
		{"malformed", 0, []byte("{"), nil},
		{"wrong-identity", 0, bytes.Replace(valid, []byte(`"operation":"inspect"`), []byte(`"operation":"validate"`), 1), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				return ModelRoutingProcessResult{ExitCode: tt.code, Stdout: tt.body}, tt.err
			}).Inspect(context.Background(), ModelRoutingRequestContext{Target: ModelRoutingTargetProject})
			var clientErr *ModelRoutingClientError
			if err == nil || !reflect.DeepEqual(got, ModelRoutingInspection{}) || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorProtocol || !errors.Is(err, ErrModelRoutingClientProtocol) {
				t.Fatalf("protocol result/error = %#v, %T %v", got, err, err)
			}
		})
	}
}
func TestModelRoutingClientTransportAndBoundaries(t *testing.T) {
	for _, kind := range []TransportErrorKind{TransportErrorInvalidOptions, TransportErrorInvalidPath, TransportErrorInvalidRequest, TransportErrorStart, TransportErrorWait, TransportErrorCanceled, TransportErrorTimeout, TransportErrorStdoutOverflow, TransportErrorStderrOverflow, TransportErrorTermination, TransportErrorUnsupportedPlatform} {
		t.Run(string(kind), func(t *testing.T) {
			cause := errors.New("transport-secret")
			calls := 0
			_, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				calls++
				return ModelRoutingProcessResult{}, transportError(kind, cause)
			}).Inspect(context.Background(), ModelRoutingRequestContext{})
			var clientErr *ModelRoutingClientError
			var transportErr *TransportError
			if calls != 1 || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorTransport || !errors.As(err, &transportErr) || transportErr.Kind != kind || !errors.Is(err, cause) || strings.Contains(err.Error(), "transport-secret") {
				t.Fatalf("transport error = %T %v; calls=%d", err, err, calls)
			}
		})
	}
	var nilClient *ModelRoutingClient
	if _, err := nilClient.Inspect(context.Background(), ModelRoutingRequestContext{}); !errors.Is(err, ErrModelRoutingClientInvalidClient) {
		t.Fatalf("nil client error = %v", err)
	}
	if _, err := newModelRoutingClient(ModelRoutingCandidate{}, clientCapabilities(), nil).Inspect(context.Background(), ModelRoutingRequestContext{}); !errors.Is(err, ErrModelRoutingClientTransport) {
		t.Fatalf("nil transport error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := newModelRoutingClient(ModelRoutingCandidate{}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		calls++
		return ModelRoutingProcessResult{}, nil
	}).Inspect(ctx, ModelRoutingRequestContext{})
	if calls != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context = %v; calls=%d", err, calls)
	}
}
