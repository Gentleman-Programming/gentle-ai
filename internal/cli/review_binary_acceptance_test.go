package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestMainBinaryAcceptsCorrectedCandidateFromLinkedWorktree(t *testing.T) {
	binary := os.Getenv("GENTLE_AI_TEST_BINARY")
	if binary == "" {
		t.Skip("requires GENTLE_AI_TEST_BINARY built from the branch under test")
	}
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("GENTLE_AI_TEST_BINARY: %v", err)
	}

	t.Run("approves corrected linked worktree", func(t *testing.T) {
		_, corrected, started := prepareBinaryCorrection(t, binary)
		writeBinaryCandidate(t, corrected, "fixed")
		validation := filepath.Join(t.TempDir(), "validation.json")
		writeReviewCLIJSON(t, validation, facadeValidationResult{
			OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance passed"}},
			CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression passed"}},
			FollowUps:            []reviewtransaction.FollowUp{},
		})
		var validating ReviewFacadeFinalizeResult
		decodeBinaryJSON(t, runReviewBinary(t, binary, true, "finalize", "--cwd", corrected, "--validation", validation), &validating)
		if validating.State != reviewtransaction.StateValidating {
			t.Fatalf("validation state = %q", validating.State)
		}
		status := binaryReviewStatus(t, binary, corrected, started.LineageID)
		if status.Projection.InitialReviewTree == status.Projection.CurrentCandidateTree {
			t.Fatal("corrected candidate tree remained unchanged")
		}
		evidence := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidence, []byte("focused and full tests pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var approved ReviewFacadeFinalizeResult
		decodeBinaryJSON(t, runReviewBinary(t, binary, true, "finalize", "--cwd", corrected, "--evidence", evidence), &approved)
		status = binaryReviewStatus(t, binary, corrected, started.LineageID)
		if approved.State != reviewtransaction.StateApproved || status.Authority == nil || status.Authority.State != reviewtransaction.StateApproved || status.Receipt.Status != ReviewReceiptPresent || status.Receipt.Identity == "" {
			t.Fatalf("approved status = %#v, finalize = %#v", status, approved)
		}
		var validated ReviewValidateResult
		decodeBinaryJSON(t, runReviewBinary(t, binary, true,
			"validate", "--cwd", corrected, "--lineage", started.LineageID, "--gate", string(reviewtransaction.GatePostApply)), &validated)
		if !validated.Allowed || validated.Result != reviewtransaction.GateAllow {
			t.Fatalf("post-apply validation = %#v", validated)
		}
		var binding map[string]any
		decodeBinaryJSON(t, runReviewBinary(t, binary, true,
			"bind-sdd", "--cwd", corrected, "--change", "binary-review", "--lineage", started.LineageID, "--expected-binding-revision="), &binding)
		if binding["schema"] != "gentle-ai.sdd-review-binding/v1" {
			t.Fatalf("SDD review binding = %#v", binding)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "rejects unchanged candidate", mutate: func(t *testing.T, repo string) { writeBinaryCandidate(t, repo, "wrong") }},
		{name: "rejects path expansion", mutate: func(t *testing.T, repo string) {
			writeBinaryCandidate(t, repo, "fixed")
			if err := os.WriteFile(filepath.Join(repo, "expanded.txt"), []byte("outside frozen scope\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, corrected, started := prepareBinaryCorrection(t, binary)
			test.mutate(t, corrected)
			validation := filepath.Join(t.TempDir(), "validation.json")
			writeReviewCLIJSON(t, validation, facadeValidationResult{
				OriginalCriteria:     facadeValidationCheck{Passed: true, Evidence: []string{"original acceptance passed"}},
				CorrectionRegression: facadeValidationCheck{Passed: true, Evidence: []string{"targeted regression passed"}},
				FollowUps:            []reviewtransaction.FollowUp{},
			})
			runReviewBinary(t, binary, false, "finalize", "--cwd", corrected, "--validation", validation)
			status := binaryReviewStatus(t, binary, repo, started.LineageID)
			if status.Authority == nil || status.Authority.State != reviewtransaction.StateCorrectionRequired || status.Receipt.Status != ReviewReceiptExpectedMissing {
				t.Fatalf("rejected correction mutated public authority: %#v", status)
			}
		})
	}
}

func prepareBinaryCorrection(t *testing.T, binary string) (string, string, ReviewFacadeStartResult) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	change := filepath.Join(repo, "openspec", "changes", "binary-review")
	for path, content := range map[string]string{
		"tasks.md":    "- [x] 1.1 Exercise the native review lifecycle\n",
		"proposal.md": "# Binary review acceptance\n",
	} {
		fullPath := filepath.Join(change, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "openspec")
	runReviewCLIGit(t, repo, "commit", "-qm", "add binary review fixture")
	writeBinaryCandidate(t, repo, "wrong")
	var started ReviewFacadeStartResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "start", "--cwd", repo), &started)
	reviewer := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, reviewer, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate returns the wrong terminal value",
		ProofRefs: []string{"differential test fails only on candidate"}, EvidenceClass: reviewtransaction.EvidenceDeterministic, CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"focused differential test failed"}})
	var correction ReviewFacadeFinalizeResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "finalize", "--cwd", repo, "--result", reviewer), &correction)
	if correction.State != reviewtransaction.StateCorrectionRequired {
		t.Fatalf("review state = %q", correction.State)
	}
	runReviewBinary(t, binary, true, "finalize", "--cwd", repo, "--correction-lines", "2")
	corrected := filepath.Join(t.TempDir(), "corrected")
	runReviewCLIGit(t, repo, "worktree", "add", "--detach", corrected, "HEAD")
	return repo, corrected, started
}

func writeBinaryCandidate(t *testing.T, repo, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\n"+value+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runReviewBinary(t *testing.T, binary string, wantSuccess bool, args ...string) []byte {
	t.Helper()
	command := exec.Command(binary, append([]string{"review"}, args...)...)
	output, err := command.CombinedOutput()
	if (err == nil) != wantSuccess {
		t.Fatalf("gentle-ai review %v: %v\n%s", args, err, output)
	}
	return output
}

func decodeBinaryJSON(t *testing.T, payload []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatalf("decode binary output: %v\n%s", err, payload)
	}
}

func binaryReviewStatus(t *testing.T, binary, repo, lineage string) ReviewTargetStatusResult {
	t.Helper()
	var status ReviewTargetStatusResult
	decodeBinaryJSON(t, runReviewBinary(t, binary, true, "status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage), &status)
	return status
}
