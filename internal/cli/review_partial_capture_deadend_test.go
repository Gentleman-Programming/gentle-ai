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
// This is the condition the abandonment class below has to end.
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

// TestAbandonQuarantinesReviewingLineageWithIncompleteInspection is the fix for
// issue #1958. A maintainer who has read the lineage can retire it explicitly,
// discarding the review rather than approving it, with the captured work
// recorded in the audit trail instead of silently dropped.
func TestAbandonQuarantinesReviewingLineageWithIncompleteInspection(t *testing.T) {
	repo, lineage, snapshotIdentity, captured, missing := partiallyCapturedReview(t)
	revision := reviewAuthorityRevisionForTest(t, repo, lineage)
	reason := "lens " + missing + " could not inspect the frozen trees"
	actor := "maintainer@example.com"

	var out bytes.Buffer
	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", revision, "--incomplete-inspection",
		"--reason", reason, "--actor", actor,
		"--maintainer-authorization", reviewtransaction.RenderCompactIncompleteAbandonAuthorization(
			lineage, revision, snapshotIdentity, captured, actor, reason),
	}, &out); err != nil {
		t.Fatalf("review abandon --incomplete-inspection: %v\n%s", err, out.String())
	}

	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, out.Bytes(), &result)
	proof := result.Record.IncompleteAbandonment
	if proof == nil {
		t.Fatalf("incomplete abandonment proof missing: %#v", result.Record)
	}
	if !reflectDeepEqualStrings(proof.CapturedLenses, captured) || proof.UncapturedLenses[0] != missing {
		t.Fatalf("proof lenses = captured %v uncaptured %v, want %v / [%s]", proof.CapturedLenses, proof.UncapturedLenses, captured, missing)
	}
	result.Record.Status = reviewtransaction.CompactReclaimPrepared
	if err := os.WriteFile(filepath.Join(result.Record.QuarantinePath, "reclaim-record.json"), []byte(mustReviewJSON(t, result.Record)), 0o644); err != nil {
		t.Fatal(err)
	}
	replayed, err := reviewtransaction.AbandonPristineCompactStore(context.Background(), repo, reviewtransaction.CompactAbandonRequest{
		LineageID: lineage, ExpectedRevision: revision, IncompleteInspection: true,
		Reason: reason, Actor: actor,
		MaintainerAuthorization: reviewtransaction.RenderCompactIncompleteAbandonAuthorization(lineage, revision, snapshotIdentity, captured, actor, reason),
	})
	if err != nil || replayed.Status != reviewtransaction.CompactReclaimCommitted || replayed.QuarantinePath != result.Record.QuarantinePath {
		t.Fatalf("prepared abandonment replay = %#v, %v", replayed, err)
	}

	// The store entry is gone: the lineage is terminal, not waiting.
	if _, err := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage)); !os.IsNotExist(err) {
		t.Fatalf("abandoned entry still present: %v", err)
	}
	// And it approved nothing: the candidate remains unreviewed.
	if err := RunReview([]string{
		"validate", "--cwd", repo, "--gate", string(reviewtransaction.GatePostApply),
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("post-apply gate allowed after an incomplete-inspection abandonment")
	}
}

// TestIncompleteAbandonRefusesWithoutExplicitClass keeps the ordinary pristine
// gate exactly as strict as it was: captured work is never discarded by the
// plain abandonment path.
func TestIncompleteAbandonRefusesWithoutExplicitClass(t *testing.T) {
	repo, lineage, snapshotIdentity, _, _ := partiallyCapturedReview(t)
	revision := reviewAuthorityRevisionForTest(t, repo, lineage)
	reason := "retire without naming the class"
	actor := "maintainer@example.com"
	err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", revision, "--reason", reason, "--actor", actor,
		"--maintainer-authorization", reviewtransaction.RenderCompactAbandonAuthorization(
			lineage, revision, snapshotIdentity, actor, reason),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("plain abandon accepted a lineage holding captured reviewer results")
	}
}

// TestIncompleteAbandonRefusesAuthorizationOverWrongCapturedLenses is the
// security guard on the class: the binding carries the captured lenses, so a
// maintainer can only discard the exact amount of review work they were shown.
func TestIncompleteAbandonRefusesAuthorizationOverWrongCapturedLenses(t *testing.T) {
	repo, lineage, snapshotIdentity, captured, missing := partiallyCapturedReview(t)
	revision := reviewAuthorityRevisionForTest(t, repo, lineage)
	reason := "understated how much review work is being discarded"
	actor := "maintainer@example.com"
	resultPath := filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage, reviewtransaction.CompactReviewerResultsDir, fmt.Sprintf("%02d-%s.json", len(captured), missing))
	if err := os.Symlink(resultPath, resultPath); err != nil {
		t.Skipf("cannot create result inspection failure: %v", err)
	}
	if err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", revision, "--incomplete-inspection",
		"--reason", reason, "--actor", actor,
		"--maintainer-authorization", reviewtransaction.RenderCompactIncompleteAbandonAuthorization(
			lineage, revision, snapshotIdentity, captured, actor, reason),
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "inspect incomplete review capture") {
		t.Fatalf("result inspection error = %v", err)
	}
	if err := os.Remove(resultPath); err != nil {
		t.Fatal(err)
	}
	err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", revision, "--incomplete-inspection",
		"--reason", reason, "--actor", actor,
		"--maintainer-authorization", reviewtransaction.RenderCompactIncompleteAbandonAuthorization(
			lineage, revision, snapshotIdentity, captured[:len(captured)-1], actor, reason),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("abandon accepted a binding that understated the captured lenses")
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", lineage)); statErr != nil {
		t.Fatalf("refused abandonment still moved the entry: %v", statErr)
	}
}

// TestIncompleteAbandonRefusesFullyCapturedReview keeps the class from becoming
// a way to drop a complete review's findings: a plan that captured every lens
// can finalize, so it is never abandonable.
func TestIncompleteAbandonRefusesFullyCapturedReview(t *testing.T) {
	repo, started, _, record, _ := capturedArtifacts(t, true)
	lineage := started.LineageID
	revision := reviewAuthorityRevisionForTest(t, repo, lineage)
	reason := "attempt to discard a complete review"
	actor := "maintainer@example.com"
	err := RunReview([]string{
		"abandon", "--cwd", repo, "--lineage", lineage,
		"--expected-revision", revision, "--incomplete-inspection",
		"--reason", reason, "--actor", actor,
		"--maintainer-authorization", reviewtransaction.RenderCompactIncompleteAbandonAuthorization(
			lineage, revision, record.State.InitialSnapshot.Identity, record.State.SelectedLenses, actor, reason),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("abandon discarded a fully captured review")
	}
	if !strings.Contains(err.Error(), "can finalize") {
		t.Fatalf("refusal does not name the continuation: %v", err)
	}
}

// TestIncompleteAbandonInputsRefusalPrintsItsOwnTemplate keeps the operator from
// being handed the six-line pristine token for a seven-line gate.
func TestIncompleteAbandonInputsRefusalPrintsItsOwnTemplate(t *testing.T) {
	err := RunReview([]string{"abandon", "--cwd", t.TempDir(), "--incomplete-inspection"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("abandon accepted an empty incomplete-inspection request")
	}
	if !strings.Contains(err.Error(), reviewtransaction.CompactIncompleteAbandonAuthorizationSchema) ||
		!strings.Contains(err.Error(), "captured_lenses") {
		t.Fatalf("incomplete-inspection refusal printed the wrong template: %v", err)
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
