package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

// TestValidateBindsScopeChangedRecoveryChainAtPublicationGates covers
// gentle-ai issue #1422 end to end: an approved review is delivered as one
// commit, its scope_changed recovery successor is created from the live
// repository (a pristine, degenerate snapshot), and `review validate` must
// still bind and allow pre-push and pre-pr through the composed recovery
// chain instead of denying with candidate-or-paths-mismatch or the empty
// one-commit topology context.
func TestValidateBindsScopeChangedRecoveryChainAtPublicationGates(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	_, store := approveDiscoveryMarkdown(t, repo, "chained-cli-root", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed delivery")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", "chained-cli-root",
		"--expected-predecessor-revision", record.Revision, "--successor-lineage", "chained-cli-root-r1",
		"--disposition", "scope_changed", "--reason", "delivery committed after approval", "--actor", "maintainer@example.com",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("recover scope_changed successor: %v", err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "chained-cli-root-r1"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize recovered successor: %v", err)
	}

	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		t.Run(string(gate), func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{
				"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
				"--gate", string(gate), "--base-ref", "origin/" + branch,
			}, &output); err != nil {
				t.Fatalf("chained recovery %s validation: %v\n%s", gate, err, output.String())
			}
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
			if !result.Allowed || result.Context.LineageID != "chained-cli-root-r1" {
				t.Fatalf("chained recovery %s result = %#v", gate, result)
			}
			if result.Context.FixDeltaHash == reviewtransaction.EmptyFixDeltaHash || result.Context.FixDeltaHash == "" {
				t.Fatalf("chained recovery %s fix delta = %q", gate, result.Context.FixDeltaHash)
			}
			if !result.Context.BaseRelationshipValid {
				t.Fatalf("chained recovery %s base relationship = %#v", gate, result.Context)
			}
		})
	}

	t.Run("tampered predecessor receipt fails closed", func(t *testing.T) {
		if err := os.Remove(store.ReceiptPath()); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err := RunReview([]string{
			"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/" + branch,
		}, &output)
		if err == nil {
			var result ReviewValidateResult
			decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
			if result.Allowed {
				t.Fatalf("receiptless predecessor still allowed = %#v", result)
			}
		}
	})
}

// TestValidatePrePushDeriveFailureReportsReceiptDigests locks the denial
// context of an underivable pre-push delivery: the receipt digests must be
// reported instead of an all-blank context (issue #1422 / #1248).
func TestValidatePrePushDeriveFailureReportsReceiptDigests(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	_, store := approveDiscoveryMarkdown(t, repo, "underivable-pre-push", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed delivery")
	runReviewCLIGit(t, repo, "commit", "-q", "--allow-empty", "-m", "unreviewed extra commit")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := record.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}

	evaluation := reviewtransaction.EvaluateCompactGate(context.Background(), repo, receipt, reviewtransaction.NativeGateRequestInput{
		Gate: reviewtransaction.GatePrePush, LineageID: record.State.LineageID, BaseRef: "origin/" + branch,
	})
	if evaluation.Result != reviewtransaction.GateInvalidated ||
		!strings.Contains(evaluation.Reason, "reviewed delivery is not exactly one commit") {
		t.Fatalf("underivable pre-push evaluation = %#v", evaluation)
	}
	if evaluation.Context.LineageID != record.State.LineageID || evaluation.Context.BaseTree != receipt.BaseTree ||
		evaluation.Context.CandidateTree != receipt.FinalCandidateTree || evaluation.Context.Denial == nil {
		t.Fatalf("underivable pre-push context = %#v", evaluation.Context)
	}
}
