package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestSelectorlessCommittedCorrectionClosesOnTargetedValidation(t *testing.T) {
	for _, amend := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "amend"}[amend], func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			repo, base, lineage := forecastCommittedCorrection(t)
			writeCommittedCorrection(t, repo, amend)
			// The mutable ref must not influence the frozen correction boundary.
			runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")
			authorityStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
			if err != nil {
				t.Fatal(err)
			}
			authorityRecord, err := authorityStore.Load()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reviewtransaction.RebuildCommittedBaseDiffCorrectionCandidate(context.Background(), repo, authorityRecord.State); err != nil {
				t.Fatalf("rebuild committed correction: %v", err)
			}

			status := committedCorrectionStatus(t, repo, lineage, authorityRecord.State.InitialSnapshot.BaseTree)
			if status.Authority == nil || status.Authority.LineageID != lineage || status.ValidationRequest == nil ||
				status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
				status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
				len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation {
				t.Fatalf("post-commit correction status = %#v", status)
			}
			request := status.ValidationRequest
			previous := reviewProviderRoleHostAdapter
			reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
				return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
					return providerTargetedValidationPayload(t, *request), nil
				})
			}
			t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
			var terminalOutput bytes.Buffer
			if err := RunReviewCaptureValidation(reviewTransitionInputTokens(t, status.NextTransition.Collect.Inputs[0]), &terminalOutput); err != nil {
				t.Fatalf("capture selector-less targeted validation: %v\n%s", err, terminalOutput.String())
			}
			var terminal reviewLastEventClosureResult
			decodeStrictReviewJSON(t, terminalOutput.Bytes(), &terminal)
			if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved {
				t.Fatalf("committed correction terminal capture = %#v", terminal)
			}
			assertApprovedCompactAuthorityBurned(t, authorityStore, lineage)
		})
	}
}

func TestStagedCorrectionClosesOnTargetedValidation(t *testing.T) {
	reviewEnabledHome(t)
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n", 0o755)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "staged-correction", "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Lens: started.SelectedLenses[0], Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate is wrong",
		ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed frozen staged candidate"}})
	if err := captureReviewCLIResultFiles(t, repo, started.LineageID, []string{result}); err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, started.LineageID, 2)
	writeReviewStartCandidate(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n", 0o644)
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	var statusOutput bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--lineage", started.LineageID, "--projection", "staged", "--agent", "pi",
	}, &statusOutput); err != nil {
		t.Fatalf("staged correction status: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.ValidationRequest == nil || status.NextTransition == nil ||
		status.NextTransition.ReasonCode != "targeted_validation_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != reviewCaptureValidationCaptureOperation {
		t.Fatalf("staged correction status = %#v", status)
	}
	request := status.ValidationRequest
	previous := reviewProviderRoleHostAdapter
	reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter {
		return providerTestAdapterFunc(func(context.Context, reviewerprovider.Invocation) ([]byte, error) {
			return providerTargetedValidationPayload(t, *request), nil
		})
	}
	t.Cleanup(func() { reviewProviderRoleHostAdapter = previous })
	var terminalOutput bytes.Buffer
	if err := RunReviewCaptureValidation(reviewTransitionInputTokens(t, status.NextTransition.Collect.Inputs[0]), &terminalOutput); err != nil {
		t.Fatalf("capture staged targeted validation: %v\n%s", err, terminalOutput.String())
	}
	var terminal reviewLastEventClosureResult
	decodeStrictReviewJSON(t, terminalOutput.Bytes(), &terminal)
	if terminal.Operation != "review/capture-validation" || terminal.State != reviewtransaction.StateApproved {
		t.Fatalf("staged correction terminal capture = %#v", terminal)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	assertApprovedCompactAuthorityBurned(t, store, started.LineageID)
}

func TestSelectorlessCommittedCorrectionFailsClosedForUnreadableAuthority(t *testing.T) {
	repo, base, lineage := forecastCommittedCorrection(t)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	writeCommittedCorrection(t, repo, false)
	runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")

	statePath := filepath.Join(reviewCLICompactStoreDir(repo, "unreadable-authority"), "review-state.json")
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err = RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition", "--lineage", "unreadable-authority",
		"--base-ref", record.State.InitialSnapshot.BaseTree, "--committed-only",
	}, &output)
	if err == nil {
		t.Fatalf("selector-less status selected a recoverable lineage despite unreadable authority: %s", output.String())
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("selector-less status error = %T %[1]v, want operational path error", err)
	}
}

func TestSelectorlessCommittedCorrectionFailsClosedForOperationalReconstruction(t *testing.T) {
	repo := committedCorrectionWithOperationalReconstructionAuthority(t)

	var output bytes.Buffer
	err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
	}, &output)
	assertOperationalReconstructionFailure(t, err, output.String())
}

func TestSelectorlessCommittedCorrectionFailsClosedForOverBudgetReconstruction(t *testing.T) {
	repo := committedCorrectionWithOverBudgetReconstructionAuthority(t)

	var output bytes.Buffer
	err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1,
	}, &output)
	assertReconstructedBudgetFailure(t, err, output.String())
}

func committedCorrectionWithOperationalReconstructionAuthority(t *testing.T) string {
	t.Helper()
	repo, base, lineage := forecastCommittedCorrection(t)
	writeCommittedCorrection(t, repo, false)
	runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")

	source, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	const missingPath = "missing-reconstruction.txt"
	if err := os.WriteFile(filepath.Join(repo, missingPath), []byte("reconstruction fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseDiff, Projection: reviewtransaction.ProjectionWorkspace,
		BaseRef: record.State.InitialSnapshot.BaseTree, IntendedUntracked: []string{missingPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, missingPath)); err != nil {
		t.Fatal(err)
	}

	state := record.State
	state.LineageID = "operational-reconstruction"
	state.InitialAtomicStart = nil
	state.InitialSnapshot, state.CurrentSnapshot = snapshot, snapshot
	state.GenesisPaths = append([]string(nil), snapshot.Paths...)
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	record.State = state
	record.Revision, err = reviewtransaction.CompactRevisionForState(state)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func assertOperationalReconstructionFailure(t *testing.T, err error, output string) {
	t.Helper()
	if err == nil {
		t.Fatalf("selector-less correction selected an authority despite operational reconstruction failure: %s", output)
	}
	var gitErr *reviewtransaction.GitCommandError
	if !errors.As(err, &gitErr) {
		t.Fatalf("selector-less correction error = %T %[1]v, want operational Git command error", err)
	}
}

func assertReconstructedBudgetFailure(t *testing.T, err error, output string) {
	t.Helper()
	var budgetErr *reviewtransaction.CorrectionBudgetExceededError
	if !errors.As(err, &budgetErr) || budgetErr.Actual != 3 || budgetErr.Remaining != 2 {
		t.Fatalf("selector-less correction error = %T %[1]v, want rebuilt 3/2 budget failure", err)
	}
	if strings.Contains(output, "committed-correction") || strings.Contains(output, "healthy-reconstruction") {
		t.Fatalf("selector-less correction exposed a lineage before budget refusal: %s", output)
	}
}

func committedCorrectionWithOverBudgetReconstructionAuthority(t *testing.T) string {
	t.Helper()
	repo, base, lineage := forecastCommittedCorrection(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\nvar repaired = true\nvar extraOne = 1\nvar extraTwo = 2\nvar extraThree = 3\nvar extraFour = 4\nvar extraFive = 5\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "large candidate")
	healthySnapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).BuildStoredSnapshot(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseDiff, Projection: reviewtransaction.ProjectionWorkspace,
		BaseRef: base, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	healthy := record.State
	healthy.LineageID = "healthy-reconstruction"
	healthy.InitialAtomicStart = nil
	healthy.InitialSnapshot, healthy.CurrentSnapshot = healthySnapshot, healthySnapshot
	healthy.GenesisPaths = append([]string(nil), healthySnapshot.Paths...)
	healthy.OriginalChangedLines, healthy.CorrectionBudget = 10, 5
	forecast := 5
	healthy.ProposedCorrectionLines = &forecast
	if err := healthy.Validate(); err != nil {
		t.Fatal(err)
	}
	record.State = healthy
	record.Revision, err = reviewtransaction.CompactRevisionForState(healthy)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	healthyStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, healthy.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(healthyStore.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(healthyStore.StatePath(), append(payload, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	writeOverBudgetCommittedCorrection(t, repo)
	runReviewCLIGit(t, repo, "branch", "-f", base, "HEAD")
	return repo
}

// forecastCommittedCorrection drives a real lineage from START through a
// blocking finding to an open correction budget. Every caller continues that
// lifecycle through the native CLI, so the fixture opts in the way a real user
// does: receipt-driven development is off until someone enables it, and none of
// these transitions exist for a clone that never did.
func forecastCommittedCorrection(t *testing.T) (string, string, string) {
	t.Helper()
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := "frozen-base"
	runReviewCLIGit(t, repo, "branch", base, "HEAD")
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 1\n}\n", 0o755)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "wrong candidate")

	const lineage = "committed-correction"
	startedBytes, err := runLegacyFacadeStartForTestBytes(t, []string{
		"--cwd", repo, "--lineage", lineage, "--base-ref", base, "--committed-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, startedBytes, &started)
	resultPaths := make([]string, len(started.SelectedLenses))
	for index, lens := range started.SelectedLenses {
		findings := []facadeFinding{}
		if index == 0 {
			findings = []facadeFinding{{
				Location: "candidate.go:3", Severity: "CRITICAL", Claim: "candidate is wrong",
				ProofRefs: []string{"candidate.go:3 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
				CausalDisposition: reviewtransaction.CausalIntroduced,
			}}
		}
		resultPaths[index] = filepath.Join(t.TempDir(), "reviewer-"+strconv.Itoa(index)+".json")
		writeReviewCLIJSON(t, resultPaths[index], facadeReviewerResult{
			Lens: lens, Findings: findings, Evidence: []string{"reviewed frozen committed candidate"},
		})
	}
	if err := captureReviewCLIResultFiles(t, repo, lineage, resultPaths); err != nil {
		t.Fatal(err)
	}
	captureCorrectionPlanFromCurrentStatus(t, repo, lineage, 2)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.State.State != reviewtransaction.StateCorrectionRequired || record.State.ProposedCorrectionLines == nil || *record.State.ProposedCorrectionLines != 2 {
		t.Fatalf("committed correction fixture = %#v, want an open two-line compact correction", record.State)
	}
	return repo, base, lineage
}

func writeCommittedCorrection(t *testing.T, repo string, amend bool) {
	t.Helper()
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	if amend {
		runReviewCLIGit(t, repo, "commit", "--amend", "--no-edit")
		return
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "correct candidate")
}

func writeOverBudgetCommittedCorrection(t *testing.T, repo string) {
	t.Helper()
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 2\n}\nvar repaired = true\n", 0o644)
	runReviewCLIGit(t, repo, "add", "candidate.go")
	runReviewCLIGit(t, repo, "commit", "-qm", "over-budget correct candidate")
}

func committedCorrectionStatus(t *testing.T, repo, lineage, baseTree string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition", "--lineage", lineage,
		"--base-ref", baseTree, "--committed-only", "--agent", "pi",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}
