package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// recoveryCollectBindingFixture is the #3099/#2910 shape: an escalated
// lineage frozen with two selected untracked paths, a changed candidate, and a
// negotiated STATUS that selected the exclude scope. It returns the repository,
// the exclude selectors STATUS used, and the binding a maintainer assembles
// from the recovery_authorization_required collect alone.
func recoveryCollectBindingFixture(t *testing.T, lineage string) (string, []string, map[string]string, string) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"notes-a.txt", "notes-b.txt"} {
		writeUndeclaredWorkspaceFile(t, repo, name, "untracked but explicitly selected\n", 0o644)
	}
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	selected, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{"notes-a.txt", "notes-b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage,
		"--target", selected.Identity, "--projection", string(reviewtransaction.ProjectionWorkspace),
		"--untracked-scope=select", "--expected-untracked-inventory=" + digest,
		"--intended-untracked", "notes-a.txt", "--intended-untracked", "notes-b.txt",
	}, &output); err != nil {
		t.Fatal(negotiatedReviewStartFailure(err, output.String()))
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	escalateReviewForRecovery(t, repo, ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, SelectedLenses: started.SelectedLenses,
	})
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selectors := []string{"--untracked-scope=exclude", "--expected-untracked-inventory=" + digest}
	output.Reset()
	if err := RunReview(append([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--lineage", lineage, "--next-transition",
	}, selectors...), &output); err != nil {
		t.Fatalf("STATUS: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "recovery_authorization_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		t.Fatalf("STATUS transition = %#v", status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	bound, err := reviewTransitionArgumentMap(input.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	return repo, selectors, bound, strings.Join([]string{
		input.Schema,
		"predecessor_lineage=" + bound["lineage"],
		"predecessor_revision=" + bound["expected-revision"],
		"target_identity=" + bound["target"],
		"actor=maintainer",
		"reason=corrected candidate after escalation",
	}, "\n")
}

func recoverWithCollectBinding(repo string, bound map[string]string, successor, authorization string, selectors ...string) ([]byte, error) {
	var output bytes.Buffer
	err := RunReview(append([]string{
		"recover", "--cwd", repo,
		"--predecessor-lineage", bound["lineage"], "--expected-predecessor-revision", bound["expected-revision"],
		"--successor-lineage", successor, "--disposition", bound["disposition"],
		"--actor", "maintainer", "--reason", "corrected candidate after escalation", "--maintainer-authorization", authorization,
	}, selectors...), &output)
	return output.Bytes(), err
}

// TestRecoveryCollectBindingRunsRecoverWithTheSelectorsStatusUsed pins the
// converging half: the binding assembled from the collect is exactly the one
// `review recover` accepts once recover derives the same successor target.
func TestRecoveryCollectBindingRunsRecoverWithTheSelectorsStatusUsed(t *testing.T) {
	reviewEnabledHome(t)
	repo, selectors, bound, authorization := recoveryCollectBindingFixture(t, "collect-binding-converges")
	payload, err := recoverWithCollectBinding(repo, bound, "collect-bound-successor", authorization, selectors...)
	if err != nil {
		t.Fatalf("recover refused the binding assembled from its own collect: %v\n%s", err, payload)
	}
	var result ReviewRecoverResult
	decodeStrictReviewJSON(t, payload, &result)
	if result.LineageID != "collect-bound-successor" || result.TargetIdentity != bound["target"] || result.State != reviewtransaction.StateReviewing {
		t.Fatalf("recover = %#v, want a reviewing successor over %s", result, bound["target"])
	}
}

// TestRecoveryCollectBindingRefusalNamesARunnableRecover is the reported
// half: the same binding handed to a recover that was not given the selectors
// derives a different successor target and is refused. The refusal must name a
// recover that runs, and running it must create the successor.
func TestRecoveryCollectBindingRefusalNamesARunnableRecover(t *testing.T) {
	reviewEnabledHome(t)
	repo, _, bound, authorization := recoveryCollectBindingFixture(t, "collect-binding-refused")
	payload, err := recoverWithCollectBinding(repo, bound, "collect-refused-successor", authorization)
	if err == nil {
		t.Fatalf("recover accepted a binding over a target it did not derive:\n%s", payload)
	}
	message := err.Error()
	_, continuation, named := strings.Cut(message, "re-run: ")
	if !named || !strings.HasPrefix(continuation, "gentle-ai review recover ") || !strings.Contains(message, "key=value") {
		t.Fatalf("refusal names no runnable recover: %s", message)
	}
	if strings.Contains(message, repo) {
		t.Fatalf("refusal published the repository path: %s", message)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	fields := strings.Fields(continuation)
	var recovered bytes.Buffer
	if err := RunReview(fields[2:], &recovered); err != nil {
		t.Fatalf("printed continuation %q refused: %v\n%s", continuation, err, recovered.String())
	}
	var result ReviewRecoverResult
	decodeStrictReviewJSON(t, recovered.Bytes(), &result)
	if result.LineageID != "collect-refused-successor" || result.State != reviewtransaction.StateReviewing ||
		result.Recovery.PredecessorLineageID != bound["lineage"] {
		t.Fatalf("printed continuation = %#v, want a reviewing successor of %s", result, bound["lineage"])
	}
}
