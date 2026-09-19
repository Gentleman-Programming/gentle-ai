package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func derivedRangeTerminalStatus(t *testing.T, repo string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &output); err != nil {
		t.Fatalf("derived range STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), output.Bytes())
	return status
}

func TestNextTransitionDerivedRangeAcknowledgementStaysTerminal(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation candidate")
	offered := derivedRangeTerminalStatus(t, repo)
	if offered.NextTransition == nil || offered.NextTransition.Execute == nil {
		t.Fatalf("no committed-range START: %#v", offered)
	}
	args := []string{"start"}
	for _, argument := range offered.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("execute offered START: %v\n%s", err, output.String())
	}
	pending := derivedRangeTerminalStatus(t, repo)
	if pending.TargetIdentity != offered.TargetIdentity || pending.NextTransition == nil || pending.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("derived range lost pending acknowledgement: %#v", pending)
	}
	args = []string{"acknowledge-approved"}
	for _, argument := range pending.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	output.Reset()
	if err := RunReview(args, &output); err != nil {
		t.Fatal(err)
	}
	// A new hook session has no reminder history to hide a duplicate START.
	payload := reviewStopHookTestPayload(t, "derived-range-session", repo, false, nil)
	output.Reset()
	var diagnostics bytes.Buffer
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &output, &diagnostics); err != nil || output.Len() != 0 {
		t.Fatalf("derived range Stop = %s, %v", output.String(), err)
	}
	after := derivedRangeTerminalStatus(t, repo)
	if after.TargetIdentity != offered.TargetIdentity || after.NextTransition == nil || after.NextTransition.ReasonCode != "target_already_acknowledged" || after.Authority != nil {
		t.Fatalf("derived range not terminal: %#v", after)
	}
	consumed, err := reviewtransaction.CompactTargetConsumed(context.Background(), repo, offered.TargetIdentity)
	if err != nil || !consumed {
		t.Fatalf("derived range consumption = %v, %v", consumed, err)
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", "new committed range\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "new range")
	changed := derivedRangeTerminalStatus(t, repo)
	if changed.TargetIdentity == offered.TargetIdentity || changed.NextTransition == nil || changed.NextTransition.Execute == nil || changed.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("new committed range suppressed: %#v", changed)
	}
}
