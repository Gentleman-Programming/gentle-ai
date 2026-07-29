package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// gitDubiousOwnershipStderr is the byte-exact diagnostic git writes when it
// refuses a repository owned by a different account. It was captured from a
// genuine refusal staged against git version 2.55.0 by creating a repository,
// chowning it to a second local account, and running
// `LC_ALL=C git --no-replace-objects -C <repo> rev-parse --show-toplevel`,
// which exited 128 with exactly these bytes on stderr. %s is the repository
// path, which git interpolates twice.
const gitDubiousOwnershipStderr = "fatal: detected dubious ownership in repository at '%s'\n" +
	"To add an exception for this directory, call:\n" +
	"\n" +
	"\tgit config --global --add safe.directory %s"

// gitMissingRepositoryStderr is an unrelated git setup failure that also exits
// 128. It must never be classified as a trust refusal.
const gitMissingRepositoryStderr = "fatal: not a git repository (or any of the parent directories): .git"

// installFailingGitShim puts a `git` shim first on PATH. The shim fails only
// the repository-directory resolution triple
// (`rev-parse --show-toplevel --git-common-dir --git-dir`) that
// ResolveReviewRepositoryContext runs, and delegates every other invocation to
// the real git, so the failure is injected at exactly the point an inherited
// Git trust context breaks in the field.
func installFailingGitShim(t *testing.T, stderr string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH shim requires a POSIX shell")
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	shimDir := t.TempDir()
	shim := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"--git-common-dir\" ]; then\n" +
		"    printf '%s\\n' \"$GENTLE_AI_TEST_GIT_STDERR\" >&2\n" +
		"    exit 128\n" +
		"  fi\n" +
		"done\n" +
		"exec \"$GENTLE_AI_TEST_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "git"), []byte(shim), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENTLE_AI_TEST_REAL_GIT", realGit)
	t.Setenv("GENTLE_AI_TEST_GIT_STDERR", stderr)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestOpaqueRepositoryContextSurfacesGitTrustRefusal proves that a Git
// ownership refusal inherited from the host process is reported as its own
// typed, path-free code with an instruction the caller can actually carry out,
// instead of being collapsed into "refresh the exact native next_transition",
// which cannot change a running process's Git trust context.
func TestOpaqueRepositoryContextSurfacesGitTrustRefusal(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(t *testing.T, args []string, input string) error
	}{
		{name: "capture-result-preflight", run: func(t *testing.T, args []string, _ string) error {
			return RunReviewCaptureResult(append(append([]string{}, args...), "--preflight"), io.Discard)
		}},
		{name: "preserve-result", run: func(t *testing.T, args []string, input string) error {
			return RunReviewPreserveResult(append(append([]string{}, args...), "--input", input), io.Discard)
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			args, input, repo := startedOpaqueCaptureBinding(t, "git-trust-"+operation.name)
			installFailingGitShim(t, strings.ReplaceAll(gitDubiousOwnershipStderr, "%s", repo))

			err := operation.run(t, args, input)
			if err == nil {
				t.Fatal("operation succeeded despite a Git ownership refusal")
			}
			message := err.Error()
			if !strings.Contains(message, "git_repository_untrusted") {
				t.Fatalf("failure does not carry the typed Git trust code: %s", message)
			}
			if strings.Contains(message, "next_transition") {
				t.Fatalf("failure still advises refreshing the transition, which cannot change Git trust: %s", message)
			}
			if !strings.Contains(message, "Restart the host process") {
				t.Fatalf("failure carries no instruction the caller can carry out: %s", message)
			}
			if strings.Contains(message, repo) || strings.Contains(message, os.Getenv("HOME")) {
				t.Fatalf("failure leaked a private path: %s", message)
			}
			if scrubbed := reviewScrubDefectReportField(message); scrubbed != message {
				t.Fatalf("failure is not path-safe; privacy gate rewrote it to %q", scrubbed)
			}
		})
	}
}

// TestUnrelatedRepositoryContextFailureIsNotLabelledUntrusted pins the other
// half of the contract: an ordinary repository-context failure, including
// another git failure that also exits 128, must keep its existing generic code.
func TestUnrelatedRepositoryContextFailureIsNotLabelledUntrusted(t *testing.T) {
	args, _, repo := startedOpaqueCaptureBinding(t, "git-trust-unrelated")
	installFailingGitShim(t, gitMissingRepositoryStderr)

	err := RunReviewCaptureResult(append(append([]string{}, args...), "--preflight"), io.Discard)
	if err == nil {
		t.Fatal("preflight succeeded despite an unresolvable repository")
	}
	message := err.Error()
	if strings.Contains(message, "git_repository_untrusted") {
		t.Fatalf("an unrelated git failure was mislabelled as a Git trust refusal: %s", message)
	}
	if !strings.Contains(message, "repository_context_unavailable") {
		t.Fatalf("unrelated repository-context failure lost its existing code: %s", message)
	}
	if strings.Contains(message, repo) {
		t.Fatalf("failure leaked a private path: %s", message)
	}
}

// startedOpaqueCaptureBinding starts one negotiated review and returns the
// opaque repository-context binding arguments for its single selected lens,
// a reviewer-result input path, and the repository root.
func startedOpaqueCaptureBinding(t *testing.T, lineage string) ([]string, string, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc trust() {}\n", 0o644)
	started := runNegotiatedReviewStart(t, repo, lineage)
	if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
		t.Fatalf("START result = %#v", started)
	}
	input := filepath.Join(t.TempDir(), "reviewer.json")
	if err := os.WriteFile(input, []byte("{\"findings\":[],\"evidence\":[]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	return []string{
		"--repository-context", started.RepositoryContext.Handle,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lens", started.SelectedLenses[0], "--order", "0",
	}, input, repo
}
