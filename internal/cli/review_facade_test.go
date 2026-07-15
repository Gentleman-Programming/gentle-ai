package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/internal/sddstatus"
)

func TestReviewFacadeStartStagedProjectionFreezesOnlyIndex(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexTree := strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree"))

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Projection != reviewtransaction.ProjectionStaged {
		t.Fatalf("start projection = %q", started.Projection)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged || record.State.InitialSnapshot.CandidateTree != indexTree {
		t.Fatalf("staged authority = %#v, want index tree %s", record.State.InitialSnapshot, indexTree)
	}
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "future"}, io.Discard); err == nil || !strings.Contains(err.Error(), "unsupported review projection") {
		t.Fatalf("invalid projection error = %v", err)
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "staged candidate")
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged", "--base-ref", base, "--committed-only"}, &output); err != nil {
		t.Fatalf("staged base-diff continuation error = %v", err)
	}
	var continued ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &continued); err != nil {
		t.Fatal(err)
	}
	if continued.Action != "resumed" || continued.LineageID != started.LineageID || continued.Projection != reviewtransaction.ProjectionStaged {
		t.Fatalf("staged base-diff continuation = %#v", continued)
	}
}

func TestReviewFacadeStartReusesStagedAuthorityForCommittedBaseDiff(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("staged candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var staged ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "staged candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged divergence\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged", "--base-ref", base, "--committed-only", "--lineage", "staged-base-request"}, &output); err != nil {
		t.Fatal(err)
	}
	var resumed ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Action != "resumed" || resumed.LineageID != staged.LineageID || resumed.Projection != reviewtransaction.ProjectionStaged {
		t.Fatalf("staged committed base-diff reuse = %#v", resumed)
	}
}

func TestReviewFacadeStagedReceiptAllowsDeliveredTreePrePushAndPrePR(t *testing.T) {
	repo := initReviewCLIRepo(t)
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("staged candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"reviewed staged candidate"}})
	if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "staged candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged workspace divergence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		output.Reset()
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", string(gate), "--base-ref", "origin/" + branch}, &output); err != nil {
			t.Fatalf("%s delivered-tree validation: %v\n%s", gate, err, output.String())
		}
		assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
	}
}

func TestReviewFacadeCleanFlowReplacesOneCompactStateAndUsesOnlyReceipt(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	reviewing, err := store.Load()
	if err != nil || reviewing.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("reviewing compact authority = %#v, %v", reviewing, err)
	}
	assertCompactLineageFiles(t, store, []string{"review-state.json"})
	if _, err := os.Stat(filepath.Join(store.Dir, "events")); !os.IsNotExist(err) {
		t.Fatalf("compact start created event history: %v", err)
	}
	legacy, _ := reviewtransaction.AuthoritativeStore(context.Background(), repo, started.LineageID)
	if _, err := legacy.LoadChain(); !os.IsNotExist(err) {
		t.Fatalf("facade start wrote legacy v1 authority: %v", err)
	}

	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"focused review completed"}})
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath}, &output); err != nil {
		t.Fatal(err)
	}
	validating := decodeFacadeFinalize(t, output.Bytes())
	if validating.State != reviewtransaction.StateValidating || validating.StoreRevision == reviewing.Revision {
		t.Fatalf("validating result = %#v", validating)
	}
	loadedValidating, err := store.Load()
	if err != nil || loadedValidating.State.State != reviewtransaction.StateValidating {
		t.Fatalf("restart validating authority = %#v, %v", loadedValidating, err)
	}
	assertCompactLineageFiles(t, store, []string{"review-state.json"})

	evidencePath := filepath.Join(t.TempDir(), "tests.txt")
	if err := os.WriteFile(evidencePath, []byte("go test ./...: pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--evidence", evidencePath}, &output); err != nil {
		t.Fatal(err)
	}
	approved := decodeFacadeFinalize(t, output.Bytes())
	if approved.State != reviewtransaction.StateApproved || approved.ReceiptPath != store.ReceiptPath() {
		t.Fatalf("approved result = %#v", approved)
	}
	assertCompactLineageFiles(t, store, []string{"review-receipt.json", "review-state.json"})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo}, io.Discard); err != nil {
		t.Fatalf("terminal restart: %v", err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")

	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit} {
		output.Reset()
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output); err != nil {
			t.Fatalf("compact %s gate: %v\n%s", gate, err, output.String())
		}
		assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
	}
	output.Reset()
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", store.ReceiptPath(), "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("review validate rejected facade receipt: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)

	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	if err != nil {
		t.Fatal(err)
	}
	tampered := receipt
	tampered.FinalCandidateTree = strings.Repeat("0", len(tampered.FinalCandidateTree))
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), tampered); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err == nil {
		t.Fatal("tampered compact receipt authorized delivery")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateInvalidated)
	if err := reviewtransaction.WriteCompactReceiptAtomic(store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed after review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err == nil {
		t.Fatal("changed compact scope authorized delivery")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateScopeChanged)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePrePush, reviewtransaction.GatePrePR} {
		output.Reset()
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output); err != nil {
			t.Fatalf("compact %s gate: %v\n%s", gate, err, output.String())
		}
	}
}

func TestReviewFacadeStartSupportsCommittedBaseDiff(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	branch := strings.TrimSpace(runReviewCLIGit(t, repo, "symbolic-ref", "--short", "HEAD"))
	configureCLIReviewPublicationRemote(t, repo, branch)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("committed candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("intended untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")

	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, result.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff || record.State.InitialSnapshot.BaseTree == record.State.InitialSnapshot.CandidateTree {
		t.Fatalf("base diff snapshot = %#v", record.State.InitialSnapshot)
	}
	if !reflect.DeepEqual(record.State.InitialSnapshot.IntendedUntracked, []string{"notes.txt"}) {
		t.Fatalf("intended untracked = %v", record.State.InitialSnapshot.IntendedUntracked)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"committed diff reviewed"}})
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", result.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", result.LineageID, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", result.LineageID, "--gate", string(reviewtransaction.GatePrePR), "--base-ref", "origin/" + branch}, &output); err != nil {
		t.Fatalf("pre-pr base diff gate: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", result.LineageID, "--gate", string(reviewtransaction.GatePrePR), "--base-ref", "missing-reviewed-base"}, &output); err == nil {
		t.Fatal("unavailable pre-PR base was authorized")
	}
	var denied ReviewValidateResult
	if err := json.Unmarshal(output.Bytes(), &denied); err != nil {
		t.Fatal(err)
	}
	if denied.Allowed || denied.Context.LineageID != result.LineageID || denied.Context.PrePRBoundary == nil || denied.Context.PrePRBoundary.Selector != "missing-reviewed-base" || denied.Context.Denial == nil || denied.Context.Denial.Code != "unavailable" {
		t.Fatalf("facade unavailable base denial = %#v", denied)
	}
}

func TestReviewFacadeStartRequiresCommittedOnlyAndReusesEquivalentAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("committed candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("dirty change outside committed target\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base}, io.Discard); err == nil || !strings.Contains(err.Error(), "--committed-only") {
		t.Fatalf("dirty base-ref start error = %v, want committed-only acknowledgement", err)
	}
	if stores, err := reviewtransaction.CompactAuthorityLeaves(context.Background(), repo); err != nil || len(stores) != 0 {
		t.Fatalf("rejected start persisted authority = %v, %v", stores, err)
	}

	start := func(args ...string) ReviewFacadeStartResult {
		t.Helper()
		var output bytes.Buffer
		if err := RunReviewFacadeStart(append([]string{"--cwd", repo, "--base-ref", base, "--committed-only"}, args...), &output); err != nil {
			t.Fatal(err)
		}
		var result ReviewFacadeStartResult
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := start()
	if created.Action != "created" || !created.LensesRequired {
		t.Fatalf("created start = %#v", created)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, created.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	resumed := start("--lineage", "requested-different-lineage")
	if resumed.Action != "resumed" || !resumed.LensesRequired || resumed.LineageID != created.LineageID {
		t.Fatalf("equivalent start did not resume canonical authority: %#v", resumed)
	}
	after, err := store.Load()
	if err != nil || after.Revision != before.Revision || after.State.CorrectionBudget != before.State.CorrectionBudget {
		t.Fatalf("resume changed compact authority = %#v, %v", after, err)
	}

	policy := filepath.Join(t.TempDir(), "different-policy.md")
	if err := os.WriteFile(policy, []byte("different policy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var blockedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base, "--committed-only", "--policy", policy}, &blockedOutput); err != nil {
		t.Fatal(err)
	}
	var blocked ReviewFacadeStartResult
	if err := json.Unmarshal(blockedOutput.Bytes(), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.Action != "blocked-scope-action" || blocked.LensesRequired {
		t.Fatalf("changed policy start = %#v", blocked)
	}
	if unchanged, err := store.Load(); err != nil || unchanged.Revision != before.Revision {
		t.Fatalf("blocked start changed authority = %#v, %v", unchanged, err)
	}

	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"committed target reviewed"}})
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", created.LineageID, "--result", resultPath, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	reused := start()
	if reused.Action != "reuse-receipt" || reused.LensesRequired || reused.LineageID != created.LineageID {
		t.Fatalf("approved equivalent start = %#v", reused)
	}
	if err := os.WriteFile(store.ReceiptPath(), []byte("malformed receipt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var malformedOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base, "--committed-only"}, &malformedOutput); err != nil {
		t.Fatal(err)
	}
	var malformed ReviewFacadeStartResult
	if err := json.Unmarshal(malformedOutput.Bytes(), &malformed); err != nil {
		t.Fatal(err)
	}
	if malformed.Action != "blocked-scope-action" || malformed.LensesRequired {
		t.Fatalf("malformed receipt start = %#v", malformed)
	}
}

func TestReviewFacadeStartServiceTokenSelectsCanonicalHighRiskLenses(t *testing.T) {
	repo := initReviewCLIRepo(t)
	neutral := filepath.Join(repo, "neutral")
	if err := os.MkdirAll(neutral, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(neutral, "service-token.ts"), []byte("export const token = 'candidate'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hostile := initReviewCLIRepo(t)
	for name, value := range map[string]string{
		"GIT_DIR": filepath.Join(hostile, ".git"), "GIT_WORK_TREE": hostile,
		"GIT_COMMON_DIR": filepath.Join(hostile, ".git"), "GIT_INDEX_FILE": filepath.Join(hostile, ".git", "index"),
		"GIT_REPLACE_REF_BASE": filepath.Join(hostile, "replace"),
	} {
		t.Setenv(name, value)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(workingDirectory, repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	for index, cwd := range []string{repo, neutral, relative} {
		var output bytes.Buffer
		if err := RunReviewFacadeStart([]string{"--cwd", cwd, "--lineage", fmt.Sprintf("service-token-%d", index)}, &output); err != nil {
			t.Fatalf("facade start from %q: %v", cwd, err)
		}
		var started ReviewFacadeStartResult
		if err := json.Unmarshal(output.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		if record.State.RiskLevel != reviewtransaction.RiskHigh || !reflect.DeepEqual(record.State.SelectedLenses, want) {
			t.Fatalf("facade service-token state from %q = risk %q lenses %v, want high %v", cwd, record.State.RiskLevel, record.State.SelectedLenses, want)
		}
	}
}

func TestReviewFacadeDeniedGateRetainsObservedBoundaryWithoutAuthorizing(t *testing.T) {
	var output bytes.Buffer
	evaluation := reviewtransaction.NativeGateEvaluation{
		Result: reviewtransaction.GateInvalidated,
		Reason: "current repository target cannot be derived: explicit base is unavailable",
		Context: reviewtransaction.GateContext{
			Gate: reviewtransaction.GatePrePR, LineageID: "review-boundary-context", Generation: 1,
			PrePRBoundary: &reviewtransaction.PrePRBoundarySelection{
				Source: reviewtransaction.PrePRBoundaryExplicit, Selector: "reviewed-base", Commit: strings.Repeat("a", 40),
			},
			Denial: &reviewtransaction.GateDenial{Stage: "boundary-selection", Code: "unavailable"},
		},
	}
	if err := emitFacadeGateEvaluation(&output, evaluation); err == nil {
		t.Fatal("denied gate returned success")
	}
	var result ReviewValidateResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Allowed || result.Result != reviewtransaction.GateInvalidated || result.Context.PrePRBoundary == nil || result.Context.PrePRBoundary.Commit != strings.Repeat("a", 40) || result.Context.Denial == nil || result.Context.Denial.Code != "unavailable" {
		t.Fatalf("denied boundary result = %#v", result)
	}
}

func TestReviewFacadeStartRejectsInvalidBaseRefWithoutPersistingLineage(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "range", ref: "HEAD~1..HEAD"},
		{name: "missing ref", ref: "refs/heads/does-not-exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			lineage := "invalid-base-ref"
			err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage, "--base-ref", tt.ref}, io.Discard)
			if err == nil {
				t.Fatalf("base ref %q was accepted", tt.ref)
			}
			store, storeErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
			if storeErr != nil {
				t.Fatal(storeErr)
			}
			if _, statErr := os.Stat(store.Dir); !os.IsNotExist(statErr) {
				t.Fatalf("invalid base ref persisted lineage: %v", statErr)
			}
		})
	}
}

func TestReadFacadeReviewerResultsRejectsNonNativeFields(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "summary replaces claim", payload: `{"findings":[{"location":"x.go:1","severity":"CRITICAL","summary":"incorrect behavior","evidence_class":"deterministic","causal_disposition":"introduced","proof_refs":["test"]}],"evidence":["inspected candidate"]}`},
		{name: "top-level skill resolution", payload: `{"findings":[],"evidence":["inspected candidate"],"skill_resolution":"paths-injected"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "review.json")
			if err := os.WriteFile(path, []byte(tt.payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readFacadeReviewerResults([]string{path}); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("readFacadeReviewerResults() error = %v, want unknown field", err)
			}
		})
	}
}

func TestReviewFacadeCorrectionFlowResumesFromEachCompactIntermediateState(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"focused differential test failed on candidate"},
	})
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath}, &output); err != nil {
		t.Fatal(err)
	}
	if got := decodeFacadeFinalize(t, output.Bytes()); got.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("correction-required result = %#v", got)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	beforeForecast, _ := store.Load()
	classification := beforeForecast.State.Classifications["R3-001"]
	if classification.Causality != reviewtransaction.CausalIntroduced || beforeForecast.State.Outcomes["R3-001"] != reviewtransaction.OutcomeCorroborated || !reflect.DeepEqual(beforeForecast.State.FixFindingIDs, []string{"R3-001"}) {
		t.Fatalf("compact causal admission = %#v", beforeForecast.State)
	}
	ledgerFromState, err := reviewtransaction.CanonicalLedger(beforeForecast.State.Findings)
	if err != nil {
		t.Fatal(err)
	}
	ledgerFromLens, err := reviewtransaction.CanonicalLedger(beforeForecast.State.LensResults[0].Findings)
	if err != nil || !bytes.Equal(ledgerFromState, ledgerFromLens) {
		t.Fatalf("native compact ledger derivation differs: %v", err)
	}

	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--correction-lines", "2"}, &output); err != nil {
		t.Fatal(err)
	}
	forecasted, _ := store.Load()
	if forecasted.Revision == beforeForecast.Revision || forecasted.State.ProposedCorrectionLines == nil || *forecasted.State.ProposedCorrectionLines != 2 {
		t.Fatalf("forecasted compact authority = %#v", forecasted)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resumed := startFacadeReview(t, repo)
	if resumed.Action != "resumed" || resumed.LensesRequired || resumed.LineageID != started.LineageID || resumed.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("corrected in-scope start did not resume correction authority: %#v", resumed)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression test passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath}, &output); err != nil {
		t.Fatal(err)
	}
	if got := decodeFacadeFinalize(t, output.Bytes()); got.State != reviewtransaction.StateValidating {
		t.Fatalf("corrected validating result = %#v", got)
	}
	validating, _ := store.Load()
	if validating.State.ActualCorrectionLines == nil || *validating.State.ActualCorrectionLines != 2 || validating.State.FixDeltaHash == reviewtransaction.EmptyFixDeltaHash ||
		validating.State.OriginalCriteria == nil || validating.State.CorrectionRegression == nil ||
		!validating.State.OriginalCriteria.Passed || !validating.State.CorrectionRegression.Passed ||
		validating.State.OriginalCriteria.FixDeltaHash != validating.State.FixDeltaHash || validating.State.CorrectionRegression.FixDeltaHash != validating.State.FixDeltaHash {
		t.Fatalf("corrected compact authority = %#v", validating.State)
	}
	assertCompactLineageFiles(t, store, []string{"review-state.json"})

	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused and full tests: pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--evidence", evidencePath}, &output); err != nil {
		t.Fatal(err)
	}
	if got := decodeFacadeFinalize(t, output.Bytes()); got.State != reviewtransaction.StateApproved {
		t.Fatalf("corrected approved result = %#v", got)
	}
	assertCompactLineageFiles(t, store, []string{"review-receipt.json", "review-state.json"})
	reused := startFacadeReview(t, repo)
	if reused.Action != "reuse-receipt" || reused.LensesRequired || reused.LineageID != started.LineageID || reused.State != reviewtransaction.StateApproved {
		t.Fatalf("corrected approved target did not reuse receipt: %#v", reused)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("corrected compact gate: %v\n%s", err, output.String())
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "corrected delivery")
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base}, &output); err != nil {
		t.Fatal(err)
	}
	var committedReuse ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &committedReuse); err != nil {
		t.Fatal(err)
	}
	if committedReuse.Action != "reuse-receipt" || committedReuse.LensesRequired || committedReuse.LineageID != started.LineageID || committedReuse.State != reviewtransaction.StateApproved {
		t.Fatalf("equivalent committed corrected target did not reuse receipt: %#v", committedReuse)
	}
}

func TestReviewFacadeFinalizeRejectsCorrectionCreatedUntrackedPath(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}},
		Evidence: []string{"focused differential test failed on candidate"},
	})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath, "--correction-lines", "2"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "correction-evidence.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed for tracked.txt and correction-evidence.json"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression passed for tracked.txt and correction-evidence.json"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})

	err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "untracked") || !strings.Contains(err.Error(), "correction-evidence.json") {
		t.Fatalf("correction-created untracked path error = %v", err)
	}
	store, storeErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	record, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if record.State.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("rejected correction mutated authority to %q", record.State.State)
	}
	if _, statErr := os.Stat(store.ReceiptPath()); !os.IsNotExist(statErr) {
		t.Fatalf("rejected correction materialized receipt: %v", statErr)
	}

	if err := os.Remove(filepath.Join(repo, "correction-evidence.json")); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath}, io.Discard); err != nil {
		t.Fatalf("exact tracked correction: %v", err)
	}
	record, loadErr = store.Load()
	if loadErr != nil || record.State.State != reviewtransaction.StateValidating {
		t.Fatalf("exact correction authority = %#v, %v", record.State, loadErr)
	}
}

func TestRejectFacadeCorrectionUntrackedRespectsStagedProjection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "excluded.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := reviewtransaction.CompactState{
		InitialSnapshot: reviewtransaction.Snapshot{Projection: reviewtransaction.ProjectionStaged},
		CurrentSnapshot: reviewtransaction.Snapshot{IntendedUntracked: []string{}},
	}
	if err := rejectFacadeCorrectionUntracked(context.Background(), repo, state); err != nil {
		t.Fatalf("staged projection excluded workspace path: %v", err)
	}
	state.InitialSnapshot.Projection = reviewtransaction.ProjectionWorkspace
	if err := rejectFacadeCorrectionUntracked(context.Background(), repo, state); err == nil {
		t.Fatal("workspace projection accepted unreviewed correction path")
	}
}

func TestReviewFacadePersistsOverBudgetForecastAndActual(t *testing.T) {
	newCandidate := func(t *testing.T) (string, ReviewFacadeStartResult, string) {
		t.Helper()
		repo := initReviewCLIRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		started := startFacadeReview(t, repo)
		resultPath := filepath.Join(t.TempDir(), "review.json")
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
			Findings: []facadeFinding{{
				Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate regression",
				ProofRefs:     []string{"differential test fails only on candidate"},
				EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			}}, Evidence: []string{"focused differential test failed"},
		})
		return repo, started, resultPath
	}
	t.Run("forecast", func(t *testing.T) {
		repo, started, resultPath := newCandidate(t)
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath, "--correction-lines", "3"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		record, err := store.Load()
		if err != nil || record.State.State != reviewtransaction.StateEscalated || record.State.ProposedCorrectionLines == nil || *record.State.ProposedCorrectionLines != 3 || record.State.ActualCorrectionLines != nil {
			t.Fatalf("over-budget forecast state = %#v, %v", record.State, err)
		}
	})
	t.Run("actual", func(t *testing.T) {
		repo, started, resultPath := newCandidate(t)
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath, "--correction-lines", "2"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nfixed-one\nfixed-two\nthree\nfour\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		validationPath := filepath.Join(t.TempDir(), "validation.json")
		writeReviewCLIJSON(t, validationPath, facadeValidationResult{
			OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passes"}},
			CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"regression passes"}},
			FollowUps:            []reviewtransaction.FollowUp{},
		})
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath}, io.Discard); err != nil {
			t.Fatal(err)
		}
		store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		record, loadErr := store.Load()
		if loadErr != nil || record.State.State != reviewtransaction.StateEscalated || record.State.CumulativeCorrectionLines <= record.State.CorrectionBudget || len(record.State.CorrectionAttempts) != 1 {
			t.Fatalf("over-budget actual authority = %#v, %v", record.State, loadErr)
		}
		before := record.Revision
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--correction-lines", "1"}, io.Discard); err != nil {
			t.Fatal(err)
		}
		after, _ := store.Load()
		if after.Revision != before || after.State.State != reviewtransaction.StateEscalated {
			t.Fatalf("overflow authority resumed = %#v", after)
		}
	})
}

func TestReviewFacadeCompactRefuterAndHostileGitSelection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hostile := initReviewCLIRepo(t)
	for name, value := range map[string]string{
		"GIT_DIR": filepath.Join(hostile, ".git"), "GIT_WORK_TREE": hostile,
		"GIT_COMMON_DIR": filepath.Join(hostile, ".git"), "GIT_INDEX_FILE": filepath.Join(hostile, ".git", "index"),
	} {
		t.Setenv(name, value)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.State.InitialSnapshot.Paths, []string{"new.txt", "tracked.txt"}) || !reflect.DeepEqual(record.State.InitialSnapshot.IntendedUntracked, []string{"new.txt"}) {
		t.Fatalf("hostile environment selected wrong compact target: %#v", record.State.InitialSnapshot)
	}
}

func TestReviewStatusReportsActiveAuthorityWithoutChangingAuthorityFiles(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate behavior\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var report struct {
		Schema   string `json:"schema"`
		Complete bool   `json:"complete"`
		Entries  []struct {
			LineageID string `json:"lineage_id"`
			Status    string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != reviewtransaction.ReviewAuthorityStatusSchema || !report.Complete || len(report.Entries) != 1 ||
		report.Entries[0].LineageID != started.LineageID || report.Entries[0].Status != "active" {
		t.Fatalf("status report = %s", output.String())
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("review status mutated compact authority")
	}
}

func TestReviewFacadeHelpAndFlatCompatibilityPathsRemainAvailable(t *testing.T) {
	for _, subcommand := range []string{"start", "finalize", "validate", "status", "recover"} {
		var output bytes.Buffer
		if err := RunReview([]string{subcommand, "--help"}, &output); err != nil || !strings.Contains(output.String(), "Usage: gentle-ai review "+subcommand) {
			t.Fatalf("facade %s help: %v\n%s", subcommand, err, output.String())
		}
	}
	var validateHelp bytes.Buffer
	if err := RunReview([]string{"validate", "--help"}, &validateHelp); err != nil || !strings.Contains(validateHelp.String(), "--lineage") {
		t.Fatalf("review validate help must expose --lineage: %v\n%s", err, validateHelp.String())
	}
	for _, test := range []struct {
		run  func([]string, io.Writer) error
		want string
	}{
		{RunReviewStart, "Usage: gentle-ai review-start"}, {RunReviewStep, "Usage: gentle-ai review-step"},
		{RunReviewResume, "Usage: gentle-ai review-resume"}, {RunReviewValidate, "Usage: gentle-ai review-validate"},
		{RunReviewBundleExport, "Usage: gentle-ai review-bundle-export"}, {RunReviewBundleImport, "Usage: gentle-ai review-bundle-import"},
	} {
		var output bytes.Buffer
		if err := test.run([]string{"--help"}, &output); err != nil || !strings.Contains(output.String(), test.want) {
			t.Fatalf("flat compatibility help %q: %v\n%s", test.want, err, output.String())
		}
	}
}

func TestReviewSchemaExamplesMatchStrictFacadeContracts(t *testing.T) {
	for _, kind := range []string{"reviewer", "refuter", "validator"} {
		t.Run(kind, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunReview([]string{"schema", kind}, &output); err != nil {
				t.Fatal(err)
			}
			var document struct {
				Schema   string            `json:"$schema"`
				ID       string            `json:"$id"`
				Examples []json.RawMessage `json:"examples"`
			}
			if err := json.Unmarshal(output.Bytes(), &document); err != nil || document.Schema == "" || document.ID == "" || len(document.Examples) != 1 {
				t.Fatalf("schema document = %#v, %v", document, err)
			}
			path := filepath.Join(t.TempDir(), kind+".json")
			if err := os.WriteFile(path, document.Examples[0], 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "reviewer":
				if _, err := readFacadeReviewerResults([]string{path}); err != nil {
					t.Fatal(err)
				}
			case "refuter":
				var value facadeRefuterResult
				if err := readFacadeJSON(path, &value); err != nil {
					t.Fatal(err)
				}
			case "validator":
				var value facadeValidationResult
				if err := readFacadeJSON(path, &value); err != nil {
					t.Fatal(err)
				}
				if _, err := value.compact(reviewtransaction.EmptyFixDeltaHash, []string{}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestReviewerSchemaRequiresRuntimeMandatoryFindingEvidence(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			MinItems int `json:"minItems"`
			Items    struct {
				Required []string `json:"required"`
				AllOf    []struct {
					Then struct {
						Required []string `json:"required"`
					} `json:"then"`
				} `json:"allOf"`
				Properties map[string]struct {
					MinItems int `json:"minItems"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(reviewInputSchemas["reviewer"], &schema); err != nil {
		t.Fatal(err)
	}
	wantRequired := []string{"location", "severity", "claim", "proof_refs"}
	wantSevere := []string{"evidence_class", "causal_disposition"}
	if !reflect.DeepEqual(schema.Properties["findings"].Items.Required, wantRequired) || len(schema.Properties["findings"].Items.AllOf) != 1 || !reflect.DeepEqual(schema.Properties["findings"].Items.AllOf[0].Then.Required, wantSevere) || schema.Properties["evidence"].MinItems != 1 || schema.Properties["findings"].Items.Properties["proof_refs"].MinItems != 1 {
		t.Fatalf("reviewer schema requirements = %#v", schema)
	}

	for name, payload := range map[string]string{
		"missing location":              `{"findings":[{"severity":"CRITICAL","claim":"x","proof_refs":["proof"],"evidence_class":"deterministic","causal_disposition":"introduced"}],"evidence":["reviewed"]}`,
		"empty evidence":                `{"findings":[],"evidence":[]}`,
		"empty proof refs":              `{"findings":[{"location":"x.go:1","severity":"CRITICAL","claim":"x","proof_refs":[],"evidence_class":"deterministic","causal_disposition":"introduced"}],"evidence":["reviewed"]}`,
		"missing severe classification": `{"findings":[{"location":"x.go:1","severity":"CRITICAL","claim":"x","proof_refs":["proof"]}],"evidence":["reviewed"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reviewer.json")
			if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			results, err := readFacadeReviewerResults([]string{path})
			if err != nil {
				return
			}
			state := reviewtransaction.CompactState{SelectedLenses: []string{reviewtransaction.LensReliability}}
			input, err := prepareCompactReviewerResults(state, results, facadeRefuterResult{})
			if err == nil {
				err = state.CompleteReview(input)
			}
			if err == nil {
				t.Fatal("runtime accepted schema-invalid reviewer input")
			}
		})
	}
}

func TestReviewSchemasRequireConcreteEvidenceStrings(t *testing.T) {
	for _, kind := range []string{"reviewer", "refuter", "validator"} {
		if !bytes.Contains(reviewInputSchemas[kind], []byte(`"pattern":"\\S"`)) {
			t.Fatalf("%s schema lacks concrete-evidence pattern", kind)
		}
	}
}

func TestReviewFacadeRejectsMalformedInputsWithoutConsumingIterativeCorrection(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	assertUnchanged := func(before string, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("malformed input was accepted")
		}
		after, loadErr := store.Load()
		if loadErr != nil || after.Revision != before {
			t.Fatalf("malformed input changed authority: %v, %#v", loadErr, after)
		}
	}
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"findings":[],"evidence":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	record, _ := store.Load()
	assertUnchanged(record.Revision, RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", malformed}, io.Discard))

	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}, EvidenceClass: reviewtransaction.EvidenceInferential, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed once"}})
	if err := os.WriteFile(malformed, []byte(`{"results":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertUnchanged(record.Revision, RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", reviewer, "--refuter", malformed}, io.Discard))
	refuter := filepath.Join(t.TempDir(), "refuter.json")
	writeReviewCLIJSON(t, refuter, facadeRefuterResult{Results: []facadeRefuterOutcome{{FindingID: "R3-001", Outcome: reviewtransaction.OutcomeCorroborated, ProofRefs: []string{"independent reproduction"}}}})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", reviewer, "--refuter", refuter, "--correction-lines", "6"}, io.Discard); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n01\n02\n03\nfirst-fix\n05\n06\n07\n08\n09\n10\n11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(malformed, []byte(`{"original_criteria":{"passed":false,"evidence":["failed"]},"correction_regression":{"passed":false,"evidence":["failed"]},"follow_ups":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	record, _ = store.Load()
	assertUnchanged(record.Revision, RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", malformed}, io.Discard))
	validator := filepath.Join(t.TempDir(), "validator.json")
	writeReviewCLIJSON(t, validator, facadeValidationResult{OriginalCriteria: facadeValidationCheck{Passed: false, Evidence: []string{"acceptance still fails"}}, CorrectionRegression: facadeValidationCheck{Passed: false, Evidence: []string{"regression still fails"}}, FollowUps: []reviewtransaction.FollowUp{}})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validator}, io.Discard); err != nil {
		t.Fatal(err)
	}
	failed, _ := store.Load()
	if failed.State.State != reviewtransaction.StateCorrectionRequired || failed.State.CumulativeCorrectionLines <= 0 || len(failed.State.LensResults) != 1 {
		t.Fatalf("failed validation state = %#v", failed.State)
	}

	remaining := failed.State.CorrectionBudget - failed.State.CumulativeCorrectionLines
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--correction-lines", fmt.Sprint(remaining)}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n01\n02\n03\nfixed\n05\n06\n07\n08\n09\n10\n11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReviewCLIJSON(t, validator, facadeValidationResult{OriginalCriteria: facadeValidationCheck{Passed: true, Evidence: []string{"acceptance passes"}}, CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"regression passes"}}, FollowUps: []reviewtransaction.FollowUp{}})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validator}, io.Discard); err != nil {
		t.Fatal(err)
	}
	corrected, _ := store.Load()
	if corrected.State.State != reviewtransaction.StateValidating || corrected.State.CumulativeCorrectionLines > corrected.State.CorrectionBudget || len(corrected.State.CorrectionAttempts) != 2 || len(corrected.State.LensResults) != 1 {
		t.Fatalf("corrected retry state = %#v", corrected.State)
	}
}

func TestReviewRecoverCreatesSuccessorAndDiscoveryRejectsHistoricalAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"reviewed"}})
	if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	predecessorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	predecessor, _ := predecessorStore.Load()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-recovered",
		"--disposition", "scope_changed", "--reason", "candidate changed", "--actor", "maintainer"}, &output); err != nil {
		t.Fatal(err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.LineageID != "review-recovered" || recovered.Recovery.PredecessorRevision != predecessor.Revision {
		t.Fatalf("recovered = %#v", recovered)
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err == nil || !strings.Contains(output.String(), "superseded") {
		t.Fatalf("historical authority validation = %v\n%s", err, output.String())
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", recovered.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", recovered.LineageID, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("successor validation: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
}

func TestReviewRecoverRetainsStagedProjectionAndIgnoresUnstagedWorkspace(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--projection", "staged"}, &output); err != nil {
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
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	predecessorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	predecessor, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("recovered staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	stagedTree := strings.TrimSpace(runReviewCLIGit(t, repo, "write-tree"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-staged-recovered",
		"--disposition", "scope_changed", "--reason", "staged scope changed", "--actor", "maintainer"}, &output); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-staged-recovered")
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged || recovered.State.InitialSnapshot.CandidateTree != stagedTree {
		t.Fatalf("recovered staged authority = %#v, want index tree %s", recovered.State.InitialSnapshot, stagedTree)
	}
}

func TestReviewBindSDDRequiresExplicitInputs(t *testing.T) {
	err := RunReview([]string{"bind-sdd", "--cwd", t.TempDir(), "--change", "thin", "--lineage", "approved"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "expected-binding-revision") {
		t.Fatalf("bind-sdd missing explicit CAS input error = %v", err)
	}
}

func TestReviewBindSDDAcceptsEqualsFormForEmptyExpectedRevision(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := filepath.Join(repo, "openspec", "changes", "thin")
	for path, content := range map[string]string{"tasks.md": "- [x] 1.1 Done\n", "proposal.md": "# Proposal\n", "design.md": "# Design\n", "specs/binding/spec.md": "# Spec\n"} {
		fullPath := filepath.Join(change, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	started := startFacadeReview(t, repo)
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidence}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"bind-sdd", "--cwd", repo, "--change", "thin", "--lineage", started.LineageID, "--expected-binding-revision="}, io.Discard); err != nil {
		t.Fatalf("equals-form expected revision was rejected: %v", err)
	}
}

func TestReviewBindSDDFeedsSelectedSDDStatusRuntime(t *testing.T) {
	repo := initReviewCLIRepo(t)
	change := filepath.Join(repo, "openspec", "changes", "thin")
	if err := os.MkdirAll(change, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(change, "tasks.md"), []byte("- [x] 1.1 Done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"proposal.md":           "# Proposal\n",
		"design.md":             "# Design\n",
		"specs/binding/spec.md": "# Spec\n",
	} {
		fullPath := filepath.Join(change, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	started := startFacadeReview(t, repo)
	evidence := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidence, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidence}, io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "-A")
	runReviewCLIGit(t, repo, "commit", "-qm", "exact approved SDD candidate")
	var output bytes.Buffer
	if err := RunReview([]string{"bind-sdd", "--cwd", repo, "--change", "thin", "--lineage", started.LineageID, "--expected-binding-revision", ""}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "gentle-ai.sdd-review-binding/v1") {
		t.Fatalf("binding output = %s", output.String())
	}
	output.Reset()
	if err := RunSDDStatus([]string{"thin", "--cwd", repo, "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var status sddstatus.Status
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.NextRecommended != "verify" || status.Dependencies.Verify != sddstatus.DependencyReady || status.Dependencies.Archive != sddstatus.DependencyBlocked || status.ReviewGate == nil || status.ReviewGate.Result != reviewtransaction.GateAllow {
		t.Fatalf("bound runtime status = %#v", status)
	}
}

func TestLegacyV1LineageRemainsReadableButRejectsAppend(t *testing.T) {
	fixture := newLegacyCLIFixture(t, "legacy-read-only")
	var resumed bytes.Buffer
	if err := RunReviewResume([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage}, &resumed); err != nil {
		t.Fatalf("legacy resume: %v", err)
	}
	input := filepath.Join(t.TempDir(), "input.json")
	if err := os.WriteFile(input, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RunReviewStep([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage, "--operation", "freeze-findings", "--input", input}, io.Discard)
	if !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) {
		t.Fatalf("legacy append error = %v", err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.lineage}, io.Discard); !errors.Is(err, reviewtransaction.ErrLegacyReadOnly) {
		t.Fatalf("facade legacy mutation error = %v", err)
	}
}

func TestCompactTransportCommandsRoundTripWithoutEventReconstruction(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"review completed"}})
	if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
	bundlePath := filepath.Join(t.TempDir(), "compact-transport.json")
	if err := RunReviewBundleExport([]string{"--cwd", repo, "--lineage", started.LineageID, "--out", bundlePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"events"`) || !strings.Contains(string(payload), reviewtransaction.CompactTransportSchema) {
		t.Fatalf("compact transport reintroduced event history: %s", payload)
	}
	clone := filepath.Join(t.TempDir(), "clone")
	runReviewCLIGit(t, repo, "clone", "--no-local", repo, clone)
	if err := RunReviewBundleImport([]string{"--cwd", clone, "--bundle", bundlePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	cloneStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), clone, started.LineageID)
	if _, err := cloneStore.Load(); err != nil {
		t.Fatal(err)
	}
	assertCompactLineageFiles(t, cloneStore, []string{"review-receipt.json", "review-state.json"})
}

func TestCompactTransportAllowsCorrectedPrePushWithoutTransientBaseObject(t *testing.T) {
	source := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, source)
	sourceStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), source, started.LineageID)
	initial, err := sourceStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
		Findings: []facadeFinding{{
			Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
			ProofRefs:     []string{"differential test passes on base and fails on candidate"},
			EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
		}}, Evidence: []string{"focused differential test failed on candidate"},
	})
	if err := RunReviewFacadeFinalize([]string{"--cwd", source, "--result", resultPath, "--correction-lines", "2"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validationPath := filepath.Join(t.TempDir(), "validation.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, validationPath, facadeValidationResult{
		OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance test passed"}},
		CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression test passed"}},
		FollowUps:            []reviewtransaction.FollowUp{},
	})
	if err := os.WriteFile(evidencePath, []byte("focused and full tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", source, "--validation", validationPath, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, source, "add", "tracked.txt")
	runReviewCLIGit(t, source, "commit", "-qm", "corrected candidate")
	sourceRecord, err := sourceStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	sourceReceiptPayload, err := os.ReadFile(sourceStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(t.TempDir(), "corrected-transport.json")
	if err := RunReviewBundleExport([]string{"--cwd", source, "--lineage", started.LineageID, "--out", bundlePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	clone := filepath.Join(t.TempDir(), "clone")
	runReviewCLIGit(t, source, "clone", "--no-local", source, clone)
	runReviewCLIGit(t, source, "branch", "reviewed-base", "HEAD^")
	for _, tree := range []string{initial.State.InitialSnapshot.CandidateTree, sourceRecord.State.CurrentSnapshot.BaseTree} {
		command := exec.Command("git", "-C", clone, "cat-file", "-e", tree+"^{tree}")
		if err := command.Run(); err == nil {
			t.Fatalf("clean clone unexpectedly retained dangling intermediate tree %s", tree)
		}
	}
	if err := RunReviewBundleImport([]string{"--cwd", clone, "--bundle", bundlePath}, io.Discard); err != nil {
		t.Fatalf("corrected compact import: %v", err)
	}
	cloneStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), clone, started.LineageID)
	cloneRecord, err := cloneStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	cloneReceiptPayload, err := os.ReadFile(cloneStore.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cloneRecord, sourceRecord) || !bytes.Equal(cloneReceiptPayload, sourceReceiptPayload) {
		t.Fatal("corrected compact recovery changed state or receipt")
	}
	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{"--cwd", clone, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePrePush), "--base-ref", "origin/reviewed-base"}, &output); err != nil {
		t.Fatalf("corrected recovered gate: %v\n%s", err, output.String())
	}
	var denied ReviewValidateResult
	if err := json.Unmarshal(output.Bytes(), &denied); err != nil {
		t.Fatal(err)
	}
	if !denied.Allowed || !denied.Context.BaseRelationshipValid {
		t.Fatalf("corrected recovered gate = %#v", denied)
	}
}

func startFacadeReview(t *testing.T, repo string) ReviewFacadeStartResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func decodeFacadeFinalize(t *testing.T, payload []byte) ReviewFacadeFinalizeResult {
	t.Helper()
	var result ReviewFacadeFinalizeResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertCompactLineageFiles(t *testing.T, store reviewtransaction.CompactStore, want []string) {
	t.Helper()
	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Name()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compact lineage files = %v, want %v", got, want)
	}
}
