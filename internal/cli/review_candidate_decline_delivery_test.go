package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestCandidateDeclineNeverAuthorizesPrePushOrPrePRDelivery supersedes
// TestCandidateDeclineAllowsExactPrePushAndPrePRButNotLaterCandidate (Wave
// 5 Slice 6, design decision 6): a decline no longer resolves at pre-push
// or pre-pr either — the identical delivered candidate now denies
// receipt_missing at both gates, the same generic denial any
// never-reviewed commit reaches. The later-candidate assertion (decline
// never authorizes an unrelated subsequent candidate) stays: it was never
// specific to decline resolving anything, it proves nothing does.
func TestCandidateDeclineNeverAuthorizesPrePushOrPrePRDelivery(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-candidate-decline-delivery", "--consent", "relay",
	})).Bytes())
	runConsentRelayStart(t, invocationArgs(t, question.Choices[1].Invocation))
	runReviewCLIGit(t, repo, "add", "scripts/deploy.sh")
	runReviewCLIGit(t, repo, "commit", "-qm", "declined candidate")

	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		t.Run(string(gate), func(t *testing.T) {
			var output bytes.Buffer
			err := RunReviewFacadeValidate([]string{
				"--cwd", repo, "--gate", string(gate), "--base-ref", "origin/" + branch,
			}, &output)
			if err == nil {
				t.Fatalf("candidate-declined %s delivery unexpectedly allowed:\n%s", gate, output.String())
			}
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			if result.Allowed || result.Delivery != "" {
				t.Fatalf("candidate-declined %s result = %#v, want a plain denial", gate, result)
			}
			if result.Context.Denial == nil || result.Context.Denial.Stage != "receipt-discovery" || result.Context.Denial.Code != "receipt_missing" {
				t.Fatalf("candidate-declined %s context = %#v, want the generic receipt-discovery/receipt_missing denial", gate, result.Context)
			}
		})
	}

	writeReviewStartCandidate(t, repo, "scripts/later.sh", "echo later\n", 0o644)
	var later bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &later); err == nil {
		t.Fatalf("candidate decline authorized later candidate:\n%s", later.String())
	}
}
