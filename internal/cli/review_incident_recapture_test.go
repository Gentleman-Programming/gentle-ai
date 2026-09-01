package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
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

// TestReviewCaptureResultRecapturesSameLensAfterRejectedAdmission proves that
// rejected reviewer output creates no alternate authority. The selected slot stays
// empty, STATUS reoffers its exact binding, and a corrected result can fill it.
func TestReviewCaptureResultRecapturesSameLensAfterRejectedAdmission(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, true)
	statusArgs := []string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}
	readStatus := func() ReviewTargetStatusResult {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview(statusArgs, &output); err != nil {
			t.Fatal(err)
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		return status
	}
	initialStatus := readStatus()
	if initialStatus.NextTransition == nil || initialStatus.NextTransition.Collect == nil ||
		len(initialStatus.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("initial reviewer transition = %#v", initialStatus.NextTransition)
	}
	offered := initialStatus.NextTransition.Collect.Inputs[0]

	err := failEnvelopelessCapture(t, repo, started, record)
	// Discoverability contract: the admission-incomplete rejection must name
	// the continuation — re-run the lens and capture again with the required
	// subject_hash/inspection envelope — so the flow is not a dead end.
	for _, continuation := range []string{"subject_hash", "inspection", "re-run"} {
		if !strings.Contains(err.Error(), continuation) {
			t.Fatalf("admission-incomplete rejection does not name the continuation %q: %v", continuation, err)
		}
	}

	// A rejected capture leaves no lens, role, incident, or disposition authority.
	afterRejected, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterRejected.Revision != record.Revision || !reflect.DeepEqual(afterRejected.State, record.State) {
		t.Fatalf("rejected capture mutated reviewing authority: before=%#v after=%#v", record, afterRejected)
	}
	if len(afterRejected.State.AdmittedRoleResults) != 0 {
		t.Fatalf("rejected capture occupied a reviewer slot: %#v", afterRejected.State)
	}
	// The only compact authority tree belongs to this isolated test repository.
	// A rejected provider payload is neither a durable artifact nor an event: its
	// bytes and raw SHA-256 must be absent from every compact authority file, and
	// the retired incident sidecar must not exist.
	authorityRoot := filepath.Dir(filepath.Dir(store.Dir))
	digest := sha256.Sum256([]byte(envelopelessReviewerPayload))
	assertCompactAuthorityOmitsRejectedCapture(t, authorityRoot,
		[]byte(envelopelessReviewerPayload),
		[]byte(hex.EncodeToString(digest[:])),
		[]byte("sha256:"+hex.EncodeToString(digest[:])),
	)
	incidentPath := filepath.Join(authorityRoot, "incidents", started.LineageID)
	if _, statErr := os.Lstat(incidentPath); !os.IsNotExist(statErr) {
		t.Fatalf("rejected capture created retired compact incident ownership at %s: %v", incidentPath, statErr)
	}

	reofferedStatus := readStatus()
	if reofferedStatus.NextTransition == nil || reofferedStatus.NextTransition.Collect == nil ||
		len(reofferedStatus.NextTransition.Collect.Inputs) == 0 ||
		!reflect.DeepEqual(reofferedStatus.NextTransition.Collect.Inputs[0], offered) {
		t.Fatalf("fresh STATUS did not reoffer the rejected reviewer slot: %#v", reofferedStatus.NextTransition)
	}

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
		t.Fatalf("corrected capture after rejected admission was refused: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("corrected capture admission = %q", artifact.AdmissionDecision)
	}

	// Single assignment for admitted results still holds: exact replay converges,
	// and a different well-formed result for the same slot stays refused.
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

	if artifact.Reference == "" || artifact.Path != "" {
		t.Fatalf("same-lineage recapture did not return the record-backed opaque artifact reference: %#v", artifact)
	}
}

func assertCompactAuthorityOmitsRejectedCapture(t *testing.T, authorityRoot string, forbidden ...[]byte) {
	t.Helper()
	if err := filepath.Walk(authorityRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, value := range forbidden {
			if bytes.Contains(payload, value) {
				t.Fatalf("compact authority retained rejected reviewer capture material in %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("scan compact authority for rejected reviewer capture material: %v", err)
	}
}

func TestRetiredReviewResultVerbsAreAbsent(t *testing.T) {
	for _, verb := range []string{"preserve-result", "dispose-result"} {
		t.Run(verb, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview([]string{verb, "--help"}, &output)
			if err == nil || !strings.Contains(err.Error(), "unknown review command") {
				t.Fatalf("retired review verb %q remains callable: %v\n%s", verb, err, output.String())
			}
			if strings.Contains(output.String(), verb) {
				t.Fatalf("retired review verb %q remains in command output: %s", verb, output.String())
			}
		})
	}

	var usage bytes.Buffer
	if err := RunReview(nil, &usage); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"preserve-result", "dispose-result"} {
		if strings.Contains(usage.String(), verb) {
			t.Fatalf("retired review verb %q remains in usage: %s", verb, usage.String())
		}
	}
}

func TestReviewCaptureResultRecapturesSameLensAfterPreInspectionAccessFailure(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, true)
	statusArgs := []string{
		"status", "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--cwd", repo, "--lineage", started.LineageID,
	}
	readStatus := func() ReviewTargetStatusResult {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview(statusArgs, &output); err != nil {
			t.Fatal(err)
		}
		var status ReviewTargetStatusResult
		decodeStrictReviewJSON(t, output.Bytes(), &status)
		return status
	}

	firstStatus := readStatus()
	if firstStatus.NextTransition == nil || firstStatus.NextTransition.Kind != reviewNextTransitionCollect ||
		firstStatus.NextTransition.ReasonCode != "reviewer_results_required" || firstStatus.NextTransition.Collect == nil ||
		len(firstStatus.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("initial reviewer transition = %#v", firstStatus.NextTransition)
	}
	offered := firstStatus.NextTransition.Collect.Inputs[0]
	arguments, err := reviewTransitionArgumentMap(offered.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	lens := record.State.SelectedLenses[0]
	for name, want := range map[string]string{
		"lineage": started.LineageID, "target": record.State.InitialSnapshot.Identity,
		"expected-revision": record.State.CapturePhaseRevision, "lens": lens, "order": "0",
	} {
		if arguments[name] != want {
			t.Fatalf("initial reviewer argument %q = %q, want %q", name, arguments[name], want)
		}
	}
	if offered.ArtifactSubject == nil || offered.ArtifactSubject.LineageID != started.LineageID ||
		offered.ArtifactSubject.AuthorityRevision != record.State.CapturePhaseRevision ||
		offered.ArtifactSubject.TargetIdentity != record.State.InitialSnapshot.Identity ||
		offered.ArtifactSubject.Lens != lens || offered.ArtifactSubject.SelectedOrder != 0 {
		t.Fatalf("initial reviewer subject = %#v", offered.ArtifactSubject)
	}

	incomplete := admittedReviewerResultForTest(t, repo, record, lens, 0)
	incomplete.Inspection.Status = reviewtransaction.ArtifactInspectionStatus("incomplete")
	incomplete.Inspection.Paths = []string{}
	incomplete.Evidence = []string{"provider access failed before candidate inspection"}
	if incomplete.SubjectHash != offered.ArtifactSubject.SubjectHash {
		t.Fatalf("incomplete result subject = %q, want %q", incomplete.SubjectHash, offered.ArtifactSubject.SubjectHash)
	}
	payload, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(t.TempDir(), "incomplete.json")
	if err := os.WriteFile(failed, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	captureArgs := []string{
		"--cwd", repo, "--lineage", started.LineageID, "--expected-revision", record.State.CapturePhaseRevision,
		"--target", record.State.InitialSnapshot.Identity, "--lens", lens, "--order", "0", "--input", failed,
	}
	if err := RunReviewCaptureResult(captureArgs, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "reviewer did not report completed candidate inspection") {
		t.Fatalf("incomplete inspection capture error = %v", err)
	}
	afterFailure, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Revision != record.Revision || len(afterFailure.State.AdmittedRoleResults) != 0 {
		t.Fatalf("incomplete inspection mutated authority = %#v", afterFailure)
	}
	if len(afterFailure.State.AdmittedRoleResults) != 0 {
		t.Fatalf("incomplete inspection consumed canonical role results: %#v", afterFailure.State.AdmittedRoleResults)
	}

	reofferedStatus := readStatus()
	if reofferedStatus.NextTransition == nil || reofferedStatus.NextTransition.Collect == nil ||
		len(reofferedStatus.NextTransition.Collect.Inputs) == 0 ||
		!reflect.DeepEqual(reofferedStatus.NextTransition.Collect.Inputs[0], offered) {
		t.Fatalf("fresh STATUS did not reoffer the exact reviewer slot: %#v", reofferedStatus.NextTransition)
	}

	corrected := filepath.Join(t.TempDir(), "corrected.json")
	if err := os.WriteFile(corrected, admittedReviewerPayloadForTest(t, repo, record, lens, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	captureArgs[len(captureArgs)-1] = corrected
	var captured bytes.Buffer
	if err := RunReviewCaptureResult(captureArgs, &captured); err != nil {
		t.Fatalf("corrected capture after STATUS reoffer: %v", err)
	}
	var artifact reviewResultArtifact
	decodeStrictReviewJSON(t, captured.Bytes(), &artifact)
	if artifact.AdmissionDecision != reviewtransaction.ArtifactAdmissionCompleted {
		t.Fatalf("corrected capture admission = %q", artifact.AdmissionDecision)
	}
	if artifact.Reference == "" || artifact.Path != "" {
		t.Fatalf("STATUS-mediated recapture did not return the record-backed opaque artifact reference: %#v", artifact)
	}
}
