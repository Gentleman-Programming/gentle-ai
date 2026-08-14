package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompactPrePushMonotonicReviewedSubset(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(t *testing.T, repo string)
		wantAllow  bool
		wantReason string
	}{
		{name: "allows approved prefix and outgoing subset", wantAllow: true},
		{name: "rejects unreviewed path", wantReason: `publication path "unreviewed.txt" is outside immutable receipt scope`, mutate: func(t *testing.T, repo string) {
			commitMonotonicSubsetFile(t, repo, "unreviewed.txt", "unreviewed\n", "unreviewed path")
			if err := os.Remove(filepath.Join(repo, "unreviewed.txt")); err != nil {
				t.Fatal(err)
			}
			gitSnapshot(t, repo, "add", "-A")
			gitSnapshot(t, repo, "commit", "-m", "revert unreviewed path")
		}},
		{name: "rejects third blob", wantReason: `publication entry at "b.txt" is outside reviewed base/final endpoints`, mutate: func(t *testing.T, repo string) {
			commitMonotonicSubsetFile(t, repo, "b.txt", "third blob\n", "third blob")
			commitMonotonicSubsetFile(t, repo, "b.txt", "approved B\n", "restore approved B")
		}},
		{name: "rejects executable mode drift", wantReason: `publication entry at "b.txt" is outside reviewed base/final endpoints`, mutate: func(t *testing.T, repo string) {
			gitSnapshot(t, repo, "update-index", "--chmod=+x", "--", "b.txt")
			gitSnapshot(t, repo, "commit", "-m", "executable drift")
			gitSnapshot(t, repo, "update-index", "--chmod=-x", "--", "b.txt")
			gitSnapshot(t, repo, "commit", "-m", "restore approved mode")
		}},
		{name: "rejects final to base reversal", wantReason: `publication edge reverses or extends reviewed entry at "a.txt"`, mutate: func(t *testing.T, repo string) {
			commitMonotonicSubsetFile(t, repo, "a.txt", "base A\n", "reverse approved A")
			commitMonotonicSubsetFile(t, repo, "a.txt", "approved A\n", "restore approved A")
		}},
		{name: "rejects changed then reverted intermediate commit", wantReason: `publication entry at "a.txt" is outside reviewed base/final endpoints`, mutate: func(t *testing.T, repo string) {
			commitMonotonicSubsetFile(t, repo, "a.txt", "intermediate A\n", "intermediate A")
			commitMonotonicSubsetFile(t, repo, "a.txt", "approved A\n", "restore approved A")
		}},
		{name: "rejects unreviewed merge resolution", wantReason: `publication entry at "a.txt" is outside reviewed base/final endpoints`, mutate: func(t *testing.T, repo string) {
			branch := currentBranch(context.Background(), repo)
			mainFinal := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
			gitSnapshot(t, repo, "checkout", "-qb", "unreviewed-side", "HEAD~2")
			commitMonotonicSubsetFile(t, repo, "b.txt", "approved B\n", "side approved B endpoint")
			sideEndpoint := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
			gitSnapshot(t, repo, "checkout", "-q", branch)
			writeSnapshotFile(t, repo, "a.txt", "unreviewed merge resolution\n")
			gitSnapshot(t, repo, "add", "a.txt")
			mergeTree := trimGit(gitSnapshot(t, repo, "write-tree"))
			merge := trimGit(gitSnapshot(t, repo, "commit-tree", mergeTree, "-p", mainFinal, "-p", sideEndpoint, "-m", "resolve merge with unreviewed blob"))
			gitSnapshot(t, repo, "update-ref", "HEAD", merge)
			commitMonotonicSubsetFile(t, repo, "a.txt", "approved A\n", "restore approved A")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, state, receipt := approvedCompactMonotonicSubsetFixture(t, "compact-monotonic-"+strings.ReplaceAll(tt.name, " ", "-"))
			if tt.mutate != nil {
				tt.mutate(t, repo)
			}
			input := NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID}
			request, _, err := buildCompactGateRequest(context.Background(), repo, state, input)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, refs, err := buildCompactLifecycleSnapshot(context.Background(), repo, request)
			if err != nil {
				t.Fatal(err)
			}
			proof, err := proveCompactPrePushMonotonicSubset(context.Background(), repo, state, receipt.FinalCandidateTree, receipt.BaseTree, input, snapshot, refs)
			if err != nil || !proof.Applies || proof.Allowed != tt.wantAllow || proof.Reason != tt.wantReason {
				t.Fatalf("monotonic subset proof = %#v, %v", proof, err)
			}
			assessment, err := AssessCompactGateTarget(context.Background(), repo, state, input)
			if err != nil || (tt.wantAllow && assessment.Applicability != CompactGateTargetExact) || (!tt.wantAllow && assessment.Applicability == CompactGateTargetExact) {
				t.Fatalf("monotonic subset assessment = %#v, %v", assessment, err)
			}
			result := EvaluateCompactGate(context.Background(), repo, receipt, input)
			if tt.wantAllow && result.Result != GateAllow {
				t.Fatalf("approved monotonic subset = %#v", result)
			}
			if !tt.wantAllow && result.Result == GateAllow {
				t.Fatalf("unsafe monotonic subset = %#v", result)
			}
		})
	}
}

func TestCompactPrePushMonotonicSubsetRejectsMismatchedDerivedCandidate(t *testing.T) {
	repo, state, receipt := approvedCompactMonotonicSubsetFixture(t, "compact-monotonic-mismatched-candidate")
	state.InitialSnapshot.Kind = TargetBaseWorkspaceOverlay
	input := NativeGateRequestInput{Gate: GatePrePush, LineageID: state.LineageID}
	request, _, err := buildCompactGateRequest(t.Context(), repo, state, input)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, refs, err := buildCompactLifecycleSnapshot(t.Context(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.CandidateTree = receipt.BaseTree
	proof, err := proveCompactPrePushMonotonicSubset(t.Context(), repo, state, receipt.FinalCandidateTree, receipt.BaseTree, input, snapshot, refs)
	if err != nil || !proof.Applies || proof.Allowed || proof.Reason != "derived pre-push candidate does not match the approved final candidate" {
		t.Fatalf("mismatched derived candidate proof = %#v, %v", proof, err)
	}
}

func approvedCompactMonotonicSubsetFixture(t *testing.T, lineage string) (string, CompactState, CompactReceipt) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "a.txt", "base A\n")
	gitSnapshot(t, repo, "add", "a.txt")
	gitSnapshot(t, repo, "commit", "-m", "reviewed base")
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	gitSnapshot(t, repo, "config", "branch."+branch+".remote", "origin")
	gitSnapshot(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	commitMonotonicSubsetFile(t, repo, "a.txt", "approved A\n", "approved A")
	prefix := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	commitMonotonicSubsetFile(t, repo, "b.txt", "approved B\n", "approved B")
	state := newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	state, receipt := persistApprovedCompactState(t, repo, state)
	gitSnapshot(t, repo, "push", "-q", "origin", prefix+":refs/heads/"+branch)
	return repo, state, receipt
}

func commitMonotonicSubsetFile(t *testing.T, repo, path, content, message string) {
	t.Helper()
	writeSnapshotFile(t, repo, path, content)
	gitSnapshot(t, repo, "add", "--", path)
	gitSnapshot(t, repo, "commit", "-m", message)
}
