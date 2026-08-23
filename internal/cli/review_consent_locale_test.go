package cli

import (
	"strings"
	"testing"
)

func TestRelayedConsentSpanishLocalizesHumanFieldsWithoutChangingMachineTokens(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-consent-spanish", "--locale", "es", "--consent", "relay",
	})).Bytes())
	if question.Headline == reviewConsentHeadline || question.Value == reviewConsentValue ||
		!strings.Contains(question.Headline, "Gentle AI") || !strings.Contains(question.Reason, "revisión") {
		t.Fatalf("Spanish consent envelope did not localize human fields: %#v", question)
	}
	if len(question.Choices) != 2 || question.Choices[0].Answer != "granted" || question.Choices[1].Answer != "declined" {
		t.Fatalf("Spanish consent envelope changed machine answers: %#v", question.Choices)
	}
	for _, choice := range question.Choices {
		if !strings.Contains(choice.Invocation, "--target "+question.TargetIdentity) ||
			!strings.Contains(choice.Invocation, "--consent "+choice.Answer) ||
			!strings.Contains(choice.Invocation, "--locale es") {
			t.Fatalf("Spanish consent invocation changed its machine binding: %#v", choice)
		}
	}
	if question.OffPath.Command != reviewConsentOffPathCommand || !strings.Contains(question.OffPath.Note, "desactivar") {
		t.Fatalf("Spanish consent off path = %#v", question.OffPath)
	}
}
