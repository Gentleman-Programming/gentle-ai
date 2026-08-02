package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestUnqualifiedPrePRDiscoveryReportsPublicationDefaultResolution is the
// community-reported defect: a healthy authority, an approved receipt, and a
// publication default that cannot be resolved because the remote's HEAD symref
// names a branch that does not exist. The gate reported `authority_corrupted`,
// which sends an operator into repair machinery for a store that is intact.
// pre-push already tells the truth in the same condition
// (TestUnqualifiedPrePushDiscoveryReportsTargetResolutionWithoutMutation), and
// this path must reach that same typed statement — including the continuation
// that actually clears the block.
func TestUnqualifiedPrePRDiscoveryReportsPublicationDefaultResolution(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	_, store := approveDiscoveryMarkdown(t, repo, "review-publication-default", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed target")
	// The advertised default branch disappears; every ref the candidate needs
	// is still present and every receipt is still exactly where it was.
	runReviewCLIGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/absent-default")
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	var negotiated bytes.Buffer
	runErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR),
	}, &negotiated)
	if runErr == nil {
		t.Fatalf("unresolvable publication default validated:\n%s", negotiated.String())
	}
	failure := decodeReviewIntegrationFailure(t, negotiated.Bytes())
	if failure.Code != "target_resolution_failed" || failure.Phase != "pre_native" ||
		failure.AuthorityApplicability != "not_evaluated" || failure.NextAction != "correct_request" ||
		!reflect.DeepEqual(failure.RequiredInputs, []string{"base_ref"}) ||
		!strings.Contains(failure.Message, "--base-ref") {
		t.Fatalf("publication-default failure = %#v", failure)
	}
	if strings.Contains(strings.ToLower(failure.Message), "pre-push") {
		t.Fatalf("pre-PR failure names the pre-push gate: %q", failure.Message)
	}
	var targetErr *reviewtransaction.GateTargetResolutionError
	if !errors.As(runErr, &targetErr) {
		t.Fatalf("publication-default cause = %T %v", runErr, runErr)
	}

	var raw bytes.Buffer
	rawErr := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePrePR)}, &raw)
	var evaluation ReviewValidateResult
	decodeStrictReviewJSON(t, raw.Bytes(), &evaluation)
	if rawErr == nil || evaluation.Result != reviewtransaction.GateInvalidated ||
		evaluation.Context.Denial == nil || evaluation.Context.Denial.Stage != "target-resolution" ||
		evaluation.Context.Denial.Code != "target_resolution_failed" ||
		!strings.Contains(evaluation.Reason, "--base-ref") {
		t.Fatalf("non-negotiated publication default = %#v, %T %v", evaluation, rawErr, rawErr)
	}

	stateAfter, stateErr := os.ReadFile(store.StatePath())
	receiptAfter, receiptErr := os.ReadFile(store.ReceiptPath())
	if stateErr != nil || receiptErr != nil || !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatalf("publication-default resolution mutated authority: state=%v receipt=%v", stateErr, receiptErr)
	}

	// The branch rule: a message may name a command only if running it clears
	// the block. Run exactly what the refusal named.
	var resolved bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch,
	}, &resolved); err != nil {
		t.Fatalf("continuation named by the refusal did not clear the block: %v\n%s", err, resolved.String())
	}
	var allowed ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, resolved.Bytes()).Result, &allowed)
	if !allowed.Allowed {
		t.Fatalf("continuation named by the refusal = %#v", allowed)
	}
}

// TestUnqualifiedPrePRDiscoveryReportsUnresolvableExplicitSelector covers the
// step an operator takes immediately after following the message above and
// mistyping the branch. The named continuation must not itself dead-end in a
// false corruption report.
func TestUnqualifiedPrePRDiscoveryReportsUnresolvableExplicitSelector(t *testing.T) {
	repo := initReviewCLIRepo(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	approveDiscoveryMarkdown(t, repo, "review-explicit-selector", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed target")

	var output bytes.Buffer
	runErr := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/typo-branch",
	}, &output)
	if runErr == nil {
		t.Fatalf("unresolvable explicit selector validated:\n%s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Code != "target_resolution_failed" || !reflect.DeepEqual(failure.RequiredInputs, []string{"base_ref"}) {
		t.Fatalf("explicit-selector failure = %#v", failure)
	}
}
