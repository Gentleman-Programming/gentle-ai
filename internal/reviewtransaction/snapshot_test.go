package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	snapshotRepoTemplateOnce sync.Once
	snapshotRepoTemplateDir  string
	snapshotRepoTemplateErr  error
)

func TestMain(m *testing.M) {
	testHome, err := os.MkdirTemp("", "gentle-ai-reviewtransaction-test-home-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", testHome); err != nil {
		panic(err)
	}
	if err := os.Setenv("USERPROFILE", testHome); err != nil {
		panic(err)
	}
	code := m.Run()
	if snapshotRepoTemplateDir != "" {
		_ = os.RemoveAll(snapshotRepoTemplateDir)
	}
	_ = os.RemoveAll(testHome)
	os.Exit(code)
}

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

func TestSnapshotBuilderCurrentChangesPreservesRacyCleanDetection(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	gitSnapshot(t, repo, "config", "core.trustctime", "false")

	path := filepath.Join(repo, "racy.txt")
	fixed := time.Unix(1_700_000_000, 0)
	writeSnapshotFile(t, repo, "racy.txt", "before\n")
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatalf("Chtimes(racy baseline): %v", err)
	}
	gitSnapshot(t, repo, "add", "--", "racy.txt")
	gitSnapshot(t, repo, "commit", "-m", "racy baseline")

	indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo, indexPath)
	}
	if err := os.Chtimes(indexPath, fixed, fixed); err != nil {
		t.Fatalf("Chtimes(real index): %v", err)
	}
	writeSnapshotFile(t, repo, "racy.txt", "after!\n")
	if err := os.Chtimes(path, fixed, fixed); err != nil {
		t.Fatalf("Chtimes(racy correction): %v", err)
	}
	if gitSnapshotSucceeds(repo, "diff", "--quiet", "--", "racy.txt") {
		t.Fatal("real Git index did not detect the deliberately racy-clean correction")
	}

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build(racy correction) error = %v", err)
	}
	if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":racy.txt"); got != "after!\n" {
		t.Fatalf("racy candidate content = %q, want %q", got, "after!\n")
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

// TestSnapshotIdentityIsRepresentationInvariant is the headline proof for
// issue #2659 (root 21 of #2471): a declared intended-untracked path and the
// exact same path staged into the index instead are two REPRESENTATIONS of
// byte-identical candidate content, and the purified identity domain must not
// distinguish them. Before the purification, snapshotIdentityForProjection
// folded IntendedUntracked and its untracked-replay proof into the hash, so
// these two snapshots carried different Identity values despite an identical
// CandidateTree -- the bug this change fixes.
func TestSnapshotIdentityIsRepresentationInvariant(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "declared.txt", "same bytes\n")
	builder := SnapshotBuilder{Repo: repo}

	declared, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"declared.txt"}})
	if err != nil {
		t.Fatalf("Build(declared untracked) error = %v", err)
	}

	gitSnapshot(t, repo, "add", "--", "declared.txt")
	staged, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatalf("Build(staged instead) error = %v", err)
	}

	if declared.CandidateTree != staged.CandidateTree {
		t.Fatalf("fixture is not content-identical: declared candidate=%s staged candidate=%s", declared.CandidateTree, staged.CandidateTree)
	}
	if declared.IntendedUntrackedProof == staged.IntendedUntrackedProof && !equalStrings(declared.IntendedUntracked, staged.IntendedUntracked) {
		t.Fatalf("fixture did not actually change representation: declared=%#v staged=%#v", declared.IntendedUntracked, staged.IntendedUntracked)
	}
	if declared.Identity != staged.Identity {
		t.Fatalf("purified identity is not representation-invariant: declared=%#v staged=%#v", declared, staged)
	}
}

// TestLegacyFrozenSnapshotIdentityFailsValidationClosed is the maintainer-
// directed inverse of the migration question for #2659: there is no dual
// recognition of the retired pre-purification identity domain. A snapshot
// whose Identity was minted under the OLD hash (kind/projection tag folding
// IntendedUntracked and its proof into the hash) fails ValidateEvidence and
// ValidateLiveSnapshot closed against the purified recomputation -- it does
// not silently validate, and it does not panic or surface a cryptic
// corruption/store error. It fails with the exact same "does not match Git
// tree evidence" / "no longer matches" shape any other stale target produces,
// which is the shape internal/cli already classifies as the actionable
// reviewPreflightStaleTargetReason refusal (see
// internal/cli/review_start_evidence_test.go, which proves that refusal shape
// is reachable and actionable) -- so an operator who replays an old hint
// lands on "re-derive the exact next transition", i.e. run review again, not
// on a raw internal error.
func TestLegacyFrozenSnapshotIdentityFailsValidationClosed(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "changed\n")
	builder := SnapshotBuilder{Repo: repo}

	snapshot, err := builder.Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := builder.ValidateEvidence(context.Background(), snapshot); err != nil {
		t.Fatalf("freshly minted snapshot must validate: %v", err)
	}

	legacy := snapshot
	legacy.Identity = legacyPreCutSnapshotIdentityForTest(snapshot.Kind, snapshot.Projection, snapshot.BaseTree, snapshot.CandidateTree,
		snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	if legacy.Identity == snapshot.Identity {
		t.Fatal("legacy identity fixture did not actually differ from the purified identity")
	}

	err = builder.ValidateEvidence(context.Background(), legacy)
	if err == nil {
		t.Fatal("legacy-frozen identity validated against the purified recomputation")
	}
	if !strings.Contains(err.Error(), "match Git tree evidence") {
		t.Fatalf("legacy-frozen identity failure is not the actionable stale-target shape: %v", err)
	}

	if err := builder.ValidateLiveSnapshot(context.Background(), legacy); err == nil {
		t.Fatal("ValidateLiveSnapshot accepted a legacy-frozen identity")
	} else if !strings.Contains(err.Error(), "match Git tree evidence") {
		t.Fatalf("ValidateLiveSnapshot legacy-frozen failure is not the actionable stale-target shape: %v", err)
	}
}

// legacyPreCutSnapshotIdentityForTest reconstructs, byte for byte, the
// identity domain snapshotIdentityForProjection used before the #2659
// purification: it folds proof and intended into the hash under the original
// v1/v2/base-workspace-overlay-v1 tags. Production code deletes this domain
// outright (maintainer decision: outdated identities are unusable, not
// dual-recognized) so this exists ONLY here, as a fixture generator for
// TestLegacyFrozenSnapshotIdentityFailsValidationClosed. It must stay a
// byte-for-byte match of the retired formula or the test proves nothing.
func legacyPreCutSnapshotIdentityForTest(kind TargetKind, projection Projection, baseTree, candidateTree, pathsDigest, proof string, intended, ledgerIDs []string) string {
	hash := sha256.New()
	if kind == TargetBaseWorkspaceOverlay {
		hash.Write([]byte("gentle-ai.review-snapshot/base-workspace-overlay/v1\x00"))
	} else if projection == ProjectionStaged {
		hash.Write([]byte("gentle-ai.review-snapshot/v2\x00"))
	} else {
		hash.Write([]byte("gentle-ai.review-snapshot/v1\x00"))
	}
	values := []string{string(kind), baseTree, candidateTree, pathsDigest, proof}
	if projection == ProjectionStaged {
		values = []string{string(kind), string(projection), baseTree, candidateTree, pathsDigest, proof}
	}
	for _, value := range values {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range intended {
		writeLengthPrefixed(hash, []byte(value))
	}
	for _, value := range ledgerIDs {
		writeLengthPrefixed(hash, []byte(value))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// TestSnapshotIdentityKindDomainSeparationRegression guards the one invariant
// the #2659 purification explicitly keeps: kind and projection stay in the
// identity hash domain. Identical baseTree/candidateTree/pathsDigest/ledgerIDs
// under TargetCurrentChanges vs TargetBaseWorkspaceOverlay must still mint
// different identities, or a current-changes receipt could be recognized as a
// base-workspace-overlay review of identical bytes.
func TestSnapshotIdentityKindDomainSeparationRegression(t *testing.T) {
	baseTree := strings.Repeat("a", 40)
	candidateTree := strings.Repeat("b", 40)
	pathsDigest := digestPaths([]string{"shared.txt"})

	currentChanges := snapshotIdentityForProjection(TargetCurrentChanges, "", baseTree, candidateTree, pathsDigest, "", nil, nil)
	overlay := snapshotIdentityForProjection(TargetBaseWorkspaceOverlay, "", baseTree, candidateTree, pathsDigest, "", nil, nil)
	if currentChanges == overlay {
		t.Fatalf("current-changes and base-workspace-overlay identities collided for identical content: %s", currentChanges)
	}

	workspace := snapshotIdentityForProjection(TargetCurrentChanges, ProjectionWorkspace, baseTree, candidateTree, pathsDigest, "", nil, nil)
	staged := snapshotIdentityForProjection(TargetCurrentChanges, ProjectionStaged, baseTree, candidateTree, pathsDigest, "", nil, nil)
	if workspace == staged {
		t.Fatalf("workspace and staged projection identities collided for identical content: %s", workspace)
	}
}

func TestBuildStagedWorkspaceOverlayRecoveryUsesOnlyTheRealIndex(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "reviewed.txt", "reviewed\n")
	gitSnapshot(t, repo, "add", "reviewed.txt")
	writeSnapshotFile(t, repo, "tracked.txt", "unstaged\n")
	writeSnapshotFile(t, repo, "untracked.txt", "untracked\n")
	beforeIndex := gitSnapshot(t, repo, "diff", "--cached", "--binary")
	target := Target{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged,
		BaseRef: base, IntendedUntracked: []string{},
	}
	builder := SnapshotBuilder{Repo: repo}

	if _, err := builder.Build(context.Background(), target); err == nil {
		t.Fatal("ordinary snapshot builder accepted a staged workspace overlay")
	}
	snapshot, err := builder.BuildStagedWorkspaceOverlayRecovery(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CandidateTree != strings.TrimSpace(gitSnapshot(t, repo, "write-tree")) ||
		!equalStrings(snapshot.Paths, []string{"reviewed.txt"}) ||
		len(snapshot.IntendedUntracked) != 0 || len(snapshot.LedgerIDs) != 0 {
		t.Fatalf("staged recovery snapshot = %#v", snapshot)
	}
	if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":reviewed.txt"); got != "reviewed\n" {
		t.Fatalf("staged recovery snapshot reviewed.txt = %q, want authorized staged bytes", got)
	}
	if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":tracked.txt"); got != "base\n" {
		t.Fatalf("staged recovery snapshot tracked.txt = %q, want base index bytes", got)
	}
	if gitSnapshotSucceeds(repo, "cat-file", "-e", snapshot.CandidateTree+":untracked.txt") {
		t.Fatal("staged recovery snapshot included unrelated untracked.txt")
	}

	writeSnapshotFile(t, repo, "tracked.txt", "more unstaged\n")
	writeSnapshotFile(t, repo, "untracked.txt", "more untracked\n")
	rebuilt, err := builder.BuildStagedWorkspaceOverlayRecovery(context.Background(), target)
	if err != nil {
		t.Fatalf("rebuild staged recovery snapshot: %v", err)
	}
	if rebuilt.CandidateTree != snapshot.CandidateTree || rebuilt.Identity != snapshot.Identity {
		t.Fatalf("staged recovery identity changed with only worktree bytes: before=%#v after=%#v", snapshot, rebuilt)
	}
	if afterIndex := gitSnapshot(t, repo, "diff", "--cached", "--binary"); afterIndex != beforeIndex {
		t.Fatal("staged recovery snapshot mutated the real index")
	}
	for _, invalid := range []Target{
		{Kind: TargetBaseDiff, Projection: ProjectionStaged, BaseRef: base, IntendedUntracked: []string{}},
		{Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionWorkspace, BaseRef: base, IntendedUntracked: []string{}},
		{Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged, BaseRef: base, IntendedUntracked: []string{"untracked.txt"}},
		{Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged, BaseRef: base, IntendedUntracked: []string{}, LedgerIDs: []string{"R3-001"}},
	} {
		if _, err := builder.BuildStagedWorkspaceOverlayRecovery(context.Background(), invalid); err == nil {
			t.Fatalf("invalid staged recovery target accepted: %#v", invalid)
		}
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

// TestSnapshotBuilderCurrentChangesStagesLargeIntendedUntrackedWithoutExceedingArgv
// pins the contract behind issue 1778: a large intended-untracked set must
// reach `git add` without expanding one ":(literal)<path>" pathspec per file
// into argv, because Windows caps a process command line at 32767
// characters. 600 paths of 65 literal-pathspec characters each produce 39000
// characters before separators, comfortably over that limit, while keeping
// the hosted-Windows Git fixture bounded.
func TestSnapshotBuilderCurrentChangesStagesLargeIntendedUntrackedWithoutExceedingArgv(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)

	const count = 600
	intended := make([]string, count)
	for index := 0; index < count; index++ {
		name := fmt.Sprintf("bulk/windows-command-line-headroom-regression-%05d.txt", index)
		writeSnapshotFile(t, repo, name, fmt.Sprintf("bulk-%05d\n", index))
		intended[index] = name
	}

	var captured []string
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "--pathspec-from-file=-") {
			captured = append([]string(nil), args...)
		}
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: intended,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if captured == nil {
		t.Fatal("git add was never invoked with --pathspec-from-file=-; pathspecs were not transported over stdin")
	}
	argvLength := 0
	for _, arg := range captured {
		argvLength += len(arg)
		if strings.Contains(arg, "bulk/") {
			t.Fatalf("git add argv still contains a literal pathspec %q; pathspecs must travel over stdin", arg)
		}
	}
	if !slicesContain(captured, "add") || !slicesContain(captured, "--pathspec-from-file=-") || !slicesContain(captured, "--pathspec-file-nul") {
		t.Fatalf("git add argv = %v, want add/--pathspec-from-file=-/--pathspec-file-nul", captured)
	}
	if argvLength >= 32767 {
		t.Fatalf("git add argv length = %d, want well under the Windows 32767-character command-line limit", argvLength)
	}

	if !reflect.DeepEqual(snapshot.Paths, intended) {
		t.Fatalf("Paths length = %d, want %d matching entries", len(snapshot.Paths), len(intended))
	}
	for _, index := range []int{0, count / 2, count - 1} {
		path := intended[index]
		want := fmt.Sprintf("bulk-%05d\n", index)
		if got := gitSnapshot(t, repo, "show", snapshot.CandidateTree+":"+path); got != want {
			t.Fatalf("candidate %s = %q, want %q", path, got, want)
		}
	}
}

// TestSnapshotBuilderBaseDiffIntendedUntrackedTreeUnchangedByPathspecTransport
// is the regression half of the fix: changing how pathspecs reach `git add`
// (argv vs. stdin) must not change the resulting candidate tree. It exercises
// buildHeadWithIntended (the base-diff call site) and compares against a
// manually built tree using the pre-fix argv-based approach.
func TestSnapshotBuilderBaseDiffIntendedUntrackedTreeUnchangedByPathspecTransport(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "intended.txt", "intended\n")

	expectedTree := buildHeadIntendedCandidateTreeOldStyle(t, repo, []string{"intended.txt"})

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetBaseDiff, BaseRef: "HEAD", IntendedUntracked: []string{"intended.txt"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if snapshot.CandidateTree != expectedTree {
		t.Fatalf("CandidateTree = %q, want manually built %q", snapshot.CandidateTree, expectedTree)
	}
}

// buildHeadIntendedCandidateTreeOldStyle mirrors buildHeadWithIntended's
// pre-fix argv-based `git add -- :(literal)<path>...` invocation, so the
// stdin-transport fix can be proven tree-identical to it.
func buildHeadIntendedCandidateTreeOldStyle(t *testing.T, repo string, intended []string) string {
	t.Helper()
	tempIndex := filepath.Join(t.TempDir(), "index")
	env := []string{"GIT_INDEX_FILE=" + tempIndex}
	if _, err := runGit(context.Background(), repo, env, nil, "read-tree", "HEAD"); err != nil {
		t.Fatalf("read-tree HEAD: %v", err)
	}
	args := append([]string{"add", "--"}, literalPathspecs(intended)...)
	if _, err := runGit(context.Background(), repo, env, nil, args...); err != nil {
		t.Fatalf("add intended paths (old style): %v", err)
	}
	tree, err := runGit(context.Background(), repo, env, nil, "write-tree")
	if err != nil {
		t.Fatalf("write-tree: %v", err)
	}
	return strings.TrimSpace(string(tree))
}

// TestAddIntendedPathspecsEmptySetIsNoop proves the len(intended) == 0 guard
// still short-circuits: an empty-set call must never invoke Git at all.
func TestAddIntendedPathspecsEmptySetIsNoop(t *testing.T) {
	invoked := false
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		invoked = true
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	if err := addIntendedPathspecs(context.Background(), "unused-repo", nil, nil); err != nil {
		t.Fatalf("addIntendedPathspecs(empty) error = %v", err)
	}
	if invoked {
		t.Fatal("addIntendedPathspecs invoked git for an empty intended-untracked set")
	}
}

// TestNulJoinedPathspecsEmitsNulDelimitedLiteralPathspecs is a focused unit
// test on the new helper: no existing seam observes runGit's stdin argument
// directly (gitCommandContext only sees argv, and Stdin is assigned after
// the hook returns), so the NUL-joined-literal-pathspec contract is pinned
// here instead.
func TestNulJoinedPathspecsEmitsNulDelimitedLiteralPathspecs(t *testing.T) {
	got := nulJoinedPathspecs([]string{"a.txt", "dir/b.txt"})
	want := []byte(":(literal)a.txt\x00:(literal)dir/b.txt")
	if !bytes.Equal(got, want) {
		t.Fatalf("nulJoinedPathspecs() = %q, want %q", got, want)
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

func TestBaseWorkspaceOverlayBuildsAllUntrackedCandidateFromEmptyIndex(t *testing.T) {
	tests := []struct {
		name         string
		missingIndex bool
		files        map[string]string
		intended     []string
	}{
		{
			name: "existing empty index",
			files: map[string]string{
				"first.txt":         "first\n",
				"nested/second.txt": "second\n",
			},
			intended: []string{"first.txt", "nested/second.txt"},
		},
		{
			name:         "missing index",
			missingIndex: true,
			files:        map[string]string{"nested/only.txt": "only\n"},
			intended:     []string{"nested/only.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			gitSnapshot(t, repo, "init")
			gitSnapshot(t, repo, "config", "user.email", "test@example.com")
			gitSnapshot(t, repo, "config", "user.name", "Test")
			gitSnapshot(t, repo, "commit", "--allow-empty", "-m", "empty base")
			base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
			gitSnapshot(t, repo, "read-tree", "--empty")

			indexPath := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
			if !filepath.IsAbs(indexPath) {
				indexPath = filepath.Join(repo, indexPath)
			}
			if tt.missingIndex {
				if err := os.Remove(indexPath); err != nil {
					t.Fatalf("Remove(index): %v", err)
				}
			}

			for name, content := range tt.files {
				writeSnapshotFile(t, repo, name, content)
			}
			writeSnapshotFile(t, repo, "unrelated.txt", "outside scope\n")

			beforeIndex, beforeIndexErr := os.ReadFile(indexPath)
			if beforeIndexErr != nil && !errors.Is(beforeIndexErr, os.ErrNotExist) {
				t.Fatalf("ReadFile(index): %v", beforeIndexErr)
			}
			beforeStatus := gitSnapshot(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			expectedTree := buildEmptyIndexCandidateTree(t, repo, tt.intended)

			snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
				Kind: TargetBaseWorkspaceOverlay, BaseRef: base, IntendedUntracked: tt.intended,
			})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if snapshot.CandidateTree != expectedTree {
				t.Fatalf("CandidateTree = %q, want manually built %q", snapshot.CandidateTree, expectedTree)
			}
			if !reflect.DeepEqual(snapshot.Paths, tt.intended) {
				t.Fatalf("Paths = %v, want only intended paths %v", snapshot.Paths, tt.intended)
			}
			if afterIndex, err := os.ReadFile(indexPath); errors.Is(err, os.ErrNotExist) != errors.Is(beforeIndexErr, os.ErrNotExist) || (err != nil && !errors.Is(err, os.ErrNotExist)) || !bytes.Equal(afterIndex, beforeIndex) {
				t.Fatalf("real index changed: before error=%v bytes=%x, after error=%v bytes=%x", beforeIndexErr, beforeIndex, err, afterIndex)
			}
			if afterStatus := gitSnapshot(t, repo, "status", "--porcelain=v1", "-z", "--untracked-files=all"); afterStatus != beforeStatus {
				t.Fatalf("worktree status changed: before=%q after=%q", beforeStatus, afterStatus)
			}
			for name, want := range tt.files {
				content, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(name)))
				if err != nil || string(content) != want {
					t.Fatalf("worktree file %q changed: content=%q error=%v", name, content, err)
				}
			}
			if content, err := os.ReadFile(filepath.Join(repo, "unrelated.txt")); err != nil || string(content) != "outside scope\n" {
				t.Fatalf("unrelated worktree file changed: content=%q error=%v", content, err)
			}
		})
	}
}

func buildEmptyIndexCandidateTree(t *testing.T, repo string, intended []string) string {
	t.Helper()
	tempIndex := filepath.Join(t.TempDir(), "index")
	env := []string{"GIT_INDEX_FILE=" + tempIndex}
	if _, err := runGit(context.Background(), repo, env, nil, "read-tree", "--empty"); err != nil {
		t.Fatalf("read empty temporary index: %v", err)
	}
	args := append([]string{"add", "--"}, literalPathspecs(intended)...)
	if _, err := runGit(context.Background(), repo, env, nil, args...); err != nil {
		t.Fatalf("add intended paths to temporary index: %v", err)
	}
	tree, err := runGit(context.Background(), repo, env, nil, "write-tree")
	if err != nil {
		t.Fatalf("write temporary candidate tree: %v", err)
	}
	return strings.TrimSpace(string(tree))
}

func TestBaseWorkspaceOverlayPropagatesTemporaryIndexInventoryErrors(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "new.txt", "intended\n")
	realIndex := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(realIndex) {
		realIndex = filepath.Join(repo, realIndex)
	}
	t.Setenv("GENTLE_AI_TEST_REAL_INDEX", realIndex)

	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "ls-files") && slicesContain(args, "--cached") && slicesContain(args, "-z") && !slicesContain(args, "--") {
			return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEmptyIndexInventoryHelperProcess$")
		}
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	tests := []struct {
		name     string
		mode     string
		wantText string
		wantExit int
	}{
		{name: "diagnostic", mode: "diagnostic", wantText: "git inventory produced diagnostics: inventory diagnostic"},
		{name: "command failure", mode: "failure", wantText: "inventory failure", wantExit: 23},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GENTLE_AI_EMPTY_INDEX_INVENTORY_HELPER", tt.mode)
			_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
				Kind: TargetBaseWorkspaceOverlay, BaseRef: "HEAD", IntendedUntracked: []string{"new.txt"},
			})
			if tt.wantExit == 0 {
				if err == nil || err.Error() != tt.wantText {
					t.Fatalf("Build() error = %T %v, want exact diagnostic %q", err, err, tt.wantText)
				}
				return
			}
			commandErr, ok := err.(*GitCommandError)
			if !ok || commandErr.ExitCode != tt.wantExit || commandErr.Output != tt.wantText {
				t.Fatalf("Build() error = %#v, want direct GitCommandError exit=%d output=%q", err, tt.wantExit, tt.wantText)
			}
		})
	}
}

func TestEmptyIndexInventoryHelperProcess(t *testing.T) {
	mode := os.Getenv("GENTLE_AI_EMPTY_INDEX_INVENTORY_HELPER")
	if mode == "" {
		return
	}
	index := os.Getenv("GIT_INDEX_FILE")
	if index == "" || filepath.Clean(index) == filepath.Clean(os.Getenv("GENTLE_AI_TEST_REAL_INDEX")) {
		_, _ = fmt.Fprint(os.Stderr, "temporary index environment missing")
		os.Exit(72)
	}
	if mode == "diagnostic" {
		_, _ = fmt.Fprint(os.Stderr, "inventory diagnostic")
		os.Exit(0)
	}
	_, _ = fmt.Fprint(os.Stderr, "inventory failure")
	os.Exit(23)
}

// TestResolveCurrentChangesBaseUnbornHeadSelectorFree proves 1771: a
// selector-free (workspace-projection) status call on an unborn HEAD must
// resolve to the repository-native empty tree, exactly as the staged
// projection path already does, instead of surfacing the raw Git command
// failure from `git rev-parse HEAD`.
func TestResolveCurrentChangesBaseUnbornHeadSelectorFree(t *testing.T) {
	requireSnapshotGit(t)
	repo := initUnbornSnapshotRepo(t)
	builder := SnapshotBuilder{Repo: repo}

	baseTree, unborn, err := builder.resolveCurrentChangesBase(context.Background(), ProjectionWorkspace)
	if err != nil {
		var commandErr *GitCommandError
		if errors.As(err, &commandErr) {
			t.Fatalf("resolveCurrentChangesBase(workspace, unborn HEAD) surfaced a raw git failure: %v", err)
		}
		t.Fatalf("resolveCurrentChangesBase(workspace, unborn HEAD) error = %v, want empty-tree resolution", err)
	}
	if !unborn {
		t.Fatal("resolveCurrentChangesBase(workspace, unborn HEAD) unborn = false, want true")
	}
	if want := gitSnapshotEmptyTree(t, repo); baseTree != want {
		t.Fatalf("baseTree = %q, want repository-native empty tree %q", baseTree, want)
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

// TestSnapshotBuilderCurrentChangesSupportsUnbornHeadWorkspaceProjection
// proves 1771 at the full Build() level: a selector-free (workspace
// projection) current-changes target on an unborn HEAD resolves to the
// empty-tree base and the staged candidate, exactly like the existing staged
// projection path, instead of failing with a raw Git command error.
func TestSnapshotBuilderCurrentChangesSupportsUnbornHeadWorkspaceProjection(t *testing.T) {
	requireSnapshotGit(t)
	repo := initUnbornSnapshotRepo(t)
	writeSnapshotFile(t, repo, "candidate.txt", "reviewed\n")
	gitSnapshot(t, repo, "add", "--", "candidate.txt")
	expectedCandidate := strings.TrimSpace(gitSnapshot(t, repo, "write-tree"))

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build(unborn workspace) error = %v", err)
	}
	if !snapshot.UnbornHead {
		t.Fatal("Build(unborn workspace) UnbornHead = false, want true")
	}
	if want := gitSnapshotEmptyTree(t, repo); snapshot.BaseTree != want {
		t.Fatalf("BaseTree = %q, want repository-native empty tree %q", snapshot.BaseTree, want)
	}
	if snapshot.CandidateTree != expectedCandidate {
		t.Fatalf("CandidateTree = %q, want staged index tree %q", snapshot.CandidateTree, expectedCandidate)
	}
}

func TestSnapshotBuilderRealGitFailuresAreNotTreatedAsUnborn(t *testing.T) {
	requireSnapshotGit(t)
	stagedTarget := Target{Kind: TargetCurrentChanges, Projection: ProjectionStaged, IntendedUntracked: []string{}}
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

func TestSnapshotRepoTemplateContracts(t *testing.T) {
	requireSnapshotGit(t)
	first := initSnapshotRepo(t)
	second := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, first, "rev-parse", "HEAD"))

	for _, repo := range []string{first, second} {
		if status := gitSnapshot(t, repo, "status", "--porcelain=v1"); status != "" {
			t.Fatalf("initial status = %q, want clean", status)
		}
		if got := strings.TrimSpace(gitSnapshot(t, repo, "config", "user.email")); got != "snapshot@example.com" {
			t.Fatalf("user.email = %q", got)
		}
		if got := strings.TrimSpace(gitSnapshot(t, repo, "config", "user.name")); got != "Snapshot Test" {
			t.Fatalf("user.name = %q", got)
		}
		if got := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD")); got != base {
			t.Fatalf("base commit = %q, want %q", got, base)
		}
		gitDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--absolute-git-dir"))
		commonDir := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
		if !filepath.IsAbs(commonDir) {
			t.Fatalf("common dir is not absolute: %q", commonDir)
		}
		if gitDir != commonDir {
			t.Fatalf("git dir %q and common dir %q indicate a linked worktree", gitDir, commonDir)
		}
		for _, path := range []string{
			filepath.Join(gitDir, "objects", "info", "alternates"),
			filepath.Join(gitDir, "worktrees"),
		} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected shared Git state %q: %v", path, err)
			}
		}
		if gitSnapshotSucceeds(repo, "config", "--get", "remote.origin.url") {
			t.Fatal("fixture unexpectedly has an origin remote")
		}
	}

	firstCommon := strings.TrimSpace(gitSnapshot(t, first, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	secondCommon := strings.TrimSpace(gitSnapshot(t, second, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	if firstCommon == secondCommon {
		t.Fatalf("common dir is shared: %q", firstCommon)
	}

	writeSnapshotFile(t, first, "tracked.txt", "isolated\n")
	gitSnapshot(t, first, "add", "--", "tracked.txt")
	if status := gitSnapshot(t, second, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("second index changed with first fixture: %q", status)
	}
	gitSnapshot(t, first, "config", "fixture.isolated", "true")
	gitSnapshot(t, first, "commit", "-m", "isolate fixture mutation")
	if got, err := os.ReadFile(filepath.Join(second, "tracked.txt")); err != nil || string(got) != "base\n" {
		t.Fatalf("second worktree content = %q, err=%v", got, err)
	}
	if status := gitSnapshot(t, second, "status", "--porcelain=v1"); status != "" {
		t.Fatalf("second worktree status = %q", status)
	}
	if got := strings.TrimSpace(gitSnapshot(t, second, "rev-parse", "HEAD")); got != base {
		t.Fatalf("second ref = %q, want %q", got, base)
	}
	if gitSnapshotSucceeds(second, "config", "--get", "fixture.isolated") {
		t.Fatal("second config changed with first fixture")
	}
}

func TestSnapshotRepoTemplateInitializesOnceConcurrently(t *testing.T) {
	requireSnapshotGit(t)
	const callers = 16
	paths := make(chan string, callers)
	errs := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			path, err := snapshotRepoTemplate()
			paths <- path
			errs <- err
		}()
	}
	group.Wait()
	close(paths)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("snapshotRepoTemplate() error = %v", err)
		}
	}
	var template string
	for path := range paths {
		if template == "" {
			template = path
		} else if path != template {
			t.Fatalf("template paths = %q and %q", template, path)
		}
	}
	if template == "" {
		t.Fatal("snapshotRepoTemplate() returned an empty path")
	}
}

func TestUntrackedProofBatchedListingMatchesPerPathReference(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	intended := []string{"bulk/a.txt", "deep/nested/b.txt"}
	for index := range 48 {
		intended = append(intended, fmt.Sprintf("bulk/reference-%02d.txt", index))
	}
	for _, logicalPath := range intended {
		writeSnapshotFile(t, repo, logicalPath, "reference content for "+logicalPath+"\n")
	}

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	hash.Write([]byte("gentle-ai.intended-untracked/v1\x00"))
	for _, logicalPath := range snapshot.IntendedUntracked {
		entry, err := runGit(context.Background(), repo, nil, nil, "ls-tree", "-z", snapshot.CandidateTree, "--", literalPathspec(logicalPath))
		if err != nil || len(entry) == 0 {
			t.Fatalf("per-path reference for %q = %q, %v", logicalPath, entry, err)
		}
		writeLengthPrefixed(hash, []byte(logicalPath))
		writeLengthPrefixed(hash, entry)
	}
	want := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if snapshot.IntendedUntrackedProof != want {
		t.Fatalf("batched proof = %s, want per-path reference %s", snapshot.IntendedUntrackedProof, want)
	}
}

func TestSnapshotIntendedQueriesUseConstantGitInvocations(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	intended := make([]string, 128)
	for index := range intended {
		intended[index] = fmt.Sprintf("bulk/file-%03d.txt", index)
		writeSnapshotFile(t, repo, intended[index], "candidate\n")
	}

	counts := map[string]int{}
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if len(args) > 3 {
			counts[args[3]]++
		}
		return originalCommand(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: intended})
	if err != nil {
		t.Fatal(err)
	}
	if counts["ls-tree"] != 1 || counts["check-ignore"] != 1 || counts["ls-files"] != 2 {
		t.Fatalf("128-path Build git queries = ls-tree:%d check-ignore:%d ls-files:%d, want 1/1/2", counts["ls-tree"], counts["check-ignore"], counts["ls-files"])
	}
	counts = map[string]int{}
	if err := (SnapshotBuilder{Repo: repo}).ValidateEvidence(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if counts["ls-tree"] != 1 {
		t.Fatalf("128-path ValidateEvidence ls-tree invocations = %d, want 1", counts["ls-tree"])
	}
}

func TestNulSeparatedGitParsersPreserveUnusualPaths(t *testing.T) {
	oid := strings.Repeat("a", 40)
	paths := []string{"space name.txt", "line\nbreak.txt", "tab\tname.txt", "--leading-dash.txt"}
	var treeOutput, pathOutput []byte
	for _, logicalPath := range paths {
		record := []byte("100644 blob " + oid + "\t" + logicalPath)
		treeOutput = append(treeOutput, record...)
		treeOutput = append(treeOutput, 0)
		pathOutput = append(pathOutput, logicalPath...)
		pathOutput = append(pathOutput, 0)
	}
	entries, err := parseTreeEntries(treeOutput)
	if err != nil {
		t.Fatal(err)
	}
	pathSet := nulSeparatedPathSet(pathOutput)
	for _, logicalPath := range paths {
		want := []byte("100644 blob " + oid + "\t" + logicalPath + "\x00")
		if !bytes.Equal(entries[logicalPath], want) {
			t.Fatalf("tree entry for %q = %q, want %q", logicalPath, entries[logicalPath], want)
		}
		if _, present := pathSet[logicalPath]; !present {
			t.Fatalf("NUL path set lost %q", logicalPath)
		}
	}
	if _, err := parseTreeEntries([]byte("malformed\x00")); err == nil {
		t.Fatal("malformed ls-tree record was accepted")
	}
	unterminated := []byte("100644 blob " + oid + "\tunterminated.txt")
	if _, err := parseTreeEntries(unterminated); err == nil {
		t.Fatal("unterminated ls-tree record was accepted")
	}
}

func TestBatchGitProtocolReadersRejectDiagnostics(t *testing.T) {
	t.Setenv("GENTLE_AI_TEST_GIT_PROTOCOL_DIAGNOSTIC", "1")
	originalCommand := gitCommandContext
	gitCommandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBatchGitProtocolDiagnosticHelperProcess$")
	}
	t.Cleanup(func() { gitCommandContext = originalCommand })

	repo := t.TempDir()
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "ls-tree", run: func() error {
			_, err := listTreeEntries(context.Background(), repo, "HEAD")
			return err
		}},
		{name: "cat-file-batch", run: func() error {
			_, err := batchBlobContents(context.Background(), repo, []string{strings.Repeat("a", 40)}, defaultGitOutputLimit)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "git inventory produced diagnostics: protocol diagnostic") {
				t.Fatalf("protocol reader error = %v, want explicit stderr diagnostic", err)
			}
		})
	}
}

func TestBatchGitProtocolDiagnosticHelperProcess(t *testing.T) {
	if os.Getenv("GENTLE_AI_TEST_GIT_PROTOCOL_DIAGNOSTIC") != "1" {
		return
	}
	_, _ = fmt.Fprint(os.Stderr, "protocol diagnostic")
}

func TestSnapshotBuilderRejectsFirstIgnoredIntendedPathInCanonicalOrder(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, ".gitignore", "ignored/\n")
	writeSnapshotFile(t, repo, "ignored/a.txt", "a\n")
	writeSnapshotFile(t, repo, "ignored/b.txt", "b\n")
	_, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{"ignored/b.txt", "ignored/a.txt"},
	})
	if err == nil || !strings.Contains(err.Error(), `intended-untracked path "ignored/a.txt" is ignored`) {
		t.Fatalf("ignored intended error = %v, want first canonical path", err)
	}
}

func initSnapshotRepo(t *testing.T) string {
	t.Helper()
	template, err := snapshotRepoTemplate()
	if err != nil {
		t.Fatalf("snapshot repo template: %v", err)
	}
	// canonicalTempDir, not t.TempDir: every production resolver answers with
	// the resolved spelling, so a fixture that keeps the raw one compares
	// unequal to its own repository. On a Windows runner TEMP is reached
	// through an 8.3 short name and the raw path says RUNNER~1 where the
	// resolver correctly says runneradmin; on Darwin the same gap appears as
	// /var versus /private/var.
	repo := canonicalTempDir(t)
	if err := os.CopyFS(repo, os.DirFS(template)); err != nil {
		t.Fatalf("CopyFS(snapshot repo template): %v", err)
	}
	return repo
}

func snapshotRepoTemplate() (string, error) {
	snapshotRepoTemplateOnce.Do(func() {
		template, err := os.MkdirTemp("", "gentle-ai-snapshot-repo-*")
		if err != nil {
			snapshotRepoTemplateErr = fmt.Errorf("create template directory: %w", err)
			return
		}
		for _, args := range [][]string{{"init"}, {"config", "user.email", "snapshot@example.com"}, {"config", "user.name", "Snapshot Test"}, {"config", "core.autocrlf", "false"}} {
			if snapshotRepoTemplateErr = runSnapshotGit(template, args...); snapshotRepoTemplateErr != nil {
				_ = os.RemoveAll(template)
				return
			}
		}
		for name, content := range map[string]string{"tracked.txt": "base\n", "deleted.txt": "delete me\n"} {
			if err := os.WriteFile(filepath.Join(template, name), []byte(content), 0o644); err != nil {
				snapshotRepoTemplateErr = fmt.Errorf("write %s fixture: %w", name, err)
				_ = os.RemoveAll(template)
				return
			}
		}
		for _, args := range [][]string{{"add", "--", "tracked.txt", "deleted.txt"}, {"commit", "-m", "base"}} {
			if snapshotRepoTemplateErr = runSnapshotGit(template, args...); snapshotRepoTemplateErr != nil {
				_ = os.RemoveAll(template)
				return
			}
		}
		snapshotRepoTemplateDir = template
	})
	return snapshotRepoTemplateDir, snapshotRepoTemplateErr
}

func runSnapshotGit(repo string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, output)
	}
	return nil
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
