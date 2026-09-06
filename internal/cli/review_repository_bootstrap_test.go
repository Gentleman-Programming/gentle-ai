package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
)

func TestReviewLifecycleBootstrapsGenuinelyUnversionedWorkspace(t *testing.T) {
	reviewEnabledHome(t)
	operations := map[string]func(*testing.T, string, *bytes.Buffer) error{
		"negotiated status": func(t *testing.T, repo string, output *bytes.Buffer) error {
			t.Helper()
			if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, output); err != nil {
				return err
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if status.NextTransition == nil || status.NextTransition.ReasonCode != "intended_untracked_selection_required" {
				t.Fatalf("negotiated status transition = %#v, want intended untracked selection", status.NextTransition)
			}
			return nil
		},
		"direct start": func(t *testing.T, repo string, output *bytes.Buffer) error {
			t.Helper()
			err := RunReview([]string{"start", "--cwd", repo}, output)
			if err == nil || !strings.Contains(err.Error(), "--untracked-scope=exclude") {
				t.Fatalf("direct start error = %v, want the normal untracked-selection refusal", err)
			}
			return nil
		},
	}

	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			candidate := filepath.Join(repo, "candidate.txt")
			if err := os.WriteFile(candidate, []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			if err := operation(t, repo, &output); err != nil {
				t.Fatalf("review operation: %v\n%s", err, output.String())
			}
			assertLocalGitBootstrapPreservedWorkspace(t, repo, candidate)
		})
	}
}

func TestReviewStatusHelpDescribesLocalGitInitialization(t *testing.T) {
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--help"}, &output); err != nil {
		t.Fatalf("review status help: %v", err)
	}
	if strings.Contains(output.String(), "without mutation") || !strings.Contains(output.String(), "initialize local Git") {
		t.Fatalf("review status help = %q, want scoped local Git initialization", output.String())
	}
}

func TestReviewStatusReusesContainingWorktreeWithoutNestedMetadata(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
	nested := filepath.Join(repo, "nested", "workspace")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "candidate.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", nested, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &output); err != nil {
		t.Fatalf("review status from nested workspace: %v\n%s", err, output.String())
	}
	if _, err := os.Lstat(filepath.Join(nested, ".git")); !os.IsNotExist(err) {
		t.Fatalf("nested workspace metadata = %v, want no nested .git", err)
	}
	// Identity, not string equality: on Windows the runner's TEMP is an 8.3
	// short name that Git reports back in its long spelling.
	if root := strings.TrimSpace(runReviewCLIGit(t, nested, "rev-parse", "--show-toplevel")); !pathidentity.SameDirectory(root, repo) {
		t.Fatalf("nested workspace root = %q, want %q", root, repo)
	}
}

func TestReviewStatusLeavesUnusableGitMetadataUnchanged(t *testing.T) {
	repo := t.TempDir()
	metadata := filepath.Join(repo, ".git")
	before := []byte("not a git directory\n")
	if err := os.WriteFile(metadata, before, 0o600); err != nil {
		t.Fatal(err)
	}

	err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("review status accepted unusable Git metadata")
	}
	after, readErr := os.ReadFile(metadata)
	if readErr != nil {
		t.Fatalf("read Git metadata after failed status: %v", readErr)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("Git metadata changed after failed status:\nbefore: %q\nafter:  %q", before, after)
	}
}

func assertLocalGitBootstrapPreservedWorkspace(t *testing.T, repo, candidate string) {
	t.Helper()
	if info, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !info.IsDir() {
		t.Fatalf("local Git metadata = %v (%v), want directory", info, err)
	}
	if content, err := os.ReadFile(candidate); err != nil || string(content) != "candidate\n" {
		t.Fatalf("candidate after bootstrap = %q, %v", content, err)
	}
	remote := exec.Command("git", "-C", repo, "remote")
	if output, err := remote.CombinedOutput(); err != nil || len(bytes.TrimSpace(output)) != 0 {
		t.Fatalf("git remote = %q, %v; want no remotes", output, err)
	}
	status := exec.Command("git", "-C", repo, "status", "--short")
	if output, err := status.CombinedOutput(); err != nil || string(output) != "?? candidate.txt\n" {
		t.Fatalf("git status --short = %q, %v; want untracked candidate", output, err)
	}
	if output, err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "HEAD").CombinedOutput(); err == nil {
		t.Fatalf("bootstrap created HEAD %q, want unborn HEAD", output)
	}
}
