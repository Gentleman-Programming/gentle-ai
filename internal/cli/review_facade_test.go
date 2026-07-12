package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

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

	for _, gate := range []reviewtransaction.GateKind{reviewtransaction.GatePostApply, reviewtransaction.GatePreCommit} {
		output.Reset()
		if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(gate)}, &output); err != nil {
			t.Fatalf("compact %s gate: %v\n%s", gate, err, output.String())
		}
		assertReviewGateResult(t, output.Bytes(), reviewtransaction.GateAllow)
	}

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
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--lineage", result.LineageID, "--gate", string(reviewtransaction.GatePrePR), "--base-ref", base}, &output); err != nil {
		t.Fatalf("pre-pr base diff gate: %v\n%s", err, output.String())
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
	output.Reset()
	if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", string(reviewtransaction.GatePreCommit)}, &output); err != nil {
		t.Fatalf("corrected compact gate: %v\n%s", err, output.String())
	}
}

func TestReviewFacadePersistsOverBudgetForecastAndRejectsOverBudgetActual(t *testing.T) {
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
		err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--validation", validationPath}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "exceeding the frozen budget") {
			t.Fatalf("over-budget actual error = %v", err)
		}
		store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
		record, loadErr := store.Load()
		if loadErr != nil || record.State.State != reviewtransaction.StateCorrectionRequired || record.State.ActualCorrectionLines != nil || !reflect.DeepEqual(record.State.CurrentSnapshot, record.State.InitialSnapshot) {
			t.Fatalf("over-budget actual changed authority = %#v, %v", record.State, loadErr)
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

func TestReviewFacadeHelpAndFlatCompatibilityPathsRemainAvailable(t *testing.T) {
	for _, subcommand := range []string{"start", "finalize", "validate"} {
		var output bytes.Buffer
		if err := RunReview([]string{subcommand, "--help"}, &output); err != nil || !strings.Contains(output.String(), "Usage: gentle-ai review "+subcommand) {
			t.Fatalf("facade %s help: %v\n%s", subcommand, err, output.String())
		}
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

func TestCompactTransportRecoversCorrectedCurrentChangesWithoutIntermediateTrees(t *testing.T) {
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
	if err := RunReviewFacadeValidate([]string{"--cwd", clone, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePrePush)}, &output); err != nil {
		t.Fatalf("corrected recovered gate: %v\n%s", err, output.String())
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
