package reviewtransaction

import (
	"context"
	"reflect"
	"testing"
)

// The sealed rctx3 handle is the only repository context OpenCode STATUS
// issues, so the targeted validator's corrected inspection must resolve it.
func TestResolveCorrectedCandidateInspectionBindingAcceptsSealedOpenCodeHandle(t *testing.T) {
	repo, request, correction, _, binding, _ := correctedInspectionFixture(t, "corrected-inspection-rctx3", nil)
	ctx := context.Background()
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(ctx, repo, binding)
	if err != nil {
		t.Fatal(err)
	}

	_, resolved, err := ResolveCorrectedCandidateInspectionBinding(ctx, repo, handle, binding, request.RequestHash)
	if err != nil {
		t.Fatalf("sealed OpenCode handle corrected inspection: %v", err)
	}
	if !snapshotsEqual(resolved, correction) {
		t.Fatalf("resolved = %#v, want %#v", resolved, correction)
	}
}

func TestResolveCorrectedCandidateInspectionSealedHandleUsesFrozenRequestAfterLiveDrift(t *testing.T) {
	repo, request, correction, _, binding, store := correctedInspectionFixture(t, "corrected-inspection-rctx3-drift", nil)
	ctx := context.Background()
	handle, err := DeriveOpenCodeReviewRepositoryContextHandle(ctx, repo, binding)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	writeSnapshotFile(t, repo, "tracked.txt", "base\ndecoy drift\n")
	resolved, err := ResolveCorrectedCandidateInspection(ctx, t.TempDir(), handle, request)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotsEqual(resolved, correction) || !reflect.DeepEqual(after, before) {
		t.Fatalf("resolved=%#v authority changed=%t", resolved, !reflect.DeepEqual(after, before))
	}
	payload, err := (SnapshotBuilder{Repo: repo}).InspectCandidate(ctx, resolved, "object", 0, "candidate")
	if err != nil || string(payload) != "base\nfixed\n" {
		t.Fatalf("immutable corrected inspection = %q, %v", payload, err)
	}
}

func TestResolveCorrectedCandidateInspectionSealedHandleFailsClosed(t *testing.T) {
	t.Run("handle sealed for another binding", func(t *testing.T) {
		repo, request, _, _, binding, _ := correctedInspectionFixture(t, "corrected-inspection-rctx3-other", nil)
		ctx := context.Background()
		other := binding
		other.TargetIdentity = hash("9")
		handle, err := DeriveOpenCodeReviewRepositoryContextHandle(ctx, repo, other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveCorrectedCandidateInspection(ctx, repo, handle, request); err == nil {
			t.Fatal("rctx3 handle sealed for another binding resolved")
		}
	})
	t.Run("request target mismatch", func(t *testing.T) {
		repo, request, _, _, binding, _ := correctedInspectionFixture(t, "corrected-inspection-rctx3-mismatch", nil)
		ctx := context.Background()
		handle, err := DeriveOpenCodeReviewRepositoryContextHandle(ctx, repo, binding)
		if err != nil {
			t.Fatal(err)
		}
		request.CorrectionTargetIdentity = hash("9")
		request.RequestHash = targetedValidationRequestHash(request)
		if _, err := ResolveCorrectedCandidateInspection(ctx, repo, handle, request); err == nil {
			t.Fatal("rctx3 handle resolved for a request naming another correction target")
		}
	})
}
