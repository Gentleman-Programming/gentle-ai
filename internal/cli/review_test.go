package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestRunReviewStartBuildsCompleteTargetWithoutMutatingRealIndex(t *testing.T) {
	repo := initReviewCLIRepo(t)
	manifest := filepath.Join(t.TempDir(), "intended.txt")
	transactionOut := filepath.Join(t.TempDir(), "transaction.json")
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte("new.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(repo, indexPath)
	}
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReviewStart([]string{
		"--cwd", repo,
		"--lineage", "lineage-1",
		"--policy-file", policy,
		"--intended-untracked-manifest", manifest,
		"--machine-transaction-out", transactionOut,
	}, &output)
	if err != nil {
		t.Fatalf("RunReviewStart() error = %v", err)
	}

	var result ReviewStartResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal() error = %v\n%s", err, output.String())
	}
	if result.Schema != ReviewStartSchema || result.Operation != "review/start" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Transaction.State != reviewtransaction.StateReviewing || result.Transaction.Counters.FullReviews != 1 || result.StoreRevision == "" || result.StoreAuthority != "repository-git-common-dir" {
		t.Fatalf("transaction = %#v", result.Transaction)
	}
	persisted, err := os.ReadFile(transactionOut)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewtransaction.ParseTransaction(persisted); err != nil {
		t.Fatalf("transaction-out is not a native transaction: %v", err)
	}
	if len(result.Target.Paths) != 1 || result.Target.Paths[0] != "new.txt" || len(result.Target.IntendedUntracked) != 1 {
		t.Fatalf("target = %#v", result.Target)
	}
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Fatal("review-start mutated the real Git index")
	}
}

func TestRunReviewStartRequiresAuthoritativeCASAndRetryCannotResetState(t *testing.T) {
	repo := initReviewCLIRepo(t)
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	machineOut := filepath.Join(t.TempDir(), "transaction.json")
	args := []string{
		"--cwd", repo,
		"--lineage", "lineage-cas",
		"--policy-file", policy,
		"--machine-transaction-out", machineOut,
	}
	var output bytes.Buffer
	if err := RunReviewStart(args, &output); err != nil {
		t.Fatalf("RunReviewStart(first) error = %v", err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, "lineage-cas")
	if err != nil {
		t.Fatal(err)
	}
	record, firstRevision, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Transaction.FreezeFindings([]reviewtransaction.Finding{}, cliHash("1")); err != nil {
		t.Fatal(err)
	}
	advancedRevision, err := store.Append(firstRevision, reviewtransaction.Record{Operation: "review/freeze-findings", Transaction: record.Transaction})
	if err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if err := RunReviewStart(args, &output); !errors.Is(err, reviewtransaction.ErrConcurrentUpdate) {
		t.Fatalf("RunReviewStart(retry) error = %v, want ErrConcurrentUpdate", err)
	}
	loaded, revision, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if revision != advancedRevision || loaded.Transaction.State != reviewtransaction.StateFindingsFrozen || loaded.Transaction.Counters.FullReviews != 1 {
		t.Fatalf("retry reset authoritative state: revision=%q transaction=%#v", revision, loaded.Transaction)
	}
}

func TestRunReviewResumeReemitsAuthoritativeStateAfterOutputFailures(t *testing.T) {
	repo := initReviewCLIRepo(t)
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		lineage string
		start   func(args []string) error
	}{
		{
			name:    "stdout failure",
			lineage: "resume-stdout",
			start: func(args []string) error {
				return RunReviewStart(args, failingReviewWriter{})
			},
		},
		{
			name:    "machine mirror failure",
			lineage: "resume-mirror",
			start: func(args []string) error {
				args = append(args, "--machine-transaction-out", t.TempDir())
				return RunReviewStart(args, io.Discard)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"--cwd", repo, "--lineage", tt.lineage, "--policy-file", policy}
			if err := tt.start(args); err == nil {
				t.Fatal("RunReviewStart() unexpectedly hid its output failure")
			}
			store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, tt.lineage)
			if err != nil {
				t.Fatal(err)
			}
			_, committedRevision, err := store.Load()
			if err != nil {
				t.Fatalf("authoritative state was not committed before output failure: %v", err)
			}
			var retryOutput bytes.Buffer
			if err := RunReviewStart(args, &retryOutput); err != nil {
				t.Fatalf("RunReviewStart(identical retry) error = %v", err)
			}
			var retried ReviewStartResult
			if err := json.Unmarshal(retryOutput.Bytes(), &retried); err != nil {
				t.Fatal(err)
			}
			if retried.StoreRevision != committedRevision || retried.Transaction.Counters.FullReviews != 1 {
				t.Fatalf("identical retry = %#v, want committed revision %q without budget reset", retried, committedRevision)
			}

			machineOut := filepath.Join(t.TempDir(), "transaction.json")
			var output bytes.Buffer
			if err := RunReviewResume([]string{
				"--cwd", repo, "--lineage", tt.lineage, "--machine-transaction-out", machineOut,
			}, &output); err != nil {
				t.Fatalf("RunReviewResume() error = %v", err)
			}
			var result ReviewResumeResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.StoreRevision != committedRevision || result.Transaction.Counters.FullReviews != 1 || result.Transaction.State != reviewtransaction.StateReviewing {
				t.Fatalf("resume result = %#v, want committed revision %q without budget reset", result, committedRevision)
			}
			payload, err := os.ReadFile(machineOut)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reviewtransaction.ParseTransaction(payload); err != nil {
				t.Fatalf("resumed machine mirror is invalid: %v", err)
			}
		})
	}
}

func TestRunReviewStepAppendsLifecycleStateThroughAuthoritativeStore(t *testing.T) {
	repo := initReviewCLIRepo(t)
	policy := filepath.Join(t.TempDir(), "policy.md")
	ledger := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(policy, []byte("policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledger, []byte("{\"schema\":\"gentle-ai.review-ledger/v1\",\"findings\":[]}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewStart([]string{"--cwd", repo, "--lineage", "step-lineage", "--policy-file", policy}, io.Discard); err != nil {
		t.Fatal(err)
	}
	ledgerHash, err := reviewtransaction.HashLedgerArtifact(ledger)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "freeze.json")
	writeReviewCLIJSON(t, input, ReviewStepInput{Findings: []reviewtransaction.Finding{}, LedgerHash: ledgerHash})
	var output bytes.Buffer
	if err := RunReviewStep([]string{"--cwd", repo, "--lineage", "step-lineage", "--operation", "freeze-findings", "--input", input}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewResumeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Transaction.State != reviewtransaction.StateFindingsFrozen || result.StoreRevision == "" {
		t.Fatalf("review step result = %#v", result)
	}
}

func TestRunReviewStartStepAndResumeOrdinaryBoundedLensResult(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("standard executable change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var started bytes.Buffer
	if err := RunReviewStart([]string{
		"--cwd", repo,
		"--lineage", "bounded-lens",
		"--policy-file", policy,
		"--mode", string(reviewtransaction.ModeOrdinaryBounded),
		"--lens", "review-reliability",
	}, &started); err != nil {
		t.Fatalf("RunReviewStart() error = %v", err)
	}
	var startResult ReviewStartResult
	if err := json.Unmarshal(started.Bytes(), &startResult); err != nil {
		t.Fatal(err)
	}
	if startResult.Transaction.Counters != (reviewtransaction.Counters{}) || len(startResult.Transaction.SelectedLenses) != 1 {
		t.Fatalf("bounded start = %#v", startResult.Transaction)
	}

	input := filepath.Join(t.TempDir(), "lens-result.json")
	writeReviewCLIJSON(t, input, ReviewStepInput{LensResult: &reviewtransaction.LensResult{
		Lens: "review-reliability", Findings: []reviewtransaction.Finding{}, Evidence: []string{"focused reliability sweep completed"},
	}})
	if err := RunReviewStep([]string{
		"--cwd", repo, "--lineage", "bounded-lens", "--operation", "record-lens-result", "--input", input,
	}, failingReviewWriter{}); err == nil {
		t.Fatal("RunReviewStep() hid the post-commit output failure")
	}

	var resumed bytes.Buffer
	if err := RunReviewResume([]string{"--cwd", repo, "--lineage", "bounded-lens"}, &resumed); err != nil {
		t.Fatalf("RunReviewResume() error = %v", err)
	}
	var resumeResult ReviewResumeResult
	if err := json.Unmarshal(resumed.Bytes(), &resumeResult); err != nil {
		t.Fatal(err)
	}
	if len(resumeResult.Transaction.LensResults) != 1 || resumeResult.Transaction.Counters.ReliabilityExecutions != 1 || resumeResult.Transaction.State != reviewtransaction.StateReviewing {
		t.Fatalf("resumed bounded result = %#v", resumeResult.Transaction)
	}
}

func TestRunReviewStartBindsSelectedLensesToImmutableSnapshotRisk(t *testing.T) {
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		changePath string
		content    string
		lenses     []string
		wantRisk   reviewtransaction.RiskLevel
	}{
		{name: "low", changePath: "guide.md", content: "documentation only\n", wantRisk: reviewtransaction.RiskLow},
		{name: "medium", changePath: "tracked.txt", content: "standard executable change\n", lenses: []string{reviewtransaction.LensReliability}, wantRisk: reviewtransaction.RiskMedium},
		{name: "high", changePath: "internal/security/check.go", content: "package security\n", lenses: []string{reviewtransaction.LensRisk, reviewtransaction.LensResilience, reviewtransaction.LensReadability, reviewtransaction.LensReliability}, wantRisk: reviewtransaction.RiskHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, tt.changePath)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, tt.changePath), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			args := []string{"--cwd", repo, "--lineage", "risk-" + tt.name, "--policy-file", policy, "--mode", string(reviewtransaction.ModeOrdinaryBounded)}
			if tt.changePath != "tracked.txt" {
				args = append(args, "--intended-untracked", tt.changePath)
			}
			for _, lens := range tt.lenses {
				args = append(args, "--lens", lens)
			}
			var output bytes.Buffer
			if err := RunReviewStart(args, &output); err != nil {
				t.Fatalf("RunReviewStart() error = %v", err)
			}
			var result ReviewStartResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Transaction.RiskLevel != tt.wantRisk || len(result.Transaction.SelectedLenses) != len(tt.lenses) {
				t.Fatalf("bounded classification = %q, %v", result.Transaction.RiskLevel, result.Transaction.SelectedLenses)
			}
		})
	}

	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "guide.md"), []byte("documentation only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := RunReviewStart([]string{
		"--cwd", repo, "--lineage", "risk-mismatch", "--policy-file", policy,
		"--mode", string(reviewtransaction.ModeOrdinaryBounded), "--intended-untracked", "guide.md",
		"--lens", reviewtransaction.LensReliability,
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "low risk requires exactly 0") {
		t.Fatalf("RunReviewStart(risk mismatch) error = %v", err)
	}
}

func TestRunReviewStepAppendsTargetedValidationAndResumeReemitsIt(t *testing.T) {
	repo := initReviewCLIRepo(t)
	lineage := "targeted-validation"
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: cliHash("a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	revision := ""
	appendState := func(operation string) {
		t.Helper()
		var appendErr error
		revision, appendErr = store.Append(revision, reviewtransaction.Record{Operation: operation, Transaction: *tx})
		if appendErr != nil {
			t.Fatal(appendErr)
		}
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	appendState("review/start")
	if err := tx.FreezeFindings([]reviewtransaction.Finding{{ID: "R1-DET", Severity: "CRITICAL"}}, cliHash("b")); err != nil {
		t.Fatal(err)
	}
	appendState("review/freeze-findings")
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{{FindingID: "R1-DET", Class: reviewtransaction.EvidenceDeterministic, Proof: "reproduced"}}); err != nil {
		t.Fatal(err)
	}
	appendState("review/classify-evidence")
	if err := tx.BeginFix(cliHash("c")); err != nil {
		t.Fatal(err)
	}
	appendState("review/begin-fix")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetFixDiff, BaseRef: tx.FinalCandidateTree, IntendedUntracked: []string{}, LedgerIDs: []string{"R1-DET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFix(fix, cliHash("d"), []string{"R1-DET"}); err != nil {
		t.Fatal(err)
	}
	appendState("review/complete-fix")

	input := filepath.Join(t.TempDir(), "validate.json")
	writeReviewCLIJSON(t, input, ReviewStepInput{Validation: &reviewtransaction.ScopedValidationResult{
		LedgerIDs: []string{"R1-DET"}, FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: []reviewtransaction.FollowUp{},
		OriginalCriteria:     reviewtransaction.ValidationCheck{EvidenceHash: cliHash("e"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		CorrectionRegression: reviewtransaction.ValidationCheck{EvidenceHash: cliHash("f"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
	}})
	var output bytes.Buffer
	if err := RunReviewStep([]string{"--cwd", repo, "--lineage", lineage, "--operation", "validate-fix", "--input", input}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewResumeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Operation != "review/validate-targeted-fix" || result.Transaction.State != reviewtransaction.StateReadyFinalVerification {
		t.Fatalf("targeted validation result = %#v", result)
	}

	output.Reset()
	if err := RunReviewResume([]string{"--cwd", repo, "--lineage", lineage}, &output); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Operation != "review/resume" || result.Transaction.State != reviewtransaction.StateReadyFinalVerification {
		t.Fatalf("resume did not reemit authoritative targeted validation state: %#v", result)
	}
}

type failingReviewWriter struct{}

func (failingReviewWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated review output failure")
}

func TestRunReviewStartSupportsExplicitTargetKindsAndCommaSafeLedgerIDs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	firstCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "second")
	secondCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	secondTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := filepath.Join(t.TempDir(), "policy.md")
	manifest := filepath.Join(t.TempDir(), "empty-manifest.txt")
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		kind      reviewtransaction.TargetKind
		extraArgs []string
		wantIDs   []string
	}{
		{name: "base diff", kind: reviewtransaction.TargetBaseDiff, extraArgs: []string{"--base-ref", firstCommit}},
		{name: "exact range", kind: reviewtransaction.TargetExactRevision, extraArgs: []string{"--revision", firstCommit + ".." + secondCommit}},
		{name: "exact commit", kind: reviewtransaction.TargetExactRevision, extraArgs: []string{"--revision", secondCommit}},
		{name: "ledger-bound fix diff", kind: reviewtransaction.TargetFixDiff, extraArgs: []string{
			"--base-ref", secondTree,
			"--intended-untracked-manifest", manifest,
			"--ledger-id", "BRT-001",
			"--ledger-id", "BRT-009,comma-safe",
		}, wantIDs: []string{"BRT-001", "BRT-009,comma-safe"}},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{
				"--cwd", repo,
				"--kind", string(tt.kind),
				"--lineage", "target-" + strings.ReplaceAll(tt.name, " ", "-"),
				"--policy-file", policy,
			}
			args = append(args, tt.extraArgs...)
			var output bytes.Buffer
			if err := RunReviewStart(args, &output); err != nil {
				t.Fatalf("RunReviewStart(case %d) error = %v", index, err)
			}
			var result ReviewStartResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Target.Kind != tt.kind || result.Target.IntendedUntracked == nil {
				t.Fatalf("target = %#v", result.Target)
			}
			if strings.Join(result.Target.LedgerIDs, "|") != strings.Join(tt.wantIDs, "|") {
				t.Fatalf("LedgerIDs = %v, want %v", result.Target.LedgerIDs, tt.wantIDs)
			}
		})
	}
}

func TestRunReviewStartValidatesTargetSpecificRequiredArguments(t *testing.T) {
	repo := initReviewCLIRepo(t)
	policy := filepath.Join(t.TempDir(), "policy.md")
	if err := os.WriteFile(policy, []byte("bounded policy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := []string{"--cwd", repo, "--lineage", "invalid-target", "--policy-file", policy}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "base diff needs base", args: []string{"--kind", string(reviewtransaction.TargetBaseDiff)}, want: "base-diff requires --base-ref"},
		{name: "commit range needs revision", args: []string{"--kind", string(reviewtransaction.TargetExactRevision)}, want: "commit-range requires --revision"},
		{name: "fix diff needs ledger and explicit manifest", args: []string{"--kind", string(reviewtransaction.TargetFixDiff), "--base-ref", strings.Repeat("a", 40)}, want: "at least one repeatable --ledger-id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReviewStart(append(append([]string{}, base...), tt.args...), &output)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunReviewStart() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRunReviewValidateDerivesCurrentFactsAndDeniesWithJSON(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := t.TempDir()
	policyPath := filepath.Join(artifacts, "policy.md")
	ledgerPath := filepath.Join(artifacts, "ledger.json")
	evidencePath := filepath.Join(artifacts, "verify.md")
	for path, content := range map[string]string{
		policyPath:   "bounded policy\n",
		ledgerPath:   `{"schema":"gentle-ai.review-ledger/v1","findings":[]}` + "\n",
		evidencePath: "current verify evidence\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash, _ := reviewtransaction.HashArtifact(policyPath)
	ledgerHash, _ := reviewtransaction.HashArtifact(ledgerPath)
	evidenceHash, _ := reviewtransaction.HashArtifact(evidencePath)
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: "native-gate", Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, "native-gate")
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	revision, err := store.Append("", reviewtransaction.Record{Operation: "review/start", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.FreezeFindings([]reviewtransaction.Finding{}, ledgerHash)
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/freeze-findings", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{})
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/classify", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.BeginFinalVerification()
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/begin-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.CompleteFinalVerification(evidenceHash, true)
	revision, err = store.Append(revision, reviewtransaction.Record{Operation: "review/complete-final-verification", Transaction: *tx})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(artifacts, "receipt.json")
	writeReviewCLIJSON(t, receiptPath, receipt)
	request := reviewtransaction.GateRequest{
		Schema:         reviewtransaction.GateRequestSchema,
		Gate:           reviewtransaction.GatePostApply,
		Target:         reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}},
		StoreRevision:  revision,
		PolicyArtifact: policyPath, LedgerArtifact: ledgerPath, EvidenceArtifact: evidencePath,
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		t.Fatal(err)
	}
	request.GenesisRevision = bundle.GenesisRevision
	request.ChainIdentity = bundle.ChainIdentity
	request.BundleDigest = bundle.BundleDigest
	requestPath := filepath.Join(artifacts, "gate-request.json")
	writeReviewCLIJSON(t, requestPath, request)

	var output bytes.Buffer
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", receiptPath, "--request", requestPath}, &output); err != nil {
		t.Fatalf("RunReviewValidate(exact) error = %v\n%s", err, output.String())
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("scope changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", receiptPath, "--request", requestPath}, &output); err == nil {
		t.Fatal("RunReviewValidate(scope changed) returned process success")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateScopeChanged)

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, []byte(`{"schema":"gentle-ai.review-ledger/v1","findings":[{"id":"fabricated"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", receiptPath, "--request", requestPath}, &output); err == nil {
		t.Fatal("RunReviewValidate(stale ledger) returned process success")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateInvalidated)

	if err := os.WriteFile(ledgerPath, []byte(`{"schema":"gentle-ai.review-ledger/v1","findings":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request.ExternalEvidence = reviewtransaction.ExternalEvidenceEscalating
	writeReviewCLIJSON(t, requestPath, request)
	output.Reset()
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", receiptPath, "--request", requestPath}, &output); err == nil {
		t.Fatal("RunReviewValidate(escalated) returned process success")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateEscalated)

	request.Target = reviewtransaction.Target{Kind: reviewtransaction.TargetBaseDiff, BaseRef: strings.Repeat("f", 40)}
	request.ExternalEvidence = reviewtransaction.ExternalEvidenceNone
	writeReviewCLIJSON(t, requestPath, request)
	output.Reset()
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", receiptPath, "--request", requestPath}, &output); err != nil {
		t.Fatalf("RunReviewValidate ignores historical target and derives current lifecycle state: %v", err)
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
}

func TestRunReviewValidateRejectsCallerSelectedForgedTerminalStore(t *testing.T) {
	repo := initReviewCLIRepo(t)
	artifacts := t.TempDir()
	policyPath := filepath.Join(artifacts, "policy.md")
	ledgerPath := filepath.Join(artifacts, "ledger.json")
	evidencePath := filepath.Join(artifacts, "verify.md")
	for path, content := range map[string]string{
		policyPath:   "bounded policy\n",
		ledgerPath:   "{\"schema\":\"gentle-ai.review-ledger/v1\",\"findings\":[]}\n",
		evidencePath: "verified\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash, _ := reviewtransaction.HashArtifact(policyPath)
	ledgerHash, _ := reviewtransaction.HashArtifact(ledgerPath)
	evidenceHash, _ := reviewtransaction.HashArtifact(evidencePath)
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: "forged-cli-lineage", Mode: reviewtransaction.ModeOrdinary4R, Generation: 1,
		Snapshot: snapshot, PolicyHash: policyHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.StartReview()
	_ = tx.FreezeFindings([]reviewtransaction.Finding{}, ledgerHash)
	_, _ = tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{})
	_ = tx.BeginFinalVerification()
	_ = tx.CompleteFinalVerification(evidenceHash, true)
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	forgedDir := filepath.Join(artifacts, "forged-store")
	revision := writeForgedReviewStoreHead(t, forgedDir, reviewtransaction.Record{
		Operation: "review/complete-final-verification", PreviousRevision: cliHash("f"), Transaction: *tx,
	})
	receiptPath := filepath.Join(artifacts, "receipt.json")
	requestPath := filepath.Join(artifacts, "request.json")
	writeReviewCLIJSON(t, receiptPath, receipt)
	writeReviewCLIJSON(t, requestPath, reviewtransaction.GateRequest{
		Schema: reviewtransaction.GateRequestSchema, Gate: reviewtransaction.GatePostApply,
		Target:   reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{}},
		StoreDir: forgedDir, StoreRevision: revision,
		GenesisRevision: cliHash("a"), ChainIdentity: cliHash("b"), BundleDigest: cliHash("c"),
		PolicyArtifact: policyPath, LedgerArtifact: ledgerPath, EvidenceArtifact: evidencePath,
	})

	var output bytes.Buffer
	if err := RunReviewValidate([]string{"--cwd", repo, "--receipt", receiptPath, "--request", requestPath}, &output); err == nil {
		t.Fatal("RunReviewValidate(forged standalone terminal) returned process success")
	}
	assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateInvalidated)
}

func TestRunReviewBundleExportImportRecoversCorrectedLineageInCleanClone(t *testing.T) {
	source := initReviewCLIRepo(t)
	artifacts := t.TempDir()
	policyPath := filepath.Join(artifacts, "policy.md")
	ledgerPath := filepath.Join(artifacts, "ledger.json")
	fixDeltaPath := filepath.Join(artifacts, "fix-delta.patch")
	evidencePath := filepath.Join(artifacts, "evidence.md")
	for path, content := range map[string]string{
		policyPath:   "bounded policy\n",
		ledgerPath:   "{\"schema\":\"gentle-ai.review-ledger/v1\",\"findings\":[{\"id\":\"BRT1-005\"}]}\n",
		fixDeltaPath: "portable recovery correction\n",
		evidencePath: "verified corrected delivery\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	baseRevision := strings.TrimSpace(runReviewCLIGit(t, source, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(source, "delivery.txt"), []byte("initial delivery\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: source}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{"delivery.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	policyHash, _ := reviewtransaction.HashArtifact(policyPath)
	ledgerHash, _ := reviewtransaction.HashLedgerArtifact(ledgerPath)
	fixDeltaHash, _ := reviewtransaction.HashArtifact(fixDeltaPath)
	evidenceHash, _ := reviewtransaction.HashArtifact(evidencePath)
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{LineageID: "portable-corrected-cli", Mode: reviewtransaction.ModeOrdinary4R, Generation: 1, Snapshot: snapshot, PolicyHash: policyHash})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), source, tx.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	revision := ""
	appendState := func(operation string) {
		t.Helper()
		var appendErr error
		revision, appendErr = store.Append(revision, reviewtransaction.Record{Operation: operation, Transaction: *tx})
		if appendErr != nil {
			t.Fatalf("Append(%s) error = %v", operation, appendErr)
		}
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	appendState("review/start")
	finding := reviewtransaction.Finding{
		ID: "BRT1-005", Lens: "resilience", Location: "internal/reviewtransaction/bundle.go",
		Severity: "CRITICAL", Claim: "corrected lineages cannot recover authority", ProofRefs: []string{"bundle.go:209"},
	}
	if err := tx.FreezeFindings([]reviewtransaction.Finding{finding}, ledgerHash); err != nil {
		t.Fatal(err)
	}
	appendState("review/freeze-findings")
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{{
		FindingID: "BRT1-005", Class: reviewtransaction.EvidenceDeterministic, Proof: "corrected clean-clone import was rejected",
	}}); err != nil {
		t.Fatal(err)
	}
	appendState("review/classify")
	if err := tx.BeginFix(cliHash("f")); err != nil {
		t.Fatal(err)
	}
	appendState("review/begin-fix")
	if err := os.WriteFile(filepath.Join(source, "delivery.txt"), []byte("corrected delivery\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixSnapshot, err := (reviewtransaction.SnapshotBuilder{Repo: source}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetFixDiff, BaseRef: tx.FinalCandidateTree,
		IntendedUntracked: []string{"delivery.txt"}, LedgerIDs: []string{"BRT1-005"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFix(fixSnapshot, fixDeltaHash, []string{"BRT1-005"}); err != nil {
		t.Fatal(err)
	}
	appendState("review/complete-fix")
	if err := tx.ValidateFixDeltaResult(reviewtransaction.ScopedValidationResult{
		LedgerIDs: []string{"BRT1-005"}, FixCausedFindings: []reviewtransaction.Finding{}, FollowUps: []reviewtransaction.FollowUp{},
		OriginalCriteria:     reviewtransaction.ValidationCheck{EvidenceHash: cliHash("5"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
		CorrectionRegression: reviewtransaction.ValidationCheck{EvidenceHash: cliHash("6"), FixDeltaHash: tx.FixDeltaHash, Passed: true},
	}); err != nil {
		t.Fatal(err)
	}
	appendState("review/validate-targeted-fix")
	if err := tx.BeginFinalVerification(); err != nil {
		t.Fatal(err)
	}
	appendState("review/begin-final-verification")
	if err := tx.CompleteFinalVerification(evidenceHash, true); err != nil {
		t.Fatal(err)
	}
	appendState("review/complete-final-verification")
	receipt, err := tx.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, source, "add", "--", "delivery.txt")
	runReviewCLIGit(t, source, "commit", "-qm", "corrected delivery")
	finalRevision := strings.TrimSpace(runReviewCLIGit(t, source, "rev-parse", "HEAD"))

	bundlePath := filepath.Join(artifacts, "chain-bundle.json")
	var exportOutput bytes.Buffer
	if err := RunReviewBundleExport([]string{"--cwd", source, "--lineage", tx.LineageID, "--out", bundlePath}, &exportOutput); err != nil {
		t.Fatalf("RunReviewBundleExport() error = %v", err)
	}
	bundlePayload, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := reviewtransaction.ParseChainBundle(bundlePayload)
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(artifacts, "receipt.json")
	requestPath := filepath.Join(artifacts, "request.json")
	writeReviewCLIJSON(t, receiptPath, receipt)
	request := reviewtransaction.GateRequest{
		Schema: reviewtransaction.GateRequestSchema, Gate: reviewtransaction.GatePostApply,
		Target: reviewtransaction.Target{
			Kind: reviewtransaction.TargetExactRevision, Revision: baseRevision + ".." + finalRevision,
		},
		StoreRevision: bundle.HeadRevision, GenesisRevision: bundle.GenesisRevision,
		ChainIdentity: bundle.ChainIdentity, BundleDigest: bundle.BundleDigest,
		PolicyArtifact: policyPath, LedgerArtifact: ledgerPath,
		FixDeltaArtifact: fixDeltaPath, EvidenceArtifact: evidencePath,
	}
	writeReviewCLIJSON(t, requestPath, request)

	wrongFixPath := filepath.Join(artifacts, "wrong-fix-delta.patch")
	if err := os.WriteFile(wrongFixPath, []byte("different correction\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrongRequest := request
	wrongRequest.FixDeltaArtifact = wrongFixPath
	wrongRequestPath := filepath.Join(artifacts, "wrong-request.json")
	writeReviewCLIJSON(t, wrongRequestPath, wrongRequest)
	wrongClone := filepath.Join(t.TempDir(), "wrong-clone")
	command := exec.Command("git", "clone", "-q", source, wrongClone)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, output)
	}
	if err := RunReviewBundleImport([]string{"--cwd", wrongClone, "--bundle", bundlePath, "--receipt", receiptPath, "--request", wrongRequestPath}, io.Discard); err != nil {
		t.Fatalf("RunReviewBundleImport() trusted unrelated fix-delta prose: %v", err)
	}
	wrongStore, err := reviewtransaction.AuthoritativeStore(context.Background(), wrongClone, tx.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongStore.LoadChain(); err != nil {
		t.Fatalf("derived corrected import did not install authoritative chain: %v", err)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	command = exec.Command("git", "clone", "-q", source, clone)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, output)
	}
	var importOutput bytes.Buffer
	if err := RunReviewBundleImport([]string{"--cwd", clone, "--bundle", bundlePath, "--receipt", receiptPath, "--request", requestPath}, &importOutput); err != nil {
		t.Fatalf("RunReviewBundleImport() error = %v", err)
	}
	var result ReviewBundleResult
	if err := json.Unmarshal(importOutput.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.BundleDigest != bundle.BundleDigest || result.StoreRevision != bundle.HeadRevision || result.ChainIdentity != bundle.ChainIdentity {
		t.Fatalf("bundle import result = %#v", result)
	}
}

func assertReviewGateResult(t *testing.T, payload []byte, want reviewtransaction.GateResult) {
	t.Helper()
	var result ReviewValidateResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("gate output is not JSON: %v\n%s", err, payload)
	}
	if result.Result != want || result.Allowed != (want == reviewtransaction.GateAllow) {
		t.Fatalf("gate result = %#v, want %q", result, want)
	}
}

func initReviewCLIRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runReviewCLIGit(t, repo, "init", "-q")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "base")
	return repo
}

func runReviewCLIGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func writeReviewCLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeForgedReviewStoreHead(t *testing.T, dir string, record reviewtransaction.Record) string {
	t.Helper()
	record.Schema = reviewtransaction.RecordSchema
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	revision := "sha256:" + hex.EncodeToString(sum[:])
	events := filepath.Join(dir, "events")
	if err := os.MkdirAll(events, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(events, strings.TrimPrefix(revision, "sha256:")+".json"), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte(revision+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return revision
}

func cliHash(char string) string { return "sha256:" + strings.Repeat(char, 64) }
