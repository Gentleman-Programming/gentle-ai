package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCanonicalPathsRejectsDuplicateInput(t *testing.T) {
	if _, err := canonicalPaths([]string{"tracked.txt", "tracked.txt"}); err == nil {
		t.Fatal("canonicalPaths duplicate input error = nil")
	}
}

func TestSnapshotBuilderCurrentChangesIsCompleteAndPreservesRealIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("uses real git commands")
	}
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged\n")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatalf("Remove(deleted.txt): %v", err)
	}
	writeSnapshotFile(t, repo, "staged.txt", "staged\n")
	gitSnapshot(t, repo, "add", "--", "staged.txt")
	writeSnapshotFile(t, repo, "intended.txt", "intended\n")
	writeSnapshotFile(t, repo, "excluded.txt", "excluded\n")

	indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	beforeCached := gitSnapshot(t, repo, "diff", "--cached", "--binary")
	beforeIndex, err := os.ReadFile(filepath.Join(repo, indexPath))
	if err != nil {
		t.Fatalf("ReadFile(index): %v", err)
	}

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind:              TargetCurrentChanges,
		IntendedUntracked: []string{"intended.txt"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	afterIndex, err := os.ReadFile(filepath.Join(repo, indexPath))
	if err != nil {
		t.Fatalf("ReadFile(index after): %v", err)
	}
	if !reflect.DeepEqual(afterIndex, beforeIndex) {
		t.Fatal("SnapshotBuilder mutated the user's real index")
	}
	if afterCached := gitSnapshot(t, repo, "diff", "--cached", "--binary"); afterCached != beforeCached {
		t.Fatalf("cached diff changed:\nbefore:\n%s\nafter:\n%s", beforeCached, afterCached)
	}

	wantPaths := []string{"deleted.txt", "intended.txt", "staged.txt", "tracked.txt"}
	if !reflect.DeepEqual(snapshot.Paths, wantPaths) {
		t.Fatalf("Paths = %v, want %v", snapshot.Paths, wantPaths)
	}
	for path, want := range map[string]string{
		"tracked.txt":  "unstaged\n",
		"staged.txt":   "staged\n",
		"intended.txt": "intended\n",
	} {
		if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":"+path); got != want {
			t.Fatalf("candidate %s = %q, want %q", path, got, want)
		}
	}
	for _, absent := range []string{"deleted.txt", "excluded.txt"} {
		if gitSnapshotSucceeds(repo, "show", snapshot.CandidateTree+":"+absent) {
			t.Fatalf("candidate unexpectedly contains %s", absent)
		}
	}
	for name, value := range map[string]string{
		"base tree": snapshot.BaseTree, "candidate tree": snapshot.CandidateTree,
		"paths digest": snapshot.PathsDigest, "untracked proof": snapshot.IntendedUntrackedProof,
		"identity": snapshot.Identity,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s is empty", name)
		}
	}

	again, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind:              TargetCurrentChanges,
		IntendedUntracked: []string{"intended.txt"},
	})
	if err != nil {
		t.Fatalf("Build(repeat) error = %v", err)
	}
	if again.Identity != snapshot.Identity || again.CandidateTree != snapshot.CandidateTree {
		t.Fatalf("snapshot is not deterministic: first=%#v second=%#v", snapshot, again)
	}
}

func TestSnapshotBuilderStagedProjectionUsesExactIndexAndPreservesWorkspace(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "staged\n")
	writeSnapshotFile(t, repo, "added.txt", "added\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt", "added.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged\n")
	writeSnapshotFile(t, repo, "untracked.txt", "untracked\n")

	beforeIndex := gitSnapshot(t, repo, "diff", "--cached", "--binary")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build(staged) error = %v", err)
	}
	if snapshot.Projection != ProjectionStaged {
		t.Fatalf("Projection = %q, want %q", snapshot.Projection, ProjectionStaged)
	}
	if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":tracked.txt"); got != "staged\n" {
		t.Fatalf("staged candidate tracked.txt = %q", got)
	}
	if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":added.txt"); got != "added\n" {
		t.Fatalf("staged candidate added.txt = %q", got)
	}
	if gitSnapshotSucceeds(repo, "show", snapshot.CandidateTree+":untracked.txt") {
		t.Fatal("staged candidate included an untracked worktree path")
	}
	if afterIndex := gitSnapshot(t, repo, "diff", "--cached", "--binary"); afterIndex != beforeIndex {
		t.Fatalf("staged snapshot mutated index:\nbefore:\n%s\nafter:\n%s", beforeIndex, afterIndex)
	}
	if got, err := os.ReadFile(filepath.Join(repo, "tracked.txt")); err != nil || string(got) != "unstaged\n" {
		t.Fatalf("staged snapshot mutated worktree: %q, %v", got, err)
	}
	if err := (SnapshotBuilder{Repo: repo}).ValidateEvidence(context.Background(), snapshot); err != nil {
		t.Fatalf("ValidateEvidence(staged) error = %v", err)
	}
}

func TestSnapshotBuilderStagedProjectionPreservesExactIndexFidelity(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "one\ntwo\nthree\nfour\nfive\n")
	writeSnapshotFile(t, repo, "rename-old.txt", "rename me\n")
	writeSnapshotFile(t, repo, "mode.txt", "mode me\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt", "rename-old.txt", "mode.txt")
	gitSnapshot(t, repo, "commit", "-m", "fidelity baseline")

	writeSnapshotFile(t, repo, "tracked.txt", "ONE\ntwo\nthree\nfour\nFIVE\n")
	patch := filepath.Join(t.TempDir(), "partial.patch")
	if err := os.WriteFile(patch, []byte("diff --git a/tracked.txt b/tracked.txt\nindex b2f931a..b3c5a95 100644\n--- a/tracked.txt\n+++ b/tracked.txt\n@@ -1,4 +1,4 @@\n-one\n+ONE\n two\n three\n four\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "apply", "--cached", patch)
	writeSnapshotFile(t, repo, "added.txt", "added\n")
	gitSnapshot(t, repo, "add", "--", "added.txt")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-u", "--", "deleted.txt")
	if err := os.Rename(filepath.Join(repo, "rename-old.txt"), filepath.Join(repo, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A", "--", "rename-old.txt", "renamed.txt")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(repo, "mode.txt"), 0o755); err != nil {
			t.Fatal(err)
		}
		gitSnapshot(t, repo, "add", "--", "mode.txt")
	}
	if err := os.Symlink("tracked.txt", filepath.Join(repo, "link.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "--", "link.txt")
	writeSnapshotFile(t, repo, "untracked.txt", "ignore me\n")

	beforeIndex := gitSnapshot(t, repo, "diff", "--cached", "--binary")
	indexTree := strings.TrimSpace(gitSnapshot(t, repo, "write-tree"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CandidateTree != indexTree {
		t.Fatalf("staged candidate tree = %s, want exact index %s", snapshot.CandidateTree, indexTree)
	}
	if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":tracked.txt"); got != "ONE\ntwo\nthree\nfour\nfive\n" {
		t.Fatalf("partial staged hunk = %q", got)
	}
	if !gitSnapshotSucceeds(repo, "cat-file", "-e", snapshot.CandidateTree+":added.txt") || gitSnapshotSucceeds(repo, "cat-file", "-e", snapshot.CandidateTree+":deleted.txt") || !gitSnapshotSucceeds(repo, "cat-file", "-e", snapshot.CandidateTree+":renamed.txt") || gitSnapshotSucceeds(repo, "cat-file", "-e", snapshot.CandidateTree+":rename-old.txt") {
		t.Fatal("staged candidate does not retain add/delete/rename index entries")
	}
	if runtime.GOOS != "windows" {
		if got := gitSnapshot(t, repo, "ls-tree", snapshot.CandidateTree, "mode.txt"); !strings.HasPrefix(got, "100755 ") {
			t.Fatalf("staged mode entry = %q", got)
		}
	}
	if got := gitSnapshot(t, repo, "ls-tree", snapshot.CandidateTree, "link.txt"); !strings.HasPrefix(got, "120000 ") {
		t.Fatalf("staged symlink entry = %q", got)
	}
	if gitSnapshotSucceeds(repo, "cat-file", "-e", snapshot.CandidateTree+":untracked.txt") {
		t.Fatal("staged candidate included untracked worktree content")
	}
	if afterIndex := gitSnapshot(t, repo, "diff", "--cached", "--binary"); afterIndex != beforeIndex {
		t.Fatal("building staged projection mutated the real index")
	}
}

func TestSnapshotProjectionValidationAndIdentity(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "changed\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	builder := SnapshotBuilder{Repo: repo}

	workspace, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	explicitWorkspace, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(workspace, explicitWorkspace) || workspace.Projection != "" {
		t.Fatalf("explicit workspace changed legacy identity:\nimplicit=%#v\nexplicit=%#v", workspace, explicitWorkspace)
	}
	staged, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if staged.CandidateTree != workspace.CandidateTree || staged.Identity == workspace.Identity {
		t.Fatalf("projection identity was not domain-separated: workspace=%#v staged=%#v", workspace, staged)
	}
	if _, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, Projection: Projection("future"), IntendedUntracked: []string{}}); err == nil || !strings.Contains(err.Error(), "unsupported projection") {
		t.Fatalf("unknown projection error = %v", err)
	}
	stagedBaseDiff, err := builder.Build(context.Background(), Target{Kind: TargetBaseDiff, Projection: ProjectionStaged, BaseRef: "HEAD", IntendedUntracked: []string{}})
	if err != nil || stagedBaseDiff.Projection != ProjectionStaged || stagedBaseDiff.CandidateTree != strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}")) {
		t.Fatalf("staged base-diff snapshot = %#v, err=%v", stagedBaseDiff, err)
	}
	if _, err := builder.Build(context.Background(), Target{Kind: TargetBaseDiff, Projection: ProjectionStaged, BaseRef: "HEAD", IntendedUntracked: []string{"untracked.txt"}}); err == nil || !strings.Contains(err.Error(), "does not accept intended-untracked") {
		t.Fatalf("staged base-diff intended-untracked error = %v", err)
	}
	if _, err := builder.Build(context.Background(), Target{Kind: TargetExactRevision, Projection: ProjectionStaged, Revision: "HEAD", IntendedUntracked: []string{}}); err == nil || !strings.Contains(err.Error(), "only supported") {
		t.Fatalf("staged exact-revision/release error = %v", err)
	}
	if _, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{"untracked.txt"}}); err == nil || !strings.Contains(err.Error(), "does not accept intended-untracked") {
		t.Fatalf("staged intended-untracked error = %v", err)
	}
}

func TestPreCommitSnapshotAllowsOnlyCompleteStagedIntendedTransition(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	intended := []string{"first.txt", "second.txt"}
	for _, path := range intended {
		writeSnapshotFile(t, repo, path, "reviewed "+path+"\n")
	}
	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: intended}
	builder := SnapshotBuilder{Repo: repo}
	want, err := builder.Build(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}

	gitDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--absolute-git-dir"))
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte("first.txt\nsecond.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-f", "--", "first.txt")
	if _, err := builder.build(context.Background(), target, true); err == nil || !strings.Contains(err.Error(), "all untracked or all staged") {
		t.Fatalf("partial staged transition error = %v", err)
	}

	gitSnapshot(t, repo, "add", "-f", "--", "second.txt")
	got, err := builder.build(context.Background(), target, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("staged transition changed snapshot:\nwant=%#v\ngot=%#v", want, got)
	}
	if _, err := builder.Build(context.Background(), target); err == nil || !strings.Contains(err.Error(), "already tracked") {
		t.Fatalf("ordinary snapshot derivation accepted staged intended paths: %v", err)
	}
}

func TestSnapshotUsesEffectiveGitIgnoresWithoutMutatingOperationalState(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	globalExclude := filepath.Join(t.TempDir(), "global-exclude")
	writeSnapshotFile(t, filepath.Dir(globalExclude), filepath.Base(globalExclude), "global-*\n")
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	gitSnapshot(t, repo, "config", "--global", "core.excludesFile", globalExclude)
	gitDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--absolute-git-dir"))
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte("info-*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, ".gitignore", "nested/*\n!nested/keep.txt\n")
	for _, path := range []string{"nested/tracked.txt", "info-tracked.txt", "global-tracked.txt"} {
		writeSnapshotFile(t, repo, path, "base\n")
	}
	gitSnapshot(t, repo, "add", "-f", ".gitignore", "nested/tracked.txt", "info-tracked.txt", "global-tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "ignore fixtures")

	for _, path := range []string{"nested/tracked.txt", "info-tracked.txt", "global-tracked.txt"} {
		writeSnapshotFile(t, repo, path, "reviewed\n")
	}
	for _, path := range []string{"nested/operational.txt", "info-operational.txt", "global-operational.txt"} {
		writeSnapshotFile(t, repo, path, "private\n")
	}
	writeSnapshotFile(t, repo, "nested/keep.txt", "intended\n")
	indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	beforeIndex, err := os.ReadFile(filepath.Join(repo, indexPath))
	if err != nil {
		t.Fatal(err)
	}

	builder := SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"nested/keep.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"global-tracked.txt", "info-tracked.txt", "nested/keep.txt", "nested/tracked.txt"}
	if !reflect.DeepEqual(snapshot.Paths, want) {
		t.Fatalf("snapshot paths = %v, want %v", snapshot.Paths, want)
	}
	if err := builder.ValidateEvidence(context.Background(), snapshot); err != nil {
		t.Fatalf("ValidateEvidence() error = %v", err)
	}
	afterIndex, err := os.ReadFile(filepath.Join(repo, indexPath))
	if err != nil || !reflect.DeepEqual(afterIndex, beforeIndex) {
		t.Fatalf("real index changed: err=%v", err)
	}
	for _, path := range []string{"nested/operational.txt", "info-operational.txt", "global-operational.txt"} {
		if content, err := os.ReadFile(filepath.Join(repo, path)); err != nil || string(content) != "private\n" {
			t.Fatalf("operational path %q changed: content=%q err=%v", path, content, err)
		}
		objectsBefore := gitSnapshot(t, repo, "count-objects", "-v")
		_, err = builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{path}})
		if err == nil || !strings.Contains(err.Error(), "is ignored") {
			t.Fatalf("ignored intent %q error = %v", path, err)
		}
		if objectsAfter := gitSnapshot(t, repo, "count-objects", "-v"); objectsAfter != objectsBefore {
			t.Fatalf("ignored intent %q created Git objects", path)
		}
	}
}

func TestSnapshotHonorsLinkedWorktreeExcludes(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "worktree-tracked.txt", "base\n")
	gitSnapshot(t, repo, "add", "worktree-tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "worktree fixture")
	gitSnapshot(t, repo, "config", "extensions.worktreeConfig", "true")
	linked := filepath.Join(t.TempDir(), "linked")
	gitSnapshot(t, repo, "worktree", "add", "-b", "linked-snapshot", linked, "HEAD")
	excludes := filepath.Join(t.TempDir(), "linked-excludes")
	if err := os.WriteFile(excludes, []byte("worktree-*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, linked, "config", "--worktree", "core.excludesFile", excludes)
	writeSnapshotFile(t, linked, "worktree-tracked.txt", "reviewed\n")
	writeSnapshotFile(t, linked, "worktree-operational.txt", "private\n")
	builder := SnapshotBuilder{Repo: linked}
	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil || !reflect.DeepEqual(snapshot.Paths, []string{"worktree-tracked.txt"}) {
		t.Fatalf("linked snapshot = %#v, err=%v", snapshot, err)
	}
	_, err = builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"worktree-operational.txt"}})
	if err == nil || !strings.Contains(err.Error(), "is ignored") {
		t.Fatalf("worktree-specific ignored intent error = %v", err)
	}
	if staged := gitSnapshot(t, linked, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("linked worktree index changed: %q", staged)
	}
}

func TestSnapshotExactRevisionKeepsHistoricalTrackedIgnoredPath(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, ".gitignore", "historical.txt\n")
	gitSnapshot(t, repo, "add", ".gitignore")
	gitSnapshot(t, repo, "commit", "-m", "ignore historical path")
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "historical.txt", "reviewed\n")
	gitSnapshot(t, repo, "add", "-f", "historical.txt")
	gitSnapshot(t, repo, "commit", "-m", "track ignored path")
	candidate := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "checkout", "--detach", base)

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: base + ".." + candidate})
	if err != nil || !reflect.DeepEqual(snapshot.Paths, []string{"historical.txt"}) {
		t.Fatalf("historical snapshot = %#v, err=%v", snapshot, err)
	}
	if err := (SnapshotBuilder{Repo: repo}).ValidateEvidence(context.Background(), snapshot); err != nil {
		t.Fatalf("ValidateEvidence() error = %v", err)
	}
}

func TestBaseDiffPreservesIntendedAuthorityAfterTrackedTransition(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "delivery.txt", "reviewed\n")
	target := Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{"delivery.txt"}}
	builder := SnapshotBuilder{Repo: repo}
	reviewed, err := builder.Build(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	gitDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--absolute-git-dir"))
	if err := os.WriteFile(filepath.Join(gitDir, "info", "exclude"), []byte("delivery.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-f", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "deliver reviewed path")
	committed, err := builder.Build(context.Background(), target)
	if err != nil || !reflect.DeepEqual(committed, reviewed) {
		t.Fatalf("tracked transition = %#v, err=%v; want %#v", committed, err, reviewed)
	}
	if err := builder.ValidateEvidence(context.Background(), committed); err != nil {
		t.Fatalf("ValidateEvidence() error = %v", err)
	}
	writeSnapshotFile(t, repo, "delivery.txt", "drifted\n")
	gitSnapshot(t, repo, "commit", "-am", "content drift")
	drifted, err := builder.Build(context.Background(), target)
	if err != nil || drifted.CandidateTree == reviewed.CandidateTree || drifted.IntendedUntrackedProof == reviewed.IntendedUntrackedProof {
		t.Fatalf("content drift did not change authority: %#v, err=%v", drifted, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(repo, "delivery.txt"), 0o755); err != nil {
			t.Fatal(err)
		}
		gitSnapshot(t, repo, "add", "delivery.txt")
		gitSnapshot(t, repo, "commit", "-m", "mode drift")
		modeDrifted, err := builder.Build(context.Background(), target)
		if err != nil || modeDrifted.IntendedUntrackedProof == drifted.IntendedUntrackedProof {
			t.Fatalf("mode drift did not change proof: %#v, err=%v", modeDrifted, err)
		}
	}
	gitSnapshot(t, repo, "rm", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "path drift")
	if _, err := builder.Build(context.Background(), target); err == nil {
		t.Fatal("path drift retained intended-untracked authority")
	}
}

func TestSnapshotTempIndexesAreRemovedAfterGitAddErrors(t *testing.T) {
	requireSnapshotGit(t)
	for _, kind := range []TargetKind{TargetCurrentChanges, TargetBaseDiff} {
		t.Run(string(kind), func(t *testing.T) {
			repo := initSnapshotRepo(t)
			tempDir := t.TempDir()
			t.Setenv("TMPDIR", tempDir)
			writeSnapshotFile(t, repo, ".gitattributes", "unsupported.txt filter=snapshotfail\n")
			gitSnapshot(t, repo, "add", ".gitattributes")
			gitSnapshot(t, repo, "commit", "-m", "failing filter fixture")
			gitSnapshot(t, repo, "config", "filter.snapshotfail.clean", "git rev-parse --verify refs/heads/gentle-ai-filter-must-fail")
			gitSnapshot(t, repo, "config", "filter.snapshotfail.required", "true")
			writeSnapshotFile(t, repo, "unsupported.txt", "cannot clean\n")
			target := Target{Kind: kind, IntendedUntracked: []string{"unsupported.txt"}}
			if kind == TargetBaseDiff {
				target.BaseRef = "HEAD"
			}
			if _, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target); err == nil {
				t.Fatal("Build() accepted an unsupported worktree entry")
			}
			matches, err := filepath.Glob(filepath.Join(tempDir, "gentle-ai-review-index-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("temporary indexes remain: %v, err=%v", matches, err)
			}
			if staged := gitSnapshot(t, repo, "diff", "--cached", "--name-only"); staged != "" {
				t.Fatalf("real index changed: %q", staged)
			}
		})
	}
}

func TestSnapshotDiffStatsExcludeGeneratedGoldensOnlyFromAuthoredLines(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	goldenPath := "testdata/golden/rendered.golden"
	if err := os.MkdirAll(filepath.Join(repo, "testdata", "golden"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, goldenPath, strings.Repeat("generated\n", 500))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{goldenPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := (SnapshotBuilder{Repo: repo}).DiffStats(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	changedLines, err := CountChangedLines(stats)
	if err != nil {
		t.Fatal(err)
	}
	if changedLines != 2 || !equalStrings(snapshot.Paths, []string{"testdata/golden/rendered.golden", "tracked.txt"}) {
		t.Fatalf("authored lines/snapshot paths = %d/%v", changedLines, snapshot.Paths)
	}
	risk, originalChangedLines, err := (SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := CorrectionBudget(originalChangedLines)
	if err != nil || risk != RiskMedium || originalChangedLines != 2 || budget != 1 {
		t.Fatalf("repository risk/original/budget = %q/%d/%d, err %v", risk, originalChangedLines, budget, err)
	}
	generated := false
	for _, stat := range stats {
		if stat.Path == goldenPath {
			generated = stat.Generated
		}
	}
	if !generated {
		t.Fatalf("DiffStats() did not recognize generated golden: %#v", stats)
	}
}

func TestSnapshotDiffStatsIncludesCanonicalRawModesForModeOnlyChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git worktree executable-bit transitions are POSIX-only")
	}
	repo := initSnapshotRepo(t)
	gitSnapshot(t, repo, "config", "core.filemode", "true")
	if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := (SnapshotBuilder{Repo: repo}).DiffStats(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want := []DiffStat{{Path: "tracked.txt", OldMode: "100644", NewMode: "100755", ModeOnly: true}}
	if !reflect.DeepEqual(stats, want) {
		t.Fatalf("DiffStats() = %#v, want %#v", stats, want)
	}
	if lines, err := CountChangedLines(stats); err != nil || lines != 0 {
		t.Fatalf("CountChangedLines(mode-only) = %d, %v; want 0, nil", lines, err)
	}
}

func TestSnapshotDiffStatsDistinguishesContentAndModeChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git worktree executable-bit transitions are POSIX-only")
	}
	repo := initSnapshotRepo(t)
	gitSnapshot(t, repo, "config", "core.filemode", "true")
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := (SnapshotBuilder{Repo: repo}).DiffStats(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].OldMode != "100644" || stats[0].NewMode != "100755" || stats[0].ModeOnly || stats[0].Additions != 1 || stats[0].Deletions != 1 {
		t.Fatalf("content-plus-mode DiffStats() = %#v", stats)
	}
}

func TestGeneratedGoldenPathMatchesRepositorySegments(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "root golden", path: "testdata/golden/rendered.golden", want: true},
		{name: "nested golden", path: "internal/testdata/golden/rendered.golden", want: true},
		{name: "lookalike segment", path: "internal/not-testdata/golden/rendered.golden", want: false},
		{name: "non-golden fixture", path: "internal/testdata/golden/rendered.json", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGeneratedGoldenPath(tt.path); got != tt.want {
				t.Fatalf("isGeneratedGoldenPath(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestSnapshotBuilderRequiresExplicitIntendedUntrackedAndLedgerBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("uses real git commands")
	}
	repo := initSnapshotRepo(t)
	builder := SnapshotBuilder{Repo: repo}
	if _, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges}); err == nil {
		t.Fatal("Build() accepted current changes without an explicit intended-untracked list")
	}
	baseTree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
	if _, err := builder.Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: baseTree, IntendedUntracked: []string{}}); err == nil {
		t.Fatal("Build() accepted fix diff without ledger IDs")
	}
	if _, err := builder.Build(context.Background(), Target{
		Kind: TargetFixDiff, BaseRef: baseTree, IntendedUntracked: []string{}, LedgerIDs: []string{"R1-001"},
	}); err != nil {
		t.Fatalf("Build(valid fix diff) error = %v", err)
	}
}

func TestSnapshotBuilderSupportsBaseDiffAndExactCommitRange(t *testing.T) {
	if testing.Short() {
		t.Skip("uses real git commands")
	}
	repo := initSnapshotRepo(t)
	firstCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "second\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "second")
	secondCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	builder := SnapshotBuilder{Repo: repo}
	writeSnapshotFile(t, repo, "notes.txt", "intended untracked\n")
	baseDiff, err := builder.Build(context.Background(), Target{Kind: TargetBaseDiff, BaseRef: firstCommit, IntendedUntracked: []string{"notes.txt"}})
	if err != nil {
		t.Fatalf("Build(base diff) error = %v", err)
	}
	exact, err := builder.Build(context.Background(), Target{Kind: TargetExactRevision, Revision: firstCommit + ".." + secondCommit})
	if err != nil {
		t.Fatalf("Build(exact range) error = %v", err)
	}
	if baseDiff.BaseTree != exact.BaseTree {
		t.Fatalf("base diff and exact range bases disagree: base=%#v exact=%#v", baseDiff, exact)
	}
	if !reflect.DeepEqual(baseDiff.Paths, []string{"notes.txt", "tracked.txt"}) {
		t.Fatalf("base diff paths = %v", baseDiff.Paths)
	}
	if !reflect.DeepEqual(baseDiff.IntendedUntracked, []string{"notes.txt"}) {
		t.Fatalf("base diff intended untracked = %v", baseDiff.IntendedUntracked)
	}
	if err := builder.ValidateEvidence(context.Background(), baseDiff); err != nil {
		t.Fatalf("ValidateEvidence(base diff) error = %v", err)
	}
}

func TestSnapshotBuilderExactRevisionIgnoresReplacementObjects(t *testing.T) {
	if testing.Short() {
		t.Skip("uses real git commands")
	}
	repo := initSnapshotRepo(t)
	firstCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "tracked.txt", "original\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "original")
	originalCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	target := Target{Kind: TargetExactRevision, Revision: originalCommit}
	baseline, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target)
	if err != nil {
		t.Fatalf("Build(baseline) error = %v", err)
	}

	gitSnapshot(t, repo, "checkout", "--detach", firstCommit)
	writeSnapshotFile(t, repo, "tracked.txt", "replacement\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "replacement")
	replacementCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "replace", originalCommit, replacementCommit)

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if snapshot.Identity != baseline.Identity {
		t.Fatalf("Identity = %q, want replacement-independent identity %q", snapshot.Identity, baseline.Identity)
	}
	if snapshot.BaseTree != strings.TrimSpace(gitSnapshot(t, repo, "--no-replace-objects", "rev-parse", firstCommit+"^{tree}")) {
		t.Fatalf("BaseTree = %q, want the original parent tree", snapshot.BaseTree)
	}
	if snapshot.CandidateTree != strings.TrimSpace(gitSnapshot(t, repo, "--no-replace-objects", "rev-parse", originalCommit+"^{tree}")) {
		t.Fatalf("CandidateTree = %q, want the original commit tree", snapshot.CandidateTree)
	}
}

func TestBaseWorkspaceOverlayFreezesFullBoundaryWithoutMutation(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "committed.txt", "committed\n")
	gitSnapshot(t, repo, "add", "committed.txt")
	gitSnapshot(t, repo, "commit", "-m", "branch")
	writeSnapshotFile(t, repo, "tracked.txt", "staged\n")
	gitSnapshot(t, repo, "add", "tracked.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "workspace wins\n")
	writeSnapshotFile(t, repo, "new.txt", "intended\n")

	beforeIndex := strings.TrimSpace(gitSnapshot(t, repo, "write-tree"))
	beforeStatus := gitSnapshot(t, repo, "status", "--porcelain=v1")
	target := Target{Kind: TargetBaseWorkspaceOverlay, BaseRef: base, IntendedUntracked: []string{"new.txt"}}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Paths, []string{"committed.txt", "new.txt", "tracked.txt"}) || gitSnapshot(t, repo, "show", snapshot.CandidateTree+":tracked.txt") != "workspace wins\n" {
		t.Fatalf("overlay snapshot = %#v", snapshot)
	}
	if strings.TrimSpace(gitSnapshot(t, repo, "write-tree")) != beforeIndex || gitSnapshot(t, repo, "status", "--porcelain=v1") != beforeStatus {
		t.Fatal("overlay snapshot mutated the real index or worktree")
	}

	headBase, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetBaseWorkspaceOverlay, BaseRef: "HEAD", IntendedUntracked: []string{"new.txt"}})
	if err != nil || headBase.CandidateTree != snapshot.CandidateTree || headBase.Identity == snapshot.Identity {
		t.Fatalf("base identity binding = %#v, %v", headBase, err)
	}
	writeSnapshotFile(t, repo, "new.txt", "changed bytes\n")
	changed, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), target)
	if err != nil || changed.Identity == snapshot.Identity {
		t.Fatalf("byte identity binding = %#v, %v", changed, err)
	}
}

func TestSnapshotBuilderCurrentChangesSupportsUnbornHeadStagedProjection(t *testing.T) {
	requireSnapshotGit(t)
	repo := initUnbornSnapshotRepo(t)
	writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
	writeSnapshotFile(t, repo, "nested/inner.txt", "inner\n")
	gitSnapshot(t, repo, "add", "--", "candidate.txt", "nested/inner.txt")
	expectedCandidate := strings.TrimSpace(gitSnapshot(t, repo, "write-tree"))
	indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo, indexPath)
	}
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build(unborn staged) error = %v", err)
	}
	if want := gitSnapshotEmptyTree(t, repo); snapshot.BaseTree != want {
		t.Fatalf("BaseTree = %q, want repository-native empty tree %q", snapshot.BaseTree, want)
	}
	if snapshot.CandidateTree != expectedCandidate {
		t.Fatalf("CandidateTree = %q, want staged index tree %q", snapshot.CandidateTree, expectedCandidate)
	}
	if want := []string{"candidate.txt", "nested/inner.txt"}; !reflect.DeepEqual(snapshot.Paths, want) {
		t.Fatalf("Paths = %v, want every staged candidate path %v", snapshot.Paths, want)
	}
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexAfter, indexBefore) {
		t.Fatal("snapshot construction mutated the real index")
	}
}

func TestSnapshotBuilderUnbornHeadWithNothingStagedRefusesActionably(t *testing.T) {
	requireSnapshotGit(t)
	repo := initUnbornSnapshotRepo(t)
	writeSnapshotFile(t, repo, "untracked.txt", "not staged\n")
	indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo, indexPath)
	}
	if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("real index unexpectedly exists before snapshot construction: %v", err)
	}

	_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{},
	})
	if err == nil || !strings.Contains(err.Error(), "git add") {
		t.Fatalf("unborn empty-candidate error = %v, want actionable staging guidance", err)
	}
	var commandErr *GitCommandError
	if errors.As(err, &commandErr) {
		t.Fatalf("unborn empty-candidate refusal surfaced a raw git failure: %v", err)
	}
	if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot construction created the real index: %v", err)
	}
}

func TestSnapshotBuilderRealGitFailuresAreNotTreatedAsUnborn(t *testing.T) {
	requireSnapshotGit(t)
	stagedTarget := Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}}
	t.Run("workspace projection", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
			Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
		})
		var commandErr *GitCommandError
		if err == nil || !errors.As(err, &commandErr) {
			t.Fatalf("unborn workspace error = %v, want the raw git failure", err)
		}
	})
	t.Run("detached HEAD at missing object", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte(strings.Repeat("1", 40)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), stagedTarget)
		var commandErr *GitCommandError
		if err == nil || !errors.As(err, &commandErr) {
			t.Fatalf("detached missing-object error = %v, want the raw git failure", err)
		}
	})
	t.Run("existing branch ref at missing object", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		ref := strings.TrimSpace(gitSnapshot(t, repo, "symbolic-ref", "HEAD"))
		refPath := filepath.Join(repo, ".git", filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(refPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(refPath, []byte(strings.Repeat("2", 40)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), stagedTarget)
		var commandErr *GitCommandError
		if err == nil || !errors.As(err, &commandErr) {
			t.Fatalf("existing-but-unresolvable branch error = %v, want the raw git failure", err)
		}
	})
	t.Run("non-local symbolic ref", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("ref: refs/remotes/origin/main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), stagedTarget)
		var commandErr *GitCommandError
		if err == nil || !errors.As(err, &commandErr) {
			t.Fatalf("non-local symbolic HEAD error = %v, want the raw git failure", err)
		}
	})
	t.Run("malformed HEAD", func(t *testing.T) {
		repo := initUnbornSnapshotRepo(t)
		writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
		gitSnapshot(t, repo, "add", "--", "candidate.txt")
		if err := os.WriteFile(filepath.Join(repo, ".git", "HEAD"), []byte("not a ref\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), stagedTarget)
		var commandErr *GitCommandError
		if err == nil || !errors.As(err, &commandErr) {
			t.Fatalf("malformed HEAD error = %v, want the raw git failure", err)
		}
	})
}

func initSnapshotRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt", "deleted.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	return repo
}

func initUnbornSnapshotRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitSnapshot(t, repo, "init")
	gitSnapshot(t, repo, "config", "user.email", "snapshot@example.com")
	gitSnapshot(t, repo, "config", "user.name", "Snapshot Test")
	return repo
}

func gitSnapshotEmptyTree(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "mktree")
	cmd.Stdin = strings.NewReader("")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git mktree: %v\n%s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func requireSnapshotGit(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("uses real git commands")
	}
}

func writeSnapshotFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func gitSnapshot(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func gitSnapshotSucceeds(repo string, args ...string) bool {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	return cmd.Run() == nil
}
