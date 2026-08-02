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

// TestReviewRecoverEscalatedBaseDiffSuccessorOverCurrentChangesPredecessor
// proves issue-1744: the CLI's `review recover` flag-shape pre-gate no longer
// couples the requested target kind to the predecessor's kind. An escalated
// recovery over a current-changes predecessor may request a base-diff
// successor via --base-ref/--committed-only, and the CLI does not reject it
// on flag shape alone: `validateCompactRecoveryEdge` makes the semantic
// admission decision (spec: "Compact recovery edge admission" scenario
// "Compatible predecessor reaches semantic admission (1744)").
//
// Maintainer-verified correction: re-targeting across kinds is a deliberate
// recovery capability, not a gap. Real protection is identity-bound
// authorization (compactRecoveryAuthorizationBinding), substantive scope
// change for approved recovery, and the base-tree pin for base-diff
// predecessors — none of which is kind matching. This test and its four
// negative siblings below prove the CLI relaxation does not widen authority.
func TestReviewRecoverEscalatedBaseDiffSuccessorOverCurrentChangesPredecessor(t *testing.T) {
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-1744-escalated")

	successorIdentity := reviewRecoverBaseDiffSuccessorIdentity(t, repo, baseRef)
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, successorIdentity, "maintainer", "recover into base-diff scope")

	var output bytes.Buffer
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-1744-escalated-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only", "--maintainer-authorization", authorization,
	}, &output)
	if err != nil {
		t.Fatalf("escalated base-diff successor over current-changes predecessor rejected: %v", err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatalf("decode recover result: %v\n%s", err, output.String())
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1744-escalated-successor")
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff {
		t.Fatalf("recovered successor kind = %q, want base-diff", record.State.InitialSnapshot.Kind)
	}
}

// TestReviewRecoverEscalatedRequiresExactSuccessorBoundAuthorization proves
// the amended spec's negative scenario "Escalated recovery without bound
// maintainer authorization is still rejected": relaxing the CLI kind pre-gate
// did not weaken the identity-bound authorization check, regardless of the
// requested target kind.
func TestReviewRecoverEscalatedRequiresExactSuccessorBoundAuthorization(t *testing.T) {
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-1744-wrong-auth")
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-1744-wrong-auth-successor",
		"--disposition", "escalated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only", "--maintainer-authorization", "not the exact successor-bound authorization",
	}, io.Discard)
	if err == nil {
		t.Fatal("escalated recovery with an inexact maintainer authorization was admitted")
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1744-wrong-auth-successor")
	if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverInvalidatedRequiresInvalidatedPredecessor proves the
// amended spec's negative scenario "Invalidated recovery still requires an
// invalidated predecessor": a healthy (escalated, not invalidated)
// predecessor cannot be recovered with --disposition invalidated, regardless
// of the requested target kind.
func TestReviewRecoverInvalidatedRequiresInvalidatedPredecessor(t *testing.T) {
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-1744-wrong-disposition")
	successorIdentity := reviewRecoverBaseDiffSuccessorIdentity(t, repo, baseRef)
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, successorIdentity, "maintainer", "recover into base-diff scope")
	err := RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-1744-wrong-disposition-successor",
		"--disposition", "invalidated", "--reason", "recover into base-diff scope", "--actor", "maintainer",
		"--base-ref", baseRef, "--committed-only", "--maintainer-authorization", authorization,
	}, io.Discard)
	if err == nil {
		t.Fatal("invalidated disposition over a non-invalidated (escalated) predecessor was admitted")
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1744-wrong-disposition-successor")
	if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverBaseDiffPredecessorStillBindsFrozenBaseTree proves the
// amended spec's negative scenario "A base-diff predecessor still binds its
// frozen base tree": relaxing the CLI kind pre-gate did not weaken the
// existing base-tree pin (`runReviewFacadeRecover`'s
// "recovery base-ref does not match predecessor base" check) for a
// predecessor that was already base-diff.
func TestReviewRecoverBaseDiffPredecessorStillBindsFrozenBaseTree(t *testing.T) {
	repo := initReviewCLIRepo(t)
	firstBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-m", "candidate")

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", firstBase, "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"reviewed"}})
	if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	predecessorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff {
		t.Fatalf("predecessor fixture is not base-diff: %#v", predecessor.State)
	}

	// A different, unrelated base commit: same repository, but not the frozen
	// predecessor base tree.
	if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("unrelated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "unrelated.txt")
	runReviewCLIGit(t, repo, "commit", "-m", "unrelated base")
	differentBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))

	err = RunReviewRecover([]string{
		"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-1744-different-base-successor",
		"--disposition", "scope_changed", "--reason", "recover with a different base", "--actor", "maintainer",
		"--base-ref", differentBase, "--committed-only",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "recovery base-ref does not match predecessor base") {
		t.Fatalf("base-diff predecessor with a changed base-ref = %v, want frozen base-tree rejection", err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "recover-1744-different-base-successor")
	if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected recovery created a successor: %v", statErr)
	}
}

// TestReviewRecoverArgvCoherenceStillRejectsMismatchedBaseRefFlags proves the
// amended spec's negative scenario "the argv-coherence rule pairing
// --base-ref with --committed-only MUST remain": the surviving clause
// (*committedOnly != (base != "")) still rejects --base-ref without
// --committed-only and --committed-only without --base-ref.
func TestReviewRecoverArgvCoherenceStillRejectsMismatchedBaseRefFlags(t *testing.T) {
	repo, baseRef, predecessor := escalatedCurrentChangesRecoveryFixture(t, "recover-1744-argv-coherence")
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"base-ref without committed-only", []string{"--base-ref", baseRef}},
		{"committed-only without base-ref", []string{"--committed-only"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{
				"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
				"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "recover-1744-argv-" + strings.ReplaceAll(tt.name, " ", "-"),
				"--disposition", "escalated", "--reason", "argv coherence", "--actor", "maintainer",
			}, tt.args...)
			err := RunReviewRecover(args, io.Discard)
			if err == nil || !strings.Contains(err.Error(), "base-diff recovery requires matching --base-ref and --committed-only") {
				t.Fatalf("mismatched argv shape (%s) = %v, want the surviving argv-coherence rejection", tt.name, err)
			}
		})
	}
}

// escalatedCurrentChangesRecoveryFixture reaches StateEscalated over a
// TargetCurrentChanges predecessor purely through the shipped product
// mechanism: an over-budget correction forecast (compact.go's BeginCorrection
// escalates when the proposed lines exceed the correction budget), the same
// mechanism TestReviewFacadePersistsOverBudgetForecastAndActual exercises.
func escalatedCurrentChangesRecoveryFixture(t *testing.T, lineage string) (string, string, reviewtransaction.CompactRecord) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	baseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
			ProofRefs:     []string{"differential test fails only on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"focused differential test failed"},
	})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", lineage, "--result", resultPath, "--correction-lines", "1000"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if predecessor.State.State != reviewtransaction.StateEscalated || predecessor.State.InitialSnapshot.Kind != reviewtransaction.TargetCurrentChanges {
		t.Fatalf("predecessor fixture did not reach an escalated current-changes authority: %#v", predecessor.State)
	}
	return repo, baseRef, predecessor
}

// reviewRecoverBaseDiffSuccessorIdentity mirrors the exact target the CLI
// builds internally for a --base-ref/--committed-only recovery (workspace
// projection, discovered intended-untracked), so tests can pre-compute the
// successor identity needed to construct an exact-bound maintainer
// authorization before calling RunReviewRecover.
func reviewRecoverBaseDiffSuccessorIdentity(t *testing.T, repo, baseRef string) string {
	t.Helper()
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverIntendedUntracked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseDiff, BaseRef: baseRef, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intended,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Identity
}
