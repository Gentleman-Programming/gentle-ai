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
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// Issues #3942, #2791 and #1867: an in-process reviewer runtime returns free
// text, so one out-of-schema or truncated result used to be a dead end. The
// capture now grants exactly one corrective re-invocation that carries the
// exact admission error, and every rejected payload is preserved outside the
// authority store so a report can quote the bytes.

// reviewerPayloadWithNestedLens injects the #3942 shape: a `lens` field
// inside `inspection`, which the strict decoder rejects as unknown.
func reviewerPayloadWithNestedLens(t *testing.T, valid []byte) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(valid, &object); err != nil {
		t.Fatal(err)
	}
	var inspection map[string]any
	if err := json.Unmarshal(object["inspection"], &inspection); err != nil {
		t.Fatal(err)
	}
	inspection["lens"] = "review-reliability"
	nested, err := json.Marshal(inspection)
	if err != nil {
		t.Fatal(err)
	}
	object["inspection"] = nested
	payload, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// recordingProviderAdapter installs a fake in-process reviewer that answers
// the scripted payloads in order and records every prompt it received.
func recordingProviderAdapter(t *testing.T, answers ...func() ([]byte, error)) *[][]byte {
	t.Helper()
	prompts := &[][]byte{}
	previous := reviewProviderAdapterFor
	reviewProviderAdapterFor = func(_ reviewerprovider.Contract, agent model.AgentID) (reviewerprovider.Adapter, error) {
		if agent != model.AgentClaudeCode {
			return nil, errors.New("unexpected runtime")
		}
		return providerTestAdapterFunc(func(_ context.Context, invocation reviewerprovider.Invocation) ([]byte, error) {
			*prompts = append(*prompts, invocation.Prompt())
			if len(*prompts) > len(answers) {
				t.Fatalf("provider reviewer was invoked %d times, want at most %d", len(*prompts), len(answers))
			}
			return answers[len(*prompts)-1]()
		}), nil
	}
	t.Cleanup(func() { reviewProviderAdapterFor = previous })
	return prompts
}

func rejectedResultsDir(t *testing.T, repo, lineage string) string {
	t.Helper()
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(lease.Identity().GitCommonDir, "gentle-ai", reviewRejectedResultDirName, lineage)
}

func readRejectedResults(t *testing.T, dir string) map[string]reviewRejectedResultEnvelope {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rejected results %s: %v", dir, err)
	}
	envelopes := map[string]reviewRejectedResultEnvelope{}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("rejected result %s mode = %o, want 0600", path, info.Mode().Perm())
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope reviewRejectedResultEnvelope
		decodeStrictReviewJSON(t, payload, &envelope)
		envelopes[path] = envelope
	}
	return envelopes
}

func assertRejectedEnvelope(t *testing.T, envelope reviewRejectedResultEnvelope, lineage, lens string, attempt int, raw []byte) {
	t.Helper()
	digest := sha256.Sum256(raw)
	if envelope.Schema != reviewRejectedResultSchema || envelope.LineageID != lineage || envelope.Lens != lens ||
		envelope.Attempt != attempt || envelope.RawSHA256 != hex.EncodeToString(digest[:]) || envelope.Raw != string(raw) ||
		envelope.Reason == "" || envelope.CapturedAt == "" {
		t.Fatalf("rejected result envelope = %#v, want lineage %q lens %q attempt %d", envelope, lineage, lens, attempt)
	}
}

func TestProviderCaptureRetriesOnceWithAdmissionFeedback(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	valid, err := json.Marshal(admittedReviewerResultForTest(t, repo, record, lens, 0))
	if err != nil {
		t.Fatal(err)
	}
	invalid := reviewerPayloadWithNestedLens(t, valid)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return invalid, nil },
		func() ([]byte, error) { return valid, nil },
	)

	var output bytes.Buffer
	if err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), &output); err != nil {
		t.Fatalf("capture with one corrective attempt: %v\n%s", err, output.String())
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 2", len(*prompts))
	}
	first, second := (*prompts)[0], (*prompts)[1]
	if !bytes.HasPrefix(second, first) {
		t.Fatal("corrective prompt does not start with the original materialized prompt")
	}
	for _, want := range []string{reviewProviderCorrectiveFeedbackHeader, `unknown field "lens"`, "exactly one JSON object", reviewLensContextResultSchema} {
		if !bytes.Contains(second, []byte(want)) {
			t.Fatalf("corrective prompt lacks %q:\n%s", want, second[len(first):])
		}
	}
	if bytes.Contains(first, []byte(reviewProviderCorrectiveFeedbackHeader)) {
		t.Fatal("first prompt already carries admission feedback")
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected results = %d, want 1", len(preserved))
	}
	for path, envelope := range preserved {
		if !strings.HasPrefix(filepath.Base(path), lens+"-1-") {
			t.Fatalf("rejected result file name = %q, want <lens>-1-<sha>.json", filepath.Base(path))
		}
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalid)
	}
}

func TestProviderCaptureRefusesAfterTwoRejectedResultsAndPreservesBoth(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	valid, err := json.Marshal(admittedReviewerResultForTest(t, repo, record, lens, 0))
	if err != nil {
		t.Fatal(err)
	}
	invalid := reviewerPayloadWithNestedLens(t, valid)
	truncated := []byte(`{"subject_hash":"sha256:0","inspection":{"status":"completed","paths":["tracked.txt"]},"findings":[{"id":"R3-001","proof_refs":["x"`)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return invalid, nil },
		func() ([]byte, error) { return truncated, nil },
	)

	var output bytes.Buffer
	err = RunReview(append(append([]string{"capture-result"}, binding...), "--agent", string(model.AgentClaudeCode)), &output)
	failure := decodeCaptureRefusalEnvelope(t, err, output.Bytes())
	if failure.Code != reviewIntegrationInvalidRequestCode || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("second rejection envelope = %#v", failure)
	}
	if len(*prompts) != 2 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 2", len(*prompts))
	}

	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 2 {
		t.Fatalf("preserved rejected results = %d, want 2", len(preserved))
	}
	for path, envelope := range preserved {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("refusal does not name preserved payload %s: %v", path, err)
		}
		switch envelope.Attempt {
		case 1:
			assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalid)
		case 2:
			assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 2, truncated)
		default:
			t.Fatalf("unexpected preserved attempt %#v", envelope)
		}
	}
	for _, want := range []string{`unknown field "lens"`, "no complete JSON object", "gentle-ai review status"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal lacks %q: %v", want, err)
		}
	}

	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, record.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != reviewtransaction.StateReviewing || !reflect.DeepEqual(after.State, record.State) {
		t.Fatalf("two rejected results mutated reviewing authority: before=%#v after=%#v", record.State, after.State)
	}
}

func TestProviderCaptureTransportErrorSkipsCorrectionAndPreservation(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) { return nil, errors.New("reviewer subprocess exited 137") },
		func() ([]byte, error) { t.Fatal("transport failure must not be retried"); return nil, nil },
	)
	err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode)), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invoke provider reviewer") || !strings.Contains(err.Error(), "exited 137") {
		t.Fatalf("transport failure = %v", err)
	}
	if len(*prompts) != 1 {
		t.Fatalf("provider reviewer invocations = %d, want exactly 1", len(*prompts))
	}
	if _, statErr := os.Stat(rejectedResultsDir(t, repo, record.State.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("transport failure preserved a rejected result: %v", statErr)
	}
}

func TestRawInputCapturePreservesRejectedResultWithoutInvokingProvider(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	lens := record.State.SelectedLenses[0]
	valid, err := json.Marshal(admittedReviewerResultForTest(t, repo, record, lens, 0))
	if err != nil {
		t.Fatal(err)
	}
	invalid := reviewerPayloadWithNestedLens(t, valid)
	prompts := recordingProviderAdapter(t)
	input := filepath.Join(t.TempDir(), "rejected.json")
	if err := os.WriteFile(input, invalid, 0o600); err != nil {
		t.Fatal(err)
	}

	err = RunReviewCaptureResult(append(binding, "--input", input), io.Discard)
	if err == nil || !strings.Contains(err.Error(), `unknown field "lens"`) {
		t.Fatalf("raw input rejection = %v", err)
	}
	if len(*prompts) != 0 {
		t.Fatalf("raw input capture invoked the provider reviewer %d times", len(*prompts))
	}
	preserved := readRejectedResults(t, rejectedResultsDir(t, repo, record.State.LineageID))
	if len(preserved) != 1 {
		t.Fatalf("preserved rejected results = %d, want 1", len(preserved))
	}
	for path, envelope := range preserved {
		if !strings.Contains(err.Error(), path) {
			t.Fatalf("refusal does not name preserved payload %s: %v", path, err)
		}
		assertRejectedEnvelope(t, envelope, record.State.LineageID, lens, 1, invalid)
	}
}

func TestProviderCaptureRefusesPreflightBeforeInvocationOrPreservation(t *testing.T) {
	repo, binding, record, _ := newCandidateInspectionReview(t, "candidate\n", false)
	prompts := recordingProviderAdapter(t,
		func() ([]byte, error) {
			t.Fatal("--preflight must refuse before the provider is invoked")
			return nil, nil
		},
	)
	err := RunReviewCaptureResult(append(binding, "--agent", string(model.AgentClaudeCode), "--preflight"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--agent cannot be combined with --preflight") {
		t.Fatalf("preflight with a provider runtime = %v", err)
	}
	if len(*prompts) != 0 {
		t.Fatalf("provider reviewer invocations = %d, want none", len(*prompts))
	}
	if _, statErr := os.Stat(rejectedResultsDir(t, repo, record.State.LineageID)); !os.IsNotExist(statErr) {
		t.Fatalf("preflight preserved a rejected result: %v", statErr)
	}
}
