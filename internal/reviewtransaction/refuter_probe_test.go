package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMaterializeRefuterProbeTreeWritesTheExactCandidateTree pins the S11
// probe copy: the refuter's scratch directory holds the frozen candidate tree
// byte for byte -- candidate edits, deletions, intended untracked files, the
// executable bit, and symlinks -- and nothing from the live workspace, not even
// its .git directory.
func TestMaterializeRefuterProbeTreeWritesTheExactCandidateTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes and symlinks")
	}
	repo := initSnapshotRepo(t)
	gitSnapshot(t, repo, "rm", "-q", "--", "deleted.txt")
	if err := os.Symlink("tracked.txt", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "--", "link")
	gitSnapshot(t, repo, "commit", "-q", "-m", "link")
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	if err := os.MkdirAll(filepath.Join(repo, "nested", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "nested", "dir", "run.sh"), []byte("#!/bin/sh\necho probe\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{"nested/dir/run.sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// The live workspace drifts after START; the probe must see the frozen tree.
	writeSnapshotFile(t, repo, "tracked.txt", "drifted after start\n")

	dir := t.TempDir()
	if err := MaterializeRefuterProbeTree(context.Background(), repo, snapshot.CandidateTree, dir); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "tracked.txt")); err != nil || string(got) != "candidate\n" {
		t.Fatalf("tracked.txt = %q, %v; want the frozen candidate bytes", got, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "deleted.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted.txt survived in the probe copy: %v", err)
	}
	script, err := os.Stat(filepath.Join(dir, "nested", "dir", "run.sh"))
	if err != nil || script.Mode().Perm()&0o100 == 0 {
		t.Fatalf("run.sh = %v, %v; want the executable candidate file", script, err)
	}
	if target, err := os.Readlink(filepath.Join(dir, "link")); err != nil || target != "tracked.txt" {
		t.Fatalf("link = %q, %v; want the candidate symlink", target, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("probe copy carries repository metadata: %v", err)
	}
}

func TestMaterializeRefuterProbeTreeRefusesAnUnknownTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	err := MaterializeRefuterProbeTree(context.Background(), repo, strings.Repeat("0", 40), t.TempDir())
	if err == nil {
		t.Fatal("materializing a missing tree succeeded")
	}
}
