package cli

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestUnachievableLensSlotRegression(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]

	frozen, err := reviewerArtifactFrozenContext(context.Background(), repo, record.State)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, frozen, lens, 0, "")
	if err != nil {
		t.Fatal(err)
	}

	captureAttempt := func(index int) {
		req := reviewtransaction.CaptureReviewerAttemptRequest{
			StoreDir:          store.Dir,
			LineageID:         started.LineageID,
			TargetIdentity:    record.State.InitialSnapshot.Identity,
			AuthorityRevision: record.Revision,
			Lens:              lens,
			SelectedOrder:     0,
			SubjectHash:       subject.SubjectHash,
			Admission: reviewtransaction.ArtifactAdmission{
				Schema:      reviewtransaction.ArtifactAdmissionSchema,
				Decision:    reviewtransaction.ArtifactAdmissionUnachievable,
				SubjectHash: subject.SubjectHash,
				Diagnostic:  "provider model refused or crashed",
			},
			RawPayload:       []byte("model failure"),
			CanonicalPayload: []byte("model failure"),
		}
		rec, err := store.CaptureUnachievableReviewerAttempt(context.Background(), req)
		if err != nil {
			t.Fatalf("attempt %d capture error: %v", index, err)
		}
		if rec.AttemptIndex != index {
			t.Fatalf("rec.AttemptIndex = %d, want %d", rec.AttemptIndex, index)
		}
	}

	getStatus := func() ReviewTargetStatusResult {
		var buf bytes.Buffer
		statusArgs := []string{
			"status", "--contract", ReviewIntegrationContractV1, "--next-transition",
			"--cwd", repo, "--lineage", started.LineageID,
		}
		if err := RunReview(statusArgs, &buf); err != nil {
			t.Fatalf("status error: %v", err)
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, buf.Bytes(), &status)
		return status
	}

	// 1st attempt: collect re-offered
	captureAttempt(1)
	s1 := getStatus()
	if s1.NextTransition == nil || s1.NextTransition.Kind != reviewNextTransitionCollect || s1.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("status after attempt 1 = %#v, want collect reviewer_results_required", s1.NextTransition)
	}

	// 2nd attempt: collect re-offered
	captureAttempt(2)
	s2 := getStatus()
	if s2.NextTransition == nil || s2.NextTransition.Kind != reviewNextTransitionCollect || s2.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("status after attempt 2 = %#v, want collect reviewer_results_required", s2.NextTransition)
	}

	// 3rd attempt: bounded retry limit reached -> terminal stop
	captureAttempt(3)
	s3 := getStatus()
	if s3.NextTransition == nil || s3.NextTransition.Kind != reviewNextTransitionStop || s3.NextTransition.ReasonCode != "unachievable_reviewer_attempt" {
		t.Fatalf("status after attempt 3 = %#v, want stop unachievable_reviewer_attempt", s3.NextTransition)
	}

	// Finalize must be rejected
	finalizeErr := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--captured-results"}, &bytes.Buffer{})
	if finalizeErr == nil {
		t.Fatal("finalize succeeded on exhausted unachieved slot")
	}

	// Receipt must NOT be published
	if _, err := os.Stat(store.ReceiptPath()); !os.IsNotExist(err) {
		t.Fatalf("receipt was published on unachieved review: %v", err)
	}
}
