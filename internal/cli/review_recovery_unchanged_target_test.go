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

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

// newUnchangedTargetRecoveryFixture builds a StateInvalidated predecessor with
// a TargetBaseDiff initial snapshot, ready for a recovery attempt whose
// recomputed successor target Identity is identical to the predecessor's
// (same --base-ref, same candidate content).
func newUnchangedTargetRecoveryFixture(t *testing.T) (repo, baseRef string, predecessor reviewtransaction.CompactRecord) {
	t.Helper()
	repo = initReviewCLIRepo(t)
	baseRef = strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", baseRef, "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	pristine, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewInvalidate([]string{"--cwd", repo, "--lineage", started.LineageID,
		"--expected-revision", pristine.Revision, "--reason", "external evidence invalidation"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	invalidated, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	return repo, baseRef, invalidated
}

func TestUnchangedTargetRecovery_InvalidatedAdmitted(t *testing.T) {
	repo, baseRef, predecessor := newUnchangedTargetRecoveryFixture(t)
	var output bytes.Buffer
	err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-invalidated-recovered",
		"--disposition", "invalidated", "--reason", "redo after external evidence", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only"}, &output)
	if err != nil {
		t.Fatalf("unchanged-target invalidated recovery = %v", err)
	}
	var recovered ReviewRecoverResult
	if unmarshalErr := json.Unmarshal(output.Bytes(), &recovered); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if recovered.LineageID != "review-invalidated-recovered" || recovered.Recovery.Disposition != reviewtransaction.RecoveryInvalidated {
		t.Fatalf("recovered = %#v", recovered)
	}
}

func TestUnchangedTargetRecovery_EscalatedStillBlocked(t *testing.T) {
	repo, baseRef, predecessor := newUnchangedTargetRecoveryFixture(t)
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	successorLineage := "review-escalated-unchanged"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.State.LineageID +
		"\npredecessor_revision=" + predecessor.Revision + "\ntarget_identity=" + predecessor.State.InitialSnapshot.Identity +
		"\nactor=maintainer\nreason=escalated retry"
	var output bytes.Buffer
	err = RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", successorLineage,
		"--disposition", "escalated", "--reason", "escalated retry", "--actor", "maintainer",
		"--maintainer-authorization", authorization, "--base-ref", baseRef, "--committed-only"}, &output)
	if err == nil || err.Error() != "recovery scope has not changed" {
		t.Fatalf("unchanged-target escalated recovery = %v", err)
	}
	after, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("predecessor state mutated by rejected escalated recovery")
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", successorLineage)); !os.IsNotExist(statErr) {
		t.Fatalf("successor persisted despite rejected escalated recovery: %v", statErr)
	}
}

func TestUnchangedTargetRecovery_ScopeChangedStillBlocked(t *testing.T) {
	repo, baseRef, predecessor := newUnchangedTargetRecoveryFixture(t)
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	successorLineage := "review-scope-changed-unchanged"
	var output bytes.Buffer
	err = RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", successorLineage,
		"--disposition", "scope_changed", "--reason", "attempted scope-changed retry", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only"}, &output)
	if err == nil || err.Error() != "recovery scope has not changed" {
		t.Fatalf("unchanged-target scope_changed recovery = %v", err)
	}
	after, err := os.ReadFile(predecessorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("predecessor state mutated by rejected scope_changed recovery")
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", successorLineage)); !os.IsNotExist(statErr) {
		t.Fatalf("successor persisted despite rejected scope_changed recovery: %v", statErr)
	}
}

func TestUnchangedTargetRecovery_BaseMismatchStillBlocked(t *testing.T) {
	repo, _, predecessor := newUnchangedTargetRecoveryFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "other.txt"), []byte("advanced base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "other.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "advance base")
	advancedBaseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	var output bytes.Buffer
	err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-base-mismatch",
		"--disposition", "invalidated", "--reason", "base mismatch check", "--actor", "maintainer",
		"--base-ref", advancedBaseRef, "--committed-only"}, &output)
	if err == nil || err.Error() != "recovery base-ref does not match predecessor base" {
		t.Fatalf("base mismatch invalidated recovery = %v", err)
	}
}

func TestUnchangedTargetRecovery_BaseMismatchStillBlockedForScopeChanged(t *testing.T) {
	repo, _, predecessor := newUnchangedTargetRecoveryFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "other.txt"), []byte("advanced base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "other.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "advance base")
	advancedBaseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	var output bytes.Buffer
	err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-base-mismatch-scope-changed",
		"--disposition", "scope_changed", "--reason", "base mismatch check", "--actor", "maintainer",
		"--base-ref", advancedBaseRef, "--committed-only"}, &output)
	if err == nil || err.Error() != "recovery base-ref does not match predecessor base" {
		t.Fatalf("base mismatch scope_changed recovery = %v", err)
	}
}

func TestUnchangedTargetRecovery_BaseMismatchStillBlockedForEscalated(t *testing.T) {
	repo, _, predecessor := newUnchangedTargetRecoveryFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "other.txt"), []byte("advanced base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "other.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "advance base")
	advancedBaseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.State.LineageID +
		"\npredecessor_revision=" + predecessor.Revision + "\ntarget_identity=" + predecessor.State.InitialSnapshot.Identity +
		"\nactor=maintainer\nreason=base mismatch check"
	var output bytes.Buffer
	err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-base-mismatch-escalated",
		"--disposition", "escalated", "--reason", "base mismatch check", "--actor", "maintainer",
		"--maintainer-authorization", authorization, "--base-ref", advancedBaseRef, "--committed-only"}, &output)
	if err == nil || err.Error() != "recovery base-ref does not match predecessor base" {
		t.Fatalf("base mismatch escalated recovery = %v", err)
	}
}

func TestUnchangedTargetRecovery_ChangedTargetInvalidatedStillSucceeds(t *testing.T) {
	repo, baseRef, predecessor := newUnchangedTargetRecoveryFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "changed candidate")
	var output bytes.Buffer
	err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-changed-target-invalidated",
		"--disposition", "invalidated", "--reason", "changed candidate recovery", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only"}, &output)
	if err != nil {
		t.Fatalf("changed-target invalidated recovery = %v", err)
	}
}
