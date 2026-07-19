package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func injectCLIRetiredZeroEditEscalation(t *testing.T, statePath string) {
	t.Helper()
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	state, ok := record["state"].(map[string]any)
	if !ok {
		t.Fatal("compact record has no state object")
	}
	state["zero_edit_escalation"] = true
	statePayload, err := json.Marshal(record["state"])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append([]byte(reviewtransaction.CompactStateSchema+"\x00"), statePayload...))
	record["revision"] = "sha256:" + hex.EncodeToString(sum[:])
	updated, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(updated, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func injectCLIRetiredRecoveryReviewStart(t *testing.T, statePath string) {
	t.Helper()
	payload, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	state, ok := record["state"].(map[string]any)
	if !ok {
		t.Fatal("compact record has no state object")
	}
	recovery, ok := state["recovery"].(map[string]any)
	if !ok {
		t.Fatal("compact state has no recovery object")
	}
	recovery["review_start"] = map[string]any{"operation": "review/start", "resumed": true}
	statePayload, err := json.Marshal(record["state"])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append([]byte(reviewtransaction.CompactStateSchema+"\x00"), statePayload...))
	record["revision"] = "sha256:" + hex.EncodeToString(sum[:])
	updated, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, append(updated, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReviewStatusReportsHistoricalRecoveredStartLineage(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("recovered candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	predecessor := startFacadeReview(t, repo)
	predecessorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunReviewInvalidate([]string{"--cwd", repo, "--lineage", predecessor.LineageID,
		"--expected-revision", record.Revision, "--reason", "recovered-start fixture"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	invalidated, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	successorLineage := "recovered-start-successor"
	if err := RunReviewRecover([]string{"--cwd", repo, "--predecessor-lineage", predecessor.LineageID,
		"--expected-predecessor-revision", invalidated.Revision, "--successor-lineage", successorLineage,
		"--disposition", "invalidated", "--reason", "resume recovered start", "--actor", "maintainer"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successorLineage)
	if err != nil {
		t.Fatal(err)
	}
	injectCLIRetiredRecoveryReviewStart(t, successorStore.StatePath())
	before, err := os.ReadFile(successorStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := RunReviewStatus([]string{"--cwd", repo}, &output); err != nil {
		t.Fatalf("review status with historical recovered-start record: %v", err)
	}
	if strings.Contains(output.String(), "unknown field") {
		t.Fatalf("review status surfaced a strict-decode failure: %s", output.String())
	}
	var report reviewtransaction.AuthorityStatusReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative {
		t.Fatalf("historical recovered-start report = %#v", report)
	}
	found := false
	for _, entry := range report.Entries {
		if entry.LineageID == successorLineage {
			found = true
			if entry.Status != reviewtransaction.AuthorityStatusRecovered {
				t.Fatalf("historical recovered-start entry = %#v", entry)
			}
		}
	}
	if !found {
		t.Fatalf("historical recovered-start lineage missing from report: %#v", report)
	}
	if after, err := os.ReadFile(successorStore.StatePath()); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("review status rewrote historical authority bytes: %v", err)
	}
}

func TestReviewBundleExportRefusesHistoricalZeroEditEscalationClearly(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("historical candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	historical := startFacadeReview(t, repo)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, historical.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	injectCLIRetiredZeroEditEscalation(t, store.StatePath())
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "bundle.json")
	exportErr := RunReviewBundleExport([]string{"--cwd", repo, "--lineage", historical.LineageID, "--out", out}, io.Discard)
	if exportErr == nil || !errors.Is(exportErr, reviewtransaction.ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical bundle export error = %v, want %v", exportErr, reviewtransaction.ErrHistoricalCompatReadOnly)
	}
	if strings.Contains(exportErr.Error(), "invalid compact review transport") {
		t.Fatalf("historical bundle export masked the real cause: %v", exportErr)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("refused historical export left a bundle: %v", err)
	}
	if after, err := os.ReadFile(store.StatePath()); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("historical authority bytes changed: %v", err)
	}
}

func TestReviewFacadeExplicitFinalizeCompletesWithHistoricalZeroEditEscalationRecord(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("historical candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	historical := startFacadeReview(t, repo)
	evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(evidencePath, []byte("go test ./...: pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"--cwd", repo, "--lineage", historical.LineageID}, facadeReviewerResultArgs(t, historical)...)
	if err := RunReviewFacadeFinalize(append(args, "--evidence", evidencePath), io.Discard); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "historical candidate")

	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("current candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := startFacadeReview(t, repo)
	if current.LineageID == historical.LineageID {
		t.Fatalf("current review reused historical lineage %q", current.LineageID)
	}
	historicalStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, historical.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	injectCLIRetiredZeroEditEscalation(t, historicalStore.StatePath())
	before, err := os.ReadFile(historicalStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}

	currentEvidence := filepath.Join(t.TempDir(), "current-evidence.txt")
	if err := os.WriteFile(currentEvidence, []byte("go test ./...: pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizeArgs := append([]string{"--cwd", repo, "--lineage", current.LineageID}, facadeReviewerResultArgs(t, current)...)
	var output bytes.Buffer
	if err := RunReviewFacadeFinalize(append(finalizeArgs, "--evidence", currentEvidence), &output); err != nil {
		t.Fatalf("explicit-lineage finalize with unrelated historical record: %v", err)
	}
	finalized := decodeFacadeFinalize(t, output.Bytes())
	if finalized.State != reviewtransaction.StateApproved || finalized.LineageID != current.LineageID {
		t.Fatalf("finalize result = %#v", finalized)
	}
	report, err := reviewtransaction.InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete {
		t.Fatalf("historical record left status incomplete: %#v", report)
	}
	for _, entry := range report.Entries {
		if entry.LineageID == historical.LineageID && entry.Status == reviewtransaction.AuthorityStatusInvalid {
			t.Fatalf("historical entry reported invalid: %#v", entry)
		}
	}
	if after, err := os.ReadFile(historicalStore.StatePath()); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("finalize rewrote historical authority bytes: %v", err)
	}
}
