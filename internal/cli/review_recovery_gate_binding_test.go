package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestValidateBindsScopeChangedRecoveryChainAtPublicationGates covers
// gentle-ai issue #1422 end to end: an approved review is delivered as one
// commit, its scope_changed recovery successor is created from the live
// repository (a pristine, degenerate snapshot), and `review validate` must
// still bind and allow pre-push and pre-pr through the composed recovery
// chain instead of denying with candidate-or-paths-mismatch or the empty
// one-commit topology context.
func TestValidateBindsScopeChangedRecoveryChainAtPublicationGates(t *testing.T) {
	t.Parallel()

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

// TestValidateAllowsTheNamedRecoveryAfterThePredecessorDeliveryWasPushed
// drives the complete reported organic sequence for a recovery whose
// predecessor delivery is already published: approve, push, disable reviews,
// commit unreviewed, re-enable, gate, run exactly the `review.recover` the
// denial named, finalize, gate again. The governing rule is that a message may
// name a command only if running that command resolves the block, so the
// second gate must ALLOW. It previously denied with "compact recovery delivery
// changed during final authorization" because final authorization re-ran the
// whole-chain delivery verification the evaluation never relied on: the
// predecessor's segment is already on the remote, so the composed chain base
// can never be the live publication base.
func TestValidateAllowsTheNamedRecoveryAfterThePredecessorDeliveryWasPushed(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	_, store := approveDiscoveryMarkdown(t, repo, "pushed-root", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed docs delivery")
	if lineage := reviewPrePushAllowedLineage(t, repo, branch); lineage != "pushed-root" {
		t.Fatalf("approved docs delivery pre-push lineage = %q", lineage)
	}
	runReviewCLIGit(t, repo, "push", "-q", "origin", branch)

	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("disable receipt-driven development: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "unreviewed.md"), []byte("unreviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "unreviewed change")
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--scope", "clone"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("re-enable receipt-driven development: %v", err)
	}

	scope := reviewPrePushScopeChangeDenial(t, repo, branch)
	if scope.RecoveryOperation != "review.recover" || scope.PredecessorLineageID != "pushed-root" || scope.PredecessorRevision == "" {
		t.Fatalf("gate denial did not name a runnable recovery: %#v", scope)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if scope.PredecessorRevision != record.Revision {
		t.Fatalf("named predecessor revision = %q, live authority revision = %q", scope.PredecessorRevision, record.Revision)
	}

	// Exactly the inputs the denial named; actor and reason are self-derived
	// for an approved predecessor. The committed delta since the published
	// boundary is what is unreviewed, so that is the successor's target.
	var recovered bytes.Buffer
	if err := RunReviewRecover([]string{
		"--cwd", repo,
		"--predecessor-lineage", scope.PredecessorLineageID,
		"--expected-predecessor-revision", scope.PredecessorRevision,
		"--successor-lineage", "pushed-root-r1",
		"--disposition", "scope_changed",
		"--committed-only", "--base-ref", "origin/" + branch,
	}, &recovered); err != nil {
		t.Fatalf("run the recovery the denial named: %v\n%s", err, recovered.String())
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", "pushed-root-r1"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize the named successor: %v", err)
	}

	if lineage := reviewPrePushAllowedLineage(t, repo, branch); lineage != "pushed-root-r1" {
		t.Fatalf("gate after the named recovery bound lineage %q", lineage)
	}
}

// reviewPrePushAllowedLineage runs the pre-push gate and fails the test unless
// it allows, returning the lineage the allow was bound to.
func reviewPrePushAllowedLineage(t *testing.T, repo, branch string) string {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/" + branch,
	}, &output); err != nil {
		t.Fatalf("pre-push validation: %v\n%s", err, output.String())
	}
	var result ReviewValidateResult
	decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, output.Bytes()).Result, &result)
	if !result.Allowed {
		t.Fatalf("pre-push validation denied: %#v", result)
	}
	return result.Context.LineageID
}

// reviewPrePushScopeChangeDenial runs the pre-push gate and fails the test
// unless it denies with scope-change recovery diagnostics.
func reviewPrePushScopeChangeDenial(t *testing.T, repo, branch string) ReviewIntegrationScopeChange {
	t.Helper()
	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/" + branch,
	}, &output)
	if err == nil {
		t.Fatalf("unreviewed commit was not denied:\n%s", output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("pre-push denial is not a negotiated failure: %v\n%s", err, output.String())
	}
	if failure.Failure.Context == nil || failure.Failure.Context.ScopeChange == nil {
		t.Fatalf("pre-push denial carries no scope-change diagnostics: %#v\n%s", failure.Failure, output.String())
	}
	return *failure.Failure.Context.ScopeChange
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

// TestScopeChangedDenialRequiresOnlyWhatTheRecoveryActuallyDemands covers the
// Flow 15 report: the human denial listed `reason` and `actor` under
// "requires:" for a shape where `review recover` self-mints both. A required
// input the command does not demand is a false instruction, and an operator
// who believes it goes looking for values that do not exist.
//
// The negotiated `recovery_required_inputs` is deliberately NOT the place to
// scope this: contracts/review-integration/v1/schemas/failure.schema.json pins
// it to exactly six constants, so the wire contract keeps naming the complete
// operation input set. The gate-conditional refinement belongs in the human
// message, exactly like the committed base-diff selectors already there.
func TestScopeChangedDenialRequiresOnlyWhatTheRecoveryActuallyDemands(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	_, store := approveDiscoveryMarkdown(t, repo, "requires-root", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed docs delivery")
	runReviewCLIGit(t, repo, "push", "-q", "origin", branch)
	if err := os.WriteFile(filepath.Join(repo, "docs", "unreviewed.md"), []byte("unreviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "unreviewed change")

	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateApproved {
		t.Fatalf("predecessor state = %q, want the self-deriving approved shape", record.State.State)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"validate", "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/" + branch,
	}, &output)
	var denied ReviewGateDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("pre-push denial = %v\n%s", err, output.String())
	}
	message := denied.Error()
	requires, ok := strings.CutPrefix(message[strings.Index(message, "(requires: "):], "(requires: ")
	if !ok {
		t.Fatalf("denial names no requires list: %q", message)
	}
	listed := strings.Split(strings.TrimSuffix(requires, ")"), ", ")

	// Proven by the command itself below: these four are demanded, and the
	// other two are self-minted from the approved predecessor state.
	want := []string{"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition"}
	if !reflect.DeepEqual(listed, want) {
		t.Fatalf("denial requires: %v, want %v\nfull message: %q", listed, want, message)
	}

	// The wire contract is pinned to six by the published v1 schema and must
	// keep naming all six, unscoped.
	if scope := denied.Context.ScopeChange; scope == nil || !reflect.DeepEqual(scope.RecoveryRequiredInputs, []string{
		"predecessor_lineage_id", "expected_predecessor_revision", "successor_lineage_id", "disposition", "reason", "actor",
	}) {
		t.Fatalf("negotiated recovery_required_inputs changed: %#v", denied.Context.ScopeChange)
	}

	// The claim is falsifiable: run the recovery with exactly the listed
	// inputs and nothing else.
	if err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", denied.Context.ScopeChange.PredecessorLineageID,
		"--expected-predecessor-revision", denied.Context.ScopeChange.PredecessorRevision,
		"--successor-lineage", "requires-root-r1", "--disposition", "scope_changed",
		"--committed-only", "--base-ref", "origin/" + branch,
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("recovery refused the inputs the denial listed: %v", err)
	}
}
