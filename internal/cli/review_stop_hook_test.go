package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// reviewStopHookTestPayload builds one Stop hook stdin payload. Extra fields
// are accepted through overrides so a test can assert unknown keys are
// ignored.
func reviewStopHookTestPayload(t *testing.T, sessionID, cwd string, stopHookActive bool, extra map[string]any) string {
	t.Helper()
	fields := map[string]any{
		"session_id":       sessionID,
		"transcript_path":  "/tmp/does-not-matter.jsonl",
		"cwd":              cwd,
		"hook_event_name":  "Stop",
		"stop_hook_active": stopHookActive,
	}
	for key, value := range extra {
		fields[key] = value
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func stageReviewStopHookCandidate(t *testing.T, repo string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func enableReviewStopHookRDD(t *testing.T, repo string) {
	t.Helper()
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "global"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func decodeReviewStopHookResult(t *testing.T, stdout []byte) reviewStopHookOutput {
	t.Helper()
	var result reviewStopHookOutput
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode stop-hook output: %v\n%s", err, stdout)
	}
	return result
}

func reviewStopHookStateFile(home, sessionID string) string {
	return filepath.Join(home, ".gentle-ai", "review-stop-hook", "v1", sessionID+".json")
}

func TestReviewStopHookBlocksWithReasonAndBothCommands(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-block", repo, false, nil))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	result := decodeReviewStopHookResult(t, stdout.Bytes())
	if result.Schema != reviewStopHookSchema || result.Decision != "block" {
		t.Fatalf("result = %#v", result)
	}
	wantStatusCommand := fmt.Sprintf("gentle-ai review status --cwd %s --contract %s --agent claude-code --next-transition", repo, ReviewIntegrationContractV2)
	if !strings.Contains(result.Reason, wantStatusCommand) {
		t.Fatalf("reason missing canonical STATUS command:\n%s", result.Reason)
	}
	if !strings.Contains(result.Reason, "gentle-ai review start") || !strings.Contains(result.Reason, "--consent=relay") {
		t.Fatalf("reason missing the returned START command with --consent=relay:\n%s", result.Reason)
	}
	if !strings.Contains(result.Reason, "next_transition") {
		t.Fatalf("reason does not name next_transition routing:\n%s", result.Reason)
	}

	stateFile := reviewStopHookStateFile(home, "sess-block")
	if _, err := os.Stat(stateFile); err != nil {
		t.Fatalf("expected reminder state file: %v", err)
	}
}

func TestReviewStopHookSilentWhenStopHookActive(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-active", repo, true, nil))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected silence, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestReviewStopHookSilentWhenRDDOff(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	// Receipt-driven development is opt-in and off by default: no enable call.

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-off", repo, false, nil))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected silence, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(reviewStopHookStateFile(home, "sess-off")); !os.IsNotExist(err) {
		t.Fatalf("expected no reminder state file while RDD is off, stat err=%v", err)
	}
}

func TestReviewStopHookSilentForNonRepositoryCwd(t *testing.T) {
	reviewModeHome(t)
	nonRepo := t.TempDir()

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-non-repo", nonRepo, false, nil))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected silence, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, err := os.Lstat(filepath.Join(nonRepo, ".git")); !os.IsNotExist(err) {
		t.Fatalf("expected no git metadata to be bootstrapped, stat err=%v", err)
	}
}

func TestReviewStopHookSilentForCleanWorktree(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	enableReviewStopHookRDD(t, repo)
	// No candidate staged: the worktree is clean.

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-clean", repo, false, nil))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected silence, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestReviewStopHookSecondRunSameSessionIsSilent(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	payload := reviewStopHookTestPayload(t, "sess-repeat", repo, false, nil)
	var first bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &first, io.Discard); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if decodeReviewStopHookResult(t, first.Bytes()).Decision != "block" {
		t.Fatalf("first run did not block: %s", first.String())
	}

	var second, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &second, &stderr); err != nil {
		t.Fatalf("second run: %v\nstderr: %s", err, stderr.String())
	}
	if second.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected the second run for the same session and candidate to be silent, got stdout=%q stderr=%q", second.String(), stderr.String())
	}
}

func TestReviewStopHookDifferentSessionRemindsAgain(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	var first bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(reviewStopHookTestPayload(t, "sess-a", repo, false, nil)), &first, io.Discard); err != nil {
		t.Fatalf("first session run: %v", err)
	}
	if decodeReviewStopHookResult(t, first.Bytes()).Decision != "block" {
		t.Fatalf("first session did not block: %s", first.String())
	}

	var second bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(reviewStopHookTestPayload(t, "sess-b", repo, false, nil)), &second, io.Discard); err != nil {
		t.Fatalf("second session run: %v", err)
	}
	if decodeReviewStopHookResult(t, second.Bytes()).Decision != "block" {
		t.Fatalf("a different session was not reminded: %s", second.String())
	}
}

func TestReviewStopHookInvalidSessionIDRemindsWithoutPersisting(t *testing.T) {
	home := reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "not/a valid id!", repo, false, nil))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if decodeReviewStopHookResult(t, stdout.Bytes()).Decision != "block" {
		t.Fatalf("invalid session id should still remind: %s", stdout.String())
	}
	dir := filepath.Join(home, ".gentle-ai", "review-stop-hook", "v1")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no persisted reminder for an invalid session id, found %v", entries)
	}

	// A missing session id behaves the same way: still remind, never persist.
	stdin = strings.NewReader(reviewStopHookTestPayload(t, "", repo, false, nil))
	var stdout2 bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout2, io.Discard); err != nil {
		t.Fatalf("runReviewStopHook (missing session id): %v", err)
	}
	if decodeReviewStopHookResult(t, stdout2.Bytes()).Decision != "block" {
		t.Fatalf("missing session id should still remind: %s", stdout2.String())
	}
}

func TestReviewStopHookIgnoresUnknownJSONFields(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-unknown-fields", repo, false, map[string]any{
		"permission_mode": "default",
		"some_future_key": []int{1, 2, 3},
	}))
	var stdout, stderr bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
		t.Fatalf("runReviewStopHook: %v\nstderr: %s", err, stderr.String())
	}
	if decodeReviewStopHookResult(t, stdout.Bytes()).Decision != "block" {
		t.Fatalf("unknown fields should not change the decision: %s", stdout.String())
	}
}

func TestReviewStopHookRequiresSupportedAgent(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stageReviewStopHookCandidate(t, repo)
	enableReviewStopHookRDD(t, repo)

	tests := []struct {
		name string
		args []string
	}{
		{name: "missing --agent", args: nil},
		{name: "unsupported --agent pi", args: []string{"--agent", "pi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-agent", repo, false, nil))
			var stdout, stderr bytes.Buffer
			err := runReviewStopHook(tt.args, stdin, &stdout, &stderr)
			if err == nil {
				t.Fatalf("expected a preflight error, stdout=%q", stdout.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("expected no stdout on a preflight error, got %q", stdout.String())
			}
		})
	}
}
