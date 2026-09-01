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

// TestRelayedCandidateDeclineNeverAuthorizesLaterGateDelivery supersedes
// TestRelayedCandidateDeclineAllowsOnlyExactPreCommitDelivery (Wave 5 Slice
// 6, design decision 6): a relayed decline still reports `declined` for the
// one `review start` call itself and still creates no review lineage or
// receipt, but the identical candidate now denies (receipt_missing) at
// pre-commit instead of reaching a decline-specific unmanaged delivery —
// there is no ResolveCandidateDeclineForGate left to resolve it, and
// nothing was ever recorded (RecordCandidateDecline is deleted) for a
// later gate call to find. A drifted candidate still denies too, exactly
// as before, for the identical structural reason (no receipt governs it).
func TestRelayedCandidateDeclineNeverAuthorizesLaterGateDelivery(t *testing.T) {
	reviewEnabledHome(t)
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
	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("candidate-declined pre-commit delivery: %v\n%s", err, output.String())
	}
	assertEnabledUnmanagedGatePayload(t, output.Bytes(), reviewtransaction.GatePreCommit)

	if err := os.WriteFile(filepath.Join(repo, "scripts", "deploy.sh"), []byte("echo drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "scripts/deploy.sh")
	var drifted bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &drifted); err != nil {
		t.Fatalf("candidate-decline drifted delivery: %v\n%s", err, drifted.String())
	}
	assertEnabledUnmanagedGatePayload(t, drifted.Bytes(), reviewtransaction.GatePreCommit)
}

func TestCandidateDeclineNeverAuthorizesRelease(t *testing.T) {
	reviewEnabledHome(t)
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
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GateRelease)}, &output); err != nil {
		t.Fatalf("candidate decline release delivery: %v\n%s", err, output.String())
	}
	assertEnabledUnmanagedGatePayload(t, output.Bytes(), reviewtransaction.GateRelease)
}
