package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestRelayedCandidateDeclineAllowsOnlyExactPreCommitDelivery(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-candidate-decline", "--consent", "relay",
	})).Bytes())
	decline := runConsentRelayStart(t, invocationArgs(t, question.Choices[1].Invocation))
	var declined ReviewFacadeStartResult
	decodeStrictReviewJSON(t, decline.Bytes(), &declined)
	if declined.Consent != ReviewStartConsentDeclinedThisCandidate || declined.LineageID != "" {
		t.Fatalf("declined start = %#v", declined)
	}
	if store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-candidate-decline"); err != nil {
		t.Fatal(err)
	} else if _, err := store.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("declined candidate created review lineage: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "review-candidate-decline", "receipt.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("declined candidate created receipt: %v", err)
	}

	runReviewCLIGit(t, repo, "add", "scripts/deploy.sh")
	var allowed bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &allowed); err != nil {
		t.Fatalf("candidate-declined exact pre-commit delivery blocked: %v\n%s", err, allowed.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, allowed.Bytes(), &result)
	if result.Delivery != reviewtransaction.RDDDeliveryCandidateDeclinedUnmanaged || result.Allowed || result.Result == reviewtransaction.GateAllow {
		t.Fatalf("candidate-declined delivery = %#v", result)
	}
	if result.Context.Denial == nil || result.Context.Denial.Stage != "candidate-decline" {
		t.Fatalf("candidate-declined delivery did not expose its unmanaged choice: %#v", result.Context)
	}

	if err := os.WriteFile(filepath.Join(repo, "scripts", "deploy.sh"), []byte("echo drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "scripts/deploy.sh")
	var drifted bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &drifted); err == nil {
		t.Fatalf("candidate-decline authorized drifted content:\n%s", drifted.String())
	}
}

func TestCandidateDeclineNeverAuthorizesRelease(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-candidate-decline-release", "--consent", "relay",
	})).Bytes())
	runConsentRelayStart(t, invocationArgs(t, question.Choices[1].Invocation))
	runReviewCLIGit(t, repo, "add", "scripts/deploy.sh")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")

	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GateRelease)}, &output); err == nil {
		t.Fatalf("candidate decline authorized release:\n%s", output.String())
	}
}
