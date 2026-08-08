package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// This file is the internal/cli half of D2's (#2471 option a, filed against
// #2388) evidence trail. internal/reviewtransaction's
// compact_gate_local_advance_test.go carries the full explanation of why
// wiring deriveBaseAdvanceCompatibility into GatePreCommit/GatePrePush is
// stopped rather than attempted with a weaker check: neither local gate's own
// target derivation can ever produce the "base moved, candidate untouched"
// precondition the shared, unchanged content checks require. These CLI-level
// tests reproduce the exact end-user commands #2388 ran (`gentle-ai review
// validate --gate pre-commit/pre-push`) against a real approved, published
// receipt, proving the negotiated failure envelope an operator actually sees
// is unchanged by this work unit -- no regression, and no CLI-visible
// lookalike allow.

// TestValidatePrePushStillRefusesAfterLocalMergeOfAnAdvancedParent reproduces
// #2388's pre-push shape end to end through the real CLI surface: an approved,
// published receipt; the parent (origin/<branch>) advances on a disjoint
// path; the local branch merges that advance in (a real merge commit, so HEAD
// itself changes); `review validate --gate pre-push` still denies with
// candidate-or-paths-mismatch, exactly as it did before this change --
// deriveBaseAdvanceCompatibility's requireAttestation parameter exists and is
// unit-tested (internal/reviewtransaction), but nothing in this CLI path
// invokes it for a local gate, so the CLI-visible outcome for this shape is
// byte-identical to main.
func TestValidatePrePushStillRefusesAfterLocalMergeOfAnAdvancedParent(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	approveDiscoveryMarkdown(t, repo, "local-gate-root", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed delivery")
	runReviewCLIGit(t, repo, "push", "-q", "origin", branch)

	// Advance the parent on a remote client, on a path disjoint from the
	// delivered one, then merge that advance in locally.
	client := filepath.Join(t.TempDir(), "advance-client")
	runReviewCLIGit(t, repo, "clone", "-q", remote, client)
	runReviewCLIGit(t, client, "config", "user.email", "advance@example.com")
	runReviewCLIGit(t, client, "config", "user.name", "advance")
	writeReviewCLIFile(t, client, "docs/unrelated.md", "parent advance\n")
	runReviewCLIGit(t, client, "add", "-A")
	runReviewCLIGit(t, client, "commit", "-qm", "advance the parent")
	runReviewCLIGit(t, client, "push", "-q", "origin", "HEAD:"+branch)
	runReviewCLIGit(t, repo, "fetch", "-q", "origin")
	runReviewCLIGit(t, repo, "merge", "-q", "--no-ff", "--no-edit", "origin/"+branch)

	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/" + branch,
	}, &output)
	if err == nil {
		t.Fatalf("pre-push validation after a local merge of an advanced parent was allowed:\n%s", output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("pre-push denial is not a negotiated failure: %v\n%s", err, output.String())
	}
	// The disjoint parent advance plus the local merge changed both base and
	// candidate away from the receipt, so this is refused at discovery
	// (receipt_scope_changed) rather than reaching native gate evaluation with
	// full ScopeChange diagnostics -- either shape proves the same thing: no
	// compatible-base-advance allow leaked through for this shape.
	if failure.Failure.Code != "receipt_scope_changed" {
		t.Fatalf("pre-push denial code = %q, want receipt_scope_changed: %#v\n%s", failure.Failure.Code, failure.Failure, output.String())
	}
}

// TestValidatePreCommitStillRefusesStagedMergeOfAnAdvancedParent is
// pre-commit's counterpart, staging (not committing) the parent's advance --
// the literal #2388 reproduction shape -- and confirming the CLI's negotiated
// pre-commit validation still denies with the same diagnostics it always has.
func TestValidatePreCommitStillRefusesStagedMergeOfAnAdvancedParent(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	remote := filepath.Join(t.TempDir(), "remote.git")
	runReviewCLIGit(t, repo, "clone", "--bare", repo, remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)

	approveDiscoveryMarkdown(t, repo, "local-gate-precommit-root", "docs/reviewed.md", "reviewed\n")
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed delivery")
	runReviewCLIGit(t, repo, "push", "-q", "origin", branch)

	client := filepath.Join(t.TempDir(), "advance-client-precommit")
	runReviewCLIGit(t, repo, "clone", "-q", remote, client)
	runReviewCLIGit(t, client, "config", "user.email", "advance@example.com")
	runReviewCLIGit(t, client, "config", "user.name", "advance")
	writeReviewCLIFile(t, client, "docs/unrelated.md", "parent advance\n")
	runReviewCLIGit(t, client, "add", "-A")
	runReviewCLIGit(t, client, "commit", "-qm", "advance the parent")
	runReviewCLIGit(t, client, "push", "-q", "origin", "HEAD:"+branch)
	runReviewCLIGit(t, repo, "fetch", "-q", "origin")
	runReviewCLIGit(t, repo, "merge", "-q", "--no-commit", "--no-ff", "origin/"+branch)

	var output bytes.Buffer
	err := RunReview([]string{
		"validate", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "local-gate-precommit-root", "--gate", string(reviewtransaction.GatePreCommit),
	}, &output)
	if err == nil {
		t.Fatalf("pre-commit validation of a staged merge of an advanced parent was allowed:\n%s", output.String())
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("pre-commit denial is not a negotiated failure: %v\n%s", err, output.String())
	}
	if failure.Failure.Context == nil || failure.Failure.Context.ScopeChange == nil {
		t.Fatalf("pre-commit denial carries no scope-change diagnostics: %#v\n%s", failure.Failure, output.String())
	}
}

func writeReviewCLIFile(t *testing.T, repo, logicalPath, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(logicalPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
