package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// mismatchedSubjectReviewerPayload returns a fully well-formed reviewer result
// for this lineage's live lens slot whose ONLY defect is the echoed subject: it
// carries a syntactically valid subject hash that is not the binding's.
func mismatchedSubjectReviewerPayload(t *testing.T, repo string, record reviewtransaction.CompactRecord, lens string, order int) []byte {
	t.Helper()
	result := admittedReviewerResultForTest(t, repo, record, lens, order)
	result.SubjectHash = "sha256:" + strings.Repeat("b", 64)
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// TestReviewCaptureResultRecapturesSameLensAfterBindingMismatch is the sibling
// guard of TestReviewCaptureResultRecapturesSameLensAfterRejectedAdmission.
//
// The omitted-subject rejection already names its continuation: the rejected
// admission never consumed the lens slot, so the lens can be re-run and
// captured again on the same lineage. The binding_mismatch rejection --
// "reviewer result echoed a different artifact subject" -- said nothing
// equivalent, even though it is raised from the immediately adjacent branch of
// the same AdmitArtifact validation, strictly before any store append.
//
// This test proves the recovery is the SAME one, rather than assuming it: it
// asserts the slot is unconsumed and authority unmoved after the mismatch, then
// re-captures on the same lineage and lens and requires the lineage to finalize
// normally. It also requires the rejection to hand back the subject hash the
// binding actually expects, because that is the one value the operator cannot
// derive from the failure alone.
func TestReviewCaptureResultRecapturesSameLensAfterBindingMismatch(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	lens := record.State.SelectedLenses[0]

	frozen, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).FrozenCandidateContext(t.Context(), record.State.InitialSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := reviewtransaction.NewArtifactSubject(record.State, record.Revision, frozen, lens, 0, "")
	if err != nil {
		t.Fatal(err)
	}

	mismatched := filepath.Join(t.TempDir(), "mismatched.json")
	if err := os.WriteFile(mismatched, mismatchedSubjectReviewerPayload(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	captureArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", lens, "--order", "0", "--input", mismatched,
	}
	rejected := RunReviewCaptureResult(captureArgs, io.Discard)
	if rejected == nil {
		t.Fatal("a reviewer result echoing a different artifact subject was admitted; the fail-closed rejection was weakened")
	}
	// The classified admission decision must not change: this stays a
	// binding_mismatch, message-only.
	if !strings.Contains(rejected.Error(), "reviewer artifact admission binding_mismatch") ||
		!strings.Contains(rejected.Error(), "echoed a different artifact subject") {
		t.Fatalf("binding mismatch lost its classified admission decision: %v", rejected)
	}
	// Discoverability contract: the rejection must name the same continuation
	// its omitted-subject sibling names, and the subject hash to echo.
	for _, continuation := range []string{"did not consume the lens slot", "re-run", "capture-result", "subject_hash"} {
		if !strings.Contains(rejected.Error(), continuation) {
			t.Fatalf("binding_mismatch rejection does not name the continuation %q: %v", continuation, rejected)
		}
	}
	if !strings.Contains(rejected.Error(), subject.SubjectHash) {
		t.Fatalf("binding_mismatch rejection does not hand back the expected subject hash %q: %v", subject.SubjectHash, rejected)
	}

	// The claim the message makes must be true: the rejection consumed no slot
	// and moved no authority.
	if _, statErr := os.Stat(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)); !os.IsNotExist(statErr) {
		t.Fatalf("rejected binding_mismatch admission consumed the immutable result slot: %v", statErr)
	}
	assertArtifactRevision(t, store, record.Revision)

	// Run EXACTLY the continuation the message names: capture again on the same
	// lineage and lens with a result that echoes the binding's subject hash.
	corrected := filepath.Join(t.TempDir(), "corrected.json")
	if err := os.WriteFile(corrected, admittedReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	correctedArgs := append(append([]string{}, captureArgs[:len(captureArgs)-1]...), corrected)
	var captured bytes.Buffer
	if err := RunReviewCaptureResult(correctedArgs, &captured); err != nil {
		t.Fatalf("recapture after binding_mismatch was refused (named continuation dead-ends): %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("recapture admission = %q, want completed", artifact.AdmissionDecision)
	}
	// The block is only cleared when the lineage completes.
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", strings.TrimSpace(captured.String()),
	}, io.Discard); err != nil {
		t.Fatalf("finalize after binding_mismatch recapture failed: %v", err)
	}
}
