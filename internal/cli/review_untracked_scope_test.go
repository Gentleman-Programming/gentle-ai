package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Issue #2394. An untracked, unignored file that merely sits in the worktree
// used to be swept into the frozen candidate as intended-untracked scope, so
// its exact bytes reached a reviewer prompt. These constants are obviously
// synthetic on purpose: the fixture must look credential-shaped to the risk
// classifier without ever carrying a real secret.
const (
	unrelatedCredentialPath     = "unrelated-credentials.env"
	unrelatedCredentialContents = "EXAMPLE_API_TOKEN=synthetic-placeholder-value-0000\n" +
		"EXAMPLE_SECRET_KEY=synthetic-placeholder-value-1111\n"
	unrelatedCredentialMarker = "synthetic-placeholder-value-0000"
)

// startedChangedPathManifest returns the frozen manifest a negotiated START
// published, failing the test when the response carried none at all.
func startedChangedPathManifest(t *testing.T, started ReviewIntegrationStartResult) []reviewtransaction.ChangedPathManifestEntry {
	t.Helper()
	if started.ChangedPathManifest == nil {
		t.Fatal("negotiated start published no changed path manifest")
	}
	return *started.ChangedPathManifest
}

func runNegotiatedReviewStartExcludingUntracked(t *testing.T, repo, lineage string) ReviewIntegrationStartResult {
	t.Helper()
	_, digest, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).IntendedUntrackedInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", lineage,
		"--untracked-scope=exclude", "--expected-untracked-inventory=" + digest,
	}), &output); err != nil {
		t.Fatal(err)
	}
	return decodeNegotiatedReviewStart(t, output.Bytes())
}

// startWithUnrelatedUntrackedCredential freezes a candidate whose only real
// change is one tracked file, while an unrelated credential-shaped file sits
// untracked and undeclared next to it.
func startWithUnrelatedUntrackedCredential(t *testing.T, lineage string) (string, ReviewIntegrationStartResult) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "reviewed candidate\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	return repo, runNegotiatedReviewStartExcludingUntracked(t, repo, lineage)
}

// TestNegotiatedStartKeepsUndeclaredUntrackedFileOutOfCandidate pins the first
// property of #2394: existing in the worktree is not a declaration of review
// scope, so an unrelated untracked file never becomes part of the frozen
// candidate and never appears as intended-untracked provenance.
func TestNegotiatedStartKeepsUndeclaredUntrackedFileOutOfCandidate(t *testing.T) {
	reviewEnabledHome(t)
	_, started := startWithUnrelatedUntrackedCredential(t, "untracked-scope-excluded")

	manifest := startedChangedPathManifest(t, started)
	for _, entry := range manifest {
		if entry.Path == unrelatedCredentialPath {
			t.Fatalf("frozen candidate admitted the undeclared untracked path %q: %#v", entry.Path, entry)
		}
		if entry.IntendedUntracked {
			t.Fatalf("frozen candidate marked %q as intended-untracked without a declaration", entry.Path)
		}
	}
	if len(manifest) != 1 || manifest[0].Path != "tracked.txt" {
		t.Fatalf("frozen candidate = %#v, want only the declared tracked change", manifest)
	}
}

// TestNegotiatedStartKeepsDeclaredUntrackedFileInCandidate pins the second
// property: a new file the user actually declared with `git add` is still
// reviewed. Declaration is the existing Git index, so nothing has to be
// configured for the safe default to also be the useful one.
func TestNegotiatedStartKeepsDeclaredUntrackedFileInCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "reviewed candidate\n", 0o644)
	writeReviewStartCandidate(t, repo, "declared-helper.txt", "declared new file\n", 0o644)
	writeUndeclaredWorkspaceFile(t, repo, unrelatedCredentialPath, unrelatedCredentialContents, 0o600)
	runReviewCLIGit(t, repo, "add", "declared-helper.txt")

	started := runNegotiatedReviewStartExcludingUntracked(t, repo, "untracked-scope-declared")

	declared := false
	manifest := startedChangedPathManifest(t, started)
	for _, entry := range manifest {
		if entry.Path == unrelatedCredentialPath {
			t.Fatalf("frozen candidate admitted the undeclared untracked path %q", entry.Path)
		}
		if entry.Path == "declared-helper.txt" {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("frozen candidate = %#v, want the declared new file in scope", manifest)
	}
}

// TestReviewerLensContextOmitsUndeclaredUntrackedBytes pins the third
// property on the bytes that actually reach a model. The reviewer block is
// the delivered artifact, so the assertion is on its exact text rather than on
// any intermediate structure that only feeds it.
func TestReviewerLensContextOmitsUndeclaredUntrackedBytes(t *testing.T) {
	reviewEnabledHome(t)
	repo, started := startWithUnrelatedUntrackedCredential(t, "untracked-scope-delivery")

	block := lensContextBlock(t, started.RepositoryContext.Handle, started.SelectedLenses[0])

	if strings.Contains(block, unrelatedCredentialMarker) {
		t.Fatal("delivered reviewer context carries the contents of an undeclared untracked file")
	}
	if strings.Contains(block, unrelatedCredentialPath) {
		t.Fatalf("delivered reviewer context names the undeclared untracked path %q", unrelatedCredentialPath)
	}
	// The fixture must really be on disk, or the assertions above would pass
	// against an empty worktree and prove nothing.
	if _, err := os.Stat(filepath.Join(repo, unrelatedCredentialPath)); err != nil {
		t.Fatalf("fixture credential file is missing, so the assertions proved nothing: %v", err)
	}
}
