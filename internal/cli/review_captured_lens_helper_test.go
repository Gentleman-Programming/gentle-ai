package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// finalizeReviewCLIArgs rewrites a legacy `--result <file>` finalize invocation
// into the native admission route the product now requires, and runs it.
//
// `--result` was retired because it bound nothing: it accepted any JSON with
// findings and evidence arrays, so a hand-written file could mint an approval.
// The suite had proved almost the entire review lifecycle through that route,
// which is why the hole survived. Rather than restate every test's fixture, this
// helper keeps each test's OWN result payload -- the findings and evidence it
// deliberately set up -- and supplies only the parts a reviewer cannot invent:
// the provider-issued subject hash and the inspected path manifest, both read
// back from the product's own capture binding.
//
// The result is that these tests now drive `review capture-result` and
// `review finalize --captured-results`, the exact production path, instead of a
// route that no longer exists.
func finalizeReviewCLIArgs(t *testing.T, repo string, args []string, stdout io.Writer) error {
	t.Helper()
	rest, resultPaths, lineage := splitReviewCLIResultArgs(args)
	if len(resultPaths) == 0 {
		return RunReviewFacadeFinalize(args, stdout)
	}
	// A payload the provider refuses to admit is returned, not fataled: tests
	// that deliberately supply a malformed reviewer result used to observe that
	// refusal as finalize's error, and admission rejecting it earlier is the
	// same observation. Fataling here would silently convert those assertions
	// into unconditional test failures.
	if err := captureReviewCLIResultFiles(t, repo, lineage, resultPaths); err != nil {
		return err
	}
	return RunReviewFacadeFinalize(append(rest, "--captured-results=true"), stdout)
}

// splitReviewCLIResultArgs separates the retired --result arguments from the
// rest of the invocation and reports the explicit --lineage when one was given.
func splitReviewCLIResultArgs(args []string) (rest, resultPaths []string, lineage string) {
	rest = make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "--result" && index+1 < len(args):
			resultPaths = append(resultPaths, args[index+1])
			index++
		case strings.HasPrefix(args[index], "--result="):
			resultPaths = append(resultPaths, strings.TrimPrefix(args[index], "--result="))
		case args[index] == "--lineage" && index+1 < len(args):
			lineage = args[index+1]
			rest = append(rest, args[index], args[index+1])
			index++
		default:
			if strings.HasPrefix(args[index], "--lineage=") {
				lineage = strings.TrimPrefix(args[index], "--lineage=")
			}
			rest = append(rest, args[index])
		}
	}
	return rest, resultPaths, lineage
}

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
	for order, lens := range state.SelectedLenses {
		if order >= len(resultPaths) {
			break
		}
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
