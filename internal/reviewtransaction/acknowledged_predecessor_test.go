package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

// acknowledgedPredecessorFixture commits one reviewable change on top of the
// snapshot repository's base commit and records that committed base-diff
// candidate as approved and burned, exactly as acknowledgement leaves it: a
// terminal consumption fact keyed by the candidate identity, with no live
// authority directory.
func acknowledgedPredecessorFixture(t *testing.T) (repo, baseCommit string) {
	t.Helper()
	requireSnapshotGit(t)
	repo = initSnapshotRepo(t)
	baseCommit = strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "feature.go", "package feature\n\nfunc Run() {}\n")
	gitSnapshot(t, repo, "add", "feature.go")
	gitSnapshot(t, repo, "commit", "-qm", "feature")
	acknowledged := buildCommittedRange(t, repo, baseCommit)
	authorityBase, root, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	record := CompactRecord{State: CompactState{LineageID: "review-0123456789abcdef", CurrentSnapshot: acknowledged}}
	if err := prepareCompactTerminalConsumption(authorityBase, root, record); err != nil {
		t.Fatal(err)
	}
	if consumed, err := CompactTargetConsumed(context.Background(), repo, acknowledged.Identity); err != nil || !consumed {
		t.Fatalf("fixture candidate consumed = %v, %v", consumed, err)
	}
	return repo, baseCommit
}

func buildCommittedRange(t *testing.T, repo, baseCommit string) Snapshot {
	t.Helper()
	snapshot, err := (SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(context.Background(), Target{Kind: TargetBaseDiff, BaseRef: baseCommit})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func commitSnapshotFile(t *testing.T, repo, name, content string) {
	t.Helper()
	writeSnapshotFile(t, repo, name, content)
	gitSnapshot(t, repo, "add", name)
	gitSnapshot(t, repo, "commit", "-qm", "update "+name)
}

func requirePassivePredecessor(t *testing.T, repo string, live Snapshot, want bool) {
	t.Helper()
	got, err := AcknowledgedPassivePredecessor(context.Background(), repo, live)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("AcknowledgedPassivePredecessor = %v, want %v (paths %v)", got, want, live.Paths)
	}
}

func TestAcknowledgedPassivePredecessorSuppressesPassiveDelta(t *testing.T) {
	repo, baseCommit := acknowledgedPredecessorFixture(t)
	commitSnapshotFile(t, repo, "odd/tasks/feature.md", "# Feature\n\n- PR opened.\n")
	requirePassivePredecessor(t, repo, buildCommittedRange(t, repo, baseCommit), true)
	// Several passive commits still trace back to the same acknowledgement.
	commitSnapshotFile(t, repo, "docs/feature.md", "# Feature\n\nUsage notes.\n")
	requirePassivePredecessor(t, repo, buildCommittedRange(t, repo, baseCommit), true)
}

func TestAcknowledgedPassivePredecessorOffersReviewForNonPassiveDelta(t *testing.T) {
	for name, file := range map[string]struct{ path, content string }{
		"code":          {"feature.go", "package feature\n\nfunc Run() { panic(1) }\n"},
		"test":          {"feature_test.go", "package feature\n\nimport \"testing\"\n\nfunc TestRun(t *testing.T) { Run() }\n"},
		"configuration": {"package.json", "{\"name\": \"feature\"}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			repo, baseCommit := acknowledgedPredecessorFixture(t)
			commitSnapshotFile(t, repo, "odd/tasks/feature.md", "# Feature\n")
			commitSnapshotFile(t, repo, file.path, file.content)
			requirePassivePredecessor(t, repo, buildCommittedRange(t, repo, baseCommit), false)
		})
	}
}

func TestAcknowledgedPassivePredecessorRequiresAnAcknowledgedAncestor(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	baseCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	commitSnapshotFile(t, repo, "feature.go", "package feature\n")
	commitSnapshotFile(t, repo, "odd/tasks/feature.md", "# Feature\n")
	requirePassivePredecessor(t, repo, buildCommittedRange(t, repo, baseCommit), false)
}

func TestAcknowledgedPassivePredecessorIgnoresAnotherBase(t *testing.T) {
	repo, baseCommit := acknowledgedPredecessorFixture(t)
	featureCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	commitSnapshotFile(t, repo, "odd/tasks/feature.md", "# Feature\n")
	// The acknowledged identity is bound to its own base tree: the same
	// passive commit measured from another base has no acknowledged ancestor.
	requirePassivePredecessor(t, repo, buildCommittedRange(t, repo, featureCommit), false)
	// A rewritten history drops the acknowledged tree from the ancestry.
	gitSnapshot(t, repo, "reset", "-q", "--hard", baseCommit)
	commitSnapshotFile(t, repo, "feature.go", "package feature\n\nfunc Run() { _ = 1 }\n")
	commitSnapshotFile(t, repo, "odd/tasks/feature.md", "# Feature\n")
	requirePassivePredecessor(t, repo, buildCommittedRange(t, repo, baseCommit), false)
}

func TestAcknowledgedPassivePredecessorIgnoresOtherTargetKinds(t *testing.T) {
	repo, _ := acknowledgedPredecessorFixture(t)
	commitSnapshotFile(t, repo, "odd/tasks/feature.md", "# Feature\n")
	writeSnapshotFile(t, repo, "tracked.txt", "dirty\n")
	live, err := (SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	requirePassivePredecessor(t, repo, live, false)
}
