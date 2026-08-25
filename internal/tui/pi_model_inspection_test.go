package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
)

func restorePiModelInspectionFns(t *testing.T) {
	t.Helper()
	getwd, home, env := piModelInspectionGetwdFn, piModelInspectionHomeDirFn, piModelInspectionEnvFn
	dir, enumerate := piModelInspectionAgentDirFn, piModelInspectionEnumerateFn
	selectCandidate, inspect, load := piModelInspectionSelectFn, piModelInspectionInspectFn, piModelInspectionLoadFn
	validate, validateLoad := piModelValidationFn, piModelValidationLoadFn
	apply, applyLoad := piModelApplyFn, piModelApplyLoadFn
	t.Cleanup(func() {
		piModelInspectionGetwdFn, piModelInspectionHomeDirFn, piModelInspectionEnvFn = getwd, home, env
		piModelInspectionAgentDirFn, piModelInspectionEnumerateFn = dir, enumerate
		piModelInspectionSelectFn, piModelInspectionInspectFn, piModelInspectionLoadFn = selectCandidate, inspect, load
		piModelValidationFn, piModelValidationLoadFn = validate, validateLoad
		piModelApplyFn, piModelApplyLoadFn = apply, applyLoad
	})
}

func TestResolvePiModelInspectionAgentDir(t *testing.T) {
	homeErr := errors.New("home unavailable")
	cases := []struct {
		name       string
		env        string
		home       string
		homeFnErr  error
		want       string
		wantErr    error
		homeCalled bool
	}{
		{name: "environment override", env: "/custom/pi", homeFnErr: homeErr, want: "/custom/pi"},
		{name: "default home", home: "/home/test", want: filepath.Join("/home/test", ".pi", "agent"), homeCalled: true},
		{name: "home error", homeFnErr: homeErr, wantErr: homeErr, homeCalled: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			restorePiModelInspectionFns(t)
			called := false
			piModelInspectionEnvFn = func(string) string { return tt.env }
			piModelInspectionHomeDirFn = func() (string, error) {
				called = true
				return tt.home, tt.homeFnErr
			}
			got, err := resolvePiModelInspectionAgentDir()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("directory = %q, want %q", got, tt.want)
			}
			if called != tt.homeCalled {
				t.Fatalf("home called = %v, want %v", called, tt.homeCalled)
			}
			if tt.wantErr != nil && !strings.Contains(err.Error(), "resolve Pi agent directory") {
				t.Fatalf("error = %q, want actionable resolver context", err)
			}
		})
	}
}

func TestPiModelInspectionCommandCapturesInputsBeforeExecution(t *testing.T) {
	restorePiModelInspectionFns(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	cwd, agentDir := "/before/project", "/before/agent"
	var gotContext context.Context
	var gotCWD, gotAgentDir string
	piModelInspectionGetwdFn = func() (string, error) { return cwd, nil }
	piModelInspectionAgentDirFn = resolvePiModelInspectionAgentDir
	piModelInspectionEnvFn = func(string) string { return agentDir }
	piModelInspectionLoadFn = func(gotCtx context.Context, gotCwd, gotDir string) (pi.ModelRoutingInspection, error) {
		gotContext, gotCWD, gotAgentDir = gotCtx, gotCwd, gotDir
		return pi.ModelRoutingInspection{Contract: "fixture"}, nil
	}

	cmd := piModelInspectionCmd(ctx, 42)
	cwd, agentDir = "/after/project", "/after/agent"
	msg, ok := cmd().(piModelInspectionMsg)
	if !ok {
		t.Fatalf("message type = %T, want piModelInspectionMsg", cmd())
	}
	if msg.requestID != 42 || msg.err != nil || msg.inspection.Contract != "fixture" {
		t.Fatalf("message = %+v", msg)
	}
	if gotContext != ctx || gotCWD != "/before/project" || gotAgentDir != "/before/agent" {
		t.Fatalf("captured inputs = %v/%q/%q", gotContext, gotCWD, gotAgentDir)
	}
}

func TestPiModelInspectionCommandReportsResolutionErrorsWithRequestID(t *testing.T) {
	getwdErr := errors.New("cwd unavailable")
	dirErr := errors.New("agent directory unavailable")
	for _, tt := range []struct {
		name  string
		getwd func() (string, error)
		dir   func() (string, error)
		want  error
	}{
		{name: "cwd", getwd: func() (string, error) { return "", getwdErr }, dir: func() (string, error) { return "/unused", nil }, want: getwdErr},
		{name: "agent directory", getwd: func() (string, error) { return "/project", nil }, dir: func() (string, error) { return "", dirErr }, want: dirErr},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restorePiModelInspectionFns(t)
			piModelInspectionGetwdFn, piModelInspectionAgentDirFn = tt.getwd, tt.dir
			called := false
			piModelInspectionLoadFn = func(context.Context, string, string) (pi.ModelRoutingInspection, error) {
				called = true
				return pi.ModelRoutingInspection{}, nil
			}
			msg := piModelInspectionCmd(context.Background(), 91)().(piModelInspectionMsg)
			if msg.requestID != 91 || msg.err != tt.want {
				t.Fatalf("message = %+v, want request ID 91 and error %v", msg, tt.want)
			}
			if called {
				t.Fatal("loader ran after command input resolution failed")
			}
		})
	}
}

func TestLoadPiModelInspectionUsesExactPiInputs(t *testing.T) {
	restorePiModelInspectionFns(t)
	ctx := context.WithValue(context.Background(), "key", "value")
	candidates := []pi.ModelRoutingCandidate{{Path: "/pi", Source: "PATH"}}
	selected := pi.ModelRoutingCandidate{Path: "/selected", Source: "package"}
	capabilities := pi.Capabilities{Contract: "contract", Supported: true, Operations: []string{"inspect"}}
	wantRequest := pi.ModelRoutingRequestContext{CWD: "/project", AgentDir: "/agents", Target: pi.ModelRoutingTargetProject}
	wantInspection := pi.ModelRoutingInspection{Contract: "fixture"}
	var gotSelectContext, gotInspectContext context.Context
	var gotCandidates []pi.ModelRoutingCandidate
	var gotCandidate pi.ModelRoutingCandidate
	var gotCapabilities pi.Capabilities
	var gotRequest pi.ModelRoutingRequestContext
	piModelInspectionEnumerateFn = func(cwd, agentDir string) ([]pi.ModelRoutingCandidate, error) {
		if cwd != wantRequest.CWD || agentDir != wantRequest.AgentDir {
			t.Fatalf("enumerate arguments = %q/%q", cwd, agentDir)
		}
		return candidates, nil
	}
	piModelInspectionSelectFn = func(gotCtx context.Context, got []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error) {
		gotSelectContext, gotCandidates = gotCtx, got
		return selected, capabilities, nil
	}
	piModelInspectionInspectFn = func(gotCtx context.Context, candidate pi.ModelRoutingCandidate, gotCaps pi.Capabilities, request pi.ModelRoutingRequestContext) (pi.ModelRoutingInspection, error) {
		gotInspectContext, gotCandidate, gotCapabilities, gotRequest = gotCtx, candidate, gotCaps, request
		return wantInspection, nil
	}

	got, err := loadPiModelInspection(ctx, wantRequest.CWD, wantRequest.AgentDir)
	if err != nil || !reflect.DeepEqual(got, wantInspection) {
		t.Fatalf("inspection/error = %+v/%v", got, err)
	}
	if gotSelectContext != ctx || gotInspectContext != ctx || !reflect.DeepEqual(gotCandidates, candidates) || gotCandidate != selected || !reflect.DeepEqual(gotCapabilities, capabilities) || gotRequest != wantRequest {
		t.Fatalf("forwarded inputs = %v/%v/%+v/%+v/%+v/%+v", gotSelectContext, gotInspectContext, gotCandidates, gotCandidate, gotCapabilities, gotRequest)
	}
}

func TestLoadPiModelInspectionPropagatesTypedFailures(t *testing.T) {
	candidates := []pi.ModelRoutingCandidate{{Path: "/pi", Source: "PATH"}}
	selected := candidates[0]
	cases := []struct {
		name  string
		stage string
		err   error
	}{
		{name: "candidate enumeration", stage: "enumerate", err: &pi.CandidateError{Source: "test", Path: "/pi", Kind: "fixture"}},
		{name: "candidate selection", stage: "select", err: &pi.SelectionError{Kind: pi.SelectionErrorNoCandidates}},
		{name: "inspection", stage: "inspect", err: &pi.ModelRoutingClientError{Candidate: selected, Kind: pi.ModelRoutingClientErrorProtocol}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			restorePiModelInspectionFns(t)
			piModelInspectionEnumerateFn = func(string, string) ([]pi.ModelRoutingCandidate, error) {
				if tt.stage == "enumerate" {
					return nil, tt.err
				}
				return candidates, nil
			}
			piModelInspectionSelectFn = func(context.Context, []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error) {
				if tt.stage == "select" {
					return pi.ModelRoutingCandidate{}, pi.Capabilities{}, tt.err
				}
				return selected, pi.Capabilities{}, nil
			}
			piModelInspectionInspectFn = func(context.Context, pi.ModelRoutingCandidate, pi.Capabilities, pi.ModelRoutingRequestContext) (pi.ModelRoutingInspection, error) {
				if tt.stage == "inspect" {
					return pi.ModelRoutingInspection{}, tt.err
				}
				return pi.ModelRoutingInspection{}, nil
			}
			got, err := loadPiModelInspection(context.Background(), "/project", "/agents")
			if !reflect.DeepEqual(got, pi.ModelRoutingInspection{}) || err != tt.err {
				t.Fatalf("inspection/error = %+v/%v, want zero/%v", got, err, tt.err)
			}
		})
	}
}

func TestLoadPiModelInspectionPropagatesCanceledContext(t *testing.T) {
	restorePiModelInspectionFns(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wantErr := &pi.SelectionError{Kind: pi.SelectionErrorCanceled, Cause: context.Canceled}
	var gotContext context.Context
	piModelInspectionEnumerateFn = func(string, string) ([]pi.ModelRoutingCandidate, error) {
		return []pi.ModelRoutingCandidate{{Path: "/pi", Source: "PATH"}}, nil
	}
	piModelInspectionSelectFn = func(gotCtx context.Context, _ []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error) {
		gotContext = gotCtx
		return pi.ModelRoutingCandidate{}, pi.Capabilities{}, wantErr
	}

	_, err := loadPiModelInspection(ctx, "/project", "/agents")
	if gotContext != ctx || err != wantErr || !errors.Is(err, context.Canceled) {
		t.Fatalf("context/error = %v/%v", gotContext, err)
	}
}

func TestPiModelInspectionLoaderHasNoMutationOperations(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(file), "pi_model_inspection.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func loadPiModelInspection")
	end := strings.Index(text[start:], "func inspectPiModelRoutingClient")
	if start < 0 || end < 0 {
		t.Fatal("inspection loader boundaries not found")
	}
	if strings.Contains(text[start:start+end], "Validate") || strings.Contains(text[start:start+end], "Apply") {
		t.Fatal("inspection loader contains an operation seam or call")
	}
}
func TestPiModelValidationCommandCapturesInputsBeforeExecution(t *testing.T) {
	restorePiModelInspectionFns(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	target := pi.ModelRoutingTargetGlobal
	model := "provider/model"
	draft := pi.ModelRoutingDraft{"worker": {Model: &model}}
	piModelInspectionGetwdFn = func() (string, error) { return "/before/project", nil }
	piModelInspectionAgentDirFn = func() (string, error) { return "/before/agent", nil }
	var gotContext context.Context
	var gotCWD, gotAgentDir string
	var gotTarget pi.ModelRoutingTarget
	var gotDraft pi.ModelRoutingDraft
	piModelValidationLoadFn = func(gotCtx context.Context, cwd, agentDir string, gotTargetValue pi.ModelRoutingTarget, gotDraftValue pi.ModelRoutingDraft) (pi.ModelRoutingValidationResult, error) {
		gotContext, gotCWD, gotAgentDir, gotTarget, gotDraft = gotCtx, cwd, agentDir, gotTargetValue, gotDraftValue
		return pi.ModelRoutingValidationResult{OK: true}, nil
	}
	cmd := piModelValidationCmd(ctx, 42, target, draft)
	model = "mutated"
	draft["worker"] = pi.ModelRoutingDraftAssignment{}
	msg, ok := cmd().(piModelValidationMsg)
	if !ok {
		t.Fatalf("message type = %T", cmd())
	}
	assignment := gotDraft["worker"]
	if msg.requestID != 42 || msg.err != nil || !msg.result.OK || gotContext != ctx || gotCWD != "/before/project" || gotAgentDir != "/before/agent" || gotTarget != target || assignment.Model == nil || *assignment.Model != "provider/model" {
		t.Fatalf("message/inputs = %+v/%v/%q/%q/%q/%#v", msg, gotContext, gotCWD, gotAgentDir, gotTarget, gotDraft)
	}
}
func TestLoadPiModelValidationUsesExactInputsAndSupportsEmptyDrafts(t *testing.T) {
	restorePiModelInspectionFns(t)
	ctx := context.WithValue(context.Background(), "key", "value")
	wantCandidate, wantCaps, wantTarget := pi.ModelRoutingCandidate{Path: "/selected"}, pi.Capabilities{Contract: "contract", Supported: true}, pi.ModelRoutingTargetGlobal
	var gotCtx context.Context
	var gotDraft pi.ModelRoutingDraft
	piModelInspectionEnumerateFn = func(cwd, agentDir string) ([]pi.ModelRoutingCandidate, error) {
		if cwd != "/project" || agentDir != "/agents" {
			t.Fatalf("enumerate = %q/%q", cwd, agentDir)
		}
		return []pi.ModelRoutingCandidate{{Path: "/candidate"}}, nil
	}
	piModelInspectionSelectFn = func(got context.Context, candidates []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error) {
		gotCtx = got
		if len(candidates) != 1 {
			t.Fatalf("candidates = %#v", candidates)
		}
		return wantCandidate, wantCaps, nil
	}
	calls := 0
	piModelValidationFn = func(got context.Context, candidate pi.ModelRoutingCandidate, caps pi.Capabilities, request pi.ModelRoutingRequestContext, draft pi.ModelRoutingDraft) (pi.ModelRoutingValidationResult, error) {
		calls++
		gotDraft = draft
		if got != ctx || candidate != wantCandidate || !reflect.DeepEqual(caps, wantCaps) || request != (pi.ModelRoutingRequestContext{CWD: "/project", AgentDir: "/agents", Target: wantTarget}) {
			t.Fatalf("validate inputs = %v/%#v/%#v/%#v", got, candidate, caps, request)
		}
		return pi.ModelRoutingValidationResult{OK: true}, nil
	}
	for _, tt := range []struct {
		draft pi.ModelRoutingDraft
		nilOK bool
	}{{nil, true}, {pi.ModelRoutingDraft{}, false}} {
		gotDraft = nil
		got, err := loadPiModelValidation(ctx, "/project", "/agents", wantTarget, tt.draft)
		if err != nil || !got.OK || gotCtx != ctx || (tt.nilOK != (gotDraft == nil)) || (!tt.nilOK && len(gotDraft) != 0) {
			t.Fatalf("result/error/calls/inputs = %#v/%v/%d/%v/%#v", got, err, calls, gotCtx, gotDraft)
		}
	}
	if calls != 2 {
		t.Fatalf("validate calls = %d, want one per draft", calls)
	}
}

func TestPiModelApplyCommandCapturesInputsAndClonesDraft(t *testing.T) {
	restorePiModelInspectionFns(t)
	ctx := context.WithValue(context.Background(), "key", "value")
	target, draft := pi.ModelRoutingTargetGlobal, pi.ModelRoutingDraft{"worker": {}}
	model := "provider/model"
	draft["worker"] = pi.ModelRoutingDraftAssignment{Model: &model}
	var gotContext context.Context
	var gotCWD, gotAgentDir string
	var gotTarget pi.ModelRoutingTarget
	var gotDraft pi.ModelRoutingDraft
	piModelInspectionGetwdFn = func() (string, error) { return "/before/project", nil }
	piModelInspectionAgentDirFn = func() (string, error) { return "/before/agent", nil }
	piModelApplyLoadFn = func(got context.Context, cwd, agentDir string, target pi.ModelRoutingTarget, draft pi.ModelRoutingDraft) (pi.ModelRoutingApplyResult, error) {
		gotContext, gotCWD, gotAgentDir, gotTarget, gotDraft = got, cwd, agentDir, target, draft
		return pi.ModelRoutingApplyResult{Outcome: pi.ModelRoutingApplyOutcomeSuccess, Saved: true}, nil
	}
	cmd := piModelApplyCmd(ctx, 42, target, draft)
	model = "mutated"
	draft["worker"] = pi.ModelRoutingDraftAssignment{}
	msg := cmd().(piModelApplyMsg)
	assignment := gotDraft["worker"]
	if msg.requestID != 42 || msg.err != nil || gotContext != ctx || gotCWD != "/before/project" || gotAgentDir != "/before/agent" || gotTarget != target || assignment.Model == nil || *assignment.Model != "provider/model" {
		t.Fatalf("captured apply inputs = %+v/%v/%q/%q/%q/%#v", msg, gotContext, gotCWD, gotAgentDir, gotTarget, gotDraft)
	}
}

func TestLoadPiModelApplyEnumeratesSelectsAndAppliesOnce(t *testing.T) {
	restorePiModelInspectionFns(t)
	candidate, caps := pi.ModelRoutingCandidate{Path: "/selected"}, pi.Capabilities{Contract: "contract"}
	calls := 0
	piModelInspectionEnumerateFn = func(cwd, agentDir string) ([]pi.ModelRoutingCandidate, error) {
		return []pi.ModelRoutingCandidate{{Path: "/candidate"}}, nil
	}
	piModelInspectionSelectFn = func(context.Context, []pi.ModelRoutingCandidate) (pi.ModelRoutingCandidate, pi.Capabilities, error) {
		return candidate, caps, nil
	}
	piModelApplyFn = func(_ context.Context, gotCandidate pi.ModelRoutingCandidate, gotCaps pi.Capabilities, request pi.ModelRoutingRequestContext, _ pi.ModelRoutingDraft) (pi.ModelRoutingApplyResult, error) {
		calls++
		if gotCandidate != candidate || !reflect.DeepEqual(gotCaps, caps) || request != (pi.ModelRoutingRequestContext{CWD: "/project", AgentDir: "/agents", Target: pi.ModelRoutingTargetProject}) {
			t.Fatalf("apply inputs = %#v/%#v/%#v", gotCandidate, gotCaps, request)
		}
		return pi.ModelRoutingApplyResult{Outcome: pi.ModelRoutingApplyOutcomeSuccess, Saved: true}, nil
	}
	got, err := loadPiModelApply(context.Background(), "/project", "/agents", pi.ModelRoutingTargetProject, nil)
	if err != nil || !got.Saved || calls != 1 {
		t.Fatalf("result/error/calls = %#v/%v/%d", got, err, calls)
	}
}
