package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// gentle-shell#1494: the agent that made a change may raise its assess risk to
// high by citing one item of the shared high-risk list. It can never lower a
// tier; lowering stays deterministic.

func runEscalatedAssess(t *testing.T, repo string, extra ...string) (ReviewAssessmentResult, string, error) {
	t.Helper()
	var output bytes.Buffer
	args := append([]string{"assess", "--cwd", repo, "--json"}, extra...)
	err := RunReview(args, &output)
	var result ReviewAssessmentResult
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil && err == nil {
		t.Fatalf("decode review assess envelope: %v\n%s", decodeErr, output.String())
	}
	return result, output.String(), err
}

func TestReviewAssessAgentEscalationRaisesMediumToHigh(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "notes/scratch.txt", "some plain scratch content\n", 0o644)

	result, raw, err := runEscalatedAssess(t, repo, "--escalate-item", "2", "--escalate-reason", "weakens the permission check in utils.ts")
	if err != nil {
		t.Fatalf("review assess --escalate-item: %v\n%s", err, raw)
	}
	if result.Risk != "high" || !result.ReviewDue || result.ReviewDueReason != "high_risk" || result.NextTransition == nil {
		t.Fatalf("escalated medium candidate = %#v, want high, review due, with a next_transition", result)
	}
	last := result.Reasons[len(result.Reasons)-1]
	if last.Code != "agent_escalation" || !strings.Contains(last.Detail, "item 2 (security)") || !strings.Contains(last.Detail, "weakens the permission check in utils.ts") {
		t.Fatalf("escalation reason = %#v, want agent_escalation naming item 2 (security) and the reason", last)
	}
}

func TestReviewAssessAgentEscalationRaisesPassiveToHigh(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", "ordinary documentation prose.\n", 0o644)

	result, raw, err := runEscalatedAssess(t, repo, "--escalate-item", "3", "--escalate-reason", "documents a published flag")
	if err != nil {
		t.Fatalf("review assess: %v\n%s", err, raw)
	}
	if result.Risk != "high" {
		t.Fatalf("escalated passive candidate risk = %q, want high", result.Risk)
	}
}

func TestReviewAssessWithoutEscalationKeepsNativeTier(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "notes/scratch.txt", "some plain scratch content\n", 0o644)

	result, raw, err := runEscalatedAssess(t, repo)
	if err != nil {
		t.Fatalf("review assess: %v\n%s", err, raw)
	}
	if result.Risk != "medium" {
		t.Fatalf("unescalated candidate risk = %q, want medium", result.Risk)
	}
	for _, reason := range result.Reasons {
		if reason.Code == "agent_escalation" {
			t.Fatalf("unescalated candidate carries an escalation reason: %#v", result.Reasons)
		}
	}
}

func TestReviewAssessRejectsMalformedEscalation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "notes/scratch.txt", "some plain scratch content\n", 0o644)

	for name, extra := range map[string][]string{
		"item without reason": {"--escalate-item", "2"},
		"reason without item": {"--escalate-reason", "x"},
		"item zero":           {"--escalate-item", "0", "--escalate-reason", "x"},
		"item seven":          {"--escalate-item", "7", "--escalate-reason", "x"},
		"item not a number":   {"--escalate-item", "two", "--escalate-reason", "x"},
		"blank reason":        {"--escalate-item", "2", "--escalate-reason", "   "},
		"oversized reason":    {"--escalate-item", "2", "--escalate-reason", strings.Repeat("x", 501)},
	} {
		t.Run(name, func(t *testing.T) {
			_, raw, err := runEscalatedAssess(t, repo, extra...)
			if err == nil || !strings.Contains(err.Error(), "escalate") {
				t.Fatalf("malformed escalation %v error = %v, want an escalate error\n%s", extra, err, raw)
			}
		})
	}
}
