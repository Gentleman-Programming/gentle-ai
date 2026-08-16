package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// approvedCommittedCorrectionReceipt drives the #2017 predecessor to its
// terminal receipt: a committed base-diff review of an already-merged
// candidate finds a deterministic CRITICAL, the one bounded correction lands
// as a follow-up commit, targeted validation passes, and finalize approves.
// It returns the repository, the lineage, and the merged candidate commit
// (the publication base of the correction-only PR).
func approvedCommittedCorrectionReceipt(t *testing.T) (string, string, string) {
	t.Helper()
	repo, _, lineage := forecastCommittedCorrection(t)
	merged := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeCommittedCorrection(t, repo, false)

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
	request := capturePassedCorrectionEvidenceForTest(t, repo, lineage)
	validation := filepath.Join(t.TempDir(), "validation.json")
	writeReviewCLIJSON(t, validation, facadeValidationResult{
		TargetedValidationRequestHash: request.RequestHash,
		CorrectionTargetIdentity:      request.CorrectionTargetIdentity,
		OriginalCriteria:              facadeValidationCheck{Passed: true, Evidence: []string{"original criteria passed"}},
		CorrectionRegression:          facadeValidationCheck{Passed: true, Evidence: []string{"correction regression passed"}},
		FollowUps:                     []reviewtransaction.FollowUp{},
	})
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--contract", ReviewIntegrationContractV1, "--validation", validation, "--captured-evidence",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize corrected candidate: %v", err)
	}
	record, err := authorityStore.Load()
	if err != nil || record.State.State != reviewtransaction.StateApproved {
		t.Fatalf("fixture did not reach an approved receipt: %#v, %v", record.State.State, err)
	}
	return repo, lineage, merged
}

// TestPrePRAllowsCorrectionOnlyDeliveryOverMergedCandidate is issue #2017.
//
// The approved receipt covers the original merged candidate plus its one
// bounded correction. The corrective PR's publication base is the merged
// candidate itself, so its diff contains only the correction. The reporter's
// v2.2.2 gate classified that exact projection as unrelated and denied it,
// then STATUS recommended a START that returned blocked-scope-action — a
// dead end with no delivery path for content the system itself prescribed.
//
// The accepted design: the receipt authorizes exactly this one derived
// projection — base tree equal to the receipt's original candidate tree,
// candidate tree equal to its corrected candidate tree — and nothing else.
func TestPrePRAllowsCorrectionOnlyDeliveryOverMergedCandidate(t *testing.T) {
	repo, lineage, merged := approvedCommittedCorrectionReceipt(t)

	// The corrective PR: base = the merged candidate commit on the remote
	// mainline, head = the correction commit. Pre-PR only accepts an
	// advertised <remote>/<branch> base.
	remote := filepath.Join(t.TempDir(), "origin.git")
	runReviewCLIGit(t, repo, "init", "--bare", remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "push", "origin", merged+":refs/heads/merged-main")

	var output bytes.Buffer
	if err := RunReviewFacadeValidate([]string{
		"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePrePR),
		"--base-ref", "origin/merged-main",
	}, &output); err != nil {
		t.Fatalf("pre-pr denied the correction-only delivery the receipt approves (#2017): %v\n%s", err, output.String())
	}
}

// TestPrePRCorrectionOnlyMatchesOnlyTheDerivedPair pins the #2017 boundary:
// the receipt authorizes exactly one derived projection. A head that is not
// the corrected tree, or a base that is not the merged original candidate,
// still denies — the relaxation is not "the receipt covers any subset".
func TestPrePRCorrectionOnlyMatchesOnlyTheDerivedPair(t *testing.T) {
	repo, lineage, merged := approvedCommittedCorrectionReceipt(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	runReviewCLIGit(t, repo, "init", "--bare", remote)
	runReviewCLIGit(t, repo, "remote", "add", "origin", remote)
	runReviewCLIGit(t, repo, "push", "origin", merged+":refs/heads/merged-main")

	t.Run("head beyond the corrected tree denies", func(t *testing.T) {
		writeReviewStartCandidate(t, repo, "extra.go", "package candidate\n\nvar smuggled = true\n", 0o644)
		runReviewCLIGit(t, repo, "add", "extra.go")
		runReviewCLIGit(t, repo, "commit", "-qm", "unreviewed extra on top of the correction")
		t.Cleanup(func() { runReviewCLIGit(t, repo, "reset", "--hard", "HEAD~1") })

		var output bytes.Buffer
		if err := RunReviewFacadeValidate([]string{
			"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePrePR),
			"--base-ref", "origin/merged-main",
		}, &output); err == nil {
			t.Fatalf("pre-pr allowed unreviewed content on top of the correction:\n%s", output.String())
		}
	})

	t.Run("pre-merge base keeps the full projection allowed", func(t *testing.T) {
		unrelated := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", merged+"~1"))
		runReviewCLIGit(t, repo, "push", "origin", unrelated+":refs/heads/premerge-main")

		var output bytes.Buffer
		err := RunReviewFacadeValidate([]string{
			"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePrePR),
			"--base-ref", "origin/premerge-main",
		}, &output)
		// The pre-merge base IS the receipt's own reviewed base, so this is
		// the full projection the gate always allowed; it must keep working.
		if err != nil {
			t.Fatalf("pre-pr broke the original full projection: %v\n%s", err, output.String())
		}
	})

	t.Run("foreign base intersecting the reviewed scope denies", func(t *testing.T) {
		// A base that is neither the receipt's reviewed base nor the merged
		// original candidate, whose drift touches the reviewed path itself.
		// Disjoint drift is compatible-base-advance territory and stays
		// allowed; intersecting drift must stay outside both that proof and
		// the #2017 derived pair.
		runReviewCLIGit(t, repo, "branch", "foreign", merged+"~1")
		runReviewCLIGit(t, repo, "checkout", "-q", "foreign")
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\nfunc value() int {\n\treturn 99\n}\n", 0o644)
		runReviewCLIGit(t, repo, "add", "candidate.go")
		runReviewCLIGit(t, repo, "commit", "-qm", "foreign drift")
		foreign := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
		runReviewCLIGit(t, repo, "checkout", "-q", "-")
		runReviewCLIGit(t, repo, "push", "origin", foreign+":refs/heads/foreign-main")

		var output bytes.Buffer
		if err := RunReviewFacadeValidate([]string{
			"--cwd", repo, "--lineage", lineage, "--gate", string(reviewtransaction.GatePrePR),
			"--base-ref", "origin/foreign-main",
		}, &output); err == nil {
			t.Fatalf("pre-pr allowed delivery over a base the receipt never derived:\n%s", output.String())
		}
	})
}
