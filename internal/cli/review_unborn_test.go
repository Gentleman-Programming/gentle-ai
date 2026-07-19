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

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
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
	args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, started)...)
	args = append(args, "--evidence", evidencePath)
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
		t.Fatalf("finalize unborn review: %v", err)
	}
}

func TestReviewFacadeUnbornHeadStagedLifecycle(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
	writeUnbornReviewCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "--", "candidate.go", "candidate.md")

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
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
	if want := reviewCLIEmptyTree(t, repo); record.State.InitialSnapshot.BaseTree != want {
		t.Fatalf("frozen base tree = %q, want repository-native empty tree %q", record.State.InitialSnapshot.BaseTree, want)
	}
	if want := []string{"candidate.go", "candidate.md"}; !reflect.DeepEqual(record.State.GenesisPaths, want) {
		t.Fatalf("genesis paths = %v, want every candidate path %v", record.State.GenesisPaths, want)
	}

	finalizeUnbornFacadeReview(t, repo, started)

	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit} {
		output.Reset()
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", string(gate)}, &output); err != nil {
			t.Fatalf("%s while unborn: %v\n%s", gate, err, output.String())
		}
		assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
	}

	runReviewCLIGit(t, repo, "commit", "-qm", "first commit")
	if headTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}")); headTree != record.State.CurrentSnapshot.CandidateTree {
		t.Fatalf("first commit tree = %q, want approved candidate %q", headTree, record.State.CurrentSnapshot.CandidateTree)
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", "pre-commit"}, &output); err != nil {
		t.Fatalf("pre-commit after the first commit: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
}

func TestReviewFacadeUnbornHeadStagedWithNothingStagedRefusesActionably(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
	writeUnbornReviewCandidate(t, repo)

	err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "git add") {
		t.Fatalf("unborn nothing-staged start error = %v, want actionable staging guidance", err)
	}
}

func TestReviewFacadeUnbornReceiptDeniesFirstPublicationGates(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
	writeUnbornReviewCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "--", "candidate.go", "candidate.md")

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatalf("unborn staged review start: %v", err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	finalizeUnbornFacadeReview(t, repo, started)
	runReviewCLIGit(t, repo, "commit", "-qm", "first commit")
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)

	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		output.Reset()
		err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", string(gate), "--base-ref", "origin/" + branch}, &output)
		if err == nil {
			t.Fatalf("%s from an empty-base receipt must be denied\n%s", gate, output.String())
		}
		if combined := output.String() + err.Error(); !strings.Contains(combined, "first publication") {
			t.Fatalf("%s denial = %v, want explicit first-publication denial\n%s", gate, err, output.String())
		}
	}
}

func TestReviewFacadeExistingRemoteEmptyCommitAllowsPublicationGates(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "commit", "--allow-empty", "-qm", "empty base")
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	writeUnbornReviewCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "--", "candidate.go", "candidate.md")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	finalizeUnbornFacadeReview(t, repo, started)
	runReviewCLIGit(t, repo, "commit", "-qm", "deliver reviewed candidate")
	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		output.Reset()
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", string(gate), "--base-ref", "origin/" + branch}, &output); err != nil {
			t.Fatalf("%s from existing empty commit: %v\n%s", gate, err, output.String())
		}
	}
}
