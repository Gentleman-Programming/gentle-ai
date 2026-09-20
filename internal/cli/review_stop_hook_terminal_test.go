package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

func TestReviewStopHookSilentAfterAcknowledgementWithoutPostBurnStatus(t *testing.T) {
	repo, _, _, pending := startZeroLensPendingAcknowledgement(t)
	if err := reviewtransaction.AcknowledgeApprovedCompactAuthority(context.Background(), repo, pending.LineageID,
		pending.TargetIdentity, pending.ExpectedRevision, pending.Token); err != nil {
		t.Fatal(err)
	}
	// No prior Stop reminder or post-burn STATUS may be needed for silence.
	for _, session := range []string{"first-session", "another-session"} {
		var output bytes.Buffer
		payload := reviewStopHookTestPayload(t, session, repo, false, nil)
		if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &output, io.Discard); err != nil {
			t.Fatal(err)
		}
		if output.Len() != 0 {
			t.Fatalf("acknowledged target triggered Stop: %s", output.String())
		}
	}
	writeReviewStartCandidate(t, repo, "docs/ordinary-guide.md", "new work after acknowledgement\n", 0o644)
	var output bytes.Buffer
	payload := reviewStopHookTestPayload(t, "first-session", repo, false, nil)
	if err := runReviewStopHook([]string{"--agent", "claude-code"}, strings.NewReader(payload), &output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 {
		t.Fatal("changed candidate did not trigger a reminder")
	}
}
