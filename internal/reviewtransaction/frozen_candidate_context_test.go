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
	if baseline.BaseTree != snapshot.BaseTree || baseline.CandidateTree != snapshot.CandidateTree {
		t.Fatalf("frozen trees = %s..%s, want %s..%s", baseline.BaseTree, baseline.CandidateTree, snapshot.BaseTree, snapshot.CandidateTree)
	}
	baselineDiff := frozenCandidatePathDiff(t, repo, baseline)
	if wholeDiff := frozenCandidateGitDiff(t, repo, baseline, ""); !bytes.Equal(baselineDiff, wholeDiff) {
		t.Fatalf("ordered path-scoped diffs do not reconstruct the canonical tree diff\npaths=%s\nwhole=%s", baselineDiff, wholeDiff)
	}

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
	if replayedDiff := frozenCandidatePathDiff(t, repo, replayed); !bytes.Equal(replayedDiff, baselineDiff) {
		t.Fatalf("path-scoped tree diff changed after live mutation\nfirst=%s\nreplayed=%s", baselineDiff, replayedDiff)
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
		if diff := frozenCandidatePathDiff(t, repo, contextResult); !bytes.Contains(diff, []byte("+reviewed untracked")) {
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
		if diff := frozenCandidatePathDiff(t, repo, contextResult); len(diff) != 0 {
			t.Fatalf("empty candidate diff = %q", diff)
		}
		if contextResult.ChangedPathManifest == nil || len(contextResult.ChangedPathManifest) != 0 {
			t.Fatalf("empty manifest = %#v, want non-nil []", contextResult.ChangedPathManifest)
		}
	})
}

func TestFrozenCandidateReviewerDiffUsesLiteralManifestPaths(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	paths := []string{
		"literal*.txt", "literal-one.txt",
		"literal?.txt", "literal1.txt",
		"literal[1].txt", ":(top)magic.txt", "magic.txt",
	}
	if runtime.GOOS == "windows" {
		paths = []string{"literal1.txt", "literal[1].txt"}
	}
	for _, version := range []string{"base", "candidate"} {
		for _, logicalPath := range paths {
			blob := strings.TrimSpace(gitSnapshotInput(t, repo, []byte(version+" "+logicalPath+"\n"), "hash-object", "-w", "--stdin"))
			gitSnapshot(t, repo, "update-index", "--add", "--cacheinfo", "100644,"+blob+","+logicalPath)
		}
		gitSnapshot(t, repo, "commit", "-m", version)
	}
	candidateCommit := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetExactRevision, Revision: candidateCommit,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("FrozenCandidateContext() error = %v", err)
	}
	if len(frozen.ChangedPathManifest) != len(paths) {
		t.Fatalf("manifest has %d paths, want %d: %#v", len(frozen.ChangedPathManifest), len(paths), frozen.ChangedPathManifest)
	}
	for _, entry := range frozen.ChangedPathManifest {
		scoped := frozenCandidateGitDiff(t, repo, frozen, entry.Path)
		if count := bytes.Count(scoped, []byte("diff --git ")); count != 1 {
			t.Fatalf("literal path %q rendered %d diffs:\n%s", entry.Path, count, scoped)
		}
		if !bytes.Contains(scoped, []byte("+candidate "+entry.Path)) {
			t.Fatalf("literal path %q selected different candidate bytes:\n%s", entry.Path, scoped)
		}
	}
	if scoped, whole := frozenCandidatePathDiff(t, repo, frozen), frozenCandidateGitDiff(t, repo, frozen, ""); !bytes.Equal(scoped, whole) {
		t.Fatalf("literal path-scoped diffs do not reconstruct the whole tree diff\npaths=%s\nwhole=%s", scoped, whole)
	}
}

func TestFrozenCandidateReviewerDiffUsesRegularAttributesFileAndIgnoresMutableSources(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "attribute-target.txt", "base\n")
	gitSnapshot(t, repo, "add", "--", "attribute-target.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
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
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("FrozenCandidateContext() error = %v", err)
	}
	baseline, err := (SnapshotBuilder{Repo: repo}).InspectCandidate(context.Background(), snapshot, "patch", 0, "")
	if err != nil {
		t.Fatalf("InspectCandidate() error = %v", err)
	}
	if !bytes.Contains(baseline, []byte("+candidate")) || bytes.Contains(baseline, []byte("GIT binary patch")) {
		t.Fatalf("baseline reviewer diff is not canonical text:\n%s", baseline)
	}

	attributes := filepath.Join(t.TempDir(), "attributes")
	if err := os.WriteFile(attributes, []byte("attribute-target.txt binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "config", "core.attributesFile", attributes)
	for _, config := range []string{"global.gitconfig", "system.gitconfig"} {
		path := filepath.Join(t.TempDir(), config)
		writeFrozenCandidateHostileGitConfig(t, path, attributes)
		if strings.HasPrefix(config, "global") {
			t.Setenv("GIT_CONFIG_GLOBAL", path)
		} else {
			t.Setenv("GIT_CONFIG_SYSTEM", path)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "attributes"), []byte("attribute-target.txt binary\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, ".gitattributes", "attribute-target.txt binary\n")
	t.Setenv("GIT_ATTR_SOURCE", "HEAD")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.attributesFile")
	t.Setenv("GIT_CONFIG_VALUE_0", attributes)

	originalStarter := gitProcessTreeStarter
	var observedAttributesFile bool
	var carrierErr error
	verifyEmptyRegular := func(carrier, path string) error {
		if path == "" {
			return errors.New(carrier + " is missing")
		}
		if path == os.DevNull {
			return errors.New(carrier + " still uses os.DevNull")
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != 0 {
			return errors.New(carrier + " is not an empty regular file")
		}
		return nil
	}
	gitProcessTreeStarter = func(command *exec.Cmd) (func() error, error) {
		for _, arg := range command.Args {
			if !strings.HasPrefix(arg, "core.attributesFile=") {
				continue
			}
			observedAttributesFile = true
			if err := verifyEmptyRegular("core.attributesFile", strings.TrimPrefix(arg, "core.attributesFile=")); err != nil {
				carrierErr = err
			}
			for _, prefix := range []string{"GIT_CONFIG_SYSTEM=", "GIT_CONFIG_GLOBAL="} {
				foundCarrier := false
				for _, entry := range command.Env {
					if strings.HasPrefix(entry, prefix) {
						foundCarrier = true
						if err := verifyEmptyRegular(strings.TrimSuffix(prefix, "="), strings.TrimPrefix(entry, prefix)); err != nil {
							carrierErr = err
						}
						break
					}
				}
				if !foundCarrier {
					carrierErr = errors.New(strings.TrimSuffix(prefix, "=") + " is missing")
				}
			}
		}
		return originalStarter(command)
	}
	t.Cleanup(func() { gitProcessTreeStarter = originalStarter })

	replayed, err := (SnapshotBuilder{Repo: repo}).InspectCandidate(context.Background(), snapshot, "patch", 0, "")
	if err != nil {
		t.Fatalf("InspectCandidate() under hostile configuration: %v", err)
	}
	if !observedAttributesFile {
		t.Fatal("isolated candidate inspection did not run core.attributesFile")
	}
	if carrierErr != nil {
		t.Fatal(carrierErr)
	}
	if !bytes.Equal(replayed, baseline) {
		t.Fatalf("reviewer tree diff changed under mutable Git attributes\nfirst=%s\nreplayed=%s", baseline, replayed)
	}
	porcelain := frozenCandidatePorcelainGitDiff(t, repo, frozen, "attribute-target.txt")
	if bytes.Equal(porcelain, baseline) || !bytes.Contains(porcelain, []byte("GIT binary patch")) {
		t.Fatalf("hostile attributes did not exercise the porcelain regression:\n%s", porcelain)
	}
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

func TestPreparedCandidateInspectorReusesOneSetupForFourReads(t *testing.T) {
	requireSnapshotGit(t)
	repo, snapshot := preparedCandidateSnapshot(t)
	original := gitCommandContext
	children := 0
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		children++
		return original(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = original })
	inspector, err := (SnapshotBuilder{Repo: repo}).PrepareCandidateInspector(t.Context(), snapshot)
	if err != nil {
		t.Fatalf("PrepareCandidateInspector() error = %v", err)
	}
	if got := inspector.FrozenCandidateContext(); !reflect.DeepEqual(manifestPaths(got.ChangedPathManifest), snapshot.Paths) {
		t.Fatalf("prepared manifest paths = %v, want %v", manifestPaths(got.ChangedPathManifest), snapshot.Paths)
	}
	for _, read := range []struct {
		operation string
		pathIndex int
	}{
		{"name-status", -1}, {"numstat", -1}, {"patch", 0}, {"patch", 1},
	} {
		if _, err := inspector.Inspect(t.Context(), read.operation, read.pathIndex, ""); err != nil {
			t.Fatalf("Inspect(%q, %d) error = %v", read.operation, read.pathIndex, err)
		}
	}
	if children != 11 {
		t.Fatalf("Git children for prepare plus four reads = %d, want 11", children)
	}
	builder := SnapshotBuilder{Repo: repo}
	for _, inspection := range []struct {
		operation string
		pathIndex int
		side      string
	}{
		{"name-status", -1, ""}, {"numstat", -1, ""}, {"stat", 0, ""},
		{"patch", 0, ""}, {"object", 0, "base"}, {"object", 0, "candidate"},
	} {
		got, err := inspector.Inspect(t.Context(), inspection.operation, inspection.pathIndex, inspection.side)
		if err != nil {
			t.Fatalf("prepared Inspect(%q, %d, %q): %v", inspection.operation, inspection.pathIndex, inspection.side, err)
		}
		want, err := builder.InspectCandidate(t.Context(), snapshot, inspection.operation, inspection.pathIndex, inspection.side)
		if err != nil {
			t.Fatalf("one-shot InspectCandidate(%q, %d, %q): %v", inspection.operation, inspection.pathIndex, inspection.side, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("prepared Inspect(%q, %d, %q) differs from one-shot", inspection.operation, inspection.pathIndex, inspection.side)
		}
	}
	if _, err := inspector.Inspect(t.Context(), "patch", -1, ""); err == nil {
		t.Fatal("Inspect() error = nil, want missing canonical path index refusal")
	}
	if err := inspector.Close(); err != nil {
		t.Fatalf("Close() after inspection error = %v", err)
	}
	calls := 0
	failing := &PreparedCandidateInspector{cleanup: func() error {
		calls++
		return errors.New("cleanup failed")
	}}
	if err := failing.Close(); err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("Close() error = %v, want cleanup failure", err)
	}
	if err := failing.Close(); err != nil {
		t.Fatalf("second Close() = %v, want idempotent cleanup", err)
	}
	if calls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls)
	}
}

func preparedCandidateSnapshot(t *testing.T) (string, Snapshot) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "one.txt", "base\n")
	writeSnapshotFile(t, repo, "two.txt", "base\n")
	gitSnapshot(t, repo, "add", "--", "one.txt", "two.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	writeSnapshotFile(t, repo, "one.txt", "candidate one.txt\n")
	writeSnapshotFile(t, repo, "two.txt", "candidate two.txt\n")
	gitSnapshot(t, repo, "add", "--", "one.txt", "two.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	candidate := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(t.Context(), Target{Kind: TargetExactRevision, Revision: candidate})
	if err != nil {
		t.Fatal(err)
	}
	return repo, snapshot
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

func frozenCandidatePathDiff(t *testing.T, repo string, frozen FrozenCandidateContext) []byte {
	t.Helper()
	var payload []byte
	for _, entry := range frozen.ChangedPathManifest {
		payload = append(payload, frozenCandidateGitDiff(t, repo, frozen, entry.Path)...)
	}
	return payload
}

func frozenCandidateGitDiff(t *testing.T, repo string, frozen FrozenCandidateContext, logicalPath string) []byte {
	t.Helper()
	isolation, cleanup, err := isolatedImmutableTreeGit(context.Background(), repo)
	if err != nil {
		t.Fatalf("create isolated reviewer Git environment: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	args := []string{
		"--no-pager", "diff-tree", "-p", "--binary", "--full-index", "--no-color", "--no-renames",
		"--no-ext-diff", "--no-textconv", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3",
		"--ignore-submodules=none", "--src-prefix=a/", "--dst-prefix=b/", frozen.BaseTree, frozen.CandidateTree, "--",
	}
	if logicalPath != "" {
		args = append(args, ":(literal)"+logicalPath)
	}
	output, err := runGitIsolated(context.Background(), repo, isolation, nil, args...)
	if err != nil {
		t.Fatalf("isolated git path-scoped diff %q: %v\n%s", logicalPath, err, output)
	}
	return output
}

func frozenCandidatePorcelainGitDiff(t *testing.T, repo string, frozen FrozenCandidateContext, logicalPath string) []byte {
	t.Helper()
	args := []string{
		"-C", repo, "--no-pager", "diff", "--binary", "--full-index", "--no-color", "--no-renames",
		"--no-ext-diff", "--no-textconv", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3",
		"--ignore-submodules=none", "--src-prefix=a/", "--dst-prefix=b/", frozen.BaseTree, frozen.CandidateTree, "--",
		":(literal)" + logicalPath,
	}
	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git porcelain path-scoped diff %q: %v\n%s", logicalPath, err, output)
	}
	return output
}
