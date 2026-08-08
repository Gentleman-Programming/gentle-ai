package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// partiallyCapturedReview starts a high-risk review and captures every selected
// lens but the last, reproducing issue #1958: one reviewer could not inspect the
// frozen trees, so it has nothing admissible to return, while its peers already
// captured. It returns the repo, the lineage, the frozen snapshot identity, the
// captured lens names and the uncaptured one.
func partiallyCapturedReview(t *testing.T) (repo, lineage, snapshotIdentity string, captured []string, missing string) {
	t.Helper()
	repo, started, _, record := newArtifactReview(t, true)
	lenses := record.State.SelectedLenses
	if len(lenses) < 2 {
		t.Fatalf("high-risk selected lenses = %v, want a multi-lens plan", lenses)
	}
	for order := 0; order < len(lenses)-1; order++ {
		input := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", order))
		if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, lenses[order], order), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunReviewCaptureResult([]string{
			"--cwd", repo, "--lineage", started.LineageID,
			"--target", record.State.InitialSnapshot.Identity,
			"--lens", lenses[order], "--order", fmt.Sprint(order), "--input", input,
		}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	return repo, started.LineageID, record.State.InitialSnapshot.Identity, lenses[:len(lenses)-1], lenses[len(lenses)-1]
}

// TestPartialCaptureCollectTransitionNeverTerminates pins the dead-end itself:
// while a selected lens is missing, the negotiated route asks for it forever.
func TestPartialCaptureCollectTransitionNeverTerminates(t *testing.T) {
	repo, lineage, _, _, missing := partiallyCapturedReview(t)
	var out bytes.Buffer
	if err := RunReview([]string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", lineage,
	}, &out); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, out.Bytes(), &status)
	transition := status.NextTransition
	if transition == nil || transition.Kind != reviewNextTransitionCollect || transition.ReasonCode != "reviewer_results_required" ||
		transition.Collect == nil || len(transition.Collect.Inputs) != 1 {
		t.Fatalf("partial-capture transition = %#v", transition)
	}
	if !strings.Contains(fmt.Sprint(transition.Collect.Inputs[0].Arguments), missing) {
		t.Fatalf("collect input does not name the uncaptured lens %q: %#v", missing, transition.Collect.Inputs[0])
	}
}

// TestAbandonQuarantinesPartialReviewWithV2Binding proves the V2 contract for
// issue #1958: an authorized disposition may retire the dead-end, but its
// binding and audit proof must name the exact admitted results it discards.
func TestAbandonQuarantinesPartialReviewWithV2Binding(t *testing.T) {
	repo, lineage, snapshotIdentity, _, _ := partiallyCapturedReview(t)
	eligibility, err := reviewtransaction.InspectCompactPristineAbandonment(context.Background(), repo, lineage)
	if err != nil || !eligibility.Eligible {
		t.Fatalf("partial review abandonment eligibility = %#v, %v", eligibility, err)
	}
	const actor = "maintainer@example.com"
	const reason = reviewtransaction.CompactAbandonReasonOperatorDisposition
	authorization := reviewtransaction.RenderCompactAbandonAuthorization(
		lineage, eligibility.Revision, snapshotIdentity, actor, reason, eligibility.DiscardedWork)

	var out bytes.Buffer
	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", eligibility.Revision, "--reason", reason, "--actor", actor,
		"--maintainer-authorization", authorization,
	}, &out); err != nil {
		t.Fatalf("review abandon with v2 binding: %v\n%s", err, out.String())
	}

	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	proof := result.Record.Abandonment
	if proof == nil || proof.Schema != reviewtransaction.CompactAbandonAuthorizationSchema ||
		strings.Join(proof.DiscardedWork.CapturedLensResults, ",") != strings.Join(eligibility.DiscardedWork.CapturedLensResults, ",") ||
		proof.DiscardedWork.FindingsPresent != eligibility.DiscardedWork.FindingsPresent ||
		proof.DiscardedWork.EvidenceRecordsPresent != eligibility.DiscardedWork.EvidenceRecordsPresent {
		t.Fatalf("v2 abandonment proof = %#v, want discarded work %#v", proof, eligibility.DiscardedWork)
	}
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage)); !os.IsNotExist(err) {
		t.Fatalf("abandoned entry still present: %v", err)
	}
	if err := RunReview([]string{
		"validate", "--cwd", repo, "--gate", string(reviewtransaction.GatePostApply),
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("post-apply gate allowed after a v2 abandonment")
	}
}

func TestAbandonRefusesBindingWithChangedDiscardedWork(t *testing.T) {
	repo, lineage, snapshotIdentity, _, _ := partiallyCapturedReview(t)
	eligibility, err := reviewtransaction.InspectCompactPristineAbandonment(context.Background(), repo, lineage)
	if err != nil || !eligibility.Eligible || len(eligibility.DiscardedWork.CapturedLensResults) == 0 {
		t.Fatalf("partial review abandonment eligibility = %#v, %v", eligibility, err)
	}
	understated := eligibility.DiscardedWork
	understated.CapturedLensResults = nil
	err = RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", eligibility.Revision,
		"--reason", reviewtransaction.CompactAbandonReasonOperatorDisposition, "--actor", "maintainer@example.com",
		"--maintainer-authorization", reviewtransaction.RenderCompactAbandonAuthorization(
			lineage, eligibility.Revision, snapshotIdentity, "maintainer@example.com", reviewtransaction.CompactAbandonReasonOperatorDisposition, understated),
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "exact maintainer authorization binding") {
		t.Fatalf("understated discarded work binding = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage)); statErr != nil {
		t.Fatalf("refused abandonment still moved the entry: %v", statErr)
	}
}

func reviewAuthorityRevisionForTest(t *testing.T, repo, lineage string) string {
	t.Helper()
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record.Revision
}
