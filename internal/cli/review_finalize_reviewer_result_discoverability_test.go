package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestFinalizeReviewerResultRefusalsNameCaptureResultContinuation closes a gap
// two independent community testers hit from different angles (fisidj finding
// 2, ardelperal): every reviewer-result count-mismatch refusal at finalize
// blocked with a bare precondition and named no continuation. The spec's
// In-Band Recovery Discoverability requirement demands the continuation be
// named, and Phase 3b's wording standard is to name a real runnable command,
// not just describe the concept. The command name here is derived from the
// exact same CaptureOperation constant the collect transition already emits
// (review_next_transition.go's reviewCaptureInput), so it structurally cannot
// drift from what the collect form actually names.
func TestFinalizeReviewerResultRefusalsNameCaptureResultContinuation(t *testing.T) {
	t.Run("prepareCompactReviewerResults inline aggregate mismatch", func(t *testing.T) {
		state := reviewtransaction.CompactState{LineageID: "lineage-x", SelectedLenses: []string{"reviewer"}}
		_, err := prepareCompactReviewerResults(state, nil, facadeRefuterResult{})
		if err == nil {
			t.Fatal("expected a reviewer-result count mismatch error")
		}
		assertNamesCaptureResultContinuation(t, err.Error())
	})

	t.Run("readFacadeReviewerArtifacts artifact count mismatch", func(t *testing.T) {
		state := reviewtransaction.CompactState{LineageID: "lineage-x", SelectedLenses: []string{"reviewer"}}
		_, err := readFacadeReviewerArtifacts(context.Background(), "", nil, "", state, "")
		if err == nil {
			t.Fatal("expected a reviewer-artifact count mismatch error")
		}
		assertNamesCaptureResultContinuation(t, err.Error())
	})

	t.Run("readCapturedReviewerResults native discovery mismatch", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate needing a reviewer result\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		started := startFacadeReview(t, repo)
		if len(started.SelectedLenses) == 0 {
			t.Skip("risk assessment selected zero lenses for this candidate; nothing to leave uncaptured")
		}

		var output bytes.Buffer
		err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results"}, &output)
		if err == nil {
			t.Fatalf("expected a captured reviewer result count mismatch error, got success: %s", output.String())
		}
		assertNamesCaptureResultContinuation(t, err.Error())
	})
}

func assertNamesCaptureResultContinuation(t *testing.T, message string) {
	t.Helper()
	if !strings.Contains(message, reviewCaptureResultCommandName()) {
		t.Fatalf("refusal does not name the review capture-result continuation: %q", message)
	}
	if !strings.Contains(message, reviewNextTransitionRefreshCommand) {
		t.Fatalf("refusal does not point at the exact-binding source for the continuation: %q", message)
	}
}
