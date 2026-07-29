package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFrozenCandidateContextUsesImmutableTreesAndCanonicalManifest(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "binary.bin", "base\x00binary\n")
	writeSnapshotFile(t, repo, "docs/naïve guide.md", "unchanged reference target\n")
	writeSnapshotFile(t, repo, "mode.sh", "#!/bin/sh\necho stable\n")
	writeSnapshotFile(t, repo, "type-entry", "regular\n")
	gitSnapshot(t, repo, "add", "--", "binary.bin", "docs/naïve guide.md", "mode.sh", "type-entry")
	gitSnapshot(t, repo, "commit", "-m", "add context fixtures")

	writeSnapshotFile(t, repo, "added.txt", "added\n")
	writeSnapshotFile(t, repo, "tracked.txt", "changed\n")
	if err := os.WriteFile(filepath.Join(repo, "binary.bin"), []byte("changed\x00binary\x01\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "-A", "--")
	gitSnapshot(t, repo, "update-index", "--chmod=+x", "--", "mode.sh")

	symlinkBlob := strings.TrimSpace(gitSnapshotInput(t, repo, []byte("destination.txt"), "hash-object", "-w", "--stdin"))
	gitSnapshot(t, repo, "update-index", "--cacheinfo", "120000,"+symlinkBlob+",type-entry")
	baseCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+baseCommit+",vendor/submodule")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	candidateCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))

	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetExactRevision, Revision: candidateCommit,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	builder := SnapshotBuilder{Repo: repo}
	baseline, err := builder.FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("FrozenCandidateContext() error = %v", err)
	}
	baselineDiff := frozenCandidateDiffBytes(t, baseline.CandidateDiff)

	want := []ChangedPathManifestEntry{
		{Path: "added.txt", Status: CandidatePathAdded, OldMode: "000000", NewMode: "100644"},
		{Path: "binary.bin", Status: CandidatePathModified, OldMode: "100644", NewMode: "100644"},
		{Path: "deleted.txt", Status: CandidatePathDeleted, OldMode: "100644", NewMode: "000000", Deleted: true},
		{Path: "mode.sh", Status: CandidatePathModified, OldMode: "100644", NewMode: "100755", ModeOnly: true},
		{Path: "tracked.txt", Status: CandidatePathModified, OldMode: "100644", NewMode: "100644"},
		{Path: "type-entry", Status: CandidatePathTypeChanged, OldMode: "100644", NewMode: "120000", TypeChanged: true},
		{Path: "vendor/submodule", Status: CandidatePathAdded, OldMode: "000000", NewMode: "160000"},
	}
	if !reflect.DeepEqual(baseline.ChangedPathManifest, want) {
		t.Fatalf("manifest = %#v, want %#v", baseline.ChangedPathManifest, want)
	}
	if !reflect.DeepEqual(manifestPaths(baseline.ChangedPathManifest), snapshot.Paths) {
		t.Fatalf("manifest paths = %v, snapshot paths = %v", manifestPaths(baseline.ChangedPathManifest), snapshot.Paths)
	}
	for _, logicalPath := range []string{"added.txt", "deleted.txt", "docs/naïve guide.md", "tracked.txt"} {
		if stringIndex(baseline.repositoryPaths, logicalPath) < 0 {
			t.Fatalf("frozen repository path manifest omits %q: %v", logicalPath, baseline.repositoryPaths)
		}
	}
	for _, marker := range []string{
		"diff --git a/added.txt b/added.txt",
		"GIT binary patch",
		"deleted file mode 100644",
		"old mode 100644\nnew mode 100755",
		"diff --git a/type-entry b/type-entry\ndeleted file mode 100644",
		"diff --git a/type-entry b/type-entry\nnew file mode 120000",
		"new file mode 160000",
	} {
		if !bytes.Contains(baselineDiff, []byte(marker)) {
			t.Errorf("candidate diff is missing %q:\n%s", marker, baselineDiff)
		}
	}
	if bytes.Contains(baselineDiff, []byte(repo)) {
		t.Fatalf("candidate diff leaked absolute repository path %q", repo)
	}

	for key, value := range map[string]string{
		"color.ui":             "always",
		"diff.algorithm":       "minimal",
		"diff.context":         "0",
		"diff.external":        "false",
		"diff.indentHeuristic": "true",
		"diff.mnemonicPrefix":  "true",
		"diff.noprefix":        "true",
	} {
		gitSnapshot(t, repo, "config", key, value)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "live workspace mutation\n")
	replayed, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("replayed FrozenCandidateContext() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, baseline) {
		t.Fatalf("replayed context changed after hostile config/workspace mutation\nfirst=%#v\nreplayed=%#v", baseline, replayed)
	}
}

func TestFrozenCandidateContextMarksIntendedUntrackedAndSupportsEmptyCandidate(t *testing.T) {
	requireSnapshotGit(t)
	t.Run("intended untracked", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "intended.txt", "reviewed untracked\n")
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
			Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{"intended.txt"},
		})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		contextResult, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
		if err != nil {
			t.Fatalf("FrozenCandidateContext() error = %v", err)
		}
		want := []ChangedPathManifestEntry{{
			Path: "intended.txt", Status: CandidatePathAdded, OldMode: "000000", NewMode: "100644", IntendedUntracked: true,
		}}
		if !reflect.DeepEqual(contextResult.ChangedPathManifest, want) {
			t.Fatalf("manifest = %#v, want %#v", contextResult.ChangedPathManifest, want)
		}
		if diff := frozenCandidateDiffBytes(t, contextResult.CandidateDiff); !bytes.Contains(diff, []byte("+reviewed untracked")) {
			t.Fatalf("candidate diff does not contain intended-untracked bytes:\n%s", diff)
		}
	})

	t.Run("empty candidate", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
			Kind: TargetBaseDiff, Projection: ProjectionWorkspace, BaseRef: "HEAD", IntendedUntracked: []string{},
		})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		contextResult, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
		if err != nil {
			t.Fatalf("FrozenCandidateContext() error = %v", err)
		}
		if diff := frozenCandidateDiffBytes(t, contextResult.CandidateDiff); len(diff) != 0 {
			t.Fatalf("empty candidate diff = %q", diff)
		}
		if contextResult.ChangedPathManifest == nil || len(contextResult.ChangedPathManifest) != 0 {
			t.Fatalf("empty manifest = %#v, want non-nil []", contextResult.ChangedPathManifest)
		}
	})
}

func TestFrozenCandidateContextIsolatesAllGitAttributesConfigAndLocale(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, ".gitattributes", "attribute-target.txt binary\n")
	writeSnapshotFile(t, repo, "attribute-target.txt", "base\n")
	gitSnapshot(t, repo, "add", "--", ".gitattributes", "attribute-target.txt")
	gitSnapshot(t, repo, "commit", "-m", "add committed attributes")

	writeSnapshotFile(t, repo, "attribute-target.txt", "candidate\n")
	gitSnapshot(t, repo, "add", "--", "attribute-target.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	candidateCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetExactRevision, Revision: candidateCommit,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	baseline, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("FrozenCandidateContext() error = %v", err)
	}
	baselineStats, err := (SnapshotBuilder{Repo: repo}).DiffStats(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("DiffStats() error = %v", err)
	}
	baselineBytes := frozenCandidateDiffBytes(t, baseline.CandidateDiff)
	if !bytes.Contains(baselineBytes, []byte("-base\n+candidate\n")) || bytes.Contains(baselineBytes, []byte("GIT binary patch")) {
		t.Fatalf("committed attributes influenced isolated diff:\n%s", baselineBytes)
	}

	globalAttributes := filepath.Join(t.TempDir(), "global-attributes")
	if err := os.WriteFile(globalAttributes, []byte("attribute-target.txt binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	writeFrozenCandidateHostileGitConfig(t, globalConfig, globalAttributes)
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_DIFF_OPTS", "--unified=0")
	t.Setenv("LC_ALL", "hostile_INVALID.UTF-8")
	t.Setenv("LANG", "hostile_INVALID.UTF-8")
	gitSnapshot(t, repo, "config", "core.attributesFile", globalAttributes)
	gitSnapshot(t, repo, "config", "diff.algorithm", "patience")
	gitSnapshot(t, repo, "config", "diff.context", "0")
	infoAttributes := filepath.Join(repo, ".git", "info", "attributes")
	if err := os.WriteFile(infoAttributes, []byte("attribute-target.txt binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, ".gitattributes", "attribute-target.txt -diff\n")
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "core.attributesFile")
	t.Setenv("GIT_CONFIG_VALUE_0", globalAttributes)
	t.Setenv("GIT_CONFIG_KEY_1", "diff.noprefix")
	t.Setenv("GIT_CONFIG_VALUE_1", "true")
	t.Setenv("GIT_ATTR_SOURCE", "HEAD")

	replayed, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("replayed FrozenCandidateContext() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, baseline) {
		t.Fatalf("frozen context changed under mutable attributes/config/locale\nfirst=%#v\nreplayed=%#v", baseline, replayed)
	}
	replayedStats, err := (SnapshotBuilder{Repo: repo}).DiffStats(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("replayed DiffStats() error = %v", err)
	}
	if !reflect.DeepEqual(replayedStats, baselineStats) {
		t.Fatalf("frozen diff stats changed under mutable attributes/config/locale\nfirst=%#v\nreplayed=%#v", baselineStats, replayedStats)
	}
}

func TestFrozenCandidateHostileGitConfigEscapesWindowsPath(t *testing.T) {
	requireSnapshotGit(t)
	config := filepath.Join(t.TempDir(), "gitconfig")
	windowsPath := `C:\Users\reviewer\global attributes`
	writeFrozenCandidateHostileGitConfig(t, config, windowsPath)
	command := exec.Command("git", "config", "--file", config, "--get", "core.attributesFile")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("portable Git config rejected Windows path: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != windowsPath {
		t.Fatalf("portable Git config path = %q, want %q", got, windowsPath)
	}
}

func writeFrozenCandidateHostileGitConfig(t *testing.T, path, attributesFile string) {
	t.Helper()
	for _, entry := range [][2]string{
		{"core.attributesFile", attributesFile},
		{"diff.external", "false"},
		{"diff.algorithm", "minimal"},
		{"diff.context", "0"},
	} {
		command := exec.Command("git", "config", "--file", path, entry[0], entry[1])
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("write portable Git config %s: %v\n%s", entry[0], err, output)
		}
	}
}

func TestFrozenCandidateContextPreservesInvalidUTF8PatchBytesAcrossJSON(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "invalid.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "--", "invalid.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	if err := os.WriteFile(filepath.Join(repo, "invalid.txt"), []byte{'c', 'a', 'n', 'd', 0xff, '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "--", "invalid.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	candidateCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetExactRevision, Revision: candidateCommit,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	contextResult, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("FrozenCandidateContext() error = %v", err)
	}
	wantBytes := frozenCandidateDiffBytes(t, contextResult.CandidateDiff)
	if !bytes.Contains(wantBytes, []byte{0xff}) {
		t.Fatalf("candidate diff lost invalid UTF-8 byte: %x", wantBytes)
	}
	payload, err := json.Marshal(contextResult.CandidateDiff)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip FrozenCandidateDiff
	if err := json.Unmarshal(payload, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if got := frozenCandidateDiffBytes(t, roundTrip); !bytes.Equal(got, wantBytes) {
		t.Fatalf("JSON round trip changed patch bytes\nwant=%x\ngot=%x", wantBytes, got)
	}
}

func TestFrozenCandidateDiffCaptureAndMetadataEnforceRawByteLimit(t *testing.T) {
	requireSnapshotGit(t)
	if got := base64.StdEncoding.EncodedLen(MaxFrozenCandidateDiffBytes); got != MaxFrozenCandidateDiffBase64Bytes {
		t.Fatalf("encoded max = %d, want %d", got, MaxFrozenCandidateDiffBase64Bytes)
	}
	repo := initSnapshotRepo(t)
	blob := bytes.Repeat([]byte("bounded-output\n"), 64)
	oid := strings.TrimSpace(gitSnapshotInput(t, repo, blob, "hash-object", "-w", "--stdin"))
	_, err := runGitLimited(context.Background(), repo, nil, nil, 64, "cat-file", "blob", oid)
	var limitErr *GitOutputLimitError
	if !errors.As(err, &limitErr) || limitErr.Limit != 64 {
		t.Fatalf("runGitLimited() error = %v, want 64-byte GitOutputLimitError", err)
	}
	if _, err := NewFrozenCandidateDiff(make([]byte, MaxFrozenCandidateDiffBytes+1)); !errors.As(err, &limitErr) || limitErr.Limit != MaxFrozenCandidateDiffBytes {
		t.Fatalf("NewFrozenCandidateDiff() error = %v, want max-size rejection", err)
	}
}

func TestFrozenCandidateContextRejectsSnapshotMetadataMismatch(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, Projection: ProjectionWorkspace, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	snapshot.Paths = []string{"different.txt"}
	if _, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot); err == nil || !strings.Contains(err.Error(), "snapshot paths") {
		t.Fatalf("FrozenCandidateContext() error = %v, want immutable snapshot mismatch", err)
	}
}

func gitSnapshotInput(t *testing.T, repo string, input []byte, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func manifestPaths(entries []ChangedPathManifestEntry) []string {
	paths := make([]string, len(entries))
	for index, entry := range entries {
		paths[index] = entry.Path
	}
	return paths
}

func frozenCandidateDiffBytes(t *testing.T, diff FrozenCandidateDiff) []byte {
	t.Helper()
	payload, err := diff.Bytes()
	if err != nil {
		t.Fatalf("candidate diff metadata = %#v: %v", diff, err)
	}
	return payload
}
