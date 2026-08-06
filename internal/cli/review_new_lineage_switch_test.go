package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewStartNewLineageSwitchOffCreatesNoV3Entries is task 4.6 (spec
// rdd-new-lineage-activation, "Kill-Switch-Off Is Structurally Unfailable
// and Creates Nothing"): with GENTLE_AI_RDD_NEW_LINEAGE unset (the default),
// `review start` takes the legacy path and the v3/ authority root is never
// created — not even an empty directory.
func TestReviewStartNewLineageSwitchOffCreatesNoV3Entries(t *testing.T) {
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "")
	repo := initReviewCLIRepo(t)
	path := filepath.Join(repo, "docs", "kill-switch-off.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("kill-switch-off structural-unfailure fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo}, &out); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, out.Bytes(), &started)
	if started.LineageID == "" {
		t.Fatal("legacy start did not report a lineage")
	}

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	v3Root := filepath.Dir(store.Dir)
	if _, statErr := os.Stat(v3Root); !os.IsNotExist(statErr) {
		t.Fatalf("switch OFF created a v3/ authority root: %s (stat err = %v)", v3Root, statErr)
	}
}

// TestReviewStartNewLineageSwitchOnFreezesV3Authority is task 4.5's GREEN
// evidence for the ON branch: GENTLE_AI_RDD_NEW_LINEAGE set routes `review
// start` through ReviewCore.Next and AuthorityStore.Mutate, writing exactly
// review-state.json under v3/<lineage>/ with the tier, lenses, and
// correction budget the shared preamble already computed — and creates no
// legacy v1/v2 artifact for that lineage.
func TestReviewStartNewLineageSwitchOnFreezesV3Authority(t *testing.T) {
	t.Setenv("GENTLE_AI_RDD_NEW_LINEAGE", "1")
	repo := initReviewCLIRepo(t)
	lines := make([]string, 129)
	for index := range lines {
		lines[index] = "ordinary documentation line"
	}
	writeReviewStartCandidate(t, repo, "docs/kill-switch-on.md", joinReviewCLILines(lines), 0o644)

	var out bytes.Buffer
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "new-lineage-switch-on"}, &out); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartNewLineageResult
	decodeStrictReviewJSON(t, out.Bytes(), &started)
	if started.LineageID != "new-lineage-switch-on" || started.State != reviewtransaction.NewLineageStateReviewing {
		t.Fatalf("new-lineage START = %#v", started)
	}
	if started.RiskLevel != reviewtransaction.RiskLow || started.CorrectionBudget != 65 {
		t.Fatalf("new-lineage START tier/budget = %#v, want low/65", started)
	}

	store, err := reviewtransaction.NewLineageAuthorityStore(context.Background(), repo, "new-lineage-switch-on")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "review-state.json" {
		t.Fatalf("v3 lineage dir entries = %v, want exactly [review-state.json]", entries)
	}

	legacy, legacyErr := reviewtransaction.AuthoritativeStore(context.Background(), repo, "new-lineage-switch-on")
	if legacyErr != nil {
		t.Fatal(legacyErr)
	}
	if _, loadErr := legacy.LoadChain(); loadErr == nil {
		t.Fatal("switch ON also created a legacy v1/v2 authority for the same lineage")
	}
}

func joinReviewCLILines(lines []string) string {
	joined := ""
	for _, line := range lines {
		joined += line + "\n"
	}
	return joined
}
