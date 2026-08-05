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
	repo := initUnbornReviewCLIRepo(t)
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

	err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "git add") {
		t.Fatalf("unborn nothing-staged start error = %v, want actionable staging guidance", err)
	}
}

func TestReviewFacadeUnbornReceiptDeniesFirstPublicationGates(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
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
	finalizeUnbornFacadeReview(t, repo, started)
	runReviewCLIGit(t, repo, "commit", "-qm", "first commit")
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))

	// pre-push is asked about a publication that really happens, so the remote
	// must not already carry the delivery: an empty remote is the first
	// publication itself, and it advertises no branch to pass as --base-ref.
	firstPublication := filepath.Join(t.TempDir(), "first-publication.git")
	runReviewCLIGit(t, repo, "init", "--bare", "-q", firstPublication)
	runReviewCLIGit(t, repo, "remote", "add", "origin", firstPublication)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", "pre-push"}, &output); err == nil {
		t.Fatalf("pre-push from an empty-base receipt must be denied\n%s", output.String())
	} else if combined := output.String() + err.Error(); !strings.Contains(combined, "first publication") {
		t.Fatalf("pre-push denial = %v, want explicit first-publication denial\n%s", err, output.String())
	}

	// pre-PR asks whether this receipt may open a pull request against an
	// advertised boundary, which is a question about the receipt rather than
	// about whether a commit moves, so it keeps refusing after publication.
	runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", "pre-pr", "--base-ref", "origin/" + branch}, &output); err == nil {
		t.Fatalf("pre-pr from an empty-base receipt must be denied\n%s", output.String())
	} else if combined := output.String() + err.Error(); !strings.Contains(combined, "first publication") {
		t.Fatalf("pre-pr denial = %v, want explicit first-publication denial\n%s", err, output.String())
	}
}

// TestFirstPublicationEmptyBaseReceiptRefusal proves 1641: first publication
// attempted from an empty-base receipt (unborn HEAD, empty base tree) must be
// refused with a typed error naming the proven in-product escape verbatim —
// commit an authorized empty root, then run gentle-ai review start
// --committed-only with --base-ref set to that commit's SHA — instead of the
// generic "not supported" denial.
func TestFirstPublicationEmptyBaseReceiptRefusal(t *testing.T) {
	repo := initUnbornReviewCLIRepo(t)
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
	finalizeUnbornFacadeReview(t, repo, started)
	runReviewCLIGit(t, repo, "commit", "-qm", "first commit")
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))

	want := "commit an authorized empty root, then run gentle-ai review start --committed-only with --base-ref set to that commit's SHA"

	// The publication that 1641 is actually about: a remote that has none of
	// this work, so the push really does transfer the first publication. The
	// empty-base receipt cannot govern it and the refusal must name the escape.
	// An empty remote advertises no branch, so there is no --base-ref to pass;
	// the gate derives the bootstrap boundary itself.
	firstPublication := filepath.Join(t.TempDir(), "first-publication.git")
	runReviewCLIGit(t, repo, "init", "--bare", "-q", firstPublication)
	runReviewCLIGit(t, repo, "remote", "add", "origin", firstPublication)
	runReviewCLIGit(t, repo, "config", "branch."+branch+".remote", "origin")
	runReviewCLIGit(t, repo, "config", "branch."+branch+".merge", "refs/heads/"+branch)
	output.Reset()
	err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", "pre-push"}, &output)
	if err == nil {
		t.Fatalf("pre-push published a first publication from an empty-base receipt\n%s", output.String())
	}
	if combined := output.String() + err.Error(); !strings.Contains(combined, want) {
		t.Fatalf("pre-push denial = %v, want typed refusal naming %q verbatim (1641)\n%s", err, want, output.String())
	}

	// Once that same delivery is published, pre-push has nothing left to
	// transfer. The gate stops answering for the receipt at all and reports the
	// empty publication range, while pre-PR -- which asks whether this receipt
	// may open a pull request, not whether a commit moves -- keeps refusing.
	runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/"+branch)
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", "pre-push"}, &output); err != nil {
		t.Fatalf("pre-push denied a push that delivers nothing: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)

	output.Reset()
	err = RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", "pre-pr", "--base-ref", "origin/" + branch}, &output)
	if err == nil {
		t.Fatalf("pre-pr from an empty-base receipt must be denied\n%s", output.String())
	}
	if combined := output.String() + err.Error(); !strings.Contains(combined, want) {
		t.Fatalf("pre-pr denial = %v, want typed refusal naming %q verbatim (1641)\n%s", err, want, output.String())
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
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
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
