package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestOccupiedSlotUsesStatusContinuationForOpaqueCapture(t *testing.T) {
	reviewEnabledHome(t)
	const lineage = "slot-conflict-classification"
	binding, _, repo := startedOpaqueCaptureBinding(t, lineage)

	first := admissibleOpaqueReviewerResult(t, binding, "first reviewer evidence")
	var captured, replayed bytes.Buffer
	if err := RunReviewCaptureResult(append(append([]string{}, binding...), "--input", first), &captured); err != nil {
		t.Fatalf("first capture failed: %v", err)
	}
	lens := ""
	for index := range binding[:len(binding)-1] {
		if binding[index] == "--lens" {
			lens = binding[index+1]
			break
		}
	}
	if lens == "" {
		t.Fatal("opaque capture binding omits --lens")
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before := admittedReviewerRecordEntry(t, store, lens, 0)
	if err := RunReviewCaptureResult(append(append([]string{}, binding...), "--input", first), &replayed); err != nil || captured.String() != replayed.String() {
		t.Fatalf("exact replay = %q, %v; want %q, nil", replayed.String(), err, captured.String())
	}
	second := admissibleOpaqueReviewerResult(t, binding, "second reviewer evidence")
	err = RunReviewCaptureResult(append(append([]string{}, binding...), "--input", second), io.Discard)
	if err == nil {
		t.Fatal("a second reviewer result with different bytes was accepted into an occupied slot")
	}
	if !errors.Is(err, reviewtransaction.ErrCapturedReviewerResultSlotConflict) {
		t.Fatalf("occupied slot error = %v, want ErrCapturedReviewerResultSlotConflict", err)
	}
	assertOccupiedReviewerSlotStatusContinuation(t, err, repo, lineage)
	assertReviewerSlotUnchanged(t, store, lens, 0, before)
}

func TestOccupiedSlotUsesStatusContinuationForDirectCapture(t *testing.T) {
	reviewEnabledHome(t)
	repo, started, store, record := newArtifactReview(t, true)
	input := filepath.Join(t.TempDir(), "reviewer-result.json")
	args := []string{"--cwd", repo, "--lineage", started.LineageID, "--target", record.State.InitialSnapshot.Identity, "--lens", record.State.SelectedLenses[0], "--order", "0", "--input", input}
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0), 0o600); err != nil {
		t.Fatal(err)
	}
	var captured, replayed bytes.Buffer
	if err := RunReviewCaptureResult(args, &captured); err != nil {
		t.Fatalf("first capture failed: %v", err)
	}
	before := admittedReviewerRecordEntry(t, store, record.State.SelectedLenses[0], 0)
	if err := RunReviewCaptureResult(args, &replayed); err != nil || captured.String() != replayed.String() {
		t.Fatalf("exact replay = %q, %v; want %q, nil", replayed.String(), err, captured.String())
	}
	if err := os.WriteFile(input, admittedReviewerPayloadForTest(t, repo, record, record.State.SelectedLenses[0], 0, "different reviewer evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := RunReviewCaptureResult(args, io.Discard)
	if err == nil {
		t.Fatal("a second reviewer result with different bytes was accepted into an occupied slot")
	}
	if !errors.Is(err, reviewtransaction.ErrCapturedReviewerResultSlotConflict) {
		t.Fatalf("occupied slot error = %v, want ErrCapturedReviewerResultSlotConflict", err)
	}
	assertOccupiedReviewerSlotStatusContinuation(t, err, repo, started.LineageID)
	assertReviewerSlotUnchanged(t, store, record.State.SelectedLenses[0], 0, before)
}

func assertOccupiedReviewerSlotStatusContinuation(t *testing.T, err error, repo, lineage string) {
	t.Helper()
	message := err.Error()
	if !strings.Contains(message, "reviewer_result_slot_occupied") {
		t.Fatalf("occupied slot lost its generic code: %s", message)
	}
	for _, forbidden := range []string{"repository_context_capture_failed", "retry", "review capture-result", "review dispose-result", "review preserve-result"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("occupied slot advertises %q instead of STATUS continuation: %s", forbidden, message)
		}
	}
	if !strings.Contains(message, reviewNextTransitionRefreshCommandV21) || !strings.Contains(message, "authoritative continuation") {
		t.Fatalf("occupied slot does not advertise the STATUS continuation: %s", message)
	}
	var statusOutput bytes.Buffer
	if statusErr := RunReviewStatus([]string{"--cwd", repo, "--lineage", lineage, "--contract", ReviewIntegrationContractV2, "--next-transition"}, &statusOutput); statusErr != nil {
		t.Fatalf("STATUS after occupied slot: %v", statusErr)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect || status.NextTransition.ReasonCode != "reviewer_results_required" {
		t.Fatalf("STATUS continuation = %#v", status.NextTransition)
	}
}

func admittedReviewerRecordEntry(t *testing.T, store reviewtransaction.CompactStore, lens string, order int) reviewtransaction.CompactAdmittedRoleResult {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range record.State.AdmittedRoleResults {
		if entry.Role == reviewtransaction.CompactRoleLens && entry.Lens == lens && entry.SelectedOrder == order {
			return entry
		}
	}
	t.Fatalf("canonical admitted lens entry %q at order %d is absent", lens, order)
	return reviewtransaction.CompactAdmittedRoleResult{}
}

func assertReviewerSlotUnchanged(t *testing.T, store reviewtransaction.CompactStore, lens string, order int, before reviewtransaction.CompactAdmittedRoleResult) {
	t.Helper()
	after := admittedReviewerRecordEntry(t, store, lens, order)
	if after.ArtifactDigest != before.ArtifactDigest || after.ResultHash != before.ResultHash || !bytes.Equal(after.Value, before.Value) {
		t.Fatalf("conflicting capture overwrote the canonical reviewer result: before=%#v after=%#v", before, after)
	}
}

// TestGenuineRepositoryContextCaptureFailureKeepsItsCode proves an opaque
// capture persistence failure remains distinguishable from an occupied record
// entry and preserves the retry action.
func TestGenuineRepositoryContextCaptureFailureKeepsItsCode(t *testing.T) {
	reviewEnabledHome(t)
	binding, _, repo := startedOpaqueCaptureBinding(t, "slot-conflict-control")
	result := admissibleOpaqueReviewerResult(t, binding, "reviewer evidence")
	blockCanonicalAuthorityWrite(t, repo, "slot-conflict-control")

	err := RunReviewCaptureResult(append(append([]string{}, binding...), "--input", result), io.Discard)
	if err == nil {
		t.Fatal("capture succeeded despite an unwritable compact authority")
	}
	if !strings.Contains(err.Error(), "repository_context_capture_failed") {
		t.Fatalf("a real capture failure lost its code: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "retry capture-result with the same exact binding or refresh status") {
		t.Fatalf("a real capture failure lost its retry action: %s", err.Error())
	}
}

func blockCanonicalAuthorityWrite(t *testing.T, repo, lineage string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the compact authority permission failure fixture requires POSIX permissions")
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(store.Dir, info.Mode().Perm()); err != nil {
			t.Errorf("restore compact authority permissions: %v", err)
		}
	})
}
