package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// reviewContinuationPattern lifts a backticked invocation fragment out of a
// message the product actually emitted. The tests below build their argv from
// what they lift, so a message that names something unrunnable fails here
// instead of passing a wording assertion.
var reviewContinuationPattern = regexp.MustCompile("`([^`]+)`")

// A bare repository has no working tree, and a review candidate IS a
// working-tree diff, so refusing is correct. What was not correct was the
// refusal: it printed the internal `git rev-parse --show-toplevel` invocation,
// git's exit code, and git's own wording, and named nothing the operator could
// run. The operator is almost always in a bare clone or a server-side hook
// wanting to review work that lives in a checkout elsewhere.
//
// This test does not assert wording. It lifts the continuation out of the
// message the product emitted, builds an invocation from it, runs that
// invocation for real, and requires the block to be gone afterwards.
func TestReviewStartInABareRepositoryNamesAWorkingTreeThatActuallyWorks(t *testing.T) {
	reviewEnabledHome(t)
	checkout := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(t.TempDir(), "server.git")
	runReviewCLIGit(t, checkout, "clone", "--bare", "-q", checkout, bare)

	var output bytes.Buffer
	err := RunReview([]string{"start", "--cwd", bare}, &output)
	if err == nil {
		t.Fatalf("review start in a bare repository was allowed: %s", output.String())
	}
	message := err.Error()

	for _, leak := range []string{"rev-parse", "exit code", "must be run in a work tree", "fatal:"} {
		if strings.Contains(message, leak) {
			t.Fatalf("bare-repository refusal leaks git plumbing %q: %s", leak, message)
		}
	}
	if !strings.Contains(message, "bare") {
		t.Fatalf("bare-repository refusal never says the repository is bare: %s", message)
	}

	// Take the continuation out of the message and run it for real.
	fragment := ""
	for _, match := range reviewContinuationPattern.FindAllStringSubmatch(message, -1) {
		if strings.HasPrefix(match[1], "--cwd") {
			fragment = match[1]
		}
	}
	if fragment == "" {
		t.Fatalf("bare-repository refusal names no runnable continuation: %s", message)
	}
	fields := strings.Fields(fragment)
	if len(fields) != 2 || fields[0] != "--cwd" || !strings.HasPrefix(fields[1], "<") {
		t.Fatalf("continuation %q is not a flag plus a placeholder the operator can fill", fragment)
	}

	output.Reset()
	if err := RunReview([]string{"start", fields[0], checkout}, &output); err != nil {
		t.Fatalf("the continuation the refusal named did not clear the block: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "\"lineage_id\"") {
		t.Fatalf("continuation ran but started no review: %s", output.String())
	}
}

// Destroying diagnostic information is its own defect. Only the bare case gets
// the product-voice message; any other rev-parse failure must still surface the
// cause it came with, and must never claim the repository is bare.
func TestReviewStartOutsideAnyRepositoryStillSurfacesItsCause(t *testing.T) {
	reviewModeHome(t)
	outside := t.TempDir()

	var output bytes.Buffer
	err := RunReview([]string{"start", "--cwd", outside}, &output)
	if err == nil {
		t.Fatalf("review start outside a repository was allowed: %s", output.String())
	}
	if message := err.Error(); strings.Contains(message, "bare") {
		t.Fatalf("a non-bare failure was misclassified as bare: %s", message)
	}
	if message := err.Error(); !strings.Contains(message, "not a git repository") {
		t.Fatalf("an unexpected git failure lost its cause: %s", err)
	}
}

// A linked worktree has its own working tree and a .git FILE rather than a
// directory. It must never be mistaken for the bare case, because the recovery
// the bare message names would be wrong advice there.
func TestReviewStartInALinkedWorktreeIsNotTreatedAsBare(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, repo, "worktree", "add", "-q", linked, "-b", "linked-branch")
	if err := os.WriteFile(filepath.Join(linked, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", linked}, &output); err != nil {
		t.Fatalf("review start in a linked worktree was refused: %v\n%s", err, output.String())
	}
}
