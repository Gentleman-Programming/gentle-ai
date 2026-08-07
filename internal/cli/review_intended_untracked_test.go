package cli

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func intendedStatus(t *testing.T, repo string, args ...string) ReviewTargetStatusResult {
	var output bytes.Buffer
	if err := RunReviewStatus(append([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, args...), &output); err != nil {
		t.Fatalf("review status %v: %v", args, err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}
func intendedSelection(t *testing.T, status ReviewTargetStatusResult) (string, string) {
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].Name != "intended_untracked_selection" {
		t.Fatalf("untracked STATUS transition = %#v", status.NextTransition)
	}
	arguments, _ := reviewTransitionArgumentMap(status.NextTransition.Collect.Inputs[0].Arguments)
	return arguments["expected_untracked_inventory"], arguments["eligible_paths_json"]
}
func TestReviewStatusCollectsAndStartsUntrackedOnlyCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "new.go", "package candidate\n", 0o644)
	digest, _ := intendedSelection(t, intendedStatus(t, repo, "--base-ref=HEAD"))
	selection := []string{"--untracked-scope=select", "--intended-untracked=new.go", "--expected-untracked-inventory=" + digest}
	selected := intendedStatus(t, repo, append([]string{"--base-ref=HEAD"}, selection...)...)
	if directErr := RunReviewFacadeStart(append([]string{"--cwd", repo, "--base-ref=HEAD", "--committed-only"}, selection...), &bytes.Buffer{}); directErr == nil || !strings.Contains(directErr.Error(), "--untracked-scope=select") || !strings.Contains(directErr.Error(), "--intended-untracked=new.go") || !strings.Contains(directErr.Error(), "--expected-untracked-inventory="+digest) {
		t.Fatalf("direct-to-negotiated continuation lost intended-untracked scope: %v", directErr)
	}
	question := decodeConsentQuestion(t, runConsentRelayStart(t, transitionStartArgs(repo, selected)).Bytes())
	for _, choice := range question.Choices {
		if !strings.Contains(choice.Invocation, "--untracked-scope=select") || !strings.Contains(choice.Invocation, "--intended-untracked=new.go") || !strings.Contains(choice.Invocation, "--expected-untracked-inventory="+digest) {
			t.Fatalf("consent continuation lost intended-untracked scope: %q", choice.Invocation)
		}
	}
	if started := decodeNegotiatedReviewStart(t, runConsentRelayStart(t, invocationArgs(t, question.Choices[0].Invocation)).Bytes()); negotiatedStartTarget(started) != selected.TargetIdentity || !reflect.DeepEqual(selected.Projection.Paths, []string{"new.go"}) || !strings.Contains(selected.NextTransition.Execute.Command, "--intended-untracked=new.go") {
		t.Fatalf("consent continuation target = %s, want %s", negotiatedStartTarget(started), selected.TargetIdentity)
	}
}
func TestIntendedUntrackedSelectionScopesMixedCandidate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, ".gitignore", "ignored.txt\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "ignore fixture")
	writeReviewStartCandidate(t, repo, "tracked.txt", "tracked change\n", 0o644)
	for path, contents := range map[string]string{"chosen.md": "chosen\n", "scratch.txt": "noise\n", unrelatedCredentialPath: unrelatedCredentialContents, "ignored.txt": "ignored\n"} {
		writeUndeclaredWorkspaceFile(t, repo, path, contents, 0o600)
	}
	digest, inventory := intendedSelection(t, intendedStatus(t, repo))
	if inventory != `["chosen.md","scratch.txt","`+unrelatedCredentialPath+`"]` {
		t.Fatalf("eligible inventory = %v", inventory)
	}
	if paths := intendedStatus(t, repo, "--untracked-scope=exclude", "--expected-untracked-inventory="+digest).Projection.Paths; !reflect.DeepEqual(paths, []string{"tracked.txt"}) {
		t.Fatalf("excluded projection paths = %v", paths)
	}
	selected := intendedStatus(t, repo, "--untracked-scope=select", "--intended-untracked="+unrelatedCredentialPath,
		"--intended-untracked=chosen.md", "--expected-untracked-inventory="+digest)
	if !reflect.DeepEqual(selected.Projection.Paths, []string{"chosen.md", "tracked.txt", unrelatedCredentialPath}) || !reflect.DeepEqual(selected.Projection.IntendedUntracked, []string{"chosen.md", unrelatedCredentialPath}) || strings.Count(selected.NextTransition.Execute.Command, "--intended-untracked=") != 2 {
		t.Fatalf("selected mixed projection = %#v", selected.Projection)
	}
}
func TestIntendedUntrackedPreflightFailsClosed(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.md", "candidate\n", 0o644)
	digest, _ := intendedSelection(t, intendedStatus(t, repo))
	for _, args := range [][]string{
		{"--lineage", "missing-intent"},
		{"--lineage", "duplicate-path", "--untracked-scope=select", "--intended-untracked=candidate.md", "--intended-untracked=candidate.md", "--expected-untracked-inventory=" + digest},
		{"--lineage", "noncanonical-path", "--untracked-scope=select", "--intended-untracked=./candidate.md", "--expected-untracked-inventory=" + digest},
		{"--lineage", "staged-flags", "--projection=staged", "--untracked-scope=exclude", "--expected-untracked-inventory=" + digest},
	} {
		if err := RunReviewFacadeStart(append([]string{"--cwd", repo}, args...), &bytes.Buffer{}); err == nil || args[1] == "missing-intent" && (!strings.Contains(err.Error(), "--untracked-scope=exclude") || !strings.Contains(err.Error(), digest)) {
			t.Fatalf("incomplete selection succeeded: %v", args)
		}
	}
	writeUndeclaredWorkspaceFile(t, repo, "later.md", "later\n", 0o644)
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "stale-inventory", "--untracked-scope=exclude", "--expected-untracked-inventory=" + digest}, &bytes.Buffer{})
	if leaves, leavesErr := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo); err == nil || !strings.Contains(err.Error(), "inventory changed") || leavesErr != nil || len(leaves) != 0 {
		t.Fatalf("stale inventory error/authority = %v, %v, %v", err, leaves, leavesErr)
	}
}
func TestIntendedUntrackedRevalidatesAtAuthorityBoundary(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeUndeclaredWorkspaceFile(t, repo, "candidate.md", "candidate\n", 0o644)
	digest, _ := intendedSelection(t, intendedStatus(t, repo))
	original := reviewFacadeBuildStartSnapshot
	reviewFacadeBuildStartSnapshot = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, target reviewtransaction.Target) (reviewtransaction.Snapshot, error) {
		snapshot, err := original(ctx, builder, target)
		if err == nil {
			writeUndeclaredWorkspaceFile(t, repo, "late.md", "late\n", 0o644)
		}
		return snapshot, err
	}
	t.Cleanup(func() { reviewFacadeBuildStartSnapshot = original })
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "boundary-inventory", "--untracked-scope=select", "--intended-untracked=candidate.md", "--expected-untracked-inventory=" + digest}, &bytes.Buffer{})
	if leaves, leavesErr := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo); err == nil || leavesErr != nil || len(leaves) != 0 {
		t.Fatalf("boundary mutation error/authority = %v, %v, %v", err, leaves, leavesErr)
	}
}
