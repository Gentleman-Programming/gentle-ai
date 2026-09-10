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
			wantOptions := ModelRoutingProcessOptions{Timeout: modelRoutingTimeout, MaxRequestBytes: 64 << 10, MaxStdoutBytes: MaxModelRoutingResponseBytes, MaxStderrBytes: 4 << 10}
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

func TestModelRoutingValidateRequestDraftWireShape(t *testing.T) {
	model := "provider/model"
	thinking := ModelRoutingThinkingHigh
	for _, tt := range []struct {
		name  string
		draft ModelRoutingDraft
		want  string
	}{
		{name: "nil", want: `{"version":1,"contract":"` + modelRoutingContract + `","operation":"validate","cwd":"","agentDir":"","target":"","draft":{}}` + "\n"},
		{name: "empty", draft: ModelRoutingDraft{}, want: `{"version":1,"contract":"` + modelRoutingContract + `","operation":"validate","cwd":"","agentDir":"","target":"","draft":{}}` + "\n"},
		{name: "populated", draft: ModelRoutingDraft{"z": {Thinking: &thinking}, "a": {Model: &model}, "inherit": {}}, want: `{"version":1,"contract":"` + modelRoutingContract + `","operation":"validate","cwd":"","agentDir":"","target":"","draft":{"a":{"model":"provider/model"},"inherit":{},"z":{"thinking":"high"}}}` + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshalModelRoutingValidateRequest(ModelRoutingRequestContext{}, tt.draft)
			if err != nil || string(got) != tt.want {
				t.Fatalf("request = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestModelRoutingClientValidateSuccessRequestAndCopies(t *testing.T) {
	model := "provider/model"
	thinking := ModelRoutingThinkingHigh
	draft := ModelRoutingDraft{"z": {Thinking: &thinking}, "a": {Model: &model}, "inherit": {}}
	var calls int
	var gotPath string
	var gotRequest []byte
	var gotOptions ModelRoutingProcessOptions
	transport := func(_ context.Context, path string, request []byte, options ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		calls++
		gotPath, gotRequest, gotOptions = path, append([]byte(nil), request...), options
		model, thinking = "changed", ModelRoutingThinkingLow
		draft["new"] = ModelRoutingDraftAssignment{}
		return ModelRoutingProcessResult{Stdout: modelRoutingValidateEnvelope(modelRoutingValidateResult(true, `[{"code":"warn","message":"check","severity":"warning"},{"code":"info","message":"context","severity":"info"}]`), true, "success")}, nil
	}
	client := newModelRoutingClient(ModelRoutingCandidate{Path: "/tmp/model-routing/."}, clientCapabilities(), transport)
	got, err := client.Validate(context.Background(), ModelRoutingRequestContext{CWD: "/workspace", AgentDir: "/agents", Target: ModelRoutingTargetProject, ConfigHome: "/config", LoadExtensions: true}, draft)
	if err != nil || !got.OK || len(got.Diagnostics) != 2 || got.Diagnostics[0].Severity != ModelRoutingDiagnosticSeverityWarning || calls != 1 {
		t.Fatalf("validate = %#v, %v; calls=%d", got, err, calls)
	}
	want := `{"version":1,"contract":"` + modelRoutingContract + `","operation":"validate","cwd":"/workspace","agentDir":"/agents","target":"project","configHome":"/config","loadExtensions":true,"draft":{"a":{"model":"provider/model"},"inherit":{},"z":{"thinking":"high"}}}` + "\n"
	if gotPath != "/tmp/model-routing" || string(gotRequest) != want {
		t.Fatalf("path/request = %q, %q; want %q, %q", gotPath, gotRequest, "/tmp/model-routing", want)
	}
	wantOptions := ModelRoutingProcessOptions{Timeout: modelRoutingTimeout, MaxRequestBytes: 64 << 10, MaxStdoutBytes: MaxModelRoutingResponseBytes, MaxStderrBytes: 4 << 10}
	if gotOptions != wantOptions {
		t.Fatalf("options = %#v; want %#v", gotOptions, wantOptions)
	}
}

func TestModelRoutingClientValidateParserCause(t *testing.T) {
	_, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		return ModelRoutingProcessResult{Stdout: []byte("{")}, nil
	}).Validate(context.Background(), ModelRoutingRequestContext{}, nil)
	var responseErr *ModelRoutingResponseError
	if !errors.As(err, &responseErr) || responseErr.Kind != ModelRoutingResponseErrorMalformed || responseErr.ExpectedOperation != ModelRoutingOperationValidate || !errors.Is(err, ErrModelRoutingResponseMalformed) {
		t.Fatalf("parser error = %T %v", err, err)
	}
}

func TestModelRoutingClientValidateTransportAndBoundaries(t *testing.T) {
	for _, kind := range []TransportErrorKind{TransportErrorInvalidOptions, TransportErrorInvalidPath, TransportErrorInvalidRequest, TransportErrorStart, TransportErrorWait, TransportErrorCanceled, TransportErrorTimeout, TransportErrorStdoutOverflow, TransportErrorStderrOverflow, TransportErrorTermination, TransportErrorUnsupportedPlatform} {
		t.Run(string(kind), func(t *testing.T) {
			cause := errors.New("validate-transport-secret")
			calls := 0
			_, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				calls++
				return ModelRoutingProcessResult{}, transportError(kind, cause)
			}).Validate(context.Background(), ModelRoutingRequestContext{}, nil)
			var clientErr *ModelRoutingClientError
			var transportErr *TransportError
			if calls != 1 || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorTransport || !errors.As(err, &transportErr) || transportErr.Kind != kind || !errors.Is(err, cause) || strings.Contains(err.Error(), "validate-transport-secret") || strings.Contains(err.Error(), "inspect") || strings.Contains(err.Error(), "validate") {
				t.Fatalf("transport error = %T %v; calls=%d", err, err, calls)
			}
		})
	}
	var nilClient *ModelRoutingClient
	if _, err := nilClient.Validate(context.Background(), ModelRoutingRequestContext{}, nil); !errors.Is(err, ErrModelRoutingClientInvalidClient) {
		t.Fatalf("nil client error = %v", err)
	}
	if _, err := newModelRoutingClient(ModelRoutingCandidate{}, clientCapabilities(), nil).Validate(context.Background(), ModelRoutingRequestContext{}, nil); !errors.Is(err, ErrModelRoutingClientTransport) {
		t.Fatalf("nil transport error = %v", err)
	}
	for _, tt := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "nil", want: ErrTransportInvalidOptions},
		{name: "canceled", ctx: canceledContext(), want: context.Canceled},
		{name: "timeout", ctx: timeoutContext(), want: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			client := newModelRoutingClient(ModelRoutingCandidate{}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				calls++
				return ModelRoutingProcessResult{}, nil
			})
			_, err := client.Validate(tt.ctx, ModelRoutingRequestContext{}, nil)
			if calls != 0 || !errors.Is(err, tt.want) {
				t.Fatalf("context error = %v; calls=%d", err, calls)
			}
		})
	}
}

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func timeoutContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	cancel()
	return ctx
}

func TestModelRoutingClientValidateSemanticAndProtocolMatrix(t *testing.T) {
	response := func(envelopeOK, resultOK bool, class string) []byte {
		return modelRoutingValidateEnvelope(modelRoutingValidateResult(resultOK, `[{"code":"C","message":"M","severity":"warning"}]`), envelopeOK, class)
	}
	for _, tt := range []struct {
		name  string
		code  int
		class string
	}{
		{"invalid-input", 2, "invalid-input"}, {"unsupported-contract", 3, "unsupported-contract"}, {"unavailable-runtime", 4, "unavailable-runtime"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("validate-provider-secret")
			got, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				return ModelRoutingProcessResult{ExitCode: tt.code, Stdout: response(false, false, tt.class)}, transportError(TransportErrorNonzeroExit, cause)
			}).Validate(context.Background(), ModelRoutingRequestContext{}, nil)
			var clientErr *ModelRoutingClientError
			var transportErr *TransportError
			if err == nil || got.OK || len(got.Diagnostics) != 1 || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorSemantic || clientErr.ExitCode != tt.code || clientErr.ExitClass != tt.class || !errors.As(err, &transportErr) || !errors.Is(err, cause) || strings.Contains(err.Error(), "validate-provider-secret") {
				t.Fatalf("semantic result/error = %#v, %T %v", got, err, err)
			}
		})
	}
	valid := response(true, true, "success")
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
		{"envelope-mismatch", 0, response(false, true, "success"), nil},
		{"class-mismatch", 0, response(true, true, "invalid-input"), nil},
		{"result-mismatch", 0, response(true, false, "success"), nil},
		{"malformed", 0, []byte("{"), nil},
		{"wrong-identity", 0, bytes.Replace(valid, []byte(`"operation":"validate"`), []byte(`"operation":"inspect"`), 1), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				return ModelRoutingProcessResult{ExitCode: tt.code, Stdout: tt.body}, tt.err
			}).Validate(context.Background(), ModelRoutingRequestContext{}, nil)
			var clientErr *ModelRoutingClientError
			if err == nil || !reflect.DeepEqual(got, ModelRoutingValidationResult{}) || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorProtocol || !errors.Is(err, ErrModelRoutingClientProtocol) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("protocol result/error = %#v, %T %v", got, err, err)
			}
		})
	}
}

func TestModelRoutingClientApplySuccessRequest(t *testing.T) {
	model := "provider/model"
	thinking := ModelRoutingThinkingHigh
	draft := ModelRoutingDraft{"z": {Thinking: &thinking}, "a": {Model: &model}, "inherit": {}}
	var calls int
	var gotPath string
	var gotRequest []byte
	var gotOptions ModelRoutingProcessOptions
	transport := func(_ context.Context, path string, request []byte, options ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		calls++
		gotPath, gotRequest, gotOptions = path, append([]byte(nil), request...), options
		model, thinking = "changed", ModelRoutingThinkingLow
		draft["new"] = ModelRoutingDraftAssignment{}
		return ModelRoutingProcessResult{Stdout: applyEnvelope(applyResult(true, "success", true, `,"target":"global","configPath":"/cfg/model-routing.json","materialization":{"affected":["global"],"succeeded":["global"],"failed":[]}`), true, "success")}, nil
	}
	client := newModelRoutingClient(ModelRoutingCandidate{Path: "/tmp/model-routing/."}, clientCapabilities(), transport)
	got, err := client.Apply(context.Background(), ModelRoutingRequestContext{CWD: "/workspace", AgentDir: "/agents", Target: ModelRoutingTargetProject, ConfigHome: "/config", LoadExtensions: true}, draft)
	if err != nil || !got.OK || !got.Saved || got.Outcome != ModelRoutingApplyOutcomeSuccess || calls != 1 {
		t.Fatalf("apply = %#v, %v; calls=%d", got, err, calls)
	}
	want := `{"version":1,"contract":"` + modelRoutingContract + `","operation":"apply","cwd":"/workspace","agentDir":"/agents","target":"project","configHome":"/config","loadExtensions":true,"draft":{"a":{"model":"provider/model"},"inherit":{},"z":{"thinking":"high"}}}` + "\n"
	if gotPath != "/tmp/model-routing" || string(gotRequest) != want {
		t.Fatalf("path/request = %q, %q; want %q, %q", gotPath, gotRequest, "/tmp/model-routing", want)
	}
	wantOptions := ModelRoutingProcessOptions{Timeout: modelRoutingTimeout, MaxRequestBytes: 64 << 10, MaxStdoutBytes: MaxModelRoutingResponseBytes, MaxStderrBytes: 4 << 10}
	if gotOptions != wantOptions || got.Target == nil || *got.Target != ModelRoutingTargetGlobal || got.ConfigPath == nil || *got.ConfigPath != "/cfg/model-routing.json" || got.Materialization == nil {
		t.Fatalf("apply result/options = %#v, %#v", got, gotOptions)
	}
}

func TestModelRoutingClientApplyRequestWireShape(t *testing.T) {
	model := "provider/model"
	thinking := ModelRoutingThinkingHigh
	for _, tt := range []struct {
		name    string
		request ModelRoutingRequestContext
		draft   ModelRoutingDraft
		want    string
	}{
		{name: "nil", want: `{"version":1,"contract":"` + modelRoutingContract + `","operation":"apply","cwd":"","agentDir":"","target":"","draft":{}}` + "\n"},
		{name: "empty", draft: ModelRoutingDraft{}, want: `{"version":1,"contract":"` + modelRoutingContract + `","operation":"apply","cwd":"","agentDir":"","target":"","draft":{}}` + "\n"},
		{name: "populated", request: ModelRoutingRequestContext{CWD: "/workspace", AgentDir: "/agents", Target: ModelRoutingTargetProject, ConfigHome: "/config", LoadExtensions: true}, draft: ModelRoutingDraft{"z": {Thinking: &thinking}, "a": {Model: &model}, "inherit": {}}, want: `{"version":1,"contract":"` + modelRoutingContract + `","operation":"apply","cwd":"/workspace","agentDir":"/agents","target":"project","configHome":"/config","loadExtensions":true,"draft":{"a":{"model":"provider/model"},"inherit":{},"z":{"thinking":"high"}}}` + "\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshalModelRoutingApplyRequest(tt.request, tt.draft)
			if err != nil || string(got) != tt.want {
				t.Fatalf("request = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestModelRoutingClientApplySemanticMatrix(t *testing.T) {
	full := `,"target":"global","configPath":"/cfg/model-routing.json","materialization":{"affected":["global","project"],"succeeded":["global"],"failed":[{"target":"project","message":"write failed"}]}`
	for _, tt := range []struct {
		name    string
		code    int
		class   string
		outcome ModelRoutingApplyOutcome
		saved   bool
	}{
		{"invalid-input", 2, "invalid-input", ModelRoutingApplyOutcomeValidationFailure, false},
		{"unsupported-contract", 3, "unsupported-contract", ModelRoutingApplyOutcomeValidationFailure, false},
		{"unavailable-runtime", 4, "unavailable-runtime", ModelRoutingApplyOutcomeUnavailableRuntime, false},
		{"persistence", 5, "persistence", ModelRoutingApplyOutcomePersistenceFailure, false},
		{"partial", 6, "partial", ModelRoutingApplyOutcomePartial, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("apply-provider-secret")
			got, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				return ModelRoutingProcessResult{ExitCode: tt.code, Stdout: applyEnvelope(applyResult(false, string(tt.outcome), tt.saved, full), false, tt.class)}, transportError(TransportErrorNonzeroExit, cause)
			}).Apply(context.Background(), ModelRoutingRequestContext{}, nil)
			var clientErr *ModelRoutingClientError
			var transportErr *TransportError
			if err == nil || got.OK || got.Outcome != tt.outcome || got.Saved != tt.saved || got.Target == nil || *got.Target != ModelRoutingTargetGlobal || got.ConfigPath == nil || *got.ConfigPath != "/cfg/model-routing.json" || got.Materialization == nil || len(got.Materialization.Affected) != 2 || len(got.Materialization.Failed) != 1 || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorSemantic || clientErr.ExitCode != tt.code || clientErr.ExitClass != tt.class || !errors.As(err, &transportErr) || transportErr.Kind != TransportErrorNonzeroExit || !errors.Is(err, ErrModelRoutingClientSemantic) || !errors.Is(err, cause) || strings.Contains(err.Error(), "apply-provider-secret") {
				t.Fatalf("semantic result/error = %#v, %T %v", got, err, err)
			}
		})
	}
}

func TestModelRoutingClientApplyProtocolMatrix(t *testing.T) {
	valid := applyEnvelope(applyResult(true, "success", true, ""), true, "success")
	cause := errors.New("apply-protocol-secret")
	for _, tt := range []struct {
		name      string
		code      int
		body      []byte
		callError bool
	}{
		{"envelope ok", 2, applyEnvelope(applyResult(false, "validation-failure", false, ""), true, "invalid-input"), true},
		{"result ok", 2, applyEnvelope(applyResult(true, "validation-failure", false, ""), false, "invalid-input"), true},
		{"class mismatch", 2, applyEnvelope(applyResult(false, "validation-failure", false, ""), false, "unsupported-contract"), true},
		{"outcome mismatch", 2, applyEnvelope(applyResult(false, "partial", false, ""), false, "invalid-input"), true},
		{"exit5 saved true", 5, applyEnvelope(applyResult(false, "persistence-failure", true, ""), false, "persistence"), true},
		{"exit6 saved false", 6, applyEnvelope(applyResult(false, "partial", false, ""), false, "partial"), true},
		{"nonzero without error", 2, valid, false},
		{"code0 with error", 0, valid, true},
		{"unsupported exit", 7, valid, true},
		{"code0 envelope", 0, applyEnvelope(applyResult(true, "success", true, ""), false, "success"), false},
		{"code0 class", 0, applyEnvelope(applyResult(true, "success", true, ""), true, "invalid-input"), false},
		{"code0 result", 0, applyEnvelope(applyResult(false, "success", true, ""), true, "success"), false},
		{"code0 outcome", 0, applyEnvelope(applyResult(true, "partial", true, ""), true, "success"), false},
		{"code0 saved", 0, applyEnvelope(applyResult(true, "success", false, ""), true, "success"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var callErr error
			if tt.callError {
				callErr = transportError(TransportErrorNonzeroExit, cause)
			}
			got, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				return ModelRoutingProcessResult{ExitCode: tt.code, Stdout: tt.body}, callErr
			}).Apply(context.Background(), ModelRoutingRequestContext{}, nil)
			var clientErr *ModelRoutingClientError
			if err == nil || !reflect.DeepEqual(got, ModelRoutingApplyResult{}) || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorProtocol || !errors.Is(err, ErrModelRoutingClientProtocol) || tt.callError && !errors.Is(err, cause) || strings.Contains(err.Error(), "apply-protocol-secret") {
				t.Fatalf("protocol result/error = %#v, %T %v", got, err, err)
			}
		})
	}
}

func TestModelRoutingClientApplyParserTransportAndBoundaries(t *testing.T) {
	_, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		return ModelRoutingProcessResult{Stdout: []byte("{")}, nil
	}).Apply(context.Background(), ModelRoutingRequestContext{}, nil)
	var responseErr *ModelRoutingResponseError
	if !errors.As(err, &responseErr) || responseErr.Kind != ModelRoutingResponseErrorMalformed || responseErr.ExpectedOperation != ModelRoutingOperationApply {
		t.Fatalf("parser error = %T %v", err, err)
	}
	for _, kind := range []TransportErrorKind{TransportErrorInvalidOptions, TransportErrorInvalidPath, TransportErrorInvalidRequest, TransportErrorStart, TransportErrorWait, TransportErrorCanceled, TransportErrorTimeout, TransportErrorStdoutOverflow, TransportErrorStderrOverflow, TransportErrorTermination, TransportErrorUnsupportedPlatform} {
		t.Run(string(kind), func(t *testing.T) {
			calls := 0
			cause := errors.New("apply-transport-secret")
			_, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				calls++
				return ModelRoutingProcessResult{}, transportError(kind, cause)
			}).Apply(context.Background(), ModelRoutingRequestContext{}, nil)
			var clientErr *ModelRoutingClientError
			var transportErr *TransportError
			if calls != 1 || !errors.As(err, &clientErr) || clientErr.Kind != ModelRoutingClientErrorTransport || !errors.As(err, &transportErr) || transportErr.Kind != kind || !errors.Is(err, cause) || strings.Contains(err.Error(), "apply-transport-secret") {
				t.Fatalf("transport error = %T %v; calls=%d", err, err, calls)
			}
		})
	}
	var nilClient *ModelRoutingClient
	if _, err := nilClient.Apply(context.Background(), ModelRoutingRequestContext{}, nil); !errors.Is(err, ErrModelRoutingClientInvalidClient) {
		t.Fatalf("nil client error = %v", err)
	}
	if _, err := newModelRoutingClient(ModelRoutingCandidate{}, clientCapabilities(), nil).Apply(context.Background(), ModelRoutingRequestContext{}, nil); !errors.Is(err, ErrModelRoutingClientTransport) {
		t.Fatalf("nil transport error = %v", err)
	}
	for _, tt := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"nil", nil, ErrTransportInvalidOptions}, {"canceled", canceledContext(), context.Canceled}, {"timeout", timeoutContext(), context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			_, err := newModelRoutingClient(ModelRoutingCandidate{}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
				calls++
				return ModelRoutingProcessResult{}, nil
			}).Apply(tt.ctx, ModelRoutingRequestContext{}, nil)
			if calls != 0 || !errors.Is(err, tt.want) {
				t.Fatalf("context error = %v; calls=%d", err, calls)
			}
		})
	}
}

func TestModelRoutingClientApplyErrorIsDiagnosticSecretSafe(t *testing.T) {
	body := applyEdit(t, applyResult(true, "success", true, ""), "diagnostics", `[{"code":"secret-code","message":"diagnostic-secret","severity":"error"}]`)
	_, err := newModelRoutingClient(ModelRoutingCandidate{Path: "/pi"}, clientCapabilities(), func(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
		return ModelRoutingProcessResult{Stdout: applyEnvelope(body, false, "success")}, nil
	}).Apply(context.Background(), ModelRoutingRequestContext{}, nil)
	if err == nil || !errors.Is(err, ErrModelRoutingClientProtocol) || strings.Contains(err.Error(), "diagnostic-secret") || strings.Contains(err.Error(), "secret-code") {
		t.Fatalf("unsafe apply error = %v", err)
	}
}
