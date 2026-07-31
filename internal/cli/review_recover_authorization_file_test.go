package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewRecoverAuthorizationFileAndStdin(t *testing.T) {
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "recover-auth-file")
	runReviewCLIGit(t, repo, "config", "user.name", "Maintainer")
	runReviewCLIGit(t, repo, "config", "user.email", "maintainer@example.com")

	// 1. Test mutual exclusion
	err := RunReviewRecover([]string{
		"--cwd", repo,
		"--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "successor-1",
		"--disposition", "invalidated",
		"--maintainer-authorization", "auth1",
		"--maintainer-authorization-file", "auth.txt",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "cannot specify both") {
		t.Fatalf("expected mutual exclusion error, got: %v", err)
	}

	// 2. Test reading authorization from file
	authPath := filepath.Join(t.TempDir(), "auth.txt")
	authContent := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.State.LineageID + "\npredecessor_revision=" + predecessor.Revision + "\ntarget_identity=" + predecessor.State.InitialSnapshot.Identity + "\nactor=Maintainer <maintainer@example.com>\nreason=manual recovery\r\n"
	if err := os.WriteFile(authPath, []byte(authContent), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReviewRecover([]string{
		"--cwd", repo,
		"--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "successor-file-auth",
		"--disposition", "invalidated",
		"--actor", "Maintainer <maintainer@example.com>",
		"--reason", "manual recovery",
		"--maintainer-authorization-file", authPath,
	}, &output)
	if err != nil {
		t.Fatalf("RunReviewRecover with --maintainer-authorization-file failed: %v", err)
	}
}

func TestReviewRecoverNonDerivableErrorIncludesTemplate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "escalated-template-test"
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	// An ACTIVE predecessor cannot be self-derived for recovery, forcing !derivable
	err = RunReviewRecover([]string{
		"--cwd", repo,
		"--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "successor-template-err",
		"--disposition", "invalidated",
	}, io.Discard)

	if err == nil {
		t.Fatal("expected error recovering active predecessor without authorization")
	}
	for _, want := range []string{
		"review recover requires --reason, --actor, and --maintainer-authorization",
		"gentle-ai.review-recovery-authorization/v1",
		"predecessor_lineage=" + predecessor.State.LineageID,
		"predecessor_revision=" + predecessor.Revision,
		"target_identity=" + predecessor.State.InitialSnapshot.Identity,
		"actor=<actor>",
		"reason=<reason>",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error template missing %q; got: %s", want, err.Error())
		}
	}
}

func TestReviewRecoverAuthorizationFileStdin(t *testing.T) {
	repo, predecessor := invalidatedRecoverySelfDerivationPredecessor(t, "recover-auth-stdin")
	runReviewCLIGit(t, repo, "config", "user.name", "Maintainer")
	runReviewCLIGit(t, repo, "config", "user.email", "maintainer@example.com")

	authContent := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.State.LineageID + "\npredecessor_revision=" + predecessor.Revision + "\ntarget_identity=" + predecessor.State.InitialSnapshot.Identity + "\nactor=Maintainer <maintainer@example.com>\nreason=manual recovery\n"

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.Write([]byte(authContent))
		_ = w.Close()
	}()

	var output bytes.Buffer
	err = RunReviewRecover([]string{
		"--cwd", repo,
		"--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "successor-stdin-auth",
		"--disposition", "invalidated",
		"--actor", "Maintainer <maintainer@example.com>",
		"--reason", "manual recovery",
		"--maintainer-authorization-file", "-",
	}, &output)
	if err != nil {
		t.Fatalf("RunReviewRecover with stdin failed: %v", err)
	}
}
