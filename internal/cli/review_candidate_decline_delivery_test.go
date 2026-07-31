package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestCandidateDeclineAllowsExactPrePushAndPrePRButNotLaterCandidate(t *testing.T) {
	reviewModeHome(t)
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
			if err := RunReviewFacadeValidate([]string{
				"--cwd", repo, "--gate", string(gate), "--base-ref", "origin/" + branch,
			}, &output); err != nil {
				t.Fatalf("candidate-declined %s delivery blocked: %v\n%s", gate, err, output.String())
			}
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			if result.Delivery != reviewtransaction.RDDDeliveryCandidateDeclinedUnmanaged || result.Allowed {
				t.Fatalf("candidate-declined %s result = %#v", gate, result)
			}
		})
	}

	writeReviewStartCandidate(t, repo, "scripts/later.sh", "echo later\n", 0o644)
	var later bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &later); err == nil {
		t.Fatalf("candidate decline authorized later candidate:\n%s", later.String())
	}
}
