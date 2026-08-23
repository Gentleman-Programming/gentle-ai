package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func initUnbornReviewCLIRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runReviewCLIGit(t, repo, "init", "-q")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
	return repo
}

func reviewCLIEmptyTree(t *testing.T, repo string) string {
	t.Helper()
	command := exec.Command("git", "-C", repo, "mktree")
	command.Stdin = strings.NewReader("")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git mktree: %v\n%s", err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeUnbornReviewCandidate(t *testing.T, repo string) {
	t.Helper()
	code := "package candidate\n\n// Reviewed reports the initial reviewed value.\nfunc Reviewed() int {\n\treturn 1\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "candidate.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "candidate.md"), []byte("reviewed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func finalizeUnbornFacadeReview(t *testing.T, repo string, started ReviewFacadeStartResult) {
	t.Helper()
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, repo, started)...)
	args = append(args, "--evidence", evidencePath)
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
		t.Fatalf("finalize unborn review: %v", err)
	}
}

// TestReviewFacadeUnbornHeadDefaultProjectionStart proves 1771 on the direct
// `review start` path (default workspace projection, no --projection flag),
// which is the exact community repro (lu149e): plain `gentle-ai review start
// --cwd $PWD` on an unborn-HEAD repository with staged candidate files used
// to fail with "build facade review target: git rev-parse --verify
// HEAD^{tree} failed with exit code 128: fatal: Needed a single revision"
// because resolveCurrentChangesBase only resolved the empty tree for staged
// projection. It is the same fix site as the selector-free `review status`
// gap, reached through the direct start path instead.
func TestReviewFacadeUnbornHeadDefaultProjectionStart(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
	writeUnbornReviewCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "--", "candidate.go", "candidate.md")

	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("unborn default-projection review start: %v", err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Projection != reviewtransaction.ProjectionWorkspace || started.ChangedFiles != 2 {
		t.Fatalf("unborn default-projection start = %#v, want workspace projection over 2 files", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := reviewCLIEmptyTree(t, repo); record.State.InitialSnapshot.BaseTree != want || !record.State.InitialSnapshot.UnbornHead {
		t.Fatalf("frozen base tree = %#v, want unborn-marked repository-native empty tree %q", record.State.InitialSnapshot, want)
	}
}

func TestReviewFacadeUnbornHeadStagedLifecycle(t *testing.T) {
	reviewEnabledHome(t)
	repo := initUnbornReviewCLIRepo(t)
	if err := RunReviewMode([]string{"enable", "--scope", "global", "--cwd", repo}, io.Discard); err != nil {
		t.Fatalf("enable receipt-driven development: %v", err)
	}
	writeUnbornReviewCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "--", "candidate.go", "candidate.md")

	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatalf("unborn staged review start: %v", err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Projection != reviewtransaction.ProjectionStaged || started.ChangedFiles != 2 {
		t.Fatalf("unborn staged start = %#v, want staged projection over 2 files", started)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := reviewCLIEmptyTree(t, repo); record.State.InitialSnapshot.BaseTree != want || !record.State.InitialSnapshot.UnbornHead {
		t.Fatalf("frozen base snapshot = %#v, want unborn repository-native empty tree %q", record.State.InitialSnapshot, want)
	}
	if want := []string{"candidate.go", "candidate.md"}; !reflect.DeepEqual(record.State.GenesisPaths, want) {
		t.Fatalf("genesis paths = %v, want every candidate path %v", record.State.GenesisPaths, want)
	}

	finalizeUnbornFacadeReview(t, repo, started)
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)

	runReviewCLIGit(t, repo, "commit", "-qm", "first commit")
	if headTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}")); headTree != record.State.CurrentSnapshot.CandidateTree {
		t.Fatalf("first commit tree = %q, want approved candidate %q", headTree, record.State.CurrentSnapshot.CandidateTree)
	}
	output.Reset()
	if err := RunReview([]string{"validate", "--cwd", repo, "--gate", "pre-commit"}, &output); err != nil {
		t.Fatalf("shipped pre-commit after the first commit: %v\n%s", err, output.String())
	}
	assertEnabledUnmanagedGatePayload(t, output.Bytes(), reviewtransaction.GatePreCommit)
}

func TestReviewFacadeUnbornHeadStagedWithNothingStagedRefusesActionably(t *testing.T) {
	reviewModeHome(t)
	repo := initUnbornReviewCLIRepo(t)
	writeUnbornReviewCandidate(t, repo)

	err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "git add") {
		t.Fatalf("unborn nothing-staged start error = %v, want actionable staging guidance", err)
	}
}

func TestReviewFacadeExistingRemoteEmptyCommitAllowsPublicationGates(t *testing.T) {
	reviewModeHome(t)
	repo := initUnbornReviewCLIRepo(t)
	if err := RunReviewMode([]string{"enable", "--scope", "global", "--cwd", repo}, io.Discard); err != nil {
		t.Fatalf("enable receipt-driven development: %v", err)
	}
	runReviewCLIGit(t, repo, "commit", "--allow-empty", "-qm", "empty base")
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	writeUnbornReviewCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "--", "candidate.go", "candidate.md")
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	finalizeUnbornFacadeReview(t, repo, started)
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)

	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed candidate")
	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		output.Reset()
		if err := RunReview([]string{"validate", "--cwd", repo, "--gate", string(gate), "--base-ref", "origin/" + branch}, &output); err != nil {
			t.Fatalf("shipped %s from existing empty commit: %v\n%s", gate, err, output.String())
		}
		assertEnabledUnmanagedGatePayload(t, output.Bytes(), gate)
	}
}
