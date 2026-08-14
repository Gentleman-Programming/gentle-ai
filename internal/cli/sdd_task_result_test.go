package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
)

// SDD half, native dispatch verbs (change #3138, slice 6, task 6.2). The
// verbs replace the SDD half of the TS plugin (review-result-artifacts.ts):
// `sdd task-result guard` decides whether a phase launch may dispatch,
// `sdd task-result result` classifies one finished phase task and records
// the terminal latch on failure, and `clear` / `clear-all` honour the TS
// per-session latch lifecycle (session.deleted deletes one entry, dispose
// clears all). The reviewer-shim glue (slice 6.4) spawns exactly these
// verbs and forwards stdout as-is, so the protocol is deliberately plain:
// empty stdout on "nothing to report", the handoff envelope on stdout when a
// launch must be blocked or a failure recorded.
//
// The envelope bytes themselves are pinned in the sdd package
// (sdd_dispatch_test.go); these tests pin the wiring between the CLI layer
// and that contract: which verbs touch the latch, what is printed on what
// path, and that a cold latch or a non-SDD phase never blocks.

func TestRunSDDTaskResultUnknownCommandIsRefused(t *testing.T) {
	// The reviewer-shim glue spawns exactly the documented verbs, so an
	// unrecognized command is the drift signal when the installed glue and
	// this binary disagree; the operator is directed to re-sync the glue.
	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"frobnicate", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-apply", "--latch-path", latchPath(t)},
		bytes.NewReader(nil), &out,
	)
	if err == nil {
		t.Fatal("runSDDTaskResult accepted an unknown command")
	}
}

func TestRunSDDTaskResultGuardColdLatchPasses(t *testing.T) {
	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-apply", "--latch-path", latchPath(t)},
		bytes.NewReader(nil), &out,
	)
	if err != nil {
		t.Fatalf("guard on cold latch: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("guard on cold latch printed %q, want empty stdout", out.String())
	}
}

func TestRunSDDTaskResultGuardNonSDDPhaseNeverBlocks(t *testing.T) {
	// A latched session must never block a non-SDD launch: the guard only
	// speaks about SDD phases, so the glue stays safe even if a future agent
	// name collides with the phase-membership data list.
	path := latchPath(t)
	recordLatch(t, path, "s1", "sdd-propose", "sdd_task_result_empty")

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "codegen", "--latch-path", path},
		bytes.NewReader(nil), &out,
	)
	if err != nil {
		t.Fatalf("guard for non-SDD phase: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("guard for non-SDD phase printed %q, want empty stdout", out.String())
	}
}

func TestRunSDDTaskResultResultFailureRecordsLatchAndGuardBlocks(t *testing.T) {
	path := latchPath(t)
	metadata := map[string]any{"model": map[string]any{"providerID": "anthropic", "modelID": "claude-sonnet-4-5"}}
	payload := sddTaskResultPayload(t, "  \n", metadata)

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"result", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-propose", "--latch-path", path},
		bytes.NewReader(payload), &out,
	)
	if err != nil {
		t.Fatalf("result on empty output: %v", err)
	}
	wantHandoff := sdd.SDDTaskFailureEnvelope("sdd-propose", "/repo", "empty_result", metadata)
	if out.String() != wantHandoff {
		t.Fatalf("result handoff mismatch:\n got %q\nwant %q", out.String(), wantHandoff)
	}

	// The same session must now be latched: a later SDD phase launch is
	// refused with the latched envelope naming BOTH the requested phase and
	// the original failure.
	var latched bytes.Buffer
	err = runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-apply", "--latch-path", path},
		bytes.NewReader(nil), &latched,
	)
	if err != nil {
		t.Fatalf("guard after failure: %v", err)
	}
	wantLatched := sdd.SDDDispatchLatchedEnvelope("sdd-apply", sdd.SDDTaskFailure{
		Phase: "sdd-propose", Code: "sdd_task_result_empty",
	}, "/repo")
	if latched.String() != wantLatched {
		t.Fatalf("latched envelope mismatch:\n got %q\nwant %q", latched.String(), wantLatched)
	}

	// A different session is unaffected: the latch is per-session state.
	var other bytes.Buffer
	err = runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s2", "--phase", "sdd-apply", "--latch-path", path},
		bytes.NewReader(nil), &other,
	)
	if err != nil {
		t.Fatalf("guard for other session: %v", err)
	}
	if other.String() != "" {
		t.Fatalf("guard for other session printed %q, want empty stdout", other.String())
	}
}

func TestRunSDDTaskResultMalformedResultRecordsMalformedCode(t *testing.T) {
	path := latchPath(t)
	payload := sddTaskResultPayload(t, "<task id=\"t\" state=\"failed\">\n<task_result>\n{}\n</task_result>\n</task>", nil)

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"result", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-verify", "--latch-path", path},
		bytes.NewReader(payload), &out,
	)
	if err != nil {
		t.Fatalf("result on malformed output: %v", err)
	}
	wantHandoff := sdd.SDDTaskFailureEnvelope("sdd-verify", "/repo", "malformed_result", nil)
	if out.String() != wantHandoff {
		t.Fatalf("malformed handoff mismatch:\n got %q\nwant %q", out.String(), wantHandoff)
	}

	var latched bytes.Buffer
	err = runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-archive", "--latch-path", path},
		bytes.NewReader(nil), &latched,
	)
	if err != nil {
		t.Fatalf("guard after malformed failure: %v", err)
	}
	wantLatched := sdd.SDDDispatchLatchedEnvelope("sdd-archive", sdd.SDDTaskFailure{
		Phase: "sdd-verify", Code: "sdd_task_result_malformed",
	}, "/repo")
	if latched.String() != wantLatched {
		t.Fatalf("malformed latched envelope mismatch:\n got %q\nwant %q", latched.String(), wantLatched)
	}
}

func TestRunSDDTaskResultSuccessLeavesNoLatch(t *testing.T) {
	path := latchPath(t)
	payload := sddTaskResultPayload(t, "{\"subject_hash\":\"s\",\"findings\":[]}", nil)

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"result", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-apply", "--latch-path", path},
		bytes.NewReader(payload), &out,
	)
	if err != nil {
		t.Fatalf("result on success: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("success printed %q, want empty stdout", out.String())
	}

	var guard bytes.Buffer
	err = runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-verify", "--latch-path", path},
		bytes.NewReader(nil), &guard,
	)
	if err != nil {
		t.Fatalf("guard after success: %v", err)
	}
	if guard.String() != "" {
		t.Fatalf("guard after success printed %q, want empty stdout", guard.String())
	}
}

func TestRunSDDTaskResultClearUnblocksSession(t *testing.T) {
	path := latchPath(t)
	recordLatch(t, path, "s1", "sdd-propose", "sdd_task_result_empty")

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"clear", "--session", "s1", "--latch-path", path},
		bytes.NewReader(nil), &out,
	)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}

	var guard bytes.Buffer
	err = runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-apply", "--latch-path", path},
		bytes.NewReader(nil), &guard,
	)
	if err != nil {
		t.Fatalf("guard after clear: %v", err)
	}
	if guard.String() != "" {
		t.Fatalf("guard after clear printed %q, want empty stdout", guard.String())
	}
}

func TestRunSDDTaskResultClearAllRemovesEveryLatchEntry(t *testing.T) {
	path := latchPath(t)
	recordLatch(t, path, "s1", "sdd-propose", "sdd_task_result_empty")
	recordLatch(t, path, "s2", "sdd-verify", "sdd_task_result_malformed")

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"clear-all", "--latch-path", path},
		bytes.NewReader(nil), &out,
	)
	if err != nil {
		t.Fatalf("clear-all: %v", err)
	}

	for _, session := range []string{"s1", "s2"} {
		var guard bytes.Buffer
		err = runSDDTaskResult(
			[]string{"guard", "--cwd", "/repo", "--session", session, "--phase", "sdd-apply", "--latch-path", path},
			bytes.NewReader(nil), &guard,
		)
		if err != nil {
			t.Fatalf("guard for session %s after clear-all: %v", session, err)
		}
		if guard.String() != "" {
			t.Fatalf("guard for session %s after clear-all printed %q, want empty stdout", session, guard.String())
		}
	}
}

func TestRunSDDTaskResultUndecodablePayloadLatchesMalformed(t *testing.T) {
	// A payload that fails JSON decode fails closed: the session must never
	// behave as if the phase passed (SEN-SOA-2), so the malformed_result
	// latch is recorded and the terminal handoff written, and the next SDD
	// phase launch in this session is refused.
	path := latchPath(t)

	var out bytes.Buffer
	err := runSDDTaskResult(
		[]string{"result", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-verify", "--latch-path", path},
		bytes.NewReader([]byte("{not-json")), &out,
	)
	if err != nil {
		t.Fatalf("result on undecodable payload: %v", err)
	}
	wantHandoff := sdd.SDDTaskFailureEnvelope("sdd-verify", "/repo", "malformed_result", nil)
	if out.String() != wantHandoff {
		t.Fatalf("undecodable handoff mismatch:\n got %q\nwant %q", out.String(), wantHandoff)
	}

	var latched bytes.Buffer
	err = runSDDTaskResult(
		[]string{"guard", "--cwd", "/repo", "--session", "s1", "--phase", "sdd-archive", "--latch-path", path},
		bytes.NewReader(nil), &latched,
	)
	if err != nil {
		t.Fatalf("guard after undecodable failure: %v", err)
	}
	wantLatched := sdd.SDDDispatchLatchedEnvelope("sdd-archive", sdd.SDDTaskFailure{
		Phase: "sdd-verify", Code: "sdd_task_result_malformed",
	}, "/repo")
	if latched.String() != wantLatched {
		t.Fatalf("undecodable latched envelope mismatch:\n got %q\nwant %q", latched.String(), wantLatched)
	}
}

func latchPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "sdd-dispatch-latch.json")
}

func recordLatch(t *testing.T, path, session, phase, code string) {
	t.Helper()
	reason := "malformed_result"
	if code == "sdd_task_result_empty" {
		reason = "empty_result"
	}
	store := sdd.NewFileSDDLatchStore(path)
	if err := store.Record(session, sdd.SDDTaskFailure{
		Phase: phase, Code: code, Handoff: sdd.SDDTaskFailureEnvelope(phase, "/repo", reason, nil),
	}); err != nil {
		t.Fatalf("record latch: %v", err)
	}
}

func sddTaskResultPayload(t *testing.T, output string, metadata any) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"output": output, "metadata": metadata})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
