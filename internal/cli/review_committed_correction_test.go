package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestSelectorlessCommittedBaseDiffCorrectionUsesFrozenBase(t *testing.T) {
	for _, amend := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "amend"}[amend], func(t *testing.T) {
			repo, baseRef, started := forecastCommittedBaseDiffCorrection(t)
			writeCommittedCorrection(t, repo, amend, false)
			runReviewCLIGit(t, repo, "branch", "-f", baseRef, "HEAD")

			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			status := selectorlessCommittedCorrectionStatus(t, repo)
			if status.NextTransition == nil || status.NextTransition.ReasonCode != "correction_repository_verification_required" ||
				status.Authority == nil || status.Authority.LineageID != started.LineageID ||
				status.Projection.Kind != reviewtransaction.TargetBaseDiff || status.Projection.BaseTree != baseRefTree(t, repo, before) {
				t.Fatalf("selector-less committed correction status = %#v", status)
			}
			target := transitionArgumentValue(t, status.NextTransition, "target")
			captureCommittedCorrectionEvidence(t, repo, started.LineageID, status.Authority.Revision, target)
			ready := selectorlessCommittedCorrectionStatus(t, repo)
			if ready.NextTransition == nil || ready.NextTransition.ReasonCode != "targeted_validation_required" ||
				ready.ValidationRequest == nil || ready.ValidationRequest.CorrectionTargetIdentity != target {
				t.Fatalf("selector-less committed correction validation status = %#v", ready)
			}

			var finalized bytes.Buffer
			if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition"}, &finalized); err != nil {
				t.Fatal(err)
			}
			var direct ReviewIntegrationFinalizeResult
			decodeStrictReviewJSON(t, decodeReviewOperationEnvelope(t, finalized.Bytes()).Result, &direct)
			if direct.NextTransition == nil || direct.NextTransition.ReasonCode != "targeted_validation_required" ||
				direct.ValidationRequest == nil || direct.ValidationRequest.CorrectionTargetIdentity != target {
				t.Fatalf("selector-less committed correction finalize = %#v", direct)
			}
			after, err := os.ReadFile(store.StatePath())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("selector-less correction routing mutated authority: %v", err)
			}
		})
	}
}

func TestSelectorlessCommittedBaseDiffCorrectionFailsClosed(t *testing.T) {
	for _, expansion := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "scope expansion"}[expansion], func(t *testing.T) {
			repo, _, started := forecastCommittedBaseDiffCorrection(t)
			store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if expansion {
				writeCommittedCorrection(t, repo, false, true)
				var output bytes.Buffer
				if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition"}, &output); err != nil {
					t.Fatal(err)
				}
				var status ReviewTargetStatusResult
				decodeStrictReviewJSON(t, output.Bytes(), &status)
				if status.Authority != nil && status.Authority.LineageID == started.LineageID {
					t.Fatalf("scope-expanded correction remained a selector-less recovery candidate: %#v", status)
				}
				after, err := os.ReadFile(store.StatePath())
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("scope-expanded correction mutated authority: %v", err)
				}
				return
			}
			status := selectorlessCommittedCorrectionStatus(t, repo)
			if status.ValidationRequest != nil || status.Authority == nil || status.Authority.LineageID != started.LineageID {
				t.Fatalf("invalid committed correction produced validation authority: %#v", status)
			}
			if status.NextTransition == nil || status.NextTransition.ReasonCode != "corrected_candidate_unavailable" {
				t.Fatalf("unchanged committed correction status = %#v", status.NextTransition)
			}
		})
	}
}

func forecastCommittedBaseDiffCorrection(t *testing.T) (string, string, ReviewFacadeStartResult) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	baseRef := "frozen-base"
	runReviewCLIGit(t, repo, "branch", baseRef, "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nwrong\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "wrong candidate")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "committed-base-correction", "--base-ref", baseRef, "--committed-only"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, output.Bytes(), &started)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(t.TempDir(), "reviewer.json")
	writeReviewCLIJSON(t, result, facadeReviewerResult{Findings: []facadeFinding{{
		Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "candidate is wrong",
		ProofRefs: []string{"tracked.txt:5 changed hunk"}, EvidenceClass: reviewtransaction.EvidenceDeterministic,
		CausalDisposition: reviewtransaction.CausalIntroduced,
	}}, Evidence: []string{"reviewed frozen committed candidate"}})
	if err := finalizeReviewCLIArgs(t, repo, []string{"--cwd", repo, "--lineage", started.LineageID, "--result", result}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--correction-lines", "2"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff {
		t.Fatalf("initial correction target kind = %q", record.State.InitialSnapshot.Kind)
	}
	return repo, baseRef, started
}

func writeCommittedCorrection(t *testing.T, repo string, amend, expansion bool) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if expansion {
		if err := os.WriteFile(filepath.Join(repo, "expanded.txt"), []byte("outside frozen scope\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "tracked.txt", "expanded.txt")
	} else {
		runReviewCLIGit(t, repo, "add", "tracked.txt")
	}
	if amend {
		runReviewCLIGit(t, repo, "commit", "--amend", "--no-edit")
		return
	}
	runReviewCLIGit(t, repo, "commit", "-qm", "correct candidate")
}

func selectorlessCommittedCorrectionStatus(t *testing.T, repo string) ReviewTargetStatusResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo, "--contract", ReviewIntegrationContractV1, "--next-transition"}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return status
}

func captureCommittedCorrectionEvidence(t *testing.T, repo, lineage, revision, target string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(path, []byte("repository verification passed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewCaptureEvidence([]string{"--cwd", repo, "--lineage", lineage, "--target", target, "--expected-revision", revision, "--outcome", string(reviewtransaction.VerificationOutcomePassed), "--input", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func baseRefTree(t *testing.T, repo string, state []byte) string {
	t.Helper()
	var record reviewtransaction.CompactRecord
	if err := json.Unmarshal(state, &record); err != nil {
		t.Fatal(err)
	}
	return record.State.InitialSnapshot.BaseTree
}
