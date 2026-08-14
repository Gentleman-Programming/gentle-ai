package sdd

// Slice-6 RED tests for the native SDD phase task-result half (tasks 6.1,
// design #3138, REQ-SOA-1/2): isSDDPhase, the per-session sddDispatchLatched
// latch cleared on session.deleted/dispose, taskRouteModel route-token
// scrubbing, and the GENTLE_AI_SDD_FAILURE envelope byte-equal to the TS
// handoff (SEN-SOA-1..3). Catalog parity with no managed plugin (SEN-SOA-4)
// is the existing bounded-review contract suite, which must keep passing; it
// is exercised by the slice's full-package evidence, not duplicated here.
//
// These tests are RED by construction: they reference IsSDDPhase,
// TaskRouteModel, UnwrapSDDTaskResult, SDDTaskFailureEnvelope,
// SDDDispatchLatchedEnvelope, SDDLatchStore, NewInMemorySDDLatchStore, and
// NewFileSDDLatchStore, none of which exist until task 6.2 creates
// sdd_dispatch.go. The golden envelope strings below are the exact bytes the
// TS handoff (review-result-artifacts.ts, pre-split) produced for equal
// inputs -- key order, HTML-unescaped JSON, and the trailing-space prefix
// included -- so a native drift from the TS semantics fails here first.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestIsSDDPhase(t *testing.T) {
	// SDD_PHASES in review-result-artifacts.ts:
	// sdd-init, sdd-explore, sdd-propose, sdd-spec, sdd-design, sdd-tasks,
	// sdd-apply, sdd-verify, sdd-archive, sdd-onboard.
	for _, phase := range []string{
		"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
	} {
		if !IsSDDPhase(phase) {
			t.Errorf("IsSDDPhase(%q) = false, want true (exact phase)", phase)
		}
	}
	// The TS predicate is phase === agent || agent.startsWith(phase + "-"),
	// so a phase-scoped subagent (for example a phase variant carrying extra
	// tokens) is still an SDD phase.
	for _, subagent := range []string{
		"sdd-apply-orchestrator", "sdd-propose-child", "sdd-spec-v2",
		"sdd-onboard-extra-token",
	} {
		if !IsSDDPhase(subagent) {
			t.Errorf("IsSDDPhase(%q) = false, want true (phase-scoped subagent)", subagent)
		}
	}
	for _, other := range []string{
		"", "sdd", "sddx", "my-sdd-apply", "review-risk", "review-reliability",
		"sdd-", "-sdd-apply", "sdd_apply",
	} {
		if IsSDDPhase(other) {
			t.Errorf("IsSDDPhase(%q) = true, want false", other)
		}
	}
}

func TestTaskRouteModelScrubsAndValidatesRouteTokens(t *testing.T) {
	// The child's model route is the one causal fact the task hook receives;
	// only a plain route identifier may enter the handoff. Anything with
	// separators, whitespace, or path shapes is omitted entirely rather than
	// truncated (SEN-SOA-3 route-token scrubbing).
	metadata := func(model any) any {
		return map[string]any{"model": model}
	}
	for _, tc := range []struct {
		name     string
		metadata any
		want     string
	}{
		{name: "valid anthropic route", metadata: metadata(map[string]any{"providerID": "anthropic", "modelID": "claude-sonnet-4-5"}), want: "anthropic/claude-sonnet-4-5"},
		{name: "valid numeric model", metadata: metadata(map[string]any{"providerID": "openai", "modelID": "101010"}), want: "openai/101010"},
		{name: "valid colon and at tokens", metadata: metadata(map[string]any{"providerID": "models:op", "modelID": "claude@latest"}), want: "models:op/claude@latest"},
		{name: "valid underscore and dot", metadata: metadata(map[string]any{"providerID": "my_provider", "modelID": "model.v2"}), want: "my_provider/model.v2"},
		// The route token's model half admits exactly 128 characters total
		// (`{0,127}` after the leading character); the boundary pair below
		// pins both edges so a {0,126} or {0,128} regression fails here.
		{name: "max-length token", metadata: metadata(map[string]any{"providerID": "a", "modelID": strings.Repeat("b", 128)}), want: "a/" + strings.Repeat("b", 128)},
		{name: "one past max-length token", metadata: metadata(map[string]any{"providerID": "a", "modelID": strings.Repeat("b", 129)}), want: ""},
		{name: "nil metadata", metadata: nil, want: ""},
		{name: "empty metadata object", metadata: map[string]any{}, want: ""},
		{name: "missing model", metadata: metadata(nil), want: ""},
		{name: "string metadata", metadata: "nope", want: ""},
		{name: "array metadata", metadata: []any{}, want: ""},
		{name: "string model", metadata: metadata("nope"), want: ""},
		{name: "array model", metadata: metadata([]any{}), want: ""},
		{name: "non-string providerID", metadata: metadata(map[string]any{"providerID": 7, "modelID": "m"}), want: ""},
		{name: "non-string modelID", metadata: metadata(map[string]any{"providerID": "p", "modelID": true}), want: ""},
		{name: "empty providerID", metadata: metadata(map[string]any{"providerID": "", "modelID": "m"}), want: ""},
		{name: "empty modelID", metadata: metadata(map[string]any{"providerID": "p", "modelID": ""}), want: ""},
		{name: "path-shaped providerID", metadata: metadata(map[string]any{"providerID": "/etc/passwd", "modelID": "m"}), want: ""},
		{name: "slash inside providerID", metadata: metadata(map[string]any{"providerID": "a/b", "modelID": "m"}), want: ""},
		{name: "whitespace inside modelID", metadata: metadata(map[string]any{"providerID": "p", "modelID": "claude sonnet"}), want: ""},
		{name: "leading dot providerID", metadata: metadata(map[string]any{"providerID": ".hidden", "modelID": "m"}), want: ""},
		{name: "leading dash modelID", metadata: metadata(map[string]any{"providerID": "p", "modelID": "-dash"}), want: ""},
		{name: "newline inside modelID", metadata: metadata(map[string]any{"providerID": "p", "modelID": "a\nb"}), want: ""},
		{name: "over-long token", metadata: metadata(map[string]any{"providerID": "a", "modelID": strings.Repeat("b", 256)}), want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TaskRouteModel(tc.metadata); got != tc.want {
				t.Fatalf("TaskRouteModel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUnwrapSDDTaskResultClassifiesLikeTaskResult(t *testing.T) {
	// Mirrors taskResult(output, "SDD phase", "sddClass"): the class feeds
	// the terminal handoff, so empty and malformed must stay distinct
	// (#2677: an empty result means the child never ran inference at all).
	raw := `{"subject_hash":"s","findings":[],"evidence":["e"]}`
	for _, tc := range []struct {
		name   string
		output any
		want   string
		class  string
	}{
		{name: "bare text passes through", output: raw, want: raw, class: ""},
		{name: "trimmed bare text", output: "  " + raw + "\n", want: raw, class: ""},
		{name: "completed envelope unwraps", output: "<task id=\"t\" state=\"completed\">\n<task_result>\n" + raw + "\n</task_result>\n</task>", want: raw, class: ""},
		{name: "empty output", output: "", want: "", class: "empty_result"},
		{name: "blank output", output: " \n\t ", want: "", class: "empty_result"},
		{name: "non-string output", output: map[string]any{}, want: "", class: "empty_result"},
		{name: "empty envelope body", output: "<task id=\"t\" state=\"completed\">\n<task_result>\n\n</task_result>\n</task>", want: "", class: "empty_result"},
		{name: "malformed envelope", output: "<task id=\"t\" state=\"failed\">\n<task_result>\n{}\n</task_result>\n</task>", want: "", class: "malformed_result"},
		{name: "nested envelope", output: "<task id=\"t\" state=\"completed\">\n<task_result>\n<task id=\"i\" state=\"completed\">\n<task_result>\n{}\n</task_result>\n</task>\n</task_result>\n</task>", want: "", class: "nested_envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, class, err := UnwrapSDDTaskResult(tc.output)
			if tc.class == "" {
				if err != nil {
					t.Fatalf("UnwrapSDDTaskResult() error = %v, want success", err)
				}
				if class != "" {
					t.Fatalf("UnwrapSDDTaskResult() class = %q, want empty on success", class)
				}
				if body != tc.want {
					t.Fatalf("UnwrapSDDTaskResult() body = %q, want %q", body, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("UnwrapSDDTaskResult() error = nil, want class %q", tc.class)
			}
			if class != tc.class {
				t.Fatalf("UnwrapSDDTaskResult() class = %q, want %q (message: %v)", class, tc.class, err)
			}
		})
	}
}

// TestSDDTaskFailureEnvelopeByteEqualsTSHandoff pins SEN-SOA-3 at the byte
// level: the native handoff must equal the TS handoff for equal inputs. The
// golden strings below were produced by review-result-artifacts.ts
// sddTaskFailure (JSON.stringify key order, no HTML escaping, prefix with a
// trailing space).
func TestSDDTaskFailureEnvelopeByteEqualsTSHandoff(t *testing.T) {
	emptyGolden := "GENTLE_AI_SDD_FAILURE " +
		`{"schemaName":"gentle-ai.sdd-task-result-failure/v1","status":"blocked","code":"sdd_task_result_empty","phase":"sdd-propose","summary":"sdd-propose produced no task output at all. The child task returned nothing, which most often means the provider rejected the request before generation (authentication, region, or model access), the task was interrupted, or the phase genuinely wrote nothing. Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user.","continuation":"gentle-ai sdd-status --cwd '/repo' --json"}`
	malformedGolden := "GENTLE_AI_SDD_FAILURE " +
		`{"schemaName":"gentle-ai.sdd-task-result-failure/v1","status":"blocked","code":"sdd_task_result_malformed","phase":"sdd-apply","taskModel":"anthropic/claude-sonnet-4-5","summary":"sdd-apply returned no valid task result. Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user.","continuation":"gentle-ai sdd-status --cwd '/repo' --json"}`
	quotedCwdGolden := "GENTLE_AI_SDD_FAILURE " +
		`{"schemaName":"gentle-ai.sdd-task-result-failure/v1","status":"blocked","code":"sdd_task_result_empty","phase":"sdd-verify","summary":"sdd-verify produced no task output at all. The child task returned nothing, which most often means the provider rejected the request before generation (authentication, region, or model access), the task was interrupted, or the phase genuinely wrote nothing. Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user.","continuation":"gentle-ai sdd-status --cwd '/repo/it'\\''s' --json"}`

	for _, tc := range []struct {
		name   string
		phase  string
		cwd    string
		class  string
		meta   any
		golden string
	}{
		{name: "empty result omits taskModel", phase: "sdd-propose", cwd: "/repo", class: "empty_result", meta: nil, golden: emptyGolden},
		{name: "malformed result with routed taskModel", phase: "sdd-apply", cwd: "/repo", class: "malformed_result", meta: map[string]any{"model": map[string]any{"providerID": "anthropic", "modelID": "claude-sonnet-4-5"}}, golden: malformedGolden},
		{name: "single-quoted cwd uses shellQuote", phase: "sdd-verify", cwd: "/repo/it's", class: "empty_result", meta: nil, golden: quotedCwdGolden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SDDTaskFailureEnvelope(tc.phase, tc.cwd, tc.class, tc.meta)
			if got != tc.golden {
				t.Fatalf("SDDTaskFailureEnvelope() =\n%s\nwant byte-equal TS handoff:\n%s", got, tc.golden)
			}
			if !strings.HasPrefix(got, "GENTLE_AI_SDD_FAILURE ") {
				t.Fatalf("handoff missing prefix: %q", got)
			}
		})
	}
}

// TestSDDDispatchLatchedEnvelopeByteEqualsTSHandoff pins the latched-launch
// refusal: it names the phase THIS launch asked for, which earlier phase
// actually failed and how, and the exit (a new session), because the latch is
// per-session state cleared on session.deleted and dispose (#2948).
func TestSDDDispatchLatchedEnvelopeByteEqualsTSHandoff(t *testing.T) {
	golden := "GENTLE_AI_SDD_FAILURE " +
		`{"schemaName":"gentle-ai.sdd-task-result-failure/v1","status":"blocked","code":"sdd_task_dispatch_latched","phase":"sdd-verify","latchedPhase":"sdd-propose","latchedCode":"sdd_task_result_empty","summary":"sdd-verify was not dispatched. Earlier in this session sdd-propose returned sdd_task_result_empty, and SDD launches stay latched afterwards so a failed phase is never silently retried and no later phase advances on top of it. No provider call, no subagent, and no artifact write happened for this launch, so it produced no new evidence about the original failure.","continuation":"gentle-ai sdd-status --cwd '/repo' --json","exit":"Inspect the artifact state the original failure left, surface it to the user, and start a new session to launch SDD phases again. Relaunching in this session cannot dispatch."}`

	failure := SDDTaskFailure{Phase: "sdd-propose", Code: "sdd_task_result_empty"}
	got := SDDDispatchLatchedEnvelope("sdd-verify", failure, "/repo")
	if got != golden {
		t.Fatalf("SDDDispatchLatchedEnvelope() =\n%s\nwant byte-equal TS handoff:\n%s", got, golden)
	}
}

func TestSDDLatchStoreIsPerSessionAndClearsOnSessionEnd(t *testing.T) {
	// SEN-SOA-2: the latch is per-session; session.deleted/dispose clears it
	// and no stale latch persists across sessions.
	store := NewInMemorySDDLatchStore()
	failure := SDDTaskFailure{Phase: "sdd-propose", Code: "sdd_task_result_empty", Handoff: "GENTLE_AI_SDD_FAILURE {}"}
	if err := store.Record("session-a", failure); err != nil {
		t.Fatalf("Record(session-a) error = %v", err)
	}
	if err := store.Record("session-b", failure); err != nil {
		t.Fatalf("Record(session-b) error = %v", err)
	}

	got, ok, err := store.Recall("session-a")
	if err != nil || !ok || got != failure {
		t.Fatalf("Recall(session-a) = (%#v, %v, %v), want the recorded failure", got, ok, err)
	}
	other, ok, err := store.Recall("session-b")
	if err != nil || !ok || other != failure {
		t.Fatalf("Recall(session-b) = (%#v, %v, %v), want the recorded failure", other, ok, err)
	}

	// session.deleted for one session clears only that session.
	if err := store.Clear("session-a"); err != nil {
		t.Fatalf("Clear(session-a) error = %v", err)
	}
	if _, ok, err := store.Recall("session-a"); err != nil || ok {
		t.Fatalf("Recall(session-a) after Clear = (_, %v, %v), want absent", ok, err)
	}
	if _, ok, err := store.Recall("session-b"); err != nil || !ok {
		t.Fatalf("Recall(session-b) after Clear(session-a) = (_, %v, %v), want still present (SEN-SOA-2 per-session scope)", ok, err)
	}

	// dispose clears everything.
	if err := store.ClearAll(); err != nil {
		t.Fatalf("ClearAll() error = %v", err)
	}
	if _, ok, err := store.Recall("session-b"); err != nil || ok {
		t.Fatalf("Recall(session-b) after dispose = (_, %v, %v), want absent", ok, err)
	}
}

func TestFileSDDLatchStoreRecordsAfterNullPayload(t *testing.T) {
	// `null` is valid JSON that decodes into a nil map; a truncated write or
	// a hand edit landing on "null" must behave as an empty latch, not crash
	// Record with a nil-map assignment. Arguably the rarer and the more
	// dangerous failure: it happens exactly when the store is already
	// suspect, so it must fail closed, never panic.
	path := t.TempDir() + "/sdd-dispatch-latch.json"
	if err := os.WriteFile(path, []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewFileSDDLatchStore(path)
	failure := SDDTaskFailure{Phase: "sdd-spec", Code: "sdd_task_result_malformed", Handoff: "GENTLE_AI_SDD_FAILURE {\"handoff\":true}"}
	if err := store.Record("session-1", failure); err != nil {
		t.Fatalf("Record(session-1) over a \"null\" latch file error = %v", err)
	}
	got, ok, err := store.Recall("session-1")
	if err != nil || !ok || got != failure {
		t.Fatalf("Recall(session-1) = (%#v, %v, %v), want the recorded failure", got, ok, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("latch file is not valid JSON after Record over \"null\": %v\n%s", err, raw)
	}
	if _, present := decoded["session-1"]; !present {
		t.Fatalf("latch file missing session-1: %v", decoded)
	}
}

func TestFileSDDLatchStorePersistsAcrossSpawns(t *testing.T) {
	// The native verbs are spawned per hook call, so the latch must survive
	// process exits: a Record from one store instance is visible to a new
	// instance reading the same file (the CLI wires FileSDDLatchStore).
	path := t.TempDir() + "/sdd-dispatch-latch.json"
	failure := SDDTaskFailure{Phase: "sdd-spec", Code: "sdd_task_result_malformed", Handoff: "GENTLE_AI_SDD_FAILURE {\"handoff\":true}"}

	writer := NewFileSDDLatchStore(path)
	if err := writer.Record("session-1", failure); err != nil {
		t.Fatalf("Record(session-1) error = %v", err)
	}

	reader := NewFileSDDLatchStore(path)
	got, ok, err := reader.Recall("session-1")
	if err != nil || !ok {
		t.Fatalf("Recall(session-1) from a fresh store = (_, %v, %v), want the recorded failure", ok, err)
	}
	if got != failure {
		t.Fatalf("persisted latch = %#v, want %#v", got, failure)
	}

	// The persisted shape is a JSON map keyed by session ID, so the file is
	// human-inspectable and bounded to the latch domain.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(latch file) error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("latch file is not valid JSON: %v\n%s", err, raw)
	}
	if _, present := decoded["session-1"]; !present {
		t.Fatalf("latch file missing session-1: %v", decoded)
	}
}
