package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestRctx2GranularErrorCodes verifies that reviewRepositoryContextResolutionFailure
// classifies rctx2 errors into granular codes instead of the generic
// repository_context_unavailable.
func TestRctx2GranularErrorCodes(t *testing.T) {
	reviewEnabledHome(t)

	t.Run("rctx2_binding_unusable — malformed handle", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc capture() {}\n", 0o644)
		started := runNegotiatedReviewStart(t, repo, "rctx2-malformed")
		if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
			t.Fatalf("START result = %#v", started)
		}
		args := []string{
			"--cwd", repo,
			"--repository-context", "rctx2_not-base64",
			"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
			"--expected-revision", started.RepositoryContext.Revision,
			"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
		}
		err := RunReviewCaptureResult(args, io.Discard)
		if err == nil {
			t.Fatal("expected error for malformed rctx2 handle")
		}
		msg := err.Error()
		if !strings.Contains(msg, "rctx2_binding_unusable") || !strings.Contains(msg, "structurally invalid") {
			t.Fatalf("expected rctx2_binding_unusable with 'structurally invalid', got: %s", msg)
		}
	})

	t.Run("rctx2_resolution_failed — authority record absent", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc capture() {}\n", 0o644)
		started := runNegotiatedReviewStart(t, repo, "rctx2-absent")
		if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
			t.Fatalf("START result = %#v", started)
		}
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(store.StatePath()); err != nil {
			t.Fatal(err)
		}
		args := []string{
			"--cwd", repo,
			"--repository-context", started.RepositoryContext.Handle,
			"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
			"--expected-revision", started.RepositoryContext.Revision,
			"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
		}
		err = RunReviewCaptureResult(args, io.Discard)
		if err == nil {
			t.Fatal("expected error for absent authority record")
		}
		msg := err.Error()
		if !strings.Contains(msg, "rctx2_resolution_failed") || !strings.Contains(msg, "binding was valid when issued") {
			t.Fatalf("expected rctx2_resolution_failed with remediation, got: %s", msg)
		}
	})

	t.Run("rctx2_binding_unusable — wrong revision SHA format", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc capture() {}\n", 0o644)
		started := runNegotiatedReviewStart(t, repo, "rctx2-revision")
		if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
			t.Fatalf("START result = %#v", started)
		}
		args := []string{
			"--cwd", repo,
			"--repository-context", started.RepositoryContext.Handle,
			"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
			"--expected-revision", "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000",
			"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
		}
		err := RunReviewCaptureResult(args, io.Discard)
		if err == nil {
			t.Fatal("expected error for invalid revision format")
		}
		msg := err.Error()
		if !strings.Contains(msg, "rctx2_binding_unusable") {
			t.Fatalf("expected rctx2_binding_unusable, got: %s", msg)
		}
	})

	t.Run("rctx2_identity_mismatch — handle encodes different repo identity", func(t *testing.T) {
		repoA := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repoA, "candidate.go", "package candidate\n\nfunc capture() {}\n", 0o644)
		started := runNegotiatedReviewStart(t, repoA, "rctx2-identity")
		if started.RepositoryContext == nil || len(started.SelectedLenses) != 1 {
			t.Fatalf("START result = %#v", started)
		}

		// Create a second repository with a different identity.
		repoB := canonicalReviewCLITempDir(t)
		runReviewCLIGit(t, repoB, "init", "-q")
		runReviewCLIGit(t, repoB, "config", "user.email", "test@example.com")
		runReviewCLIGit(t, repoB, "config", "user.name", "Test")
		runReviewCLIGit(t, repoB, "config", "core.autocrlf", "false")
		if err := os.WriteFile(filepath.Join(repoB, "other.txt"), []byte("other\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repoB, "add", "other.txt")
		runReviewCLIGit(t, repoB, "commit", "-qm", "other")

		// Obtain repo B's live identity.
		leaseB, err := reviewtransaction.OpenRepositoryIdentityLease(t.Context(), repoB)
		if err != nil {
			t.Fatalf("open repo B lease: %v", err)
		}
		identityB := leaseB.Identity()

		// Create a synthetic handle that encodes repo B's identity but
		// carries repo A's binding (lineage, target, revision). When used
		// with --cwd repo A, the identity check will fail because the
		// handle encodes repo B's identity, not repo A's.
		//
		// This exercises the identity mismatch path while keeping authority
		// loading working (the --cwd repo A still has the authority record).
		binding := reviewtransaction.ReviewRepositoryContextBinding{
			LineageID:      started.LineageID,
			TargetIdentity: started.RepositoryContext.TargetIdentity,
			Revision:       started.RepositoryContext.Revision,
		}
		syntheticHandle, err := reviewtransaction.BuildReviewRepositoryContextV2Handle(identityB, binding)
		if err != nil {
			t.Fatalf("build synthetic handle: %v", err)
		}

		args := []string{
			"--cwd", repoA,
			"--repository-context", syntheticHandle,
			"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
			"--expected-revision", started.RepositoryContext.Revision,
			"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
		}
		err = RunReviewCaptureResult(args, io.Discard)
		if err == nil {
			t.Fatal("expected error for rctx2 identity mismatch")
		}
		msg := err.Error()
		if !strings.Contains(msg, "rctx2_identity_mismatch") {
			t.Fatalf("expected rctx2_identity_mismatch, got: %s", msg)
		}
	})
}
