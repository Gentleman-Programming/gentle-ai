package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestCandidateContentFactsReadImmutableBlob proves the facts come from the
// frozen candidate tree rather than the live worktree: the path is rewritten
// with active content after the snapshot is taken, and the classification must
// still describe the bytes that were frozen.
func TestCandidateContentFactsReadImmutableBlob(t *testing.T) {
	repo := initSnapshotRepo(t)
	logicalPath, content := "docs/guide.md", []byte("# Guide\n")
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, logicalPath), content, 0o644); err != nil {
		t.Fatal(err)
	}
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{logicalPath}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, logicalPath), []byte("<script>live()</script>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	facts, err := builder.CandidateContentFacts(context.Background(), snapshot, nil, nil)
	if err != nil || !reflect.DeepEqual(facts, []CandidateContentFact{{Path: logicalPath, Activity: VerificationContentPassive}}) {
		t.Fatalf("CandidateContentFacts() = %#v, %v", facts, err)
	}
	blob, err := ReadTreeBlob(context.Background(), repo, snapshot.CandidateTree, logicalPath, 1024)
	if err != nil || !reflect.DeepEqual(blob, content) {
		t.Fatalf("ReadTreeBlob() = %q, %v", blob, err)
	}
}

// TestCandidateContentFactsHonourCallerOperationalPaths proves a caller can
// declare a path its own domain treats as operational. The same passive bytes
// classify active once the caller names the path, which is how a consumer
// contributes policy the shared classifier does not own.
func TestCandidateContentFactsHonourCallerOperationalPaths(t *testing.T) {
	repo := initSnapshotRepo(t)
	logicalPath := "docs/guide.md"
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, logicalPath), []byte("# Guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{logicalPath}})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := builder.CandidateContentFacts(context.Background(), snapshot, []string{logicalPath}, nil)
	if err != nil || !reflect.DeepEqual(facts, []CandidateContentFact{{Path: logicalPath, Activity: VerificationContentActive}}) {
		t.Fatalf("CandidateContentFacts() with operational path = %#v, %v", facts, err)
	}
}

// TestCandidateContentFactsRejectUnchangedExclusion proves a stale exclusion
// fails instead of silently narrowing the inspected set.
func TestCandidateContentFactsRejectUnchangedExclusion(t *testing.T) {
	repo := initSnapshotRepo(t)
	logicalPath := "docs/guide.md"
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, logicalPath), []byte("# Guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{logicalPath}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.CandidateContentFacts(context.Background(), snapshot, nil, []string{"docs/absent.md"}); err == nil {
		t.Fatal("CandidateContentFacts() accepted an exclusion that is not part of the candidate")
	}
}
