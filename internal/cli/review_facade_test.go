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
	"runtime"
	"slices"
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

func TestReviewFacadeStartEmitsCaptureBindingInputs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if started.Action != "created" || started.TargetIdentity != record.State.InitialSnapshot.Identity {
		t.Fatalf("start target identity = %q, want frozen %q (action %q)", started.TargetIdentity, record.State.InitialSnapshot.Identity, started.Action)
	}
	if len(started.SelectedLenses) == 0 || len(started.LensBindings) != len(started.SelectedLenses) {
		t.Fatalf("start lens bindings = %#v, want one per selected lens %v", started.LensBindings, started.SelectedLenses)
	}
	for index, binding := range started.LensBindings {
		if binding.Lens != started.SelectedLenses[index] || binding.Order != index {
			t.Fatalf("lens binding[%d] = %#v, want {%q %d}", index, binding, started.SelectedLenses[index], index)
		}
	}
	resumed := startFacadeReview(t, repo)
	if resumed.Action != "resumed" || resumed.TargetIdentity != started.TargetIdentity ||
		!reflect.DeepEqual(resumed.LensBindings, started.LensBindings) {
		t.Fatalf("resumed binding inputs = %#v, want %#v", resumed, started)
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
	assertCompactLineageFiles(t, store, []string{"finalize-attempt-journal.json", "review-state.json"})

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
	assertCompactLineageFiles(t, store, []string{"finalize-attempt-journal.json", "review-receipt.json", "review-state.json"})
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
	tamperedPayload, err := json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.ReceiptPath(), append(tamperedPayload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err == nil {
		t.Fatal("tampered compact receipt authorized delivery")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateInvalidated)
	if err := os.WriteFile(store.ReceiptPath(), receiptPayload, 0o644); err != nil {
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
	t.Chdir(repo)
	want := []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	for index, cwd := range []string{repo, neutral, "."} {
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

func TestReviewFacadeStartProvableShellAndModeRiskSelectsCanonical4R(t *testing.T) {
	want := []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}
	tests := []struct {
		name  string
		setup func(t *testing.T, repo string)
	}{
		{
			name: "shell source",
			setup: func(t *testing.T, repo string) {
				if err := os.WriteFile(filepath.Join(repo, "run.sh"), []byte("printf '%s\\n' safe\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "GitHub workflow",
			setup: func(t *testing.T, repo string) {
				if err := os.MkdirAll(filepath.Join(repo, ".github", "workflows"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeReviewStartCandidate(t, repo, ".github/workflows/ci.yml", "jobs: {}\n", 0o644)
			},
		},
		{
			name: "mode only",
			setup: func(t *testing.T, repo string) {
				runReviewCLIGit(t, repo, "config", "core.filemode", "true")
				if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tt.name == "mode only" {
				t.Skip("Git worktree executable-bit transitions are POSIX-only")
			}
			repo := initReviewCLIRepo(t)
			tt.setup(t, repo)
			var output bytes.Buffer
			if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
				t.Fatal(err)
			}
			var started ReviewFacadeStartResult
			if err := json.Unmarshal(output.Bytes(), &started); err != nil {
				t.Fatal(err)
			}
			if started.RiskLevel != reviewtransaction.RiskHigh || !reflect.DeepEqual(started.SelectedLenses, want) {
				t.Fatalf("start risk/lenses = %q/%v, want high/%v", started.RiskLevel, started.SelectedLenses, want)
			}
		})
	}
}

func TestReviewFacadeStartUnnegotiatedJSONFieldSetRemainsCompatible(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"action", "changed_files", "changed_lines", "correction_budget", "lens_bindings", "lenses_required", "lineage_id", "operation", "projection", "risk_level", "selected_lenses", "state", "target_identity"}
	got := make([]string, 0, len(fields))
	for field := range fields {
		got = append(got, field)
	}
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unnegotiated start fields = %v, want %v", got, want)
	}
}

func TestReviewFacadeFinalizeReceiptPublicationFailureIsExactlyReplayable(t *testing.T) {
	fixture := prepareFacadeReceiptPending(t)
	if fixture.diagnostic.MutationOutcome != "committed" || fixture.diagnostic.Replayability != "exact_replay_safe" || fixture.diagnostic.LineageID != fixture.started.LineageID || !strings.HasPrefix(fixture.diagnostic.RequestDigest, "sha256:") {
		t.Fatalf("receipt publication diagnostic = %#v", fixture.diagnostic)
	}
	receipt, err := fixture.pending.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	pendingAttempt, err := fixture.store.PendingFinalizeAttempt()
	if err != nil || pendingAttempt == nil {
		t.Fatalf("pending finalize attempt = %#v, %v", pendingAttempt, err)
	}
	wantDigest := pendingAttempt.Request.RequestDigest
	if fixture.diagnostic.RequestDigest != wantDigest {
		t.Fatalf("request digest = %q, want %q", fixture.diagnostic.RequestDigest, wantDigest)
	}
	if _, err := os.Stat(fixture.store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("failed publication created receipt: %v", err)
	}

	original := writeCompactFacadeReceipt
	writes := 0
	writeCompactFacadeReceipt = func(_ context.Context, store reviewtransaction.CompactStore, got reviewtransaction.CompactReceipt) error {
		path := store.ReceiptPath()
		writes++
		if !reflect.DeepEqual(got, receipt) {
			t.Fatalf("replay receipt = %#v, want %#v", got, receipt)
		}
		return reviewtransaction.WriteCompactReceiptAtomic(path, got)
	}
	defer func() { writeCompactFacadeReceipt = original }()
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &output); err != nil {
		t.Fatalf("exact receipt replay: %v", err)
	}
	if writes != 1 {
		t.Fatalf("receipt replay writes = %d, want 1", writes)
	}
	after, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != fixture.pending.Revision || !reflect.DeepEqual(after.State, fixture.pending.State) {
		t.Fatalf("receipt replay mutated authority: before %#v after %#v", fixture.pending, after)
	}
	result := decodeFacadeFinalize(t, output.Bytes())
	if result.LineageID != fixture.started.LineageID || result.StoreRevision != fixture.pending.Revision || result.State != reviewtransaction.StateApproved {
		t.Fatalf("receipt replay result = %#v", result)
	}
}

func TestReviewFacadeFinalizeCompletesPendingJournalAfterReceiptPublication(t *testing.T) {
	fixture := prepareFacadeReceiptPending(t)
	receipt, err := fixture.pending.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, &output); err != nil {
		t.Fatalf("lineage-only replay after receipt publication: %v", err)
	}
	if pending, err := fixture.store.PendingFinalizeAttempt(); err != nil || pending != nil {
		t.Fatalf("receipt replay left pending finalize journal: %#v, %v", pending, err)
	}
	result := decodeFacadeFinalize(t, output.Bytes())
	if result.State != reviewtransaction.StateApproved || result.StoreRevision != fixture.pending.Revision {
		t.Fatalf("receipt replay result = %#v", result)
	}
}

func TestReviewFacadeFinalizeResyncsCompletedJournalBeforeTerminalReplay(t *testing.T) {
	fixture := prepareFacadeReceiptPending(t)
	receipt, err := fixture.pending.State.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.WriteCompactReceiptAtomic(fixture.store.ReceiptPath(), receipt); err != nil {
		t.Fatal(err)
	}
	pending, err := fixture.store.PendingFinalizeAttempt()
	if err != nil || pending == nil {
		t.Fatalf("pending finalize attempt = %#v, %v", pending, err)
	}
	if err := fixture.store.MarkFinalizeAttemptReceiptPublished(pending.Request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CompleteFinalizeAttempt(pending.Request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	original := reviewFacadeSyncDirectory
	syncs := 0
	reviewFacadeSyncDirectory = func(string) error {
		syncs++
		return nil
	}
	t.Cleanup(func() { reviewFacadeSyncDirectory = original })
	if err := RunReviewFacadeFinalize([]string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if syncs != 1 {
		t.Fatalf("completed journal directory syncs = %d, want 1", syncs)
	}
}

func TestReviewFacadeFinalizePlannedTransitionInterruptionResumesWithoutDuplicateCommit(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate"}})
	args := []string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}
	interrupted := errors.New("interrupt after durable transition plan")
	original := reviewFacadePlannedTransitionHook
	reviewFacadePlannedTransitionHook = func(_ context.Context, _ string, operation, _ string) error {
		if operation == "review/complete-review" {
			return interrupted
		}
		return nil
	}
	t.Cleanup(func() { reviewFacadePlannedTransitionHook = original })
	if err := RunReviewFacadeFinalize(args, io.Discard); !errors.Is(err, interrupted) {
		t.Fatalf("planned transition interruption = %v", err)
	}
	planned, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if planned.Revision != before.Revision || planned.State.State != reviewtransaction.StateReviewing {
		t.Fatalf("planned interruption committed authority: before %#v after %#v", before, planned)
	}
	pending, err := store.PendingFinalizeAttempt()
	if err != nil || pending == nil || len(pending.Transitions) == 0 || pending.Transitions[0].Operation != "review/complete-review" {
		t.Fatalf("planned interruption journal = %#v, %v", pending, err)
	}
	reviewFacadePlannedTransitionHook = original
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
		t.Fatalf("exact replay after planned interruption: %v", err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != reviewtransaction.StateValidating || after.Revision == before.Revision {
		t.Fatalf("resumed authority = %#v", after)
	}
	pending, err = store.PendingFinalizeAttempt()
	if err != nil || pending != nil {
		t.Fatalf("successful replay left pending attempt: %#v, %v", pending, err)
	}
}

func TestReviewFacadeFinalizeReceiptReplayRejectsNonExactAndUnsafeRequests(t *testing.T) {
	t.Run("explicit lineage is required", func(t *testing.T) {
		fixture := prepareFacadeReceiptPending(t)
		assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo})
	})
	t.Run("mutation inputs are rejected", func(t *testing.T) {
		fixture := prepareFacadeReceiptPending(t)
		assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID, "--evidence", fixture.evidencePath})
	})
	t.Run("malformed existing receipt is not overwritten", func(t *testing.T) {
		fixture := prepareFacadeReceiptPending(t)
		malformed := []byte("malformed receipt\n")
		if err := os.WriteFile(fixture.store.ReceiptPath(), malformed, 0o600); err != nil {
			t.Fatal(err)
		}
		assertFacadeReceiptReplayRejected(t, fixture, []string{"--cwd", fixture.repo, "--lineage", fixture.started.LineageID})
		got, err := os.ReadFile(fixture.store.ReceiptPath())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, malformed) {
			t.Fatalf("unsafe replay overwrote existing receipt: %q", got)
		}
	})
}

func TestReviewFacadeDeniedGateRetainsObservedBoundaryWithoutAuthorizing(t *testing.T) {
	var output bytes.Buffer
	evaluation := reviewtransaction.NativeGateEvaluation{
		Result: reviewtransaction.GateInvalidated,
		Reason: "current repository target cannot be derived: explicit base is unavailable",
		Cause:  &reviewtransaction.GitCommandTimeoutError{Cause: context.DeadlineExceeded},
		Context: reviewtransaction.GateContext{
			Gate: reviewtransaction.GatePrePR, LineageID: "review-boundary-context", Generation: 1,
			PrePRBoundary: &reviewtransaction.PrePRBoundarySelection{
				Source: reviewtransaction.PrePRBoundaryExplicit, Selector: "reviewed-base", Commit: strings.Repeat("a", 40),
			},
			Denial: &reviewtransaction.GateDenial{Stage: "boundary-selection", Code: "unavailable"},
		},
	}
	if err := emitFacadeGateEvaluation(&output, evaluation); !errors.Is(err, context.DeadlineExceeded) {
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
	for attempt := 0; attempt < 2; attempt++ {
		if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", resultPath}, io.Discard); err == nil || !strings.Contains(err.Error(), "only while the authority is reviewing") {
			t.Fatalf("replayed reviewer result attempt %d = %v", attempt+1, err)
		}
	}
	afterRejectedReplay, _ := store.Load()
	if afterRejectedReplay.Revision != beforeForecast.Revision || !reflect.DeepEqual(afterRejectedReplay.State, beforeForecast.State) {
		t.Fatal("rejected reviewer result replay changed authority")
	}
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
	assertCompactLineageFiles(t, store, []string{"finalize-attempt-journal.json", "review-state.json"})

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
	assertCompactLineageFiles(t, store, []string{"finalize-attempt-journal.json", "review-receipt.json", "review-state.json"})
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

func TestReviewFacadeEscalatesFalseIntroducedFindingOutsideGenesis(t *testing.T) {
	repo := initReviewCLIRepo(t)
	legacyDir := filepath.Join(repo, "internal", "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "unsafe.go"), []byte("package legacy\n\nfunc ParseLimit(value int) int {\n\tif value < 0 { panic(\"negative limit\") }\n\treturn value\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "internal/legacy/unsafe.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "add unchanged production path")
	candidateDir := filepath.Join(repo, "internal", "candidate")
	if err := os.MkdirAll(candidateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidateDir, "feature.go"), []byte("package candidate\n\nfunc Enabled() bool { return true }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "internal/legacy/unsafe.go:4", Severity: "CRITICAL", Claim: "negative input panics in the unchanged parser",
		ProofRefs: []string{"the frozen candidate deterministically reproduces the panic"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reproduced the defect without candidate-causality evidence"}})
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", reviewer}, &output); err != nil {
		t.Fatal(err)
	}
	if got := decodeFacadeFinalize(t, output.Bytes()); got.State != reviewtransaction.StateEscalated {
		t.Fatalf("false introduced result = %#v", got)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	classification := record.State.Classifications["R3-001"]
	if !reflect.DeepEqual(record.State.GenesisPaths, []string{"internal/candidate/feature.go"}) || classification.Causality != reviewtransaction.CausalUnknown || record.State.Outcomes["R3-001"] != reviewtransaction.OutcomeInconclusive || len(record.State.FixFindingIDs) != 0 || record.State.ProposedCorrectionLines != nil {
		t.Fatalf("false introduced routing = %#v", record.State)
	}
}

func TestReviewFacadeStartCannotResetActiveCorrectionBudget(t *testing.T) {
	tests := []struct {
		name       string
		negotiated bool
	}{
		{name: "pre-forecast raw"},
		{name: "pre-forecast negotiated", negotiated: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			write := func(value string) {
				t.Helper()
				content := fmt.Sprintf("base\none\ntwo\nthree\n%s\n", value)
				if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("wrong")
			started := startFacadeReview(t, repo)
			reviewer := filepath.Join(t.TempDir(), "reviewer.json")
			writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{
				Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"},
				EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
			}}, Evidence: []string{"focused differential failure"}})
			if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--result", reviewer}, io.Discard); err != nil {
				t.Fatal(err)
			}
			store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			before, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if before.State.State != reviewtransaction.StateCorrectionRequired || len(before.State.CorrectionAttempts) != 0 || before.State.CumulativeCorrectionLines != 0 {
				t.Fatalf("predecessor fixture = %#v", before.State)
			}
			write("second-byte-only-edit")

			var statusOutput bytes.Buffer
			if err := RunReviewStatus([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--lineage", started.LineageID}, &statusOutput); err != nil {
				t.Fatal(err)
			}
			var status ReviewTargetStatusResult
			if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil || status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateCorrectionRequired || status.Action != reviewtransaction.TargetStatusActionFinalize {
				t.Errorf("in-genesis correction status = %#v", status)
			}

			var output bytes.Buffer
			if tt.negotiated {
				err = RunReview([]string{"start", "--cwd", repo, "--contract", ReviewIntegrationContractV1}, &output)
			} else {
				err = RunReviewFacadeStart([]string{"--cwd", repo}, &output)
			}
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Action    string                  `json:"action"`
				LineageID string                  `json:"lineage_id"`
				State     reviewtransaction.State `json:"state"`
			}
			if err := json.Unmarshal(output.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Action != string(reviewtransaction.CompactStartBlocked) || got.LineageID != started.LineageID || got.State != reviewtransaction.StateCorrectionRequired {
				t.Errorf("in-genesis correction START = %#v", got)
			}
			after, _ := store.Load()
			stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
			if discoverErr != nil || after.Revision != before.Revision || after.State.CumulativeCorrectionLines != 0 || len(after.State.CorrectionAttempts) != 0 || len(stores) != 1 {
				t.Fatalf("START reset or mutated correction authority: before=%#v after=%#v stores=%d err=%v", before, after, len(stores), discoverErr)
			}
		})
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
			MinItems int      `json:"minItems"`
			Enum     []string `json:"enum"`
			Items    struct {
				Not struct {
					Pattern string `json:"pattern"`
				} `json:"not"`
				Required []string `json:"required"`
				AllOf    []struct {
					Then struct {
						Required []string `json:"required"`
					} `json:"then"`
				} `json:"allOf"`
				Properties map[string]struct {
					MinItems int `json:"minItems"`
					Items    struct {
						Not struct {
							Pattern string `json:"pattern"`
						} `json:"not"`
					} `json:"items"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(reviewInputSchemas["reviewer"], &schema); err != nil {
		t.Fatal(err)
	}
	wantRequired := []string{"location", "severity", "claim", "proof_refs"}
	wantSevere := []string{"evidence_class", "causal_disposition"}
	wantLenses := []string{
		"risk", "resilience", "readability", "reliability",
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	if !reflect.DeepEqual(schema.Properties["findings"].Items.Required, wantRequired) || len(schema.Properties["findings"].Items.AllOf) != 1 || !reflect.DeepEqual(schema.Properties["findings"].Items.AllOf[0].Then.Required, wantSevere) || schema.Properties["evidence"].MinItems != 1 || schema.Properties["findings"].Items.Properties["proof_refs"].MinItems != 1 {
		t.Fatalf("reviewer schema requirements = %#v", schema)
	}
	if !reflect.DeepEqual(schema.Properties["lens"].Enum, wantLenses) {
		t.Fatalf("reviewer schema lens enum = %v, want %v", schema.Properties["lens"].Enum, wantLenses)
	}
	proofSentinels := schema.Properties["findings"].Items.Properties["proof_refs"].Items.Not.Pattern
	evidenceSentinels := schema.Properties["evidence"].Items.Not.Pattern
	if proofSentinels == "" || proofSentinels != evidenceSentinels {
		t.Fatalf("reviewer schema sentinel parity = proof %q evidence %q", proofSentinels, evidenceSentinels)
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

func TestReviewFacadeRejectsMalformedInputsWithoutConsumingTerminalValidator(t *testing.T) {
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
	if failed.State.State != reviewtransaction.StateEscalated || failed.State.CumulativeCorrectionLines <= 0 || len(failed.State.LensResults) != 1 || failed.State.OriginalCriteria == nil || failed.State.CorrectionRegression == nil {
		t.Fatalf("failed validation state = %#v", failed.State)
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	if err != nil || receipt.TerminalState != reviewtransaction.TerminalEscalated || receipt.FixDeltaHash != failed.State.FixDeltaHash {
		t.Fatalf("failed validation receipt = %#v, %v", receipt, err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--correction-lines", "1"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	replayed, _ := store.Load()
	if replayed.Revision != failed.Revision || replayed.State.State != reviewtransaction.StateEscalated || len(replayed.State.CorrectionAttempts) != 1 {
		t.Fatalf("terminal failed validator replay = %#v", replayed)
	}
}

func TestReviewRecoverCreatesSuccessorAndDiscoveryRejectsHistoricalAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	resultPath := filepath.Join(t.TempDir(), "review.json")
	cleanResultPath := filepath.Join(t.TempDir(), "clean-review.json")
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{{Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "candidate regression", ProofRefs: []string{"candidate-only failure"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed"}})
	writeReviewCLIJSON(t, cleanResultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"reviewed"}})
	if err := os.WriteFile(evidencePath, []byte("tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	predecessorStore, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	predecessor, _ := predecessorStore.Load()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("changed scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "expanded.txt"), []byte("expanded scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, _ := builder.DiscoverIntendedUntracked(context.Background())
	target, _ := builder.Build(context.Background(), reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: intended})
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + started.LineageID +
		"\npredecessor_revision=" + predecessor.Revision + "\ntarget_identity=" + target.Identity + "\nactor=maintainer\nreason=candidate changed"
	var output bytes.Buffer
	if err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-recovered",
		"--disposition", "scope_changed", "--reason", "candidate changed", "--actor", "maintainer", "--maintainer-authorization", authorization}, &output); err != nil {
		t.Fatal(err)
	}
	var recovered ReviewRecoverResult
	if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.LineageID != "review-recovered" || recovered.Recovery.PredecessorRevision != predecessor.Revision {
		t.Fatalf("recovered = %#v", recovered)
	}
	before, _ := os.ReadFile(predecessorStore.StatePath())
	err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "1"}, io.Discard)
	after, _ := predecessorStore.Load()
	afterBytes, _ := os.ReadFile(predecessorStore.StatePath())
	if err == nil || !strings.Contains(err.Error(), "superseded") || after.Revision != predecessor.Revision || !bytes.Equal(before, afterBytes) {
		t.Fatalf("superseded finalize = %v, before %s, after %s", err, predecessor.Revision, after.Revision)
	}
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("historical authority validation = %v\n%s", err, output.String())
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", recovered.LineageID, "--result", cleanResultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", recovered.LineageID, "--evidence", evidencePath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt", "expanded.txt")
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("successor validation: %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
}

func TestReviewRecoverRetainsCommittedOnlyBaseDiffAndIgnoresWorkspace(t *testing.T) {
	repo := initReviewCLIRepo(t)
	baseRef := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-m", "candidate")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("ignored workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", baseRef, "--committed-only"}, &output); err != nil {
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
	runReviewCLIGit(t, repo, "commit", "-m", "recovered candidate")
	committedTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged workspace\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReview([]string{"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "review-staged-recovered",
		"--disposition", "scope_changed", "--reason", "staged scope changed", "--actor", "maintainer", "--base-ref", baseRef, "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-staged-recovered")
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff || recovered.State.InitialSnapshot.BaseTree != predecessor.State.InitialSnapshot.BaseTree || recovered.State.InitialSnapshot.CandidateTree != committedTree {
		t.Fatalf("recovered committed authority = %#v, want base %s and candidate %s", recovered.State, predecessor.State.InitialSnapshot.BaseTree, committedTree)
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
	args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, started)...)
	args = append(args, "--evidence", evidence)
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
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
	args := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, started)...)
	args = append(args, "--evidence", evidence)
	if err := RunReviewFacadeFinalize(args, io.Discard); err != nil {
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

func TestReviewFacadeRoutesStructuredCandidateCausality(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, _ := store.Load()
	for _, tt := range []struct {
		causality reviewtransaction.CausalDisposition
		want      reviewtransaction.CausalDisposition
		state     reviewtransaction.State
	}{{reviewtransaction.CausalIntroduced, reviewtransaction.CausalUnknown, reviewtransaction.StateEscalated}, {reviewtransaction.CausalWorsened, reviewtransaction.CausalUnknown, reviewtransaction.StateEscalated}, {reviewtransaction.CausalBehaviorActivated, reviewtransaction.CausalBehaviorActivated, reviewtransaction.StateCorrectionRequired}} {
		result := facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "unchanged", ProofRefs: []string{string(tt.causality) + " structured proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: tt.causality}}, Evidence: []string{"reviewed"}}
		input, err := prepareCompactReviewerResults(record.State, []facadeReviewerResult{result}, facadeRefuterResult{}, facadeRepositoryEvidence{ctx: context.Background(), repo: repo})
		state := record.State
		if err == nil {
			err = state.CompleteReview(input)
		}
		finding := state.Findings[0]
		if got := state.Classifications[finding.ID].Causality; err != nil || got != tt.want || state.State != tt.state {
			t.Fatalf("causality %q = %q, state %q, error %v", tt.causality, got, state.State, err)
		}
	}
}

func TestReviewFacadePropagatesCausalGitFailureBeforeMutation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\ncandidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	record, _ := store.Load()
	result := facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{Location: "tracked.txt:2", Severity: "CRITICAL", Claim: "candidate", ProofRefs: []string{"candidate proof"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced}}, Evidence: []string{"reviewed"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := prepareCompactReviewerResults(record.State, []facadeReviewerResult{result}, facadeRefuterResult{}, facadeRepositoryEvidence{ctx: ctx, repo: repo})
	var timeout *reviewtransaction.GitCommandTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("causal Git error = %T %v, want *GitCommandTimeoutError", err, err)
	}
	after, _ := store.Load()
	if after.Revision != record.Revision || !reflect.DeepEqual(after.State, record.State) {
		t.Fatalf("causal preflight changed authority: before %#v after %#v", record, after)
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

func facadeReviewerResultArgs(t *testing.T, started ReviewFacadeStartResult) []string {
	t.Helper()
	args := []string{}
	for index, lens := range started.SelectedLenses {
		resultPath := filepath.Join(t.TempDir(), fmt.Sprintf("reviewer-%d.json", index))
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
			Lens: lens, Findings: []facadeFinding{}, Evidence: []string{"reviewed exact candidate tree"},
		})
		args = append(args, "--result", resultPath)
	}
	return args
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

type facadeReceiptPendingFixture struct {
	repo         string
	evidencePath string
	started      ReviewFacadeStartResult
	store        reviewtransaction.CompactStore
	pending      reviewtransaction.CompactRecord
	diagnostic   *ReviewFacadeReceiptPublicationError
}

func prepareFacadeReceiptPending(t *testing.T) facadeReceiptPendingFixture {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(t.TempDir(), "review.json")
	writeReviewCLIJSON(t, resultPath, facadeReviewerResult{Findings: []facadeFinding{}, Evidence: []string{"review complete"}})
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result", resultPath}, io.Discard); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("injected receipt publication interruption")
	original := writeCompactFacadeReceipt
	writeCompactFacadeReceipt = func(context.Context, reviewtransaction.CompactStore, reviewtransaction.CompactReceipt) error {
		return sentinel
	}
	err = RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--evidence", evidencePath}, io.Discard)
	writeCompactFacadeReceipt = original
	if !errors.Is(err, sentinel) {
		t.Fatalf("receipt publication error = %v, want injected cause", err)
	}
	var diagnostic *ReviewFacadeReceiptPublicationError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("receipt publication error type = %T, want *ReviewFacadeReceiptPublicationError", err)
	}
	pending, loadErr := store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if pending.State.State != reviewtransaction.StateApproved {
		t.Fatalf("pending receipt authority state = %q, want approved", pending.State.State)
	}
	return facadeReceiptPendingFixture{
		repo: repo, evidencePath: evidencePath, started: started, store: store, pending: pending, diagnostic: diagnostic,
	}
}

func assertFacadeReceiptReplayRejected(t *testing.T, fixture facadeReceiptPendingFixture, args []string) {
	t.Helper()
	original := writeCompactFacadeReceipt
	writes := 0
	writeCompactFacadeReceipt = func(context.Context, reviewtransaction.CompactStore, reviewtransaction.CompactReceipt) error {
		writes++
		return errors.New("unexpected receipt write")
	}
	defer func() { writeCompactFacadeReceipt = original }()
	err := RunReviewFacadeFinalize(args, io.Discard)
	if err == nil {
		t.Fatal("unsafe receipt replay succeeded")
	}
	if writes != 0 {
		t.Fatalf("unsafe receipt replay writes = %d, want 0", writes)
	}
	after, loadErr := fixture.store.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if after.Revision != fixture.pending.Revision || !reflect.DeepEqual(after.State, fixture.pending.State) {
		t.Fatalf("unsafe replay mutated authority: before %#v after %#v", fixture.pending, after)
	}
}
