package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewRecoverAuthorizationFileSuccess(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-file-success")

	successorIdentity := reviewRecoverBaseDiffSuccessorIdentity(t, repo, baseRef)
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, successorIdentity, "maintainer", "recover into base-diff scope")

	// Write authorization with CRLF and trailing newlines to prove trimming and normalization.
	authFile := filepath.Join(t.TempDir(), "auth.txt")
	if err := os.WriteFile(authFile, []byte(strings.ReplaceAll(authorization, "\n", "\r\n")+"\r\n\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-file-success-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only", "--maintainer-authorization-file", authFile,
	}, &output)
	if err != nil {
		t.Fatalf("recovery via --maintainer-authorization-file rejected: %v", err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatalf("decode recover result: %v\n%s", err, output.String())
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-file-success-successor")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff {
		t.Fatalf("recovered successor kind = %q, want base-diff", record.State.InitialSnapshot.Kind)
	}
	if record.State.Recovery.MaintainerAuthorization != authorization {
		t.Fatalf("persisted authorization = %q, want %q", record.State.Recovery.MaintainerAuthorization, authorization)
	}
}

func TestReviewRecoverAuthorizationStdinSuccess(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-stdin-success")

	successorIdentity := reviewRecoverBaseDiffSuccessorIdentity(t, repo, baseRef)
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, successorIdentity, "maintainer", "recover into base-diff scope")

	// Set up pipe to simulate stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	go func() {
		defer writer.Close()
		_, _ = writer.Write([]byte(authorization + "\n"))
	}()

	oldStdin := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = oldStdin }()

	var output bytes.Buffer
	err = RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-stdin-success-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only", "--maintainer-authorization-file", "-",
	}, &output)
	if err != nil {
		t.Fatalf("recovery via --maintainer-authorization-file - rejected: %v", err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatalf("decode recover result: %v\n%s", err, output.String())
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-stdin-success-successor")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.Recovery.MaintainerAuthorization != authorization {
		t.Fatalf("persisted authorization = %q, want %q", record.State.Recovery.MaintainerAuthorization, authorization)
	}
}

func TestReviewRecoverAuthorizationMutuallyExclusive(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-mutual-exclusion")

	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-mutual-exclusion-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only",
		"--maintainer-authorization", "auth",
		"--maintainer-authorization-file", "auth.txt",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--maintainer-authorization and --maintainer-authorization-file are mutually exclusive") {
		t.Fatalf("RunReviewRecover with both flags = %v, want mutually exclusive error", err)
	}
}

func TestReviewRecoverAuthorizationFileMissing(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-file-missing")

	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-file-missing-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only",
		"--maintainer-authorization-file", filepath.Join(t.TempDir(), "nonexistent.txt"),
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "read maintainer authorization file") {
		t.Fatalf("RunReviewRecover with missing file = %v, want read error", err)
	}
}

func TestReviewRecoverAuthorizationFileInexactFormat(t *testing.T) {
	reviewEnabledHome(t)
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-file-inexact")

	authFile := filepath.Join(t.TempDir(), "auth.txt")
	if err := os.WriteFile(authFile, []byte("gentle-ai.review-recovery-authorization/v1\nwrong=fields\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-file-inexact-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only",
		"--maintainer-authorization-file", authFile,
	}, io.Discard)
	if err == nil {
		t.Fatal("RunReviewRecover with inexact file succeeded, want failure")
	}
	msg := err.Error()
	if !strings.Contains(msg, "gentle-ai.review-recovery-authorization/v1") {
		t.Fatalf("expected error to name schema, got: %s", msg)
	}
	if !strings.Contains(msg, "key=value") {
		t.Fatalf("expected error to explain key=value format, got: %s", msg)
	}
}

func TestReviewRecoverHelpDocumentsAuthorizationFile(t *testing.T) {
	var output bytes.Buffer
	if err := RunReviewRecover([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.Contains(help, "--maintainer-authorization-file") {
		t.Fatalf("review recover help missing --maintainer-authorization-file:\n%s", help)
	}
}
