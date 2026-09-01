package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestSettleRefusesReusedAcquireRequestIDAndNamesTheExit is #3872: settle
// with acquire's own --request-id found a begin receipt, refused, and sent the
// caller to status, whose live attempt was exactly the one being settled.
func TestSettleRefusesReusedAcquireRequestIDAndNamesTheExit(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "settle-reused-request-id"
	acquired, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "unit-1", 2))
	reused, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, acquired.Token, "unit-1", "passed"))
	if reused.State != "blocked" || reused.Reason != "invalid_continuation" || reused.Detail != reused.Exit {
		t.Fatalf("reused-request-id settle = %#v", reused)
	}
	assertExitNames(t, reused.Exit, "distinct --request-id", "unit-1", "`gentle-ai sdd-attempt status --cwd <repo> --change <change>`")
	settled, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, acquired.Token, "unit-1-settle", "passed"))
	if settled.State != "complete" || !strings.Contains(settled.Exit, "--work-unit \"<a different label>\"") {
		t.Fatalf("distinct-request-id settle = %#v, want complete naming the successor", settled)
	}
}

// TestSettleNamesMalformedTokenWithoutMentioningFinish is #3879: a malformed
// --token surfaced as "finish requires an exact expected runtime revision",
// naming a flag settle does not accept.
func TestSettleNamesMalformedTokenWithoutMentioningFinish(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "settle-malformed-token"
	runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "unit-1", 2))
	var output bytes.Buffer
	err := RunSDDAttempt(compactSettleArgs(repo, change, "not-a-token", "unit-1-settle", "passed"), &output)
	if err == nil {
		t.Fatalf("malformed token settle succeeded: %s", output.String())
	}
	if strings.Contains(err.Error(), "expected runtime revision") {
		t.Fatalf("malformed token settle named finish's flag: %v", err)
	}
	assertExitNames(t, err.Error(), "--token", "`gentle-ai sdd-attempt status --cwd <repo> --change <change>`", "`gentle-ai sdd-attempt settle`")
}

// TestCompleteNamesTheSuccessorWorkUnit is #3884: a completed objective
// answered `complete` with no exit, and rescope refused without naming the
// successor acquire that has existed since v2.3.0.
func TestCompleteNamesTheSuccessorWorkUnit(t *testing.T) {
	repo := initReviewCLIRepo(t)
	const change = "complete-successor"
	first, _ := runCompactSDDAttempt(t, compactWorkUnitAcquireArgs(repo, change, "slice-1-acquire", "slice-1"))
	if settled, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, first.Token, "slice-1-settle", "passed")); settled.State != "complete" {
		t.Fatalf("settle = %#v, want complete", settled)
	}

	repeated, payload := runCompactSDDAttempt(t, compactWorkUnitAcquireArgs(repo, change, "slice-1-again", "slice-1"))
	if repeated.State != "complete" || repeated.Detail != repeated.Exit {
		t.Fatalf("repeated acquire = %#v, want complete with detail mirroring exit", repeated)
	}
	assertExitNames(t, repeated.Exit, "different label", "slice-1", "`gentle-ai sdd-attempt acquire --cwd <repo> --change <change>")
	assertCompactPayloadKeys(t, payload, "state", "exit", "detail")

	status := runSDDAttemptStatus(t, []string{
		"status", "--cwd", repo, "--change", change, "--work-unit", "slice-1", "--evidence-goal", "prove compact attempt",
		"--max-attempts", "2", "--max-changed-lines", "20",
	})
	if status.BlockedExit != repeated.Exit {
		t.Fatalf("status blocked_exit = %q, want acquire's complete exit %q", status.BlockedExit, repeated.Exit)
	}

	var output bytes.Buffer
	err := RunSDDAttempt([]string{
		"rescope", "--cwd", repo, "--change", change, "--expected-revision", status.Revision, "--request-id", "slice-1-rescope",
		"--work-unit", "slice-1-narrow", "--evidence-goal", "prove compact attempt", "--max-attempts", "1", "--max-changed-lines", "10",
		"--reason", "narrowing a completed objective", "--actor", "maintainer",
	}, &output)
	if err == nil {
		t.Fatalf("rescope of a completed objective succeeded: %s", output.String())
	}
	assertExitNames(t, err.Error(), "different --work-unit", "`gentle-ai sdd-attempt acquire --cwd <repo> --change <change>")

	successor, _ := runCompactSDDAttempt(t, compactWorkUnitAcquireArgs(repo, change, "slice-2-acquire", "slice-2"))
	if successor.State != "proceed" || successor.Token == "" {
		t.Fatalf("successor acquire = %#v, want proceed with token", successor)
	}
}

func compactWorkUnitAcquireArgs(repo, change, requestID, workUnit string) []string {
	return []string{
		"acquire", "--cwd", repo, "--change", change, "--request-id", requestID,
		"--work-unit", workUnit, "--evidence-goal", "prove compact attempt",
		"--max-attempts", "2", "--max-changed-lines", "20",
	}
}

func assertExitNames(t *testing.T, exit string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(exit, want) {
			t.Fatalf("exit does not name %q:\n%s", want, exit)
		}
	}
}
