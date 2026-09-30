package cli

import (
	"context"
	"io"
	"os"
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
}
