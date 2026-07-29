package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// envelopelessReviewerPayload is the community-reported failure shape: the
// reviewer returned findings/evidence but omitted the provider-owned envelope
// (top-level subject_hash and inspection). Admission must reject it fail-closed.
const envelopelessReviewerPayload = `{"findings":[],"evidence":["inspection: reviewed every frozen candidate path"]}`

// failEnvelopelessCapture drives the real capture flow with the malformed
// payload and returns the exact rejection error after asserting the fail-closed
// admission decision that must never be weakened.
func failEnvelopelessCapture(t *testing.T, repo string, started ReviewFacadeStartResult, record reviewtransaction.CompactRecord) error {
	t.Helper()
	input := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(input, []byte(envelopelessReviewerPayload), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RunReviewCaptureResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, io.Discard)
	if err == nil {
		t.Fatal("envelope-less reviewer payload was admitted; the fail-closed rejection was weakened")
	}
	if !strings.Contains(err.Error(), "reviewer artifact admission incomplete") ||
		!strings.Contains(err.Error(), "omitted the provider-owned artifact subject") {
		t.Fatalf("envelope-less rejection lost its classified admission decision: %v", err)
	}
	return err
}

// preserveEnvelopelessIncident preserves the malformed raw payload as a native
// incident artifact for the same live lens slot and returns the artifact.
func preserveEnvelopelessIncident(t *testing.T, repo string, started ReviewFacadeStartResult, record reviewtransaction.CompactRecord) reviewIncidentArtifact {
	t.Helper()
	input := filepath.Join(t.TempDir(), "raw.json")
	if err := os.WriteFile(input, []byte(envelopelessReviewerPayload), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewPreserveResult([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input,
	}, &output); err != nil {
		t.Fatalf("preserve incident after rejected admission: %v", err)
	}
	var artifact reviewIncidentArtifact
	decodeStrictReviewJSON(t, output.Bytes(), &artifact)
	preserved, err := os.ReadFile(artifact.Path)
	if err != nil || string(preserved) != envelopelessReviewerPayload {
		t.Fatalf("incident artifact bytes mismatch: %v %q", err, preserved)
	}
	return artifact
}

// TestReviewCaptureResultRecapturesSameLensAfterRejectedAdmission reproduces
// the community-reported dead end (PR #1801): a reviewer payload without the
// provider-owned envelope is rejected fail-closed and preserved as an incident,
// and the same lineage/lens must then accept a corrected well-formed capture,
// because a slot whose admission was REJECTED was never consumed.
func TestReviewCaptureResultRecapturesSameLensAfterRejectedAdmission(t *testing.T) {
	repo, started, store, record := newArtifactReview(t, false)
	err := failEnvelopelessCapture(t, repo, started, record)
	// Discoverability contract: the admission-incomplete rejection must name
	// the continuation — re-run the lens and capture again with the required
	// subject_hash/inspection envelope — so the flow is not a dead end.
	for _, continuation := range []string{"subject_hash", "inspection", "re-run"} {
		if !strings.Contains(err.Error(), continuation) {
			t.Fatalf("admission-incomplete rejection does not name the continuation %q: %v", continuation, err)
		}
	}
	// The rejection never consumed the immutable slot and never mutated authority.
	if _, statErr := os.Stat(filepath.Join(store.Dir, reviewtransaction.CompactReviewerResultsDir)); !os.IsNotExist(statErr) {
		t.Fatalf("rejected admission consumed the immutable result slot: %v", statErr)
	}
	assertArtifactRevision(t, store, record.Revision)

	incident := preserveEnvelopelessIncident(t, repo, started, record)

	// The current dead-end probe: a corrected WELL-FORMED capture on the same
	// lineage/lens must admit normally after the rejected admission.
	corrected := filepath.Join(t.TempDir(), "corrected.json")
	if err := os.WriteFile(corrected, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	captureArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity,
		"--lens", record.State.SelectedLenses[0], "--order", "0", "--input", corrected,
	}
	var captured bytes.Buffer
	if err := RunReviewCaptureResult(captureArgs, &captured); err != nil {
		t.Fatalf("corrected capture after rejected admission was refused (same-lineage recapture dead end): %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("corrected capture admission = %q", artifact.AdmissionDecision)
	}

	// The incident stays as append-only audit history; a successful recapture
	// never deletes or replaces it.
	if preserved, err := os.ReadFile(incident.Path); err != nil || string(preserved) != envelopelessReviewerPayload {
		t.Fatalf("successful recapture disturbed the incident audit artifact: %v %q", err, preserved)
	}

	// Single assignment for ADMITTED results still holds: exact replay
	// converges, and a DIFFERENT well-formed result for the same slot stays refused.
	var replay bytes.Buffer
	if err := RunReviewCaptureResult(captureArgs, &replay); err != nil || captured.String() != replay.String() {
		t.Fatalf("exact recapture replay diverged: %v", err)
	}
	different := filepath.Join(t.TempDir(), "different.json")
	if err := os.WriteFile(different, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0, "inspection: different evidence over every frozen candidate path"), 0o600); err != nil {
		t.Fatal(err)
	}
	differentArgs := append(append([]string{}, captureArgs[:len(captureArgs)-1]...), different)
	if err := RunReviewCaptureResult(differentArgs, io.Discard); err == nil {
		t.Fatal("second different capture after a successful admission was accepted; single assignment was weakened")
	}

	// The recaptured lineage completes normally.
	if err := RunReviewFacadeFinalize([]string{"--cwd", repo, "--lineage", started.LineageID, "--result-artifact", strings.TrimSpace(captured.String())}, io.Discard); err != nil {
		t.Fatalf("finalize after same-lineage recapture failed: %v", err)
	}
}

// TestReviewAbandonAfterRejectedAdmissionIncidentArtifact answers the open
// pristineness question from the community report: a quarantined incident
// artifact records a REJECTED admission — never a capture — so it must not
// disqualify the reviewing lineage from `review abandon`, which accepts "a
// reviewing authority that never captured lens results". The incident lives
// beside the authority root (incidents/<lineage>), not inside the store entry,
// and it survives the quarantine as audit history.
func TestReviewAbandonAfterRejectedAdmissionIncidentArtifact(t *testing.T) {
	repo, started, _, record := newArtifactReview(t, false)
	failEnvelopelessCapture(t, repo, started, record)
	incident := preserveEnvelopelessIncident(t, repo, started, record)

	binding := "gentle-ai.review-abandon-authorization/v1\nlineage=" + started.LineageID +
		"\nrevision=" + record.Revision +
		"\nsnapshot_identity=" + record.State.InitialSnapshot.Identity +
		"\nactor=maintainer@example.com\nreason=abandon after rejected reviewer admission"
	var output bytes.Buffer
	err := RunReviewAbandon([]string{
		"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", record.Revision,
		"--reason", "abandon after rejected reviewer admission", "--actor", "maintainer@example.com",
		"--maintainer-authorization", binding,
	}, &output)
	if err != nil {
		t.Fatalf("abandon refused a reviewing lineage whose only artifact is a rejected-admission incident: %v", err)
	}
	var result ReviewAbandonResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Record.Status != reviewtransaction.CompactReclaimCommitted ||
		result.Record.PristineAbandonment == nil ||
		result.Record.PristineAbandonment.State != reviewtransaction.StateReviewing {
		t.Fatalf("abandon after incident result = %#v", result)
	}
	if _, statErr := os.Stat(filepath.Join(reviewCLIAuthorityRoot(t, repo), "v2", started.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("abandoned lineage entry still present: %v", statErr)
	}
	// The incident artifact remains as append-only audit history after abandon.
	if preserved, readErr := os.ReadFile(incident.Path); readErr != nil || string(preserved) != envelopelessReviewerPayload {
		t.Fatalf("abandon disturbed the incident audit artifact: %v %q", readErr, preserved)
	}
}
