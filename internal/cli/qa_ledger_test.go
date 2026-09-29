package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// These tests cover the 3A.9 CLI surface for qa-begin/qa-finish/qa-approve/
// qa-status, wired to qastage.QAStateMachine, with the flag disposition from
// docs/migration/qa-orchestrator-v3-design.md section 1.1: --evidence-goal,
// --max-attempts and --max-changed-lines are retired; --evidence-revision is
// kept but now means an artifact_revision qa-validate actually produced.

func TestQABeginHelp_DoesNotMentionRetiredFlags(t *testing.T) {
	var stdout bytes.Buffer
	if err := RunQABegin([]string{"--help"}, &stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	help := stdout.String()
	for _, retired := range []string{"--evidence-goal", "--max-attempts", "--max-changed-lines"} {
		if strings.Contains(help, retired) {
			t.Fatalf("expected qa-begin help to NOT mention retired flag %q, got:\n%s", retired, help)
		}
	}
}

func TestQABeginStatusFinish_FullCycleOverCLI(t *testing.T) {
	dir := t.TempDir()
	change := "cli-change-a"

	var beginOut bytes.Buffer
	err := RunQABegin([]string{"--change", change, "--stage", "explore", "--cwd", dir, "--request-id", "b1"}, &beginOut)
	if err != nil {
		t.Fatalf("qa-begin: unexpected error: %v", err)
	}
	var begin QABeginResult
	if err := json.Unmarshal(beginOut.Bytes(), &begin); err != nil {
		t.Fatalf("decode qa-begin output %q: %v", beginOut.String(), err)
	}
	if begin.Stage != "explore" || begin.NextAction != "finish" {
		t.Fatalf("unexpected qa-begin result: %+v", begin)
	}

	var statusOut bytes.Buffer
	if err := RunQAStatus([]string{"--change", change, "--cwd", dir}, &statusOut); err != nil {
		t.Fatalf("qa-status: unexpected error: %v", err)
	}
	var status QAStatusResult
	if err := json.Unmarshal(statusOut.Bytes(), &status); err != nil {
		t.Fatalf("decode qa-status output %q: %v", statusOut.String(), err)
	}
	if status.NextAction != "finish" || status.Complete {
		t.Fatalf("unexpected qa-status result: %+v", status)
	}

	var finishOut bytes.Buffer
	err = RunQAFinish([]string{"--change", change, "--cwd", dir, "--request-id", "f1", "--outcome", "passed", "--evidence-revision", anyCLIRevision}, &finishOut)
	if err != nil {
		t.Fatalf("qa-finish: unexpected error: %v", err)
	}
	var finish QAFinishResult
	if err := json.Unmarshal(finishOut.Bytes(), &finish); err != nil {
		t.Fatalf("decode qa-finish output %q: %v", finishOut.String(), err)
	}
	if finish.NextAction != "begin" {
		t.Fatalf("unexpected qa-finish result: %+v", finish)
	}
}

func TestQABegin_ExpectedRevisionMismatchIsRejected(t *testing.T) {
	dir := t.TempDir()
	change := "cli-change-b"

	var out bytes.Buffer
	err := RunQABegin([]string{"--change", change, "--stage", "explore", "--cwd", dir, "--request-id", "b1", "--expected-revision", "sha256:" + strings.Repeat("9", 64)}, &out)
	if err == nil {
		t.Fatal("expected an error for a stale expected-revision on a fresh change (actual revision is empty)")
	}
}

func TestQAApprove_RequiresEvidenceRevisionBoundToSpec(t *testing.T) {
	dir := t.TempDir()
	change := "cli-change-c"

	mustRunQA(t, RunQABegin, []string{"--change", change, "--stage", "explore", "--cwd", dir, "--request-id", "b1"})
	mustRunQA(t, RunQAFinish, []string{"--change", change, "--cwd", dir, "--request-id", "f1", "--outcome", "passed", "--evidence-revision", anyCLIRevision})
	mustRunQA(t, RunQABegin, []string{"--change", change, "--stage", "spec", "--cwd", dir, "--request-id", "b2"})
	mustRunQA(t, RunQAFinish, []string{"--change", change, "--cwd", dir, "--request-id", "f2", "--outcome", "passed", "--evidence-revision", specRevisionCLI})

	var approveOut bytes.Buffer
	err := RunQAApprove([]string{"--change", change, "--cwd", dir, "--stage", "spec", "--evidence-revision", specRevisionCLI, "--actor", "qa-lead", "--reason", "looks good", "--request-id", "a1"}, &approveOut)
	if err != nil {
		t.Fatalf("qa-approve: unexpected error: %v", err)
	}

	var beginApplyOut bytes.Buffer
	err = RunQABegin([]string{"--change", change, "--stage", "apply", "--cwd", dir, "--request-id", "b3"}, &beginApplyOut)
	if err != nil {
		t.Fatalf("expected apply to begin after approving spec's exact revision, got %v", err)
	}
}

const anyCLIRevision = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
const specRevisionCLI = "sha256:3333333333333333333333333333333333333333333333333333333333333333"

func mustRunQA(t *testing.T, run func([]string, io.Writer) error, args []string) {
	t.Helper()
	var buf bytes.Buffer
	if err := run(args, &buf); err != nil {
		t.Fatalf("unexpected error running %v: %v (output: %s)", args, err, buf.String())
	}
}

func TestQAReset_ClosesStuckActiveAttemptWithAudit(t *testing.T) {
	dir := t.TempDir()
	change := "cli-change-reset"

	mustRunQA(t, RunQABegin, []string{"--change", change, "--stage", "explore", "--cwd", dir, "--request-id", "b1"})

	var resetOut bytes.Buffer
	err := RunQAReset([]string{"--change", change, "--cwd", dir, "--actor", "qa-lead", "--reason", "agent died mid-stage"}, &resetOut)
	if err != nil {
		t.Fatalf("qa-reset: unexpected error: %v", err)
	}
	var reset QAResetResult
	if err := json.Unmarshal(resetOut.Bytes(), &reset); err != nil {
		t.Fatalf("decode qa-reset output %q: %v", resetOut.String(), err)
	}
	if reset.Stage != "explore" || reset.Outcome != "interrupted" {
		t.Fatalf("unexpected qa-reset result: %+v", reset)
	}

	// Retryable afterwards.
	mustRunQA(t, RunQABegin, []string{"--change", change, "--stage", "explore", "--cwd", dir, "--request-id", "b2"})
}

func TestQAReset_RequiresActorAndReason(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := RunQAReset([]string{"--change", "cli-change-reset-b", "--cwd", dir, "--actor", "qa-lead"}, &out)
	if err == nil {
		t.Fatal("expected an error when --reason is missing")
	}
}
