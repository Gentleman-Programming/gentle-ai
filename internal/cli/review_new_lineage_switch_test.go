package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewStartAlwaysCreatesCompactAuthority proves real START has no
// activation toggle or v3 fallback: enabled START writes only exact compact
// authority through CreateOrReplayAtomicStart.
func TestReviewStartAlwaysCreatesCompactAuthority(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	path := filepath.Join(repo, "docs", "atomic-start.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("atomic start fixture\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "docs/atomic-start.md")

	var out bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &out); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, out.Bytes(), &started)
	if started.LineageID == "" || started.State != reviewtransaction.StateReviewing {
		t.Fatalf("atomic START = %#v", started)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil || record.State.InitialAtomicStart == nil {
		t.Fatalf("atomic START compact authority = %#v, %v", record, err)
	}
}

// TestReviewStartCreatesNoLegacyOrV3Siblings proves an explicit START creates
// exactly its v2 compact record and no same-lineage authority in retained stores.
func TestReviewStartCreatesNoLegacyOrV3Siblings(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = "ordinary documentation line"
	}
	writeReviewStartCandidate(t, repo, "docs/atomic-start-explicit.md", joinReviewCLILines(lines), 0o755)

	var out bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "atomic-start-explicit"}, &out); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, out.Bytes(), &started)
	if started.LineageID != "atomic-start-explicit" || started.State != reviewtransaction.StateReviewing {
		t.Fatalf("compact START = %#v", started)
	}
	if started.RiskLevel == reviewtransaction.RiskLow || started.CorrectionBudget == 0 {
		t.Fatalf("compact START tier/budget = %#v, want an active non-low authority", started)
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "atomic-start-explicit")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "review-state.json" {
		t.Fatalf("compact lineage dir entries = %v, want exactly [review-state.json]", entries)
	}

	legacy, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, "atomic-start-explicit")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.LoadChain(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("atomic START also created a legacy v1 authority: %v", err)
	}
}

func joinReviewCLILines(lines []string) string {
	joined := ""
	for _, line := range lines {
		joined += line + "\n"
	}
	return joined
}
