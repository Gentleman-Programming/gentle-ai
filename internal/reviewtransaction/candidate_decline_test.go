package reviewtransaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateDeclineAuthorizationIsReplayableAndFailsClosed(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "declined candidate\n")

	ctx := context.Background()
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(ctx, Target{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatalf("build candidate: %v", err)
	}

	first, err := RecordCandidateDecline(ctx, repo, snapshot)
	if err != nil {
		t.Fatalf("record candidate decline: %v", err)
	}
	second, err := RecordCandidateDecline(ctx, repo, snapshot)
	if err != nil {
		t.Fatalf("replay candidate decline: %v", err)
	}
	if first.AuthorizationRef != second.AuthorizationRef {
		t.Fatalf("decline replay changed authorization:\nfirst=%#v\nsecond=%#v", first, second)
	}

	gitSnapshot(t, repo, "add", "tracked.txt")
	resolved, found, err := ResolveCandidateDeclineForGate(ctx, repo, NativeGateRequestInput{Gate: GatePreCommit})
	if err != nil || !found || resolved.Snapshot.Identity != snapshot.Identity {
		t.Fatalf("exact staged delivery decline = %#v, found=%v, err=%v", resolved, found, err)
	}

	root, _, err := candidateDeclineRoot(ctx, repo, false)
	if err != nil {
		t.Fatalf("candidate decline root: %v", err)
	}
	path := filepath.Join(root, candidateDeclineRecordName(snapshot.Identity))
	if err := os.WriteFile(path, []byte("{\n"), 0o600); err != nil {
		t.Fatalf("corrupt candidate decline: %v", err)
	}
	if _, _, err := ResolveCandidateDeclineForGate(ctx, repo, NativeGateRequestInput{Gate: GatePreCommit}); !errors.Is(err, ErrCandidateDeclineCorrupt) {
		t.Fatalf("corrupt candidate decline error = %v, want ErrCandidateDeclineCorrupt", err)
	}
}

func TestCandidateDeclineAmbiguityFailsClosed(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "declined candidate\n")

	ctx := context.Background()
	builder := SnapshotBuilder{Repo: repo}
	workspace, err := builder.Build(ctx, Target{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatalf("build workspace candidate: %v", err)
	}
	if _, err := RecordCandidateDecline(ctx, repo, workspace); err != nil {
		t.Fatalf("record workspace decline: %v", err)
	}
	gitSnapshot(t, repo, "add", "tracked.txt")
	staged, err := builder.Build(ctx, Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatalf("build staged candidate: %v", err)
	}
	if _, err := RecordCandidateDecline(ctx, repo, staged); err != nil {
		t.Fatalf("record staged decline: %v", err)
	}

	if _, _, err := ResolveCandidateDeclineForGate(ctx, repo, NativeGateRequestInput{Gate: GatePreCommit}); !errors.Is(err, ErrCandidateDeclineAmbiguous) {
		t.Fatalf("ambiguous candidate declines error = %v, want ErrCandidateDeclineAmbiguous", err)
	}
}
