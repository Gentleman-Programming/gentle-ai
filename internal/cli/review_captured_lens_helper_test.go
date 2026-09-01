package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// captureReviewCLIResultFiles admits one reviewer result per selected lens,
// reusing each supplied file's findings and evidence verbatim.
func captureReviewCLIResultFiles(t *testing.T, repo, lineage string, resultPaths []string) error {
	t.Helper()
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatalf("resolve review repository root: %v", err)
	}
	_, record, err := discoverCompactFacadeReview(context.Background(), root, lineage, false)
	if err != nil {
		return err
	}
	state := record.State
	if len(resultPaths) != len(state.SelectedLenses) {
		return fmt.Errorf("capture reviewer results requires %d --result files for the selected lenses, got %d", len(state.SelectedLenses), len(resultPaths))
	}
	for order, lens := range state.SelectedLenses {
		if err := captureReviewCLIResultFile(t, root, state, order, lens, resultPaths[order]); err != nil {
			return err
		}
	}
	return nil
}

func captureReviewCLIResultFile(t *testing.T, root string, state reviewtransaction.CompactState, order int, lens, resultPath string) error {
	t.Helper()
	payload, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read reviewer result %q: %v", resultPath, err)
	}
	// Decoded strictly, so a payload carrying an unknown field is refused here
	// exactly as capture-result refuses it, instead of being silently dropped by
	// a lenient re-marshal and admitted.
	var result facadeReviewerResult
	if err := decodeFacadeJSONBytes(payload, &result); err != nil {
		return err
	}
	binding := []string{
		"--cwd", root, "--lineage", state.LineageID, "--target", state.InitialSnapshot.Identity,
		"--lens", lens, "--order", strconv.Itoa(order),
	}
	var preflightOutput bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--preflight"), &preflightOutput); err != nil {
		return err
	}
	var preflight reviewCapturePreflightResult
	if err := json.Unmarshal(preflightOutput.Bytes(), &preflight); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, len(preflight.ChangedPathManifest))
	for index, entry := range preflight.ChangedPathManifest {
		paths[index] = entry.Path
	}
	// Only the binding is supplied. Findings and evidence stay exactly as the
	// caller wrote them, so a payload that omits them is refused by admission
	// rather than quietly completed into an admissible one.
	result.SubjectHash = preflight.ArtifactSubject.SubjectHash
	result.Inspection = reviewtransaction.ArtifactInspection{
		Status: reviewtransaction.ArtifactInspectionCompleted, Paths: paths,
	}
	boundPath := filepath.Join(t.TempDir(), "bound-"+strconv.Itoa(order)+".json")
	writeReviewCLIJSON(t, boundPath, result)
	return RunReviewCaptureResult(append(binding, "--input", boundPath), io.Discard)
}
