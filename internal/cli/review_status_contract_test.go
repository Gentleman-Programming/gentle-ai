package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func sortStrings(values []string) { sort.Strings(values) }

func transitionArgumentValue(t *testing.T, transition *ReviewNextTransition, name string) string {
	t.Helper()
	if transition == nil || transition.Collect == nil {
		t.Fatalf("transition has no collect input: %#v", transition)
	}
	for _, argument := range transition.Collect.Inputs[0].Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	t.Fatalf("transition argument %q is absent: %#v", name, transition)
	return ""
}

func assertReviewFailureMatchesPublishedSchema(t *testing.T, failure ReviewIntegrationFailure) {
	t.Helper()
	if err := failure.Validate(); err != nil {
		t.Fatalf("review failure violates its negotiated schema: %v", err)
	}
}

// retiredSnapshotIdentityForCLITest is retained as an opaque historical-fixture
// marker for callers that need a compact lineage occupying a selected name.
func retiredSnapshotIdentityForCLITest() {}

func relocateCompactRecordWithIdentities(t *testing.T, repo, sourceLineage, targetLineage string, _ any) {
	t.Helper()
	source, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, sourceLineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := source.Load()
	if err != nil {
		t.Fatal(err)
	}
	record.State.LineageID = targetLineage
	// This fixture represents retained historical occupancy. Its source atomic
	// START binding belongs to the original lineage and must not be replayed
	// under the relocated identity.
	record.State.InitialAtomicStart = nil
	target, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, targetLineage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Replace("", "review/start", record.State); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentStatusRoutesReviewerCaptureWithoutCompactPublication(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStart(t, repo, "status-current-capture")

	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--lineage", started.LineageID,
		"--contract", ReviewIntegrationContractV2, "--next-transition",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.Authority == nil || status.Authority.State != reviewtransaction.StateReviewing ||
		status.NextTransition == nil ||
		status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("current status = %#v", status)
	}
}

func TestHistoricalReceiptStatusRemainsReadable(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("historical candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := seedHistoricalApprovalForRepo(t, repo, "historical-status-readable")
	if fixture.Record.State.State != reviewtransaction.StateApproved {
		t.Fatalf("historical fixture = %#v", fixture.Record)
	}
}
