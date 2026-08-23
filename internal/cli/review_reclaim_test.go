package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func incompleteCompactResidueDir(t *testing.T, repo, lineage string) string {
	t.Helper()
	commonDir := filepath.Clean(strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir")))
	residue := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", lineage)
	if err := os.MkdirAll(residue, 0o755); err != nil {
		t.Fatal(err)
	}
	return residue
}

// TestAuthorityInventoryReportsIncompleteStoreEntry keeps the distinct-cause
// classification for an interrupted store write that left a directory with no
// state in it.
func TestAuthorityInventoryReportsIncompleteStoreEntry(t *testing.T) {
	repo := initReviewCLIRepo(t)
	incompleteCompactResidueDir(t, repo, "reclaim-audit")

	report, err := reviewtransaction.InventoryAuthority(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	classified := false
	for _, entry := range report.Entries {
		if entry.LineageID == "reclaim-audit" {
			classified = entry.Status == reviewtransaction.AuthorityStatusIncomplete
		}
	}
	if !classified {
		t.Fatalf("the incomplete store entry lost its distinct classification: %#v", report.Entries)
	}
}

func TestReviewReclaimQuarantinesResidue(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	residue := incompleteCompactResidueDir(t, repo, "reclaim-audit")

	var reclaimOutput bytes.Buffer
	if err := RunReview([]string{
		"reclaim", "--cwd", repo, "--lineage", "reclaim-audit",
		"--reason", "interrupted store write residue", "--actor", "maintainer@example.com",
	}, &reclaimOutput); err != nil {
		t.Fatalf("review reclaim: %v\n%s", err, reclaimOutput.String())
	}
	var result ReviewReclaimResult
	decodeStrictReviewJSON(t, reclaimOutput.Bytes(), &result)
	if result.Operation != "review/reclaim" || result.Record.LineageID != "reclaim-audit" || result.Record.QuarantinePath == "" {
		t.Fatalf("review reclaim result = %#v", result)
	}
	if _, err := os.Stat(residue); !os.IsNotExist(err) {
		t.Fatalf("reclaimed residue still present: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Record.QuarantinePath, "reclaim-record.json")); err != nil {
		t.Fatalf("reclaim audit record missing: %v", err)
	}

}

func TestReviewStartSucceedsDespiteIncompleteStoreResidue(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	incompleteCompactResidueDir(t, repo, "reclaim-audit")
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "start.md"), []byte("start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/start.md")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-reclaim-start"}, &output); err != nil {
		t.Fatalf("review start with incomplete residue present: %v\n%s", err, output.String())
	}
}

// TestReviewReclaimClearsEnumeratedResidueThatNeverBlockedStart used to prove
// reclaim was the way OUT of a start poisoned by enumerated residue. Nothing
// is poisoned now -- a stray file in a state-less directory is that
// directory's problem -- so what is left to prove is that reclaim still does
// its own job: the residue is quarantined with an audit record, and start
// works on both sides of that, not only after it.
func TestReviewReclaimClearsEnumeratedResidueThatNeverBlockedStart(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	residue := incompleteCompactResidueDir(t, repo, "reclaim-audit")
	if err := os.WriteFile(filepath.Join(residue, "stray.tmp"), []byte("stray\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "start.md"), []byte("start\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/start.md")
	var output bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-reclaim-start"}, &output); err != nil {
		t.Fatalf("enumerated residue blocked an unrelated start: %v\n%s", err, output.String())
	}
	if _, err := os.Stat(filepath.Join(residue, "stray.tmp")); err != nil {
		t.Fatalf("the start consumed or cleaned the residue it should have ignored: %v", err)
	}

	if err := RunReview([]string{
		"reclaim", "--cwd", repo, "--lineage", "reclaim-audit",
		"--reason", "interrupted store write residue", "--actor", "maintainer@example.com",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("reclaim enumerated residue: %v", err)
	}
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-reclaim-start"}, &output); err != nil {
		t.Fatalf("review start after reclaim: %v\n%s", err, output.String())
	}
}
