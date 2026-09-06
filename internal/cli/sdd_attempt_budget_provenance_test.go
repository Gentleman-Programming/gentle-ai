package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestSDDAttemptBeginReportsMaxChangedLinesSource is issue #2589: an omitted
// --max-changed-lines silently applied the compiled 200-line default with no
// marker distinguishing it from a maintainer who typed 200 on purpose.
// `begin`'s RuntimeStatus answer now names which one happened.
func TestSDDAttemptBeginReportsMaxChangedLinesSource(t *testing.T) {
	t.Run("omitted flag reports default", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		status := runSDDAttemptStatus(t, []string{
			"begin", "--cwd", repo, "--change", "budget-default", "--expected-revision=",
			"--request-id", "budget-default-begin", "--work-unit", "budget-proof",
			"--evidence-goal", "prove the default budget is reported",
		})
		if status.Objective == nil || status.Objective.MaxChangedLines != 200 || status.Objective.MaxChangedLinesSource != "default" {
			t.Fatalf("omitted-flag objective = %#v, want 200 changed lines sourced \"default\"", status.Objective)
		}
	})

	t.Run("explicit flag reports explicit", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		status := runSDDAttemptStatus(t, []string{
			"begin", "--cwd", repo, "--change", "budget-explicit", "--expected-revision=",
			"--request-id", "budget-explicit-begin", "--work-unit", "budget-proof",
			"--evidence-goal", "prove an explicit budget is reported",
			"--max-attempts", "2", "--max-changed-lines", "200",
		})
		if status.Objective == nil || status.Objective.MaxChangedLines != 200 || status.Objective.MaxChangedLinesSource != "explicit" {
			t.Fatalf("explicit-flag objective = %#v, want 200 changed lines sourced \"explicit\"", status.Objective)
		}
	})
}

// TestSDDAttemptAcquireReportsAppliedDefaultOnly is issue #2589's acquire
// half: the compact projection stays bounded for an explicit budget
// (TestRunSDDAttemptCompactOutputStaysBoundedAcrossHistory already pins
// that), but an omitted --max-changed-lines now surfaces the applied default
// on the same proceed answer instead of only through a separate `status`
// call.
func TestSDDAttemptAcquireReportsAppliedDefaultOnly(t *testing.T) {
	t.Run("omitted flag surfaces the default", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		acquired, payload := runCompactSDDAttempt(t, []string{
			"acquire", "--cwd", repo, "--change", "acquire-budget-default",
			"--request-id", "acquire-budget-default-1", "--work-unit", "compact-unit",
			"--evidence-goal", "prove acquire reports the applied default",
		})
		if acquired.State != "proceed" || acquired.MaxChangedLines != 200 || acquired.MaxChangedLinesSource != "default" {
			t.Fatalf("acquire with omitted flag = %#v, want proceed with 200 changed lines sourced \"default\"", acquired)
		}
		assertCompactPayloadKeys(t, payload, "state", "token", "max_changed_lines", "max_changed_lines_source")
	})

	t.Run("explicit flag stays bounded", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		acquired, payload := runCompactSDDAttempt(t, compactAcquireArgs(repo, "acquire-budget-explicit", "acquire-budget-explicit-1", 2))
		if acquired.State != "proceed" || acquired.MaxChangedLines != 0 || acquired.MaxChangedLinesSource != "" {
			t.Fatalf("acquire with explicit flag = %#v, want no budget fields", acquired)
		}
		assertCompactPayloadKeys(t, payload, "state", "token")
	})
}

// TestSDDAttemptBeginRequestDigestSurvivesMaxChangedLinesExplicitField pins
// the replay-time hazard #2589's new BeginAttemptRequest.MaxChangedLinesExplicit
// field could have introduced: validateRuntimeBeginEvent reconstructs the
// original request from the persisted event to check its digest, and had to
// learn the new field too, or every explicit-budget begin would fail replay
// with "begin_request_digest_match" the moment its own record was read back.
func TestSDDAttemptBeginRequestDigestSurvivesMaxChangedLinesExplicitField(t *testing.T) {
	repo := initReviewCLIRepo(t)
	args := []string{
		"begin", "--cwd", repo, "--change", "digest-survives-explicit", "--expected-revision=",
		"--request-id", "digest-begin-1", "--work-unit", "digest-proof",
		"--evidence-goal", "prove the request digest survives explicit provenance",
		"--max-attempts", "2", "--max-changed-lines", "40",
	}
	began := runSDDAttemptStatus(t, args)
	if began.Objective == nil || began.ActiveAttempt == nil {
		t.Fatalf("begin = %#v", began)
	}
	// Replay: reading status back re-derives the whole chain from disk,
	// which is exactly where validateRuntimeBeginEvent runs.
	var output bytes.Buffer
	if err := RunSDDAttempt([]string{"status", "--cwd", repo, "--change", "digest-survives-explicit"}, &output); err != nil {
		t.Fatalf("status after begin: %v", err)
	}
	var decoded struct {
		Objective *struct {
			MaxChangedLines       int    `json:"max_changed_lines"`
			MaxChangedLinesSource string `json:"max_changed_lines_source"`
		} `json:"objective"`
	}
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Objective == nil || decoded.Objective.MaxChangedLines != 40 || decoded.Objective.MaxChangedLinesSource != "explicit" {
		t.Fatalf("replayed objective = %#v", decoded.Objective)
	}
}
