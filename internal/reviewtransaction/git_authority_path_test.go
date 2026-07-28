package reviewtransaction

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalGitDirectoryRejectsMalformedRecords(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.Mkdir(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	tests := []struct {
		name    string
		output  []byte
		want    string
		wantErr bool
	}{
		{name: "relative directory", output: []byte(".git\n"), want: gitDir},
		{name: "absolute linked worktree common directory", output: []byte(outside + "\n"), want: outside},
		{name: "unsupported option echo", output: []byte("--path-format=absolute\n"), wantErr: true},
		{name: "option echo plus path", output: []byte("--path-format=absolute\n.git\n"), wantErr: true},
		{name: "multiple paths", output: []byte(".git\nother\n"), wantErr: true},
		{name: "empty", output: []byte("\n"), wantErr: true},
		{name: "nul", output: []byte(".git\x00outside\n"), wantErr: true},
		{name: "relative escape", output: []byte("../" + filepath.Base(outside) + "\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalGitDirectory(repo, tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("canonicalGitDirectory() = %q, want error", got)
				}
				return
			}
			if err != nil || got != filepath.Clean(tt.want) {
				t.Fatalf("canonicalGitDirectory() = %q, %v; want %q", got, err, filepath.Clean(tt.want))
			}
		})
	}
}

func TestGitAuthorityResolversDoNotUsePathFormatAbsolute(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	original := gitCommandContext
	var forbidden [][]string
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		for _, arg := range args {
			if arg == "--path-format=absolute" {
				forbidden = append(forbidden, append([]string(nil), args...))
				break
			}
		}
		return original(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = original })

	if _, _, err := reviewAuthorityRoot(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(isolation) == 0 {
		t.Fatal("isolatedImmutableTreeGit() returned no environment")
	}
	cleanup()
	if _, err := reviewRepositoryIdentity(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if len(forbidden) != 0 {
		t.Fatalf("runtime Git authority resolution used --path-format=absolute: %v", forbidden)
	}
}

func TestReviewAuthorityRootRejectsLegacyOptionEchoWithoutMutation(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	original := gitCommandContext
	t.Setenv("GENTLE_AI_TEST_GIT_PATH_OUTPUT", base64.StdEncoding.EncodeToString([]byte("--path-format=absolute\n.git\n")))
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "--git-common-dir") {
			return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitAuthorityPathHelperProcess$")
		}
		return original(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = original })

	if _, _, err := reviewAuthorityRoot(context.Background(), repo); err == nil {
		t.Fatal("reviewAuthorityRoot() accepted legacy option echo")
	}
	for _, path := range []string{
		filepath.Join(repo, "--path-format=absolute\n.git"),
		filepath.Join(repo, "gentle-ai"),
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("malformed Git output created %q", path)
		}
	}
}

func TestResolveRepositoryRootRejectsUnrelatedAbsoluteOutput(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	outside := t.TempDir()
	original := gitCommandContext
	t.Setenv("GENTLE_AI_TEST_GIT_PATH_OUTPUT", base64.StdEncoding.EncodeToString([]byte(outside+"\n")))
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "--show-toplevel") {
			return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitAuthorityPathHelperProcess$")
		}
		return original(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = original })

	if root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background()); err == nil {
		t.Fatalf("ResolveRepositoryRoot() = %q, want unrelated-root error", root)
	}
}

func TestReviewAuthorityRootPreservesSymlinkedGitDirectory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	externalGitDir := filepath.Join(t.TempDir(), "git-dir")
	if err := os.Rename(filepath.Join(repo, ".git"), externalGitDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalGitDir, filepath.Join(repo, ".git")); err != nil {
		t.Skipf("symlinked Git directories are unavailable: %v", err)
	}

	root, resolvedRepo, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(externalGitDir, "gentle-ai", "review-transactions")
	if root != want || resolvedRepo != repo {
		t.Fatalf("reviewAuthorityRoot() = %q, %q; want %q, %q", root, resolvedRepo, want, repo)
	}
}

func TestReviewRepositoryIdentityAcceptsSupportedGitTopologies(t *testing.T) {
	requireSnapshotGit(t)
	ordinary := initSnapshotRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if err := runSnapshotGit(ordinary, "worktree", "add", "-b", "common-dir-linked", linked, "HEAD"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runSnapshotGit(ordinary, "worktree", "remove", "--force", linked) })

	separateParent := t.TempDir()
	separate := filepath.Join(separateParent, "worktree")
	separateGitDir := filepath.Join(separateParent, "metadata")
	command := exec.Command("git", "init", "--separate-git-dir", separateGitDir, separate)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init --separate-git-dir: %v\n%s", err, output)
	}

	superproject := initSnapshotRepo(t)
	moduleSource := initSnapshotRepo(t)
	gitSnapshot(t, superproject, "-c", "protocol.file.allow=always", "submodule", "add", moduleSource, "module")
	submodule := filepath.Join(superproject, "module")

	for name, repo := range map[string]string{
		"ordinary": ordinary, "linked worktree": linked, "separate Git directory": separate, "submodule": submodule,
	} {
		t.Run(name, func(t *testing.T) {
			identity, err := reviewRepositoryIdentity(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if identity.RepositoryRoot == "" || identity.GitCommonDir == "" || identity.GitDir == "" {
				t.Fatalf("incomplete repository identity: %#v", identity)
			}
			isolation, cleanup, err := isolatedImmutableTreeGit(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			cleanup()
			if len(isolation) == 0 {
				t.Fatal("isolated Git environment is empty")
			}
		})
	}
}

func TestReviewRepositoryIdentityPreservesGitCommandError(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	original := gitCommandContext
	showTopLevelCalls := 0
	t.Setenv("GENTLE_AI_TEST_GIT_PATH_FAIL", "1")
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if slicesContain(args, "--show-toplevel") {
			showTopLevelCalls++
			if showTopLevelCalls == 2 {
				return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitAuthorityPathHelperProcess$")
			}
		}
		return original(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = original })

	_, err := reviewRepositoryIdentity(context.Background(), repo)
	var commandErr *GitCommandError
	if !errors.As(err, &commandErr) {
		t.Fatalf("reviewRepositoryIdentity() error = %T %v, want *GitCommandError", err, err)
	}
}

func TestGitCommonDirectoryMismatchFailsBeforeAuthorityMutation(t *testing.T) {
	requireSnapshotGit(t)
	operations := []struct {
		name string
		run  func(context.Context, string) error
	}{
		{name: "authoritative store", run: func(ctx context.Context, repo string) error {
			_, err := AuthoritativeStore(ctx, repo, "unrelated-common-v1")
			return err
		}},
		{name: "compact store", run: func(ctx context.Context, repo string) error {
			_, err := CompactAuthoritativeStore(ctx, repo, "unrelated-common-v2")
			return err
		}},
		{name: "maintenance lock", run: func(ctx context.Context, repo string) error {
			lock, err := AcquireReviewMaintenanceExclusive(ctx, repo)
			if lock != nil {
				_ = lock.Release()
			}
			return err
		}},
		{name: "frozen Git isolation", run: func(ctx context.Context, repo string) error {
			_, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
			cleanup()
			return err
		}},
		{name: "provider publication", run: func(ctx context.Context, repo string) error {
			_, err := PublishReviewRepositoryContext(ctx, repo, ReviewRepositoryContextBinding{
				LineageID: "unrelated-common-provider", TargetIdentity: "sha256:" + strings.Repeat("a", 64),
				Revision: "sha256:" + strings.Repeat("b", 64),
			})
			return err
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			unrelated := t.TempDir()
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			overrideGitDirectoryOutputs(t, repo, unrelated, filepath.Join(repo, ".git"), nil)
			ctx, cancel := context.WithTimeout(context.Background(), maintenanceLockTimeout)
			defer cancel()

			if err := operation.run(ctx, repo); err == nil {
				t.Error("operation accepted an unrelated Git common directory")
			}
			for _, path := range []string{filepath.Join(unrelated, "gentle-ai"), filepath.Join(home, ".gentle-ai")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Errorf("rejected common directory mutated %q: %v", path, err)
				}
			}
		})
	}
}

func TestReviewRepositoryIdentityRejectsReplacedCommonDirectory(t *testing.T) {
	requireSnapshotGit(t)
	repo := initSnapshotRepo(t)
	common := filepath.Join(t.TempDir(), "common")
	if err := os.Mkdir(common, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(common)
	if err != nil {
		t.Fatal(err)
	}
	replaced := false
	overrideGitDirectoryOutputs(t, repo, common, common+"-old", func() {
		if replaced {
			return
		}
		replaced = true
		if err := os.Rename(common, common+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(common, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := reviewRepositoryIdentity(context.Background(), repo); err == nil {
		t.Fatal("reviewRepositoryIdentity() accepted a replaced Git common directory")
	}
	after, err := os.Stat(common)
	if err != nil {
		t.Fatal(err)
	}
	if !replaced || os.SameFile(before, after) {
		t.Fatal("replacement fixture did not change the common-directory file identity")
	}
}

func TestReviewAuthorityRootRejectsMidQueryGitControlReplacement(t *testing.T) {
	repo, replacement := initSnapshotRepo(t), initSnapshotRepo(t)
	original := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		combined := slicesContain(args, "--show-toplevel") && slicesContain(args, "--git-common-dir") && slicesContain(args, "--git-dir")
		if combined || slicesContain(args, "--git-common-dir") && !slicesContain(args, "--git-dir") {
			if err := errors.Join(
				os.Rename(filepath.Join(repo, ".git"), filepath.Join(repo, ".git-old")),
				os.Rename(filepath.Join(replacement, ".git"), filepath.Join(repo, ".git")),
			); err != nil {
				t.Fatal(err)
			}
		}
		return original(ctx, name, args...)
	}
	t.Cleanup(func() { gitCommandContext = original })
	if _, _, err := reviewAuthorityRoot(context.Background(), repo); err == nil {
		t.Fatal("mid-query Git control replacement derived an authority root")
	}
	if _, err := os.Lstat(filepath.Join(repo, ".git", "gentle-ai")); !os.IsNotExist(err) {
		t.Fatalf("rejected replacement mutated authority: %v", err)
	}
}

func TestReviewRepositoryIdentityAcceptsDisjointCommonDirectory(t *testing.T) {
	repo, parent := initSnapshotRepo(t), t.TempDir()
	gitDir, commonDir := filepath.Join(parent, "git-dir"), filepath.Join(parent, "common")
	for _, path := range []string{gitDir, commonDir} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../common\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	overrideGitDirectoryOutputs(t, repo, commonDir, gitDir, nil)
	identity, err := reviewRepositoryIdentity(context.Background(), repo)
	if err != nil || identity.GitCommonDir != commonDir || identity.GitDir != gitDir {
		t.Fatalf("disjoint identity = %#v, %v", identity, err)
	}
}

func overrideGitDirectoryOutputs(t *testing.T, repo, commonDir, gitDir string, beforeGitDir func()) {
	t.Helper()
	original := gitCommandContext
	gitCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		output := ""
		switch {
		case slicesContain(args, "--show-toplevel") && slicesContain(args, "--git-common-dir") && slicesContain(args, "--git-dir"):
			if beforeGitDir != nil {
				beforeGitDir()
			}
			output = strings.Join([]string{repo, commonDir, gitDir}, "\n")
		case commonDir != "" && slicesContain(args, "--git-common-dir"):
			output = commonDir
		case gitDir != "" && slicesContain(args, "--git-dir"):
			if beforeGitDir != nil {
				beforeGitDir()
			}
			output = gitDir
		default:
			return original(ctx, name, args...)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(output + "\n"))
		return exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGitAuthorityPathHelperProcess$", "--", encoded)
	}
	t.Cleanup(func() { gitCommandContext = original })
}

func TestGitAuthorityPathHelperProcess(t *testing.T) {
	if os.Getenv("GENTLE_AI_TEST_GIT_PATH_FAIL") == "1" {
		os.Exit(23)
	}
	encoded := os.Getenv("GENTLE_AI_TEST_GIT_PATH_OUTPUT")
	if encoded == "" && len(os.Args) > 2 && os.Args[len(os.Args)-2] == "--" {
		encoded = os.Args[len(os.Args)-1]
	}
	if encoded == "" {
		return
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		os.Exit(2)
	}
	_, _ = os.Stdout.Write(payload)
	os.Exit(0)
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
